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

func TestUserImagesDeepSeekLiveRestore(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_VISION_LIVE") != "1" {
		t.Skip("protected vision smoke requires PIG_REQUIRE_VISION_LIVE=1")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected vision smoke requires DEEPSEEK_API_KEY")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "colors.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			c := color.RGBA{255, 0, 0, 255}
			if x >= 64 {
				c = color.RGBA{0, 0, 255, 255}
			}
			picture.Set(x, y, c)
		}
	}
	if err := png.Encode(f, picture); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	block, err := ai.LoadImageFile(path)
	if err != nil {
		t.Fatal(err)
	}
	options := codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Provider: ai.ProviderIDDeepSeek, Model: "deepseek-flash", API: ai.APIOpenAIResponses, APIKey: &key, NoTools: codingagent.NoToolsAll, NoContextFiles: true, Thinking: "off"}
	runtime, err := codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		t.Fatal("protected vision creation failed")
	}
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"Name the two visible colors in English, briefly."}, InitialImages: []ai.ImageContent{block}})
	saved := *runtime.Session().SessionFile()
	runtime.Dispose(context.Background())
	check := func(outcome codingagent.HeadlessOutcome, err error) {
		t.Helper()
		text := strings.ToLower(strings.Join(outcome.Text, " "))
		if err != nil || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonStop || !strings.Contains(text, "red") || !strings.Contains(text, "blue") {
			t.Fatal("protected vision did not identify both colors")
		}
	}
	check(outcome, err)
	os.Remove(path)
	manager, err := codingagent.OpenSessionManager(saved, nil, nil)
	if err != nil {
		t.Fatal("protected vision v3 restore failed")
	}
	options.SessionManager, options.API, options.Model = manager, "", ""
	runtime, err = codingagent.CreateHeadlessSession(ctx, options)
	if err != nil {
		t.Fatal("protected vision runtime restore failed")
	}
	defer runtime.Dispose(context.Background())
	outcome, err = codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"Look at the same picture again. Name its two colors in English, briefly."}})
	check(outcome, err)
	t.Log("PASS DeepSeek deepseek-flash Responses: local PNG, vision colors, v3 restore and full image replay")
}
