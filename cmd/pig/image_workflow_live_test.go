package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImageWorkflowRPCLive136(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_IMAGE_WORKFLOW_LIVE") != "1" {
		t.Skip("protected current-service RPC vision smoke")
	}
	key := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Fatal("protected key required")
	}
	picture := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 64 {
				c = color.NRGBA{B: 255, A: 255}
			}
			picture.SetNRGBA(x, y, c)
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, picture); err != nil {
		t.Fatal(err)
	}
	binary := binary133(t)
	for _, api := range []ai.API{ai.APIOpenAIResponses, ai.APIOpenAICompletions} {
		t.Run(string(api), func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "screen.png")
			if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			image, err := ai.LoadImageFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
			defer cancel()
			client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--model", "deepseek-flash", "--api", string(api), "--thinking", "off", "--no-context-files"}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": filepath.Join(root, "state"), "DEEPSEEK_API_KEY": key}})
			if err := client.Start(ctx); err != nil {
				t.Fatal("protected RPC startup failed")
			}
			defer client.Stop(context.Background())
			if _, err := client.PromptAndWait(ctx, "Visually identify the two colors in the attached image, then use read on screen.png and write colors.txt with the two English color names. Do not use bash or metadata to infer colors.", []ai.ImageContent{image}); err != nil {
				t.Fatal("protected RPC image task failed")
			}
			result, err := os.ReadFile(filepath.Join(root, "colors.txt"))
			text := strings.ToLower(string(result))
			if err != nil || !strings.Contains(text, "red") || !strings.Contains(text, "blue") {
				t.Fatal("RPC semantic color/write check failed")
			}
			messages, err := client.GetMessages(ctx)
			if err != nil {
				t.Fatal("RPC history failed")
			}
			raw, _ := json.Marshal(messages)
			if !bytes.Contains(raw, []byte(image.Data)) || !bytes.Contains(raw, []byte(`"role":"toolResult"`)) {
				t.Fatal("RPC user/tool image history missing")
			}
			os.Remove(path)
			if _, err := client.ExportHTML(ctx, filepath.Join(root, "images.html")); err != nil {
				t.Fatal("RPC independent export failed")
			}
			t.Log("PASS protected current DeepSeek " + string(api) + " RPC attachment -> vision -> read image -> write -> standalone HTML; not Pi golden")
		})
	}
}
