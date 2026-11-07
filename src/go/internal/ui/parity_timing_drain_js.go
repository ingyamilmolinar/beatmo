//go:build js

package ui

import "syscall/js"

// On WASM audio.Now() is AudioContext.currentTime, so the Go trace's idealWhen and the JS startWhen share one
// clock and drift is simply startWhen - idealWhen (positive = late), as on desktop.
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
