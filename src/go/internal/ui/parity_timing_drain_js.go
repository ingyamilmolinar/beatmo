//go:build js

package ui

import "syscall/js"

// drainEmissionReports pulls the browser emission traces recorded by audio.js
// (the `timingTraces` ring, pushed at every real `BufferSource.start(when)`
// call for a sequencer-identified note — see audio.js's timingTraces.push
// call sites) and closes the matching Go-side trace opened by
// openTimingTrace at schedule time (see game_audio_schedule.go).
//
// Drift formula: on WASM, audio.Now() == AudioContext.currentTime (see
// engine_wasm.go's Now()), so the Go trace's idealWhen (= baseNow, recorded
// in ctx-clock seconds at schedule time) and the JS trace's startWhen (the
// `when` argument passed to BufferSource.start, also ctx-clock seconds)
// share the same clock. The emitted wall-time fed to Stage-D is therefore
// simply startWhen — closeTimingTrace computes startWhen - idealWhen, i.e.
// how much later the actual scheduled start landed vs the ideal grid time
// (positive = late). This mirrors the desktop drain's semantics exactly,
// just sourced from the WebAudio clock instead of the device-clock ring.
func (g *Game) drainEmissionReports() {
	arr := js.Global().Call("getTimingTraces")
	if !arr.Truthy() {
		return
	}
	n := arr.Length()
	for i := 0; i < n; i++ {
		tr := arr.Index(i)
		row := tr.Get("row").Int()
		if row < 0 {
			continue // sentinel: fire-and-forget/preview event, never opened as a trace
		}
		abs := tr.Get("abs").Int()
		startWhen := tr.Get("startWhen").Float()
		g.closeTimingTrace(row, abs, startWhen)
	}
	js.Global().Call("resetTimingTraces")
}
