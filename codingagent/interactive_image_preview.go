package codingagent

import (
	"bytes"
	"context"
	"encoding/base64"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/tui"
	"golang.org/x/image/draw"
	"image"
	"image/png"
)

func (m *InteractiveMode) previewImage(ctx context.Context, item pendingImage) error {
	if err := ai.ValidateUserImage(item.image); err != nil {
		return err
	}
	data, _ := base64.StdEncoding.DecodeString(item.image.Data)
	picture, _, err := decodeToolImage(data)
	if err != nil {
		return err
	}
	picture = orientToolImage(picture, toolExifOrientation(data))
	width, height := picture.Bounds().Dx(), picture.Bounds().Dy()
	if width > 1024 || height > 1024 {
		if width >= height {
			height = max(1, height*1024/width)
			width = 1024
		} else {
			width = max(1, width*1024/height)
			height = 1024
		}
		resized := image.NewNRGBA(image.Rect(0, 0, width, height))
		draw.CatmullRom.Scale(resized, resized.Bounds(), picture, picture.Bounds(), draw.Src, nil)
		picture = resized
	}
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, picture); err != nil {
		return err
	}
	return m.ui.PreviewImage(ctx, base64.StdEncoding.EncodeToString(pngData.Bytes()), tui.ImageDimensions{WidthPX: picture.Bounds().Dx(), HeightPX: picture.Bounds().Dy()})
}
