package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// heapProbeEnabled is read once at init from BEATMO_HEAP_PROBE. The probe is
// off by default — when on, it samples runtime.MemStats once per second of
// playback and logs the per-second TotalAlloc rate plus the top tracked
// image-allocator tags. Tuned to be cheap enough to leave on in production
// WASM if a future regression makes it useful.
//
// The probe exists because the fast WASM OOM (Game.Update OOM after ~13 s of
// play, May 2026) was diagnosed only after writing a Go-side test and a
// pprof memprofile run; in the wild we had nothing but a panic stack trace.
// With this probe enabled, a future similar regression logs its allocation
// rate every second and surfaces the top contributors by tag, so the
// diagnosis is one console line instead of an hour of code-reading.
var heapProbeEnabled = os.Getenv("BEATMO_HEAP_PROBE") == "1"

// heapWatchEnabled is the broader gate for the 30s rollup WARN. Activated
// by either BEATMO_HEAP_WATCH=1 or BEATMO_HEAP_PROBE=1 (PROBE implies
// WATCH). Phase 3E of the long-session OOM plan: catches steady-state
// heap growth in production sessions without the per-second log spam.
var heapWatchEnabled = heapProbeEnabled ||
	os.Getenv("BEATMO_HEAP_WATCH") == "1"

const heapProbeRingSize = 60 // ~1 minute of 1-second samples

type heapProbeSample struct {
	frame      int64
	heapAlloc  uint64
	totalAlloc uint64
	imagesByTag string
}

type heapProbeState struct {
	mu        sync.Mutex
	ring      []heapProbeSample
	head      int
	prevTotal uint64
	prevFrame int64
}

var heapProbe = &heapProbeState{
	ring: make([]heapProbeSample, heapProbeRingSize),
}

// heapWatchState tracks the rolling 30s-window heap delta for the
// rollup WARN. Independent of heapProbeState so the two can be gated
// separately and one's reset doesn't perturb the other.
type heapWatchState struct {
	mu             sync.Mutex
	windowFrame    int64  // frame at which the window started
	windowHeap     uint64 // HeapAlloc at window start
	windowTotal    uint64 // TotalAlloc at window start
	windowBridge   uint64 // analyzer bridge calls at window start
	windowReads    uint64 // analyzer bridge element reads at window start
	lastEmitFrame  int64
}

var heapWatch = &heapWatchState{}

// heapWatchTickFrames is the rollup window expressed in frames. 30 s ×
// 60 fps = 1800. Computed in frames (not wall clock) so the rollup is
// deterministic in tests and bench runs.
const heapWatchTickFrames = 1800

// forcedGCRuns counts how many times heapProbeTick has triggered
// runtime.GC() via the ForceGCInterval path. Exposed through
// memSizes() so soak tests can confirm the pacing actually fires (a
// gcCount of 0 alone doesn't distinguish "trigger didn't fire" from
// "trigger fired but NumGC counter didn't move").
var (
	forcedGCRuns      uint64
	forcedGCLastFired time.Time
)

// ForcedGCRunsForTest returns the cumulative forced-GC trigger count
// for diagnostics + soak assertions. Atomic-style read is fine because
// the writer is the single Update goroutine.
func ForcedGCRunsForTest() uint64 { return forcedGCRuns }

// ResetForcedGCRunsForTest zeros the counter + last-fired tracker so a
// test can measure a clean window. Must be called before the test's
// driving loop.
func ResetForcedGCRunsForTest() {
	forcedGCRuns = 0
	forcedGCLastFired = time.Time{}
}

// heapProbeTick is called from Game.Update once per frame. It is a no-op
// when the probe is disabled. Sampling cadence is "every 60 frames" rather
// than wall-clock-based so the probe is deterministic in tests; at 60 FPS
// this samples roughly once per second.
func (g *Game) heapProbeTick() {
	// Forced GC pacing — runs regardless of BEATMO_HEAP_PROBE / WATCH env
	// vars because it's a runtime-profile knob, not a debug toggle. Single-
	// threaded WASM Go GC starves under sustained audio + parameter-dispatch
	// load (gc=0 across 300 s of synth-tab + param-drag observed 2026-05-17);
	// calling runtime.GC() at the heap-probe yield point reclaims the cycle
	// every ForceGCInterval of wall-clock time so HeapAlloc plateaus instead
	// of growing linearly to the 2 GB WASM ceiling. Wall-clock based (not
	// frame-count) because the browser frame rate under sustained load
	// drops to ~7-13 fps and a frame-count cadence would fire too rarely.
	// See RuntimeProfile field for the bench citation.
	if g != nil {
		if interval := RuntimeProf().ForceGCInterval; interval > 0 {
			now := time.Now()
			if forcedGCLastFired.IsZero() {
				forcedGCLastFired = now
			} else if now.Sub(forcedGCLastFired) >= interval {
				forcedGCRuns++
				runtime.GC()
				forcedGCLastFired = now
			}
		}
	}
	// Always run the 30 s rollup if it's enabled, even when the per-second
	// probe is off. Cheap: one ReadMemStats every 1 800 frames.
	if heapWatchEnabled && g != nil && g.frame > 0 && g.frame%heapWatchTickFrames == 0 {
		g.heapWatchEmit()
	}
	if !heapProbeEnabled {
		return
	}
	if g == nil {
		return
	}
	if g.frame%60 != 0 {
		return
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	heapProbe.mu.Lock()
	prev := heapProbe.prevTotal
	prevFrame := heapProbe.prevFrame
	delta := ms.TotalAlloc - prev
	frames := g.frame - prevFrame
	heapProbe.prevTotal = ms.TotalAlloc
	heapProbe.prevFrame = g.frame
	tags := imagesByTagSnapshot()
	sample := heapProbeSample{
		frame:       g.frame,
		heapAlloc:   ms.HeapAlloc,
		totalAlloc:  ms.TotalAlloc,
		imagesByTag: tags,
	}
	heapProbe.ring[heapProbe.head] = sample
	heapProbe.head = (heapProbe.head + 1) % heapProbeRingSize
	heapProbe.mu.Unlock()

	if g.logger != nil && prevFrame > 0 && frames > 0 {
		bytesPerFrame := float64(delta) / float64(frames)
		mbPerSec := bytesPerFrame * 60 / (1024 * 1024)
		g.logger.Infof("[heap] frame=%d heapAlloc=%dKB totalAlloc=%dKB delta=%dKB (~%.1f KB/frame ~%.2f MB/s @60fps) images=%s",
			g.frame, ms.HeapAlloc>>10, ms.TotalAlloc>>10, delta>>10,
			bytesPerFrame/1024, mbPerSec, tags)
	}
}

// heapWatchEmit emits one WARN line summarising the heap-growth slope
// across the last heapWatchTickFrames frames (~30 s at 60 FPS). Includes
// analyzer bridge stats so a per-frame analyzer leak surfaces here even
// without the per-second probe.
//
// The first call in a session is silent — the watch state is empty
// until the second tick, when we have a real window to compare against.
func (g *Game) heapWatchEmit() {
	if g == nil || g.logger == nil {
		return
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	bridgeCalls, bridgeReads := audio.AnalyzerBridgeStats()

	heapWatch.mu.Lock()
	prevFrame := heapWatch.windowFrame
	prevHeap := heapWatch.windowHeap
	prevTotal := heapWatch.windowTotal
	prevBridge := heapWatch.windowBridge
	prevReads := heapWatch.windowReads
	heapWatch.windowFrame = g.frame
	heapWatch.windowHeap = ms.HeapAlloc
	heapWatch.windowTotal = ms.TotalAlloc
	heapWatch.windowBridge = bridgeCalls
	heapWatch.windowReads = bridgeReads
	heapWatch.lastEmitFrame = g.frame
	heapWatch.mu.Unlock()

	if prevFrame == 0 {
		return // first window; nothing to compare
	}

	frames := g.frame - prevFrame
	if frames <= 0 {
		return
	}
	heapDelta := int64(ms.HeapAlloc) - int64(prevHeap)
	totalDelta := ms.TotalAlloc - prevTotal
	bridgeDelta := bridgeCalls - prevBridge
	readsDelta := bridgeReads - prevReads
	mbPerMin := float64(heapDelta) * 60 / float64(frames) / (1024 * 1024)
	churnMBPerMin := float64(totalDelta) * 60 / float64(frames) / (1024 * 1024)

	g.logger.Warnf("[heap-watch] window=%dfr heapAlloc=%dKB heap_growth=%+dKB (%.2f MB/min) churn=%.2f MB/min bridge_calls=%d reads=%d",
		frames, ms.HeapAlloc>>10, heapDelta>>10, mbPerMin, churnMBPerMin,
		bridgeDelta, readsDelta)
}

// HeapProbeSnapshot returns a human-readable dump of the heap-probe ring.
// Used by the dumpHeapProbe JS export so a WASM session can post-mortem
// inspect what was happening in the seconds before an OOM.
func HeapProbeSnapshot() string {
	heapProbe.mu.Lock()
	defer heapProbe.mu.Unlock()
	if !heapProbeEnabled {
		return "heap probe disabled (set BEATMO_HEAP_PROBE=1)"
	}
	var b strings.Builder
	b.WriteString("frame,heapAllocKB,totalAllocKB,imagesByTag\n")
	// Walk in chronological order: head points at the oldest slot.
	for i := 0; i < heapProbeRingSize; i++ {
		s := heapProbe.ring[(heapProbe.head+i)%heapProbeRingSize]
		if s.frame == 0 {
			continue
		}
		fmt.Fprintf(&b, "%d,%d,%d,%s\n", s.frame, s.heapAlloc>>10, s.totalAlloc>>10, s.imagesByTag)
	}
	return b.String()
}
