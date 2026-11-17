//go:build js && !test

package ui

import (
	"image"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
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

// Instrument parses args[i] as an instrument id (instrument.ParseID); ""
// when absent or invalid, which every instrument API treats as "none".
func (a jsArgs) Instrument(i int) instrument.ID {
	id, err := instrument.ParseID(a.Str(i))
	if err != nil {
		return ""
	}
	return id
}

// Channel parses args[i] as a mixer channel name ("main" or an instrument
// id); the zero ID when absent or invalid. "" names no channel.
func (a jsArgs) Channel(i int) channel.ID {
	id, err := channel.Parse(a.Str(i))
	if err != nil {
		return channel.ID{}
	}
	return id
}

// ChannelOr is Channel with def for an absent argument.
func (a jsArgs) ChannelOr(i int, def channel.ID) channel.ID {
	if i >= len(a.v) || a.v[i].IsUndefined() || a.v[i].IsNull() {
		return def
	}
	return a.Channel(i)
}

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
// drift guard (which scans registration source by regex). The result passes
// through jsSafe: an instrument.ID (or any named basic type) in it would make
// syscall/js panic, and nothing at compile time can see that.
func jsFn(fn func(a jsArgs) any) js.Func {
	return js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		r := jsSafe(fn(jsArgs{v: args}))
		jsExportSeq.Add(1)
		if h := afterJSExport; h != nil {
			h()
		}
		return r
	})
}

// afterJSExport runs after every bridge call. The Game points it at kickLane:
// a bridge edit happens outside Update, and on WASM the sequencer goroutine
// only runs on its next JS-timer tick — the kick makes it runnable before
// control returns to JS, so the audio lane sees the edit at once.
var afterJSExport func()

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
