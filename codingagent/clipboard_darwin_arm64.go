package codingagent

import (
	"context"
	"fmt"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nankedr/pig/ai"
)

var clipboardAppKit = sync.OnceValue(func() error {
	_, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	return err
})

func readClipboardImage(ctx context.Context) (*ai.ImageContent, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if err := clipboardAppKit(); err != nil {
		return nil, "", fmt.Errorf("clipboard unavailable: %w", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	board := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("generalPasteboard"))
	nsString := func(value string) objc.ID {
		return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
	}
	for _, format := range []struct{ uti, mime string }{{"public.png", "image/png"}, {"public.jpeg", "image/jpeg"}, {"com.compuserve.gif", "image/gif"}, {"org.webmproject.webp", "image/webp"}} {
		data := board.Send(objc.RegisterName("dataForType:"), nsString(format.uti))
		if data == 0 {
			continue
		}
		if n := objc.Send[uint64](data, objc.RegisterName("length")); n == 0 || n > ai.MaxUserImageBytes {
			return nil, "", fmt.Errorf("clipboard image must contain 1 byte to 8 MiB")
		}
		encoded := data.Send(objc.RegisterName("base64EncodedStringWithOptions:"), uint64(0))
		image := ai.ImageContent{Type: ai.ContentTypeImage, MIMEType: format.mime, Data: objc.Send[string](encoded, objc.RegisterName("UTF8String"))}
		if err := ai.ValidateUserImage(image); err != nil {
			return nil, "", err
		}
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		return &image, "", nil
	}
	text := board.Send(objc.RegisterName("stringForType:"), nsString("public.utf8-plain-text"))
	if text == 0 {
		return nil, "", fmt.Errorf("clipboard has no supported image or text; use /image add <path> (TIFF/helper conversion is V2)")
	}
	if n := objc.Send[uint64](text, objc.RegisterName("length")); n > 1<<20 {
		return nil, "", fmt.Errorf("clipboard text exceeds local limit")
	}
	return nil, objc.Send[string](text, objc.RegisterName("UTF8String")), ctx.Err()
}
