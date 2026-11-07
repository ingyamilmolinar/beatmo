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

// Off by default; when on, logs the per-second allocation rate and the top image tags. Cheap enough for
// production WASM, so an OOM in the wild gets a one-line diagnosis instead of only a panic stack.
var heapProbeEnabled = os.Getenv("BEATMO_HEAP_PROBE") == "1"

// Catches steady-state heap growth with one WARN per 30 s, without the per-second spam; PROBE implies WATCH.
var heapWatchEnabled = heapProbeEnabled ||
	os.Getenv("BEATMO_HEAP_WATCH") == "1"

const heapProbeRingSize = 60 // ~1 minute of 1-second samples

type heapProbeSample struct {
	frame       int64
	heapAlloc   uint64
	totalAlloc  uint64
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

// Independent of heapProbeState so the two gate and reset separately.
type heapWatchState struct {
	mu            sync.Mutex
	windowFrame   int64  // frame at which the window started
	windowHeap    uint64 // HeapAlloc at window start
	windowTotal   uint64 // TotalAlloc at window start
	windowBridge  uint64 // analyzer bridge calls at window start
	windowReads   uint64 // analyzer bridge element reads at window start
	lastEmitFrame int64
}

var heapWatch = &heapWatchState{}

// In frames, not wall clock, so it is deterministic in tests and benches.
const heapWatchTickFrames = 1800

// Lets soak tests confirm the pacing fires: a gcCount of 0 cannot tell "never fired" from "NumGC did not move".
var (
	forcedGCRuns      uint64
	forcedGCLastFired time.Time
)

// The single Update goroutine writes it.
func ForcedGCRunsForTest() uint64 { return forcedGCRuns }

func ResetForcedGCRunsForTest() {
	forcedGCRuns = 0
	forcedGCLastFired = time.Time{}
}

// Every 60 frames, not wall clock, so it is deterministic in tests.
func (g *Game) heapProbeTick() {
	// Always on: single-threaded WASM GC starves under sustained audio + param load (gc=0 for minutes) and the
	// heap grew to the 2 GB ceiling. Wall-clock paced because the frame rate drops to ~7 fps under that load.
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
	// Always on (not the debug gate) so perf output always carries GC; non-STW and throttled.
	if g != nil && g.frame%15 == 0 {
		g.gcMetrics.sample()
	}
	// One ReadMemStats every 1800 frames.
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

// Includes analyzer bridge stats so a per-frame analyzer leak surfaces here. Silent on the first tick: there is
// no window to compare yet.
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

// Lets a WASM session inspect the seconds before an OOM.
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
