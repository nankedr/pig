package codingagent_test

import (
	bytespkg "bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestToolImagesReadPreservesSmallImage(t *testing.T) {
	path, image := imageFile134(t)
	result := runReadTool(t, context.Background(), t.TempDir(), map[string]any{"path": path, "offset": 999, "limit": 0})
	if result.IsError || len(result.Content) != 2 {
		t.Fatalf("image read: %+v", result)
	}
	if result.Content[0].(ai.TextContent).Text != "Read image file [image/png]" || result.Content[1].(ai.ImageContent) != image {
		t.Fatal("read changed bytes/order/MIME")
	}
	bytes, _ := os.ReadFile(path)
	if image.Data != base64.StdEncoding.EncodeToString(bytes) {
		t.Fatal("identity")
	}
}

func TestToolImagesReadFixedPiOracle(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/read-images.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Commit string `json:"baseline_commit"`
		Cases  []struct {
			File       string
			AutoResize bool
			Content    []map[string]any
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Commit != issue32BaselineCommit {
		t.Fatal("unlocked Pi fixture")
	}
	for _, tc := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%t", tc.File, tc.AutoResize), func(t *testing.T) {
			result := runReadTool(t, context.Background(), t.TempDir(), map[string]any{"path": filepath.Join(issue32RepoRoot(t), "parity/services/tool-images", tc.File), "offset": 999, "limit": 0}, codingagent.ReadToolOptions{AutoResizeImages: &tc.AutoResize})
			if tc.File == "corrupt.png" && !tc.AutoResize {
				if result.IsError || len(result.Content) != 1 || !strings.Contains(result.Content[0].(ai.TextContent).Text, "Image omitted") {
					t.Fatal("Pig must explain unsafe corrupt pass-through")
				}
				return
			}
			if result.IsError || len(result.Content) != len(tc.Content) {
				t.Fatalf("read=%+v", result)
			}
			for i, block := range result.Content {
				switch b := block.(type) {
				case ai.TextContent:
					if b.Text != tc.Content[i]["text"] {
						t.Fatalf("text=%q want=%q", b.Text, tc.Content[i]["text"])
					}
				case ai.ImageContent:
					bytes, err := base64.StdEncoding.DecodeString(b.Data)
					if err != nil {
						t.Fatal(err)
					}
					pic, _, err := image.Decode(bytespkg.NewReader(bytes))
					if err != nil {
						t.Fatal(err)
					}
					config, _, err := image.DecodeConfig(bytespkg.NewReader(bytes))
					if err != nil {
						t.Fatal(err)
					}
					bounds := image.Rect(0, 0, config.Width, config.Height)
					if b.MIMEType != tc.Content[i]["mimeType"] || bounds.Dx() != int(tc.Content[i]["width"].(float64)) || bounds.Dy() != int(tc.Content[i]["height"].(float64)) {
						t.Fatalf("image dimensions/MIME %+v want=%+v", bounds, tc.Content[i])
					}
					if data, ok := tc.Content[i]["data"].(string); ok && data != b.Data {
						t.Fatal("unchanged image identity lost")
					}
					for j, p := range []image.Point{{0, 0}, {bounds.Dx() - 1, 0}, {0, bounds.Dy() - 1}, {bounds.Dx() - 1, bounds.Dy() - 1}} {
						r, _, blue, _ := pic.At(p.X, p.Y).RGBA()
						color := "other"
						if r > blue {
							color = "red"
						} else if blue > r {
							color = "blue"
						}
						if color != tc.Content[i]["corners"].([]any)[j] {
							t.Fatalf("corner %d=%s want=%s", j, color, tc.Content[i]["corners"].([]any)[j])
						}
					}
				}
			}
		})
	}
}

func TestToolImagesReadControlledOperationsAndLimits(t *testing.T) {
	_, block := imageFile134(t)
	data, _ := base64.StdEncoding.DecodeString(block.Data)
	mime := "image/png"
	off := false
	for _, resize := range []*bool{nil, &off} {
		result := runReadTool(t, t.Context(), t.TempDir(), map[string]any{"path": "remote.png"}, codingagent.ReadToolOptions{Operations: readOperationsStub{mimeType: &mime, content: data}, AutoResizeImages: resize})
		if result.IsError || len(result.Content) != 2 || result.Content[1].(ai.ImageContent) != block {
			t.Fatal("controlled read lost image")
		}
	}
	wrong := "image/jpeg"
	result := runReadTool(t, t.Context(), t.TempDir(), map[string]any{"path": "remote.png"}, codingagent.ReadToolOptions{Operations: readOperationsStub{mimeType: &wrong, content: data}})
	if result.IsError || !strings.Contains(readToolResultText(t, result), "MIME") {
		t.Fatal("MIME mismatch silently accepted")
	}
	result = runReadTool(t, t.Context(), t.TempDir(), map[string]any{"path": "huge.png"}, codingagent.ReadToolOptions{Operations: readOperationsStub{mimeType: &mime, content: make([]byte, ai.MaxUserImageBytes+1)}})
	if result.IsError || !strings.Contains(readToolResultText(t, result), "Image omitted") {
		t.Fatal("oversize image silently accepted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	core, err := promptReadTool(t, ctx, t.TempDir(), map[string]any{"path": "remote.png"}, codingagent.ReadToolOptions{Operations: readOperationsStub{mimeType: &mime, onRead: func(context.Context) ([]byte, error) { cancel(); return data, nil }}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	messages := core.State().Messages
	result = messages[len(messages)-1].(ai.ToolResultMessage)
	if !result.IsError || strings.Contains(fmt.Sprint(result.Content), block.Data) {
		t.Fatal("cancelled read published image")
	}
	one := 1
	resized, err := codingagent.ResizeImage(data, mime, codingagent.ImageResizeOptions{MaxBytes: &one})
	if err != nil || resized != nil {
		t.Fatalf("impossible limit result=%+v %v", resized, err)
	}
}
