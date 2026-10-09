package tui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image/png"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
)

type imagePreview struct {
	data          string
	dimensions    ImageDimensions
	supported     bool
	id            uint32
	rows, columns int
	headerRows    int
	done          chan struct{}
	once          sync.Once
}

func (p *imagePreview) Render(width int) ([]string, error) {
	if !p.supported {
		return WrapTextWithANSI("Image preview unavailable: V1 needs darwin-arm64 Kitty/Ghostty; use /image list or HTML export. Enter/Esc to close.", max(1, width-1))
	}
	p.columns = min(max(1, width-1), 60)
	p.rows = max(1, int(math.Ceil(float64(p.dimensions.HeightPX)*float64(p.columns)/(2*float64(p.dimensions.WidthPX)))))
	if p.rows > 10 {
		p.columns = max(1, p.columns*10/p.rows)
		p.rows = 10
	}
	header, err := WrapTextWithANSI(fmt.Sprintf("Image preview %dx%d · Enter/Esc to close", p.dimensions.WidthPX, p.dimensions.HeightPX), max(1, width-1))
	p.headerRows = len(header)
	return append(header, make([]string, p.rows)...), err
}
func (p *imagePreview) HandleInput(data string) error {
	if data == "\r" || data == "\n" || data == "\x1b" || data == "\x1b[27u" || data == "\x03" || data == "\x04" {
		p.once.Do(func() { close(p.done) })
	}
	return nil
}
func (*imagePreview) Invalidate() error { return nil }

// PreviewImage displays a validated PNG in a bounded dialog and restores the editor on close.
func (u *TextUI) PreviewImage(ctx context.Context, data string, dimensions ImageDimensions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(data) > base64.StdEncoding.EncodedLen(8<<20) {
		return fmt.Errorf("preview PNG exceeds 8 MiB")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(data)
	if err != nil {
		return fmt.Errorf("invalid preview base64: %w", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(decoded))
	if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 40_000_000/config.Height || dimensions.WidthPX != config.Width || dimensions.HeightPX != config.Height {
		return fmt.Errorf("invalid preview PNG dimensions")
	}
	if _, err := png.Decode(bytes.NewReader(decoded)); err != nil {
		return err
	}
	term := strings.ToLower(os.Getenv("TERM"))
	program := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	supported := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && os.Getenv("TMUX") == "" && os.Getenv("STY") == "" && os.Getenv("ZELLIJ") == "" && (term == "xterm-kitty" || program == "kitty" || program == "ghostty")
	id, err := AllocateImageID()
	if err != nil {
		return err
	}
	p := &imagePreview{data: data, dimensions: dimensions, supported: supported, id: id, done: make(chan struct{})}
	u.mu.Lock()
	if u.dialog != nil {
		u.mu.Unlock()
		return fmt.Errorf("another dialog is active")
	}
	u.editor.cancelAutocomplete()
	u.dialog = p
	err = u.render()
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		defer u.mu.Unlock()
		if u.dialog == p {
			u.clearImagePreview()
			u.dialog = nil
			_ = u.render()
		}
	}()
	if err != nil {
		return err
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-u.Done():
		return u.Err()
	}
}

func (u *TextUI) clearImagePreview() {
	if p, ok := u.dialog.(*imagePreview); ok && p.supported {
		deletion, _ := DeleteKittyImage(p.id)
		_ = u.terminal.Write(deletion)
	}
}

func (u *TextUI) imagePreviewOutput(state TUIMainScreenRenderState, visible, dialogRows int) string {
	p, ok := u.dialog.(*imagePreview)
	if !ok || !p.supported {
		return ""
	}
	deletion, _ := DeleteKittyImage(p.id)
	if dialogRows <= p.headerRows {
		return deletion
	}
	rows := min(p.rows, dialogRows-p.headerRows)
	columns := max(1, p.columns*rows/p.rows)
	noMove := false
	sequence, err := EncodeKitty(p.data, KittyEncodeOptions{Columns: &columns, Rows: &rows, ImageID: &p.id, MoveCursor: &noMove})
	if err != nil {
		return ""
	}
	row := visible - dialogRows + p.headerRows
	position := moveRows(row-state.HardwareCursorRow) + "\r"
	if u.mode == TUIModeFullscreen {
		position = fmt.Sprintf("\x1b[%d;1H", row+1)
	}
	return deletion + "\x1b7" + position + sequence + "\x1b8"
}
