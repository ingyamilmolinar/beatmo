//go:build js && !test

package ui

import "syscall/js"

var (
	fpRegisterRectFn js.Value
	fpClearRectsFn   js.Value
	fpInitialized    bool
)

func filePickerInit() {
	g := js.Global()
	fpRegisterRectFn = g.Get("_fpRegisterRect")
	fpClearRectsFn = g.Get("_fpClearRects")
	fpInitialized = fpRegisterRectFn.Truthy() && fpClearRectsFn.Truthy()
}

// The JS layer clicks a file input synchronously inside the trusted touchend, which a browser requires.
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

// Queued for the next Update tick: JS must never enter UI code while drum.Update holds seqMu.
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
