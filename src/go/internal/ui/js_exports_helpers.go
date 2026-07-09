//go:build js && !test

package ui

import (
	"image"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
)

// jsArgs wraps the raw js.Value argument slice passed to a JS-exported
// function with bounds- and type-safe typed accessors. Out-of-range or
// wrong-typed reads return the zero value instead of panicking, which
// collapses the per-export `if len(args) < N { return … }` guard boilerplate
// down to the cases where the sentinel return is actually meaningful.
type jsArgs struct{ v []js.Value }

// Len reports how many arguments the caller passed.
func (a jsArgs) Len() int { return len(a.v) }

// Int returns args[i] as an int, or 0 if absent / not a number.
func (a jsArgs) Int(i int) int { return jsArgInt(a.v, i, 0) }

// IntOr returns args[i] as an int, or def if absent / not a number.
func (a jsArgs) IntOr(i, def int) int { return jsArgInt(a.v, i, def) }

// Float returns args[i] as a float64, or 0 if absent / not a number.
func (a jsArgs) Float(i int) float64 { return jsArgFloat(a.v, i, 0) }

// FloatOr returns args[i] as a float64, or def if absent / not a number.
func (a jsArgs) FloatOr(i int, def float64) float64 { return jsArgFloat(a.v, i, def) }

// Str returns args[i] as a string, or "" if absent / falsy.
func (a jsArgs) Str(i int) string { return jsArgString(a.v, i, "") }

// StrOr returns args[i] as a string, or def if absent / falsy.
func (a jsArgs) StrOr(i int, def string) string { return jsArgString(a.v, i, def) }

// Bool returns args[i] truthiness, or false if absent.
func (a jsArgs) Bool(i int) bool {
	if i >= len(a.v) {
		return false
	}
	return a.v[i].Truthy()
}

// At returns the raw js.Value at index i (js.Undefined() if absent) for the
// rare export that needs a non-scalar argument — a JS array or object it must
// probe with .Type()/.Length()/.Index()/.Get().
func (a jsArgs) At(i int) js.Value {
	if i >= len(a.v) {
		return js.Undefined()
	}
	return a.v[i]
}

// jsFn adapts a jsArgs-based handler into a js.Func, hiding the
// this/args/interface{} signature boilerplate at every registration site.
// Callers keep the literal Global-Set registration form, passing jsFn as the
// second argument, so the export-name text stays visible to the catalogue
// drift guard (which scans registration source by regex).
func jsFn(fn func(a jsArgs) any) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		return fn(jsArgs{v: args})
	})
}

func boolSliceToJS(arr []bool) js.Value {
	if arr == nil {
		return js.ValueOf(nil)
	}
	jsArr := js.Global().Get("Array").New(len(arr))
	for i, v := range arr {
		jsArr.SetIndex(i, v)
	}
	return jsArr
}

func rectToJS(r image.Rectangle) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("x", r.Min.X)
	obj.Set("y", r.Min.Y)
	obj.Set("w", r.Dx())
	obj.Set("h", r.Dy())
	return obj
}

func floatSliceToJS(arr []float64) js.Value {
	jsArr := js.Global().Get("Array").New(len(arr))
	for i, v := range arr {
		jsArr.SetIndex(i, v)
	}
	return jsArr
}

func nodeTypeSliceToJS(arr []model.NodeType) js.Value {
	if arr == nil {
		return js.ValueOf(nil)
	}
	jsArr := js.Global().Get("Array").New(len(arr))
	for i, v := range arr {
		jsArr.SetIndex(i, int(v))
	}
	return jsArr
}

func transportSnapshotToJS(s gamestate.Snapshot) js.Value {
	obj := js.Global().Get("Object").New()
	obj.Set("playing", s.Playing)
	obj.Set("paused", s.Paused)
	obj.Set("appliedBPM", s.AppliedBPM)
	obj.Set("beatBase", s.BeatBase)
	obj.Set("lastBeat", s.LastBeat)
	obj.Set("lastDisplayBeat", s.LastDisplayBeat)
	obj.Set("lastStep", s.LastStep)
	obj.Set("pausedBeats", s.PausedBeats)
	obj.Set("seekFreezeFrames", s.SeekFreezeFrames)
	obj.Set("justPaused", s.JustPaused)
	obj.Set("justResumed", s.JustResumed)
	return obj
}
