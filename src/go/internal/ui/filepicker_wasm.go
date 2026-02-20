//go:build js && !test

package ui

import "syscall/js"

var (
	fpRegisterRectFn js.Value
	fpClearRectsFn   js.Value
	fpInitialized    bool
)

// filePickerInit caches JS function references for the file picker rect system.
// Called once during initJS().
func filePickerInit() {
	g := js.Global()
	fpRegisterRectFn = g.Get("_fpRegisterRect")
	fpClearRectsFn = g.Get("_fpClearRects")
	fpInitialized = fpRegisterRectFn.Truthy() && fpClearRectsFn.Truthy()
}

// filePickerRegisterRect registers a canvas region as a file picker trigger.
// On touchend inside this rect, the JS layer will create a file input and call
// input.click() synchronously within the trusted gesture handler.
func filePickerRegisterRect(id string, x, y, w, h int, accept string) {
	if !fpInitialized {
		return
	}
	fpRegisterRectFn.Invoke(id, x, y, w, h, accept)
}

// filePickerClearRects removes all registered file picker rects.
func filePickerClearRects() {
	if !fpInitialized {
		return
	}
	fpClearRectsFn.Invoke()
}
