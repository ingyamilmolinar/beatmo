//go:build js

package ui

import "syscall/js"

// newRuntimeProfile selects the browser profile under any -tags combination
// (including -tags test). This closes the build-tag hole that defaults_js.go
// + defaults_notjs.go left at GOOS=js && -tags test.
//
// If the host page sets `window.__beatmoProfileOverride = { ... }` BEFORE the
// WASM module starts, the override values are merged into the profile. Used
// by Phase F bench sweeps to flip individual knobs without rebuilding WASM
// per-knob.
func newRuntimeProfile() *RuntimeProfile {
	p := browserRuntimeProfile()
	override := js.Global().Get("__beatmoProfileOverride")
	if !override.Truthy() {
		return p
	}
	applyJSProfileOverride(p, override)
	return p
}

func applyJSProfileOverride(p *RuntimeProfile, ov js.Value) {
	if v := ov.Get("edgeCachePad"); v.Type() == js.TypeNumber {
		p.EdgeCachePad = v.Int()
	}
	if v := ov.Get("gridCachePad"); v.Type() == js.TypeNumber {
		p.GridCachePad = v.Int()
	}
	if v := ov.Get("disableNodeGlow"); v.Type() == js.TypeBoolean {
		p.DisableNodeGlow = v.Bool()
	}
	if v := ov.Get("disableEdgeArrows"); v.Type() == js.TypeBoolean {
		p.DisableEdgeArrows = v.Bool()
	}
	if v := ov.Get("simpleDrawDefault"); v.Type() == js.TypeBoolean {
		p.SimpleDrawDefault = v.Bool()
	}
	if v := ov.Get("timelineInfoThrottleMS"); v.Type() == js.TypeNumber {
		p.TimelineInfoThrottleMS = v.Int()
	}
	if v := ov.Get("audioLookaheadSec"); v.Type() == js.TypeNumber {
		p.AudioLookaheadSec = v.Float()
	}
	if v := ov.Get("audioBatchMax"); v.Type() == js.TypeNumber {
		p.AudioBatchMax = v.Int()
	}
	if v := ov.Get("sequencerTickMS"); v.Type() == js.TypeNumber {
		p.SequencerTickMS = v.Int()
	}
	if v := ov.Get("adaptivePanPad"); v.Type() == js.TypeBoolean {
		p.AdaptivePanPad = v.Bool()
	}
	if v := ov.Get("fastPanDetect"); v.Type() == js.TypeBoolean {
		p.FastPanDetect = v.Bool()
	}
	if v := ov.Get("predictorBackoffOnSlowDraw"); v.Type() == js.TypeBoolean {
		p.PredictorBackoffOnSlowDraw = v.Bool()
	}
}
