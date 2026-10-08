package codingagent_test

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestToolImagesDeepSeekLiveReadAndWrite(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_TOOL_VISION_LIVE") != "1" {
		t.Skip("protected tool vision smoke requires PIG_REQUIRE_TOOL_VISION_LIVE=1")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected tool vision smoke requires DEEPSEEK_API_KEY")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()
	cwd := t.TempDir()
	picture := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= 64 {
				c = color.NRGBA{0, 0, 255, 255}
			}
			picture.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(filepath.Join(cwd, "screenshot.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, picture)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: t.TempDir(), Model: "deepseek-flash", Provider: ai.ProviderIDDeepSeek, API: api, APIKey: &key, NoContextFiles: true, Thinking: "off"})
			if err != nil {
				t.Fatal("protected tool vision creation failed")
			}
			defer runtime.Dispose(context.Background())
			filename := string(api) + ".txt"
			if err := runtime.Session().Prompt(ctx, "Use read to open screenshot.png, visually identify its two colors, then use write to create "+filename+" containing the two English color names. Do not use bash or image metadata to infer colors."); err != nil {
				t.Fatal("protected tool vision task failed")
			}
			imageSeen := false
			for _, message := range runtime.Session().Messages() {
				if result, ok := message.(ai.ToolResultMessage); ok && result.ToolName == "read" && !result.IsError {
					for _, block := range result.Content {
						if _, ok := block.(ai.ImageContent); ok {
							imageSeen = true
						}
					}
				}
			}
			result, err := os.ReadFile(filepath.Join(cwd, filename))
			text := strings.ToLower(string(result))
			if err != nil || !imageSeen || !strings.Contains(text, "red") || !strings.Contains(text, "blue") {
				t.Fatal("protected tool vision failed read-image/write-color semantic check")
			}
			t.Log("PASS current DeepSeek tool image read -> vision -> write for " + string(api))
		})
	}
}
