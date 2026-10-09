package codingagent

import (
	"context"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nankedr/pig/ai"
)

type pendingImage struct {
	name  string
	image ai.ImageContent
}

func (m *InteractiveMode) addImage(name string, image ai.ImageContent) error {
	if err := ai.ValidateUserImage(image); err != nil {
		return err
	}
	m.imagesMu.Lock()
	defer m.imagesMu.Unlock()
	size := len(image.Data)
	for _, current := range m.images {
		size += len(current.image.Data)
	}
	if len(m.images) >= 64 || size > base64.StdEncoding.EncodedLen(16<<20) {
		return fmt.Errorf("pending images exceed 64 attachments or 16 MiB")
	}
	m.images = append(m.images, pendingImage{name, image})
	return nil
}

func (m *InteractiveMode) imageCommand(ctx context.Context, text string) (bool, error) {
	command, arg, _ := strings.Cut(strings.TrimSpace(text), " ")
	if command != "/image" {
		return false, nil
	}
	action, value, _ := strings.Cut(strings.TrimSpace(arg), " ")
	switch action {
	case "add":
		path := strings.TrimSpace(value)
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.runtime.Session().SessionManager().GetCWD(), path)
		}
		image, err := ai.LoadImageFile(path)
		if err != nil {
			return true, err
		}
		if err := m.addImage(path, image); err != nil {
			return true, err
		}
		m.imagesMu.Lock()
		n := len(m.images)
		m.imagesMu.Unlock()
		return true, m.appendNotice(fmt.Sprintf("Pending image %d: %s\n", n, path))
	case "remove":
		n, err := strconv.Atoi(value)
		m.imagesMu.Lock()
		if err != nil || n < 1 || n > len(m.images) {
			m.imagesMu.Unlock()
			return true, fmt.Errorf("invalid pending image number")
		}
		m.images = append(m.images[:n-1], m.images[n:]...)
		m.imagesMu.Unlock()
		return true, m.appendNotice(fmt.Sprintf("Removed image %d\n", n))
	case "view", "history":
		n, err := strconv.Atoi(value)
		var images []pendingImage
		if action == "history" {
			images = m.historyImages()
		} else {
			m.imagesMu.Lock()
			images = append(images, m.images...)
			m.imagesMu.Unlock()
		}
		if err != nil || n < 1 || n > len(images) {
			return true, fmt.Errorf("invalid image number")
		}
		return true, m.previewImage(ctx, images[n-1])
	case "", "list":
		m.imagesMu.Lock()
		images := append([]pendingImage(nil), m.images...)
		m.imagesMu.Unlock()
		var lines []string
		for i, item := range images {
			lines = append(lines, fmt.Sprintf("Pending image %d: %s (%s)", i+1, item.name, item.image.MIMEType))
		}
		for i, item := range m.historyImages() {
			lines = append(lines, fmt.Sprintf("Session image %d: %s (%s; inline, /image history %d)", i+1, item.name, item.image.MIMEType, i+1))
		}
		if len(lines) == 0 {
			lines = append(lines, "No pending or session images")
		}
		return true, m.appendNotice(strings.Join(lines, "\n") + "\n")
	default:
		return true, fmt.Errorf("usage: /image add <path> | list | remove <n> | view <n> | history <n>")
	}
}

func (m *InteractiveMode) historyImages() []pendingImage {
	var images []pendingImage
	for _, message := range m.transcriptMessages() {
		switch message := message.(type) {
		case ai.UserMessage:
			blocks, _ := message.Content.Blocks()
			for _, block := range blocks {
				if image, ok := block.(ai.ImageContent); ok {
					images = append(images, pendingImage{"user", image})
				}
			}
		case ai.ToolResultMessage:
			for _, block := range message.Content {
				if image, ok := block.(ai.ImageContent); ok {
					images = append(images, pendingImage{message.ToolName + "/" + message.ToolCallID, image})
				}
			}
		}
	}
	return images
}

func (m *InteractiveMode) pasteImage(ctx context.Context) error {
	image, text, err := readClipboardImage(ctx)
	if err != nil {
		return err
	}
	if image == nil {
		return m.ui.PrependEditor(text)
	}
	if err := m.addImage("clipboard", *image); err != nil {
		return err
	}
	return m.appendNotice("Pending image from clipboard; /image list to view or remove\n")
}

func imageDescription(image ai.ImageContent) string {
	return fmt.Sprintf("[image %s; inline session attachment, /image list to inspect or HTML export to view]", image.MIMEType)
}
