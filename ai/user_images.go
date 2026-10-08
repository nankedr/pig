package ai

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	_ "golang.org/x/image/webp"
)

const MaxUserImageBytes = 8 << 20

// LoadImageFile preserves image bytes, orientation metadata and dimensions.
func LoadImageFile(path string) (ImageContent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ImageContent{}, fmt.Errorf("image file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return ImageContent{}, fmt.Errorf("image file %q must be regular", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return ImageContent{}, fmt.Errorf("image file %q: %w", path, err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return ImageContent{}, err
	}
	if !info.Mode().IsRegular() {
		return ImageContent{}, fmt.Errorf("image file %q must be regular", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxUserImageBytes+1))
	if err != nil {
		return ImageContent{}, fmt.Errorf("image file %q: %w", path, err)
	}
	mime, err := userImageFormat(data)
	if err != nil {
		return ImageContent{}, fmt.Errorf("image file %q: %w", path, err)
	}
	return ImageContent{Type: ContentTypeImage, MIMEType: mime, Data: base64.StdEncoding.EncodeToString(data)}, nil
}

// ValidateUserImage checks encoding, decoded format, MIME and local limits.
func ValidateUserImage(value ImageContent) error {
	if value.Type != ContentTypeImage {
		return fmt.Errorf("invalid image type %q", value.Type)
	}
	if len(value.Data) > base64.StdEncoding.EncodedLen(MaxUserImageBytes) {
		return fmt.Errorf("image exceeds 8 MiB inline limit")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(value.Data)
	if err != nil {
		return fmt.Errorf("invalid image base64: %w", err)
	}
	mime, err := userImageFormat(data)
	if err != nil {
		return err
	}
	if value.MIMEType != mime {
		return fmt.Errorf("image MIME %q does not match actual format %q", value.MIMEType, mime)
	}
	return nil
}

func userImageFormat(data []byte) (string, error) {
	if len(data) == 0 || len(data) > MaxUserImageBytes {
		return "", fmt.Errorf("image must contain 1 byte to 8 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("invalid image format: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 40_000_000/config.Height {
		return "", fmt.Errorf("image exceeds 40 million pixel limit")
	}
	switch format {
	case "png", "jpeg", "gif", "webp":
	default:
		return "", fmt.Errorf("unsupported image format %q", format)
	}
	if format == "gif" {
		if err := validateGIFFrames(data); err != nil {
			return "", err
		}
		if _, err := gif.DecodeAll(bytes.NewReader(data)); err != nil {
			return "", fmt.Errorf("corrupt GIF image: %w", err)
		}
	} else if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("corrupt image: %w", err)
	}
	return "image/" + format, nil
}

func validateUserImages(input Context) error {
	var total, count int
	for _, message := range input.Messages {
		if m, ok := message.(*UserMessage); ok && m != nil {
			message = *m
		}
		m, ok := message.(UserMessage)
		if !ok {
			continue
		}
		blocks, _ := m.Content.Blocks()
		for _, block := range blocks {
			if image, ok := replayContentValue(block).(ImageContent); ok {
				if err := ValidateUserImage(image); err != nil {
					return err
				}
				total += len(image.Data)
				count++
			}
		}
	}
	if total > base64.StdEncoding.EncodedLen(16<<20) || count > 64 {
		return fmt.Errorf("user images exceed 16 MiB total or 64 images per request")
	}
	return nil
}

// DeepSeekVisionModel is current service configuration, outside the fixed Catalog Snapshot.
func DeepSeekVisionModel() Model {
	model := cloneModel(builtinModelsByProvider[ProviderIDDeepSeek][0])
	model.ID, model.Name = "deepseek-flash", "DeepSeek Flash (vision)"
	model.Input = []ModelInput{ModelInputText, ModelInputImage}
	return model
}

// Bound all GIF frames before DecodeAll allocates their decoded pixels.
func validateGIFFrames(data []byte) error {
	if len(data) < 13 {
		return fmt.Errorf("corrupt GIF image header")
	}
	pos := 13
	if packed := data[10]; packed&0x80 != 0 {
		pos += 3 * (1 << ((packed & 7) + 1))
	}
	frames, pixels := 0, int64(0)
	for pos < len(data) {
		kind := data[pos]
		pos++
		switch kind {
		case 0x3b:
			if frames == 0 || pos != len(data) {
				return fmt.Errorf("invalid GIF image trailer")
			}
			return nil
		case 0x21:
			pos++
		case 0x2c:
			if len(data)-pos < 9 {
				return fmt.Errorf("corrupt GIF image descriptor")
			}
			width, height := int64(binary.LittleEndian.Uint16(data[pos+4:])), int64(binary.LittleEndian.Uint16(data[pos+6:]))
			frames++
			pixels += width * height
			if frames > 64 || pixels > 40_000_000 {
				return fmt.Errorf("GIF image exceeds 64 frames or 40 million decoded pixels")
			}
			packed := data[pos+8]
			pos += 9
			if packed&0x80 != 0 {
				pos += 3 * (1 << ((packed & 7) + 1))
			}
			pos++
		default:
			return fmt.Errorf("invalid GIF image block")
		}
		for {
			if pos >= len(data) {
				return fmt.Errorf("corrupt GIF image data")
			}
			size := int(data[pos])
			pos++
			if size == 0 {
				break
			}
			if size > len(data)-pos {
				return fmt.Errorf("corrupt GIF image data")
			}
			pos += size
		}
	}
	return fmt.Errorf("missing GIF image trailer")
}
