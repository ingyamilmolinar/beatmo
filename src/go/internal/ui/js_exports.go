//go:build js && !test

package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"syscall/js"
)

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	js.Global().Set("startPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		js.Global().Get("console").Call("log", "[WASM] startPlay() called")
		g.drum.playPressed = true
		return nil
	}))
	js.Global().Set("incrementBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		b := g.drum.BPM() + 1
		g.drum.SetBPM(b)
		js.Global().Get("console").Call("log", "[WASM] incrementBPM ->", b)
		return nil
	}))
	js.Global().Set("currentBeat", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		cb := g.currentBeat()
		js.Global().Get("console").Call("log", "[WASM] currentBeat()=", cb)
		return js.ValueOf(cb)
	}))
	// ensureDefaultPath builds a minimal single-edge path if the graph is empty
	// so browser tests can start playback without interacting with the editor.
	js.Global().Set("ensureDefaultPath", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(g.nodes) == 0 {
			g.pendingStartRow = 0
			n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
			n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
			g.addEdge(n0, n1)
			g.pendingStartRow = -1
			g.start = n0
			g.graph.StartNodeID = n0.ID
			g.updateBeatInfos()
		}
		return nil
	}))
	js.Global().Set("setRowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 2 {
			return nil
		}
		row := args[0].Int()
		vol := args[1].Float()
		if row >= 0 && row < len(g.drum.Rows) {
			if vol < 0 {
				vol = 0
			}
			if vol > 1 {
				vol = 1
			}
			g.drum.Rows[row].Volume = vol
			if row < len(g.drum.rowVolSliders) {
				g.drum.rowVolSliders[row].Value = vol
			}
		}
		return nil
	}))

	// Export the current state as a JSON string.
	js.Global().Set("exportJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf("")
		}
		if data, err := g.drum.exportBytes(); err == nil {
			return js.ValueOf(string(data))
		}
		return js.ValueOf("")
	}))
	// Import a JSON string to rebuild the state.
	js.Global().Set("importJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		txt := args[0].String()
		_ = g.Import([]byte(txt))
		return nil
	}))

	// sliderRect(row) -> {x, y, w, h}
	// Exposes the on-screen rectangle for a drum row's volume slider so
	// browser tests can simulate pointer interaction precisely.
	js.Global().Set("sliderRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if len(args) < 1 {
			return nil
		}
		row := args[0].Int()
		if row < 0 || row >= len(g.drum.rowVolSliders) {
			return nil
		}
		r := g.drum.rowVolSliders[row].Rect()
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	// timelineRect() -> {x,y,w,h}
	js.Global().Set("timelineRect", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		r := g.drum.timelineRect
		obj := js.Global().Get("Object").New()
		obj.Set("x", r.Min.X)
		obj.Set("y", r.Min.Y)
		obj.Set("w", r.Dx())
		obj.Set("h", r.Dy())
		return obj
	}))
	// drumOffset() -> int
	js.Global().Set("drumOffset", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.Offset)
	}))
	// drumLength() -> int
	js.Global().Set("drumLength", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.Length)
	}))
	// rowVolume(row) -> float
	js.Global().Set("rowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0.0)
		}
		if len(args) < 1 {
			return js.ValueOf(0.0)
		}
		r := args[0].Int()
		if r < 0 || r >= len(g.drum.Rows) {
			return js.ValueOf(0.0)
		}
		return js.ValueOf(g.drum.Rows[r].Volume)
	}))
	// setRowVolume(row, value)
	js.Global().Set("setRowVolume", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 2 {
			return nil
		}
		r := args[0].Int()
		v := args[1].Float()
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if r < 0 || r >= len(g.drum.Rows) {
			return nil
		}
		g.drum.Rows[r].Volume = v
		if r < len(g.drum.rowVolSliders) {
			g.drum.rowVolSliders[r].Value = v
		}
		return nil
	}))
	// timelineBeats() -> int
	js.Global().Set("timelineBeats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.ValueOf(0)
		}
		return js.ValueOf(g.drum.timelineBeats)
	}))
	// setTimelineBeats(n)
	js.Global().Set("setTimelineBeats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		n := args[0].Int()
		if n < g.drum.Length {
			n = g.drum.Length
		}
		g.drum.timelineBeats = n
		return nil
	}))
	// setDrumLength(n)
	js.Global().Set("setDrumLength", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		n := args[0].Int()
		if n < 1 {
			n = 1
		}
		g.drum.SetLength(n)
		return nil
	}))
	// clickTimelineAt(frac) simulates a click on the timeline at a fractional
	// position [0,1], updating the drum offset using the same centering logic
	// as the UI.
	js.Global().Set("clickTimelineAt", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return nil
		}
		if len(args) < 1 {
			return nil
		}
		f := args[0].Float()
		if f < 0 {
			f = 0
		}
		if f > 1 {
			f = 1
		}
		total := g.drum.timelineBeats
		if total < g.drum.Length {
			total = g.drum.Length
		}
		center := int(f * float64(total))
		desired := center - g.drum.Length/2
		maxOff := g.drum.timelineBeats - g.drum.Length
		if maxOff < 0 {
			maxOff = 0
		}
		if desired < 0 {
			desired = 0
		}
		if desired > maxOff {
			desired = maxOff
		}
		g.drum.Offset = desired
		js.Global().Get("console").Call("log", "[WASM] clickTimelineAt", f, "-> Offset:", desired)
		return nil
	}))
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
