//go:build !darwin || !arm64

package codingagent

import (
	"context"
	"github.com/nankedr/pig/ai"
)

func readClipboardImage(context.Context) (*ai.ImageContent, string, error) {
	return nil, "", notImplemented("clipboard.images (V1 requires darwin-arm64; use /image add <path>)")
}
