//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"golang.org/x/image/bmp"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
)

func main() {
	root := "parity/services/tool-images/"
	img := image.NewNRGBA(image.Rect(0, 0, 4000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 4000; x++ {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= 2000 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	os.WriteFile(root+"wide.png", b.Bytes(), 0644)
	b.Reset()
	jpeg.Encode(&b, img, &jpeg.Options{Quality: 80})
	data := append([]byte(nil), b.Bytes()...)
	for i := 1; i <= 8; i++ {
		exif := []byte{'E', 'x', 'i', 'f', 0, 0, 'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 1, 3, 0, 1, 0, 0, 0, byte(i), 0, 0, 0, 0, 0, 0, 0}
		seg := []byte{0xff, 0xe1, 0, 0}
		binary.BigEndian.PutUint16(seg[2:], uint16(len(exif)+2))
		out := append(append(append([]byte{}, data[:2]...), seg...), exif...)
		out = append(out, data[2:]...)
		os.WriteFile(root+fmt.Sprintf("orientation-%d.jpg", i), out, 0644)
	}
	b.Reset()
	bmp.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 1)))
	os.WriteFile(root+"tiny.bmp", b.Bytes(), 0644)
	frame := image.NewPaletted(image.Rect(100, 100, 200, 200), color.Palette{color.Black, color.White})
	b.Reset()
	gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}, Config: image.Config{ColorModel: frame.Palette, Width: 3000, Height: 3000}})
	os.WriteFile(root+"canvas.gif", b.Bytes(), 0644)
	os.WriteFile(root+"corrupt.png", []byte{0x89, 'P', 'N', 'G', 13, 10, 26, 10, 0, 0, 0, 13, 'I', 'H', 'D', 'R'}, 0644)
}
