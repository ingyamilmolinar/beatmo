//go:build js && !test

package ui

import "syscall/js"

// Lets the canvas watcher sample the right band and log which compositing path drew a slim-bar artifact.
func (g *Game) initJSSlimBarDiag() {
	js.Global().Set("slimBarDiag", jsFn(func(args jsArgs) any {
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
