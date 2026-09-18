package tui

import (
	"github.com/ebitengine/purego"
	"sync"
)

var nativeFlags = sync.OnceValue(func() func(int32) uint64 {
	library, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_LOCAL)
	if err != nil {
		return nil
	}
	address, err := purego.Dlsym(library, "CGEventSourceFlagsState")
	if err != nil {
		purego.Dlclose(library)
		return nil
	}
	var flags func(int32) uint64
	purego.RegisterFunc(&flags, address)
	return flags
})

func nativeModifierPressed(key ModifierKey) bool {
	mask := map[ModifierKey]uint64{ModifierKeyShift: 1 << 17, ModifierKeyControl: 1 << 18, ModifierKeyOption: 1 << 19, ModifierKeyCommand: 1 << 20}[key]
	if mask == 0 {
		return false
	}
	flags := nativeFlags()
	return flags != nil && flags(0)&mask != 0
}
