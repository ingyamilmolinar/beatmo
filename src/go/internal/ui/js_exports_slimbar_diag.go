//go:build js && !test

package ui

import "syscall/js"

// initJSSlimBarDiag wires slimBarDiag() onto the JS global. It returns the
// current drum-row render diagnostic (geometry + which compositing path drew
// the last frame) so src/js/drum_slim_bar_watch.js can read the canvas at the
// right cell band and, when it catches the intermittent "slim bars" artifact,
// log exactly which path (windowed sub-image blit vs legacy shift-and-fill)
// produced it. See drum_render_diag.go.
func (g *Game) initJSSlimBarDiag() {
	js.Global().Set("slimBarDiag", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.drum == nil {
			return js.Null()
		}
		d := g.drum.RenderDiag()
		obj := js.Global().Get("Object").New()
		for k, v := range d {
			if k == "rows" {
				continue
			}
			obj.Set(k, jsValueOf(v))
		}
		rowsAny, _ := d["rows"].([]map[string]interface{})
		arr := js.Global().Get("Array").New()
		for i, r := range rowsAny {
			ro := js.Global().Get("Object").New()
			for k, v := range r {
				ro.Set(k, jsValueOf(v))
			}
			arr.SetIndex(i, ro)
		}
		obj.Set("rows", arr)
		return obj
	}))
}

// jsValueOf maps the scalar types RenderDiag emits to JS values.
func jsValueOf(v interface{}) interface{} {
	switch t := v.(type) {
	case int:
		return t
	case bool:
		return t
	case string:
		return t
	default:
		return js.ValueOf(nil)
	}
}
