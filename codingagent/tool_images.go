package codingagent

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	"github.com/nankedr/pig/ai"
	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
)

const inlineImageBytes = 9 * 1024 * 1024 / 2

func decodeToolImage(data []byte) (image.Image, string, error) {
	if len(data) == 0 || len(data) > ai.MaxUserImageBytes {
		return nil, "", fmt.Errorf("image must contain 1 byte to 8 MiB")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 40_000_000/config.Height {
		return nil, "", fmt.Errorf("image exceeds 40 million pixel limit")
	}
	picture, format, err := image.Decode(bytes.NewReader(data))
	if err == nil && format != "bmp" {
		err = ai.ValidateUserImage(ai.ImageContent{Type: ai.ContentTypeImage, MIMEType: "image/" + format, Data: base64.StdEncoding.EncodeToString(data)})
	}
	return picture, "image/" + format, err
}

func readImageContent(data []byte, mime string, autoResize bool) []ai.ToolResultContent {
	text := "Read image file [" + mime + "]"
	omitted := func(message string) []ai.ToolResultContent {
		return []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: text + "\n[Image omitted: " + message + "]"}}
	}
	_, actual, err := decodeToolImage(data)
	if err != nil {
		return omitted("could not be resized below the inline image size limit.")
	}
	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if normalized == "image/jpg" {
		normalized = "image/jpeg"
	}
	if normalized != actual {
		return omitted("detected MIME does not match actual image format.")
	}
	converted := actual == "image/bmp"
	if converted {
		picture, _, _ := image.Decode(bytes.NewReader(data))
		var buffer bytes.Buffer
		if err := png.Encode(&buffer, picture); err != nil {
			return omitted("could not be converted to a supported inline image format.")
		}
		data, actual = buffer.Bytes(), "image/png"
	}
	result := &ResizedImage{Data: base64.StdEncoding.EncodeToString(data), MIMEType: actual}
	if autoResize {
		result, err = ResizeImage(data, actual)
		if err != nil || result == nil {
			return omitted("could not be resized below the inline image size limit.")
		}
	}
	text = "Read image file [" + result.MIMEType + "]"
	if converted {
		text += "\n[Image converted from image/bmp to " + result.MIMEType + ".]"
	}
	if note := FormatDimensionNote(*result); note != nil {
		text += "\n" + *note
	}
	block := ai.ImageContent{Type: ai.ContentTypeImage, Data: result.Data, MIMEType: result.MIMEType}
	if err := ai.ValidateUserImage(block); err != nil {
		return omitted("processed image exceeds local attachment limits.")
	}
	return []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: text}, block}
}

func ResizeImage(data []byte, mime string, options ...ImageResizeOptions) (*ResizedImage, error) {
	width, height, maxBytes, quality := 2000, 2000, inlineImageBytes, 80
	if len(options) > 0 {
		o := options[0]
		if o.MaxWidth != nil {
			width = *o.MaxWidth
		}
		if o.MaxHeight != nil {
			height = *o.MaxHeight
		}
		if o.MaxBytes != nil {
			maxBytes = *o.MaxBytes
		}
		if o.JPEGQuality != nil {
			quality = *o.JPEGQuality
		}
	}
	if width < 1 || height < 1 || maxBytes < 1 || quality < 1 || quality > 100 {
		return nil, fmt.Errorf("invalid image resize limits")
	}
	picture, actual, err := decodeToolImage(data)
	if err != nil {
		return nil, nil
	}
	if mime != actual {
		return nil, fmt.Errorf("image MIME does not match actual format")
	}
	orientation := toolExifOrientation(data)
	config, _, _ := image.DecodeConfig(bytes.NewReader(data))
	originalWidth, originalHeight := config.Width, config.Height
	if orientation >= 5 {
		originalWidth, originalHeight = originalHeight, originalWidth
	}
	result := ResizedImage{OriginalWidth: originalWidth, OriginalHeight: originalHeight, Width: originalWidth, Height: originalHeight, MIMEType: mime}
	if originalWidth <= width && originalHeight <= height && base64.StdEncoding.EncodedLen(len(data)) < maxBytes {
		result.Data = base64.StdEncoding.EncodeToString(data)
		return &result, nil
	}
	if actual == "image/gif" && picture.Bounds() != image.Rect(0, 0, originalWidth, originalHeight) {
		canvas := image.NewNRGBA(image.Rect(0, 0, originalWidth, originalHeight))
		draw.Draw(canvas, picture.Bounds(), picture, picture.Bounds().Min, draw.Src)
		picture = canvas
	}
	if orientation != 1 {
		picture = orientToolImage(picture, orientation)
	}
	targetWidth, targetHeight := originalWidth, originalHeight
	if targetWidth > width {
		targetHeight = int(math.Round(float64(targetHeight) * float64(width) / float64(targetWidth)))
		targetWidth = width
	}
	if targetHeight > height {
		targetWidth = int(math.Round(float64(targetWidth) * float64(height) / float64(targetHeight)))
		targetHeight = height
	}
	targetWidth, targetHeight = max(1, targetWidth), max(1, targetHeight)
	for {
		resized := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
		draw.CatmullRom.Scale(resized, resized.Bounds(), picture, picture.Bounds(), draw.Src, nil)
		for _, q := range []int{0, quality, 85, 70, 55, 40} {
			var buffer bytes.Buffer
			format := "image/png"
			if q == 0 {
				err = png.Encode(&buffer, resized)
			} else {
				format = "image/jpeg"
				err = jpeg.Encode(&buffer, resized, &jpeg.Options{Quality: q})
			}
			if err != nil {
				return nil, err
			}
			encoded := base64.StdEncoding.EncodeToString(buffer.Bytes())
			if len(encoded) < maxBytes {
				result.Data, result.MIMEType, result.Width, result.Height, result.WasResized = encoded, format, targetWidth, targetHeight, true
				return &result, nil
			}
		}
		if targetWidth == 1 && targetHeight == 1 {
			return nil, nil
		}
		targetWidth, targetHeight = max(1, targetWidth*3/4), max(1, targetHeight*3/4)
	}
}

func toolExifOrientation(data []byte) int {
	var tiff []byte
	if bytes.HasPrefix(data, []byte{0xff, 0xd8}) {
		for pos := 2; pos+4 <= len(data) && data[pos] == 0xff; {
			marker := data[pos+1]
			if marker == 0xff {
				pos++
				continue
			}
			if marker == 0xda || marker == 0xd9 {
				break
			}
			size := int(binary.BigEndian.Uint16(data[pos+2:]))
			if size < 2 || size > len(data)-pos-2 {
				break
			}
			if marker == 0xe1 && size >= 8 && bytes.Equal(data[pos+4:pos+10], []byte("Exif\x00\x00")) {
				tiff = data[pos+10 : pos+2+size]
				break
			}
			pos += 2 + size
		}
	} else if len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		for pos := 12; pos+8 <= len(data); {
			size := uint64(binary.LittleEndian.Uint32(data[pos+4:]))
			if size > uint64(len(data)-pos-8) {
				break
			}
			if string(data[pos:pos+4]) == "EXIF" {
				tiff = data[pos+8 : pos+8+int(size)]
				tiff = bytes.TrimPrefix(tiff, []byte("Exif\x00\x00"))
				break
			}
			pos += 8 + int(size) + int(size%2)
		}
	}
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder = binary.BigEndian
	if string(tiff[:2]) == "II" {
		order = binary.LittleEndian
	} else if string(tiff[:2]) != "MM" {
		return 1
	}
	offset := uint64(order.Uint32(tiff[4:8]))
	if offset+2 > uint64(len(tiff)) {
		return 1
	}
	entries := int(order.Uint16(tiff[offset:]))
	for pos := int(offset) + 2; entries > 0 && pos+12 <= len(tiff); entries, pos = entries-1, pos+12 {
		if order.Uint16(tiff[pos:]) == 0x112 {
			value := int(order.Uint16(tiff[pos+8:]))
			if value >= 1 && value <= 8 {
				return value
			}
			return 1
		}
	}
	return 1
}

func orientToolImage(source image.Image, orientation int) image.Image {
	b := source.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	result := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			result.Set(dx, dy, source.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return result
}
