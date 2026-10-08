package ai_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestUserImagesLocalFormatsAndPreservedOrientation(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 2, 3))
	picture.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var pngData, jpegData, gifData bytes.Buffer
	png.Encode(&pngData, picture)
	jpeg.Encode(&jpegData, picture, nil)
	gif.Encode(&gifData, picture, nil)
	// EXIF Orientation=6; preserve the source instead of silently re-encoding it.
	exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, 6, 0, 0, 0, 0, 0, 0, 0}
	rotated := append([]byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}, exif...)
	rotated = append(rotated, jpegData.Bytes()[2:]...)
	webp, _ := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
	for name, data := range map[string][]byte{"png": pngData.Bytes(), "jpeg": rotated, "gif": gifData.Bytes(), "webp": webp} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "wrong.extension")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			block, err := ai.LoadImageFile(path)
			if err != nil || block.MIMEType != "image/"+name || block.Data != base64.StdEncoding.EncodeToString(data) {
				t.Fatalf("format/orientation/size: %+v %v", block, err)
			}
			if err := ai.ValidateUserImage(block); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUserImagesLocalLimitsAndInvalidFiles(t *testing.T) {
	pngData, _ := base64.StdEncoding.DecodeString(image134)
	tooManyPixels := append([]byte{}, pngData...)
	binary.BigEndian.PutUint32(tooManyPixels[16:20], 40_000_001)
	binary.BigEndian.PutUint32(tooManyPixels[29:33], crc32.ChecksumIEEE(tooManyPixels[12:29]))
	for name, data := range map[string][]byte{"empty": {}, "corrupt": pngData[:len(pngData)-5], "unsupported": []byte("<svg/>"), "pixels": tooManyPixels, "bytes": make([]byte, ai.MaxUserImageBytes+1)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			os.WriteFile(path, data, 0600)
			if _, err := ai.LoadImageFile(path); err == nil || !strings.Contains(err.Error(), "image") {
				t.Fatalf("invalid image file accepted: %v", err)
			}
		})
	}
	if _, err := ai.LoadImageFile(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
	if _, err := ai.LoadImageFile(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestUserImagesGIFRejectsCorruptLaterFrames(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.Black, color.White})
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "animated.gif")
	os.WriteFile(path, encoded.Bytes(), 0600)
	if block, err := ai.LoadImageFile(path); err != nil || block.Data != base64.StdEncoding.EncodeToString(encoded.Bytes()) {
		t.Fatalf("valid animation changed: %v", err)
	}
	os.WriteFile(path, encoded.Bytes()[:encoded.Len()-1], 0600)
	if _, err := ai.LoadImageFile(path); err == nil {
		t.Fatal("missing GIF trailer silently accepted")
	}
}
