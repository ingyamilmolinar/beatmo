//go:build js && !test

package ui

import "syscall/js"

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
    js.Global().Set("startPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        g.drum.playPressed = true
        return nil
    }))
	js.Global().Set("incrementBPM", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		g.drum.SetBPM(g.drum.BPM() + 1)
		return nil
	}))
    js.Global().Set("currentBeat", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        return js.ValueOf(g.currentBeat())
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
        if g.drum == nil { return js.ValueOf("") }
        if data, err := g.drum.exportBytes(); err == nil {
            return js.ValueOf(string(data))
        }
        return js.ValueOf("")
    }))
    // Import a JSON string to rebuild the state.
    js.Global().Set("importJSON", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        if len(args) < 1 { return nil }
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
        if g.drum == nil { return nil }
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
        if g.drum == nil { return js.ValueOf(0) }
        return js.ValueOf(g.drum.Offset)
    }))
    // drumLength() -> int
    js.Global().Set("drumLength", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        if g.drum == nil { return js.ValueOf(0) }
        return js.ValueOf(g.drum.Length)
    }))
    // timelineBeats() -> int
    js.Global().Set("timelineBeats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
        if g.drum == nil { return js.ValueOf(0) }
        return js.ValueOf(g.drum.timelineBeats)
    }))
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
