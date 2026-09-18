//go:build !darwin

package tui

func nativeModifierPressed(ModifierKey) bool { return false }
