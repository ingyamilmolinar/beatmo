//go:build js && !test

package ui

import "syscall/js"

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	js.Global().Set("startPlay", js.FuncOf(func(js.Value, []js.Value) any {
		g.drum.playPressed = true
		return nil
	}))
	js.Global().Set("incrementBPM", js.FuncOf(func(js.Value, []js.Value) any {
		g.drum.SetBPM(g.drum.BPM() + 1)
		return nil
	}))
	js.Global().Set("currentBeat", js.FuncOf(func(js.Value, []js.Value) any {
		return js.ValueOf(g.currentBeat())
	}))
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
