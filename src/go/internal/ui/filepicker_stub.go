//go:build !js || test

package ui

// capturedFilePickerRect records a filePickerRegisterRect call for test inspection.
type capturedFilePickerRect struct {
	ID         string
	X, Y, W, H int
	Accept     string
}

// testCapturedFilePickerRects, when non-nil, causes filePickerRegisterRect to append here.
var testCapturedFilePickerRects *[]capturedFilePickerRect

//nolint:unused // called from js_exports_init.go (WASM build tag)
func filePickerInit() {}

func filePickerRegisterRect(id string, x, y, w, h int, accept string) {
	if testCapturedFilePickerRects != nil {
		*testCapturedFilePickerRects = append(*testCapturedFilePickerRects, capturedFilePickerRect{
			ID: id, X: x, Y: y, W: w, H: h, Accept: accept,
		})
	}
}

func filePickerClearRects() {
	if testCapturedFilePickerRects != nil {
		*testCapturedFilePickerRects = (*testCapturedFilePickerRects)[:0]
	}
}
