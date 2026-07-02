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

// initFilePickerActions registers the JS→Go entry points the mobile real-input
// file-picker overlays call once the user has picked a file. The overlay's
// change handler stashes the file via _fpConsumePending and then invokes one of
// these so the normal import/upload flow runs (consuming the pending pick).
// Mutations are queued so they execute on the next Update tick after seqMu is
// released — JS must never enter UI code while drum.Update holds the lock.
func (g *Game) initFilePickerActions() {
	js.Global().Set("_fpStartImport", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) {
			if g.drum != nil {
				g.drum.StartOverflowImport()
			}
		})
		return nil
	}))
	js.Global().Set("_fpStartUpload", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.QueueAction(func(g *Game) {
			if g.drum != nil {
				g.drum.StartOverflowUpload()
			}
		})
		return nil
	}))
}
