//go:build !js || test

package ui

// capturedRect records a softKeyboardRegisterRect call for test inspection.
type capturedRect struct {
	ID         string
	X, Y, W, H int
	InputMode  string
}

// testCapturedRects, when non-nil, causes softKeyboardRegisterRect to append here.
var testCapturedRects *[]capturedRect

//nolint:unused // called from js_exports_init.go (WASM build tag)
func softKeyboardInit()                 {}
func softKeyboardShow(inputmode string) {}
func softKeyboardHide()                 {}
func softKeyboardDrainChars() []rune    { return nil }
func softKeyboardActive() bool          { return false }
func softKeyboardRegisterRect(id string, x, y, w, h int, inputmode string) {
	if testCapturedRects != nil {
		*testCapturedRects = append(*testCapturedRects, capturedRect{
			ID: id, X: x, Y: y, W: w, H: h, InputMode: inputmode,
		})
	}
}

func softKeyboardClearRects() {
	if testCapturedRects != nil {
		*testCapturedRects = (*testCapturedRects)[:0]
	}
}
