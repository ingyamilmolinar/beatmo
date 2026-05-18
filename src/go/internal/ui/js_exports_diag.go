//go:build js && !test

// Package ui — diagnostic JS exports for runtime memory inspection.
//
// memSizes() and analyzerBridgeStats() let an operator (or a long-running
// browser test) sample heap state and analyzer-bridge call counts from
// DevTools without rebuilding the WASM. Pure read-only; no behaviour
// change. Added under the OOM-prevention plan as Phase 1A diagnostics.

package ui

import (
	"runtime"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// initJSDiag wires memSizes() and analyzerBridgeStats() onto the JS
// global so DevTools and Playwright tests can sample heap + bridge-call
// counters at any moment. Called once from initJS().
func (g *Game) initJSDiag() {
	js.Global().Set("memSizes", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		obj := js.Global().Get("Object").New()
		obj.Set("heapAlloc", float64(ms.HeapAlloc))
		obj.Set("heapSys", float64(ms.HeapSys))
		obj.Set("sys", float64(ms.Sys))
		obj.Set("totalAlloc", float64(ms.TotalAlloc))
		obj.Set("gcCount", float64(ms.NumGC))
		obj.Set("nextGC", float64(ms.NextGC))
		obj.Set("pauseTotalNs", float64(ms.PauseTotalNs))
		// forcedGCRuns is a heapProbeTick-side counter that bumps once per
		// runtime.GC() invocation via ForceGCIntervalFrames; lets soak tests
		// distinguish "trigger never fired" from "trigger fired but
		// runtime.NumGC didn't advance" (single-threaded WASM GC edge case).
		obj.Set("forcedGCRuns", float64(ForcedGCRunsForTest()))
		if g != nil {
			obj.Set("frame", float64(g.frame))
		}

		// Per-row archive entry counts via the timeline service accessor.
		archives := js.Global().Get("Object").New()
		if g != nil && g.drum != nil && g.timeline != nil {
			for r := 0; r < len(g.drum.Rows); r++ {
				archives.Set(rowKey(r), g.timeline.ArchiveLenForTest(r))
			}
		}
		obj.Set("timelineArchives", archives)

		// Parity bookkeeping sizes (read under parityMu).
		if g != nil {
			g.parityMu.Lock()
			obj.Set("parityAudio", len(g.parityAudio))
			seq := js.Global().Get("Object").New()
			for r, m := range g.paritySeqDecisions {
				seq.Set(rowKey(r), len(m))
			}
			obj.Set("paritySeqDecisions", seq)
			g.parityMu.Unlock()
		}

		// Channel + scheduler depths.
		if g != nil {
			obj.Set("audioChDepth", len(g.audioCh))
			obj.Set("audioChCap", cap(g.audioCh))
			if g.audioScheduler != nil {
				st := g.audioScheduler.Stats()
				obj.Set("schedulerPending", float64(st.Pending))
				obj.Set("schedulerFired", float64(st.Fired))
				obj.Set("schedulerCancelled", float64(st.Cancelled))
				obj.Set("schedulerDropped", float64(st.Dropped))
			}
		}

		// Per-NodeID maps that grow with unique nodes triggered.
		if g != nil {
			g.triggerMu.RLock()
			obj.Set("nodeAnim", len(g.nodeAnim))
			triggered := js.Global().Get("Object").New()
			for r, m := range g.lastTriggeredByRow {
				triggered.Set(rowKey(r), len(m))
			}
			obj.Set("lastTriggeredByRow", triggered)
			counts := js.Global().Get("Object").New()
			for r, m := range g.nodeTriggerCountsByRow {
				counts.Set(rowKey(r), len(m))
			}
			obj.Set("nodeTriggerCountsByRow", counts)
			g.triggerMu.RUnlock()
		}

		// Highlight retention state (per-frame eviction bound).
		if g != nil {
			obj.Set("highlightedBeats", len(g.highlightedBeats))
		}

		return obj
	}))

	js.Global().Set("analyzerBridgeStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		calls, reads := audio.AnalyzerBridgeStats()
		obj := js.Global().Get("Object").New()
		obj.Set("calls", float64(calls))
		obj.Set("elementReads", float64(reads))
		// Heuristic: each float64 read is 8 bytes copied into the Go
		// heap. Lets the operator estimate the Go-side analyzer churn
		// without instrumenting allocations directly.
		obj.Set("bytesCopiedEstimate", float64(reads*8))
		return obj
	}))

	js.Global().Set("resetAnalyzerBridgeStats", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		audio.ResetAnalyzerBridgeStats()
		return nil
	}))
}

// rowKey formats a row index for use as a JS object key.
func rowKey(r int) string {
	return "row" + itoaSmall(r)
}

// itoaSmall is a tiny non-allocating integer-to-string for non-negative
// values up to ~999. The diagnostics object only ever has a few rows, so
// avoiding strconv keeps the export lean.
func itoaSmall(n int) string {
	if n < 0 {
		n = -n
	}
	if n < 10 {
		return string('0' + byte(n))
	}
	if n < 100 {
		return string([]byte{'0' + byte(n/10), '0' + byte(n%10)})
	}
	return string([]byte{'0' + byte(n/100), '0' + byte((n/10)%10), '0' + byte(n%10)})
}
