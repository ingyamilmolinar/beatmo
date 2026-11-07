//go:build js && !test

// Read-only heap and analyzer-bridge counters, sampled from DevTools or long browser tests without a rebuild.

package ui

import (
	"os"
	"runtime"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) initJSDiag() {
	js.Global().Set("memSizes", jsFn(func(args jsArgs) any {
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
		// Tells "trigger never fired" from "fired but NumGC did not advance" (a single-threaded WASM GC edge case).
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

	js.Global().Set("analyzerBridgeStats", jsFn(func(args jsArgs) any {
		calls, reads := audio.AnalyzerBridgeStats()
		obj := js.Global().Get("Object").New()
		obj.Set("calls", float64(calls))
		obj.Set("elementReads", float64(reads))
		// Each float64 read copies 8 bytes into the Go heap: an estimate without instrumenting allocations.
		obj.Set("bytesCopiedEstimate", float64(reads*8))
		return obj
	}))

	js.Global().Set("resetAnalyzerBridgeStats", jsFn(func(args jsArgs) any {
		audio.ResetAnalyzerBridgeStats()
		return nil
	}))

	// Test-only (the __ prefix exempts it from the bridge catalogue): proves window.__beatmoEnv reaches os.Getenv.
	js.Global().Set("__envProbe", jsFn(func(args jsArgs) any {
		return os.Getenv(args.Str(0))
	}))
}

// rowKey formats a row index for use as a JS object key.
func rowKey(r int) string {
	return "row" + itoaSmall(r)
}

// Non-allocating, for the few diagnostic rows; avoids strconv.
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
