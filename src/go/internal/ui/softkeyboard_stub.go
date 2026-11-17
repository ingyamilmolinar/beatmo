//go:build !js || test

package ui

//nolint:unused // called from js_exports_init.go (WASM build tag)
func softKeyboardInit()                 {}
func softKeyboardShow(inputmode string) {}
func softKeyboardHide()                 {}
func softKeyboardDrainChars() []rune    { return nil }
func softKeyboardActive() bool          { return false }
