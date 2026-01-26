//go:build js && !test

package ui

import (
	"image"
	"syscall/js"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/gamestate"
)

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
