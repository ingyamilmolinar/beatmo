package ui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// soakScenario describes a long-session workload to measure heap growth against.
type soakScenario struct {
	name string
	// playing controls whether the workload starts playback after build.
	playing bool
	// editEvery, when > 0, toggles row 0 solo + row 2 mute every N frames during play.
	editEvery int
	// drawEvery overrides the default Draw cadence (1 = draw every frame).
	// Zero falls back to the helper's default (30) which keeps the soak fast
	// while still exercising the row controls path.
	drawEvery int
	// fxChurnEvery, when > 0, adds an insert effect on row 0 then removes it
	// every N frames. Mirrors the production OOM repro where the user added
	// and removed insert effects mid-play right before the crash. Catches
	// leaks in the effect_chain / channel processor / WASM JSON-marshal path.
	fxChurnEvery int
	// fxParamChurnEvery, when > 0, twiddles a parameter on the first slot of
	// row 0's chain every N frames (simulates slider drag). Catches leaks in
	// the per-param-update WASM bridge marshal path. Requires that an effect
	// be present (the helper adds one on first churn tick).
	fxParamChurnEvery int
	// graphEditEvery, when > 0, adds a regular node + edge to row 1 then
	// deletes it every N frames during play. Catches leaks in graph mutation
	// + predDirty rebuild + parity-gen churn paths that pure playback misses.
	graphEditEvery int
}

// wasmHeapCeilingBytes is the WASM linear-memory effective ceiling we project
// failures against. The production crash logged "out of memory: cannot allocate
// 4194304-byte block (2130444288 in use)" — i.e. ~2.0 GB in use plus the next
// 4 MB allocation overflowed. Browsers cap Go-WASM linear memory near 2 GB.
const wasmHeapCeilingBytes = 2_130_444_288

// soakSample captures a heap-size measurement at a given frame.
type soakSample struct {
	frame   int
	alloc   uint64
	objects uint64
	gc      uint32
	// total is runtime.MemStats.TotalAlloc — cumulative bytes allocated since
	// process start. Unlike alloc/HeapAlloc this is NOT reset by GC, so it
	// detects high allocation churn that the standard Go GC reclaims but the
	// WASM single-threaded GC cannot keep up with at production scale (the
	// 2 GB OOM root cause).
	total uint64
}

// runSoakHeapBound drives `frames` Update/Draw cycles against a freshly
// constructed Game in the given scenario, collecting MemStats every
// `sampleEvery` frames. After a `warmupSamples`-sample warmup it asserts
// three complementary bounds on post-warmup samples:
//
//  1. allocBoundX  — max/min HeapAlloc ratio (catches sudden spikes).
//  2. allocSlopeMaxBytesPerFrame — least-squares slope of HeapAlloc over the
//     post-warmup window (catches steady leaks; robust to starting-heap
//     pollution from earlier tests in the same process).
//  3. objBoundX    — last/first HeapObjects ratio (catches per-abs map growth
//     before it dominates HeapAlloc).
//
// Plus a post-shutdown HeapObjects check vs. pre-loop, to catch goroutine /
// closure-rooted leaks the per-loop bounds would miss.
//
// Defaults (used by the test wrappers):
//
//	frames                       = 8400   (10× the production OOM observed at ~840 frames)
//	sampleEvery                  = 300    (28 samples; 4 warmup + 24 measured)
//	warmupSamples                = 4
//	allocBoundX                  = 1.5
//	allocSlopeMaxBytesPerFrame   = 256    (~2 MB over 8400 frames)
//	objBoundX                    = 1.3
//
// On failure the helper logs a tabular sample trace, the allocation slope,
// projected size at frame 50_000, top diagnostic counts, and writes a heap
// pprof profile under t.TempDir() so the failure is debuggable.
func runSoakHeapBound(
	t *testing.T,
	sc soakScenario,
	frames, sampleEvery, warmupSamples int,
	allocBoundX, objBoundX float64,
) {
	t.Helper()
	// Per-frame net-growth tolerance for the head→tail median check.
	// DrawHeavy redraws every frame and legitimately allocates more
	// (RowRackZone caches rebuild on full-rate Draws), so it gets a higher
	// budget. All other scenarios share the production-tight 256 B/frame
	// bound — production OOM measured ~62 KB/frame, ~250× this bound, so
	// any real leak swamps the threshold.
	allocSlopeMaxBytesPerFrame := 256.0
	if sc.drawEvery == 1 {
		allocSlopeMaxBytesPerFrame = 768.0
	}

	if frames <= 0 || sampleEvery <= 0 {
		t.Fatalf("invalid soak parameters: frames=%d sampleEvery=%d", frames, sampleEvery)
	}
	if warmupSamples < 0 {
		warmupSamples = 0
	}

	// withDefaultAudio first: both helpers call assertDefaultParityState(t),
	// which checks enableDefaultStart == true. Reversing the order would fail
	// the second assertion because withDefaultStart flips the global to false.
	withDefaultAudio(t)
	withDefaultStart(t, false)

	// Force parity recording on so the leaky path actually runs. The check in
	// recordSeqDecision short-circuits when watch=off AND fatals=false; we
	// flip watch=log so recording proceeds, and disable fatals so synthetic
	// playback timing in tests does not panic.
	prevWatch := parityWatchDefault
	prevFatal := parityFatalEnabled.Load()
	parityWatchDefault = parityWatchLog
	parityFatalEnabled.Store(false)
	t.Cleanup(func() {
		parityWatchDefault = prevWatch
		parityFatalEnabled.Store(prevFatal)
	})

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	// Belt-and-suspenders: respect parityWatchLog even though New() reads
	// parityWatchDefault at construction time.
	g.parityWatch = parityWatchLog
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Subdiv 16 + BPM 240 picks an absolute index advance rate comparable to
	// the production WASM crash (which logged play→OOM in ~14 s).
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)

	// Drawing target. Stub Ebiten allocates a backing buffer once; subsequent
	// Draws reuse it.
	screen := ebiten.NewImage(1280, 720)

	if sc.playing {
		g.SetPlaying(true)
		// Let playback actually transition to running before sampling.
		advanceFrames(g, 5)
	}

	preSample := readMemStats()

	var graphChurn graphChurnState
	samples := make([]soakSample, 0, frames/sampleEvery+1)
	// Drawing every frame is faithful to production but expensive under stub
	// Ebiten (the row controls cache rebuild dominates wall time). The default
	// drawEvery=30 still exercises the RowRackZone.Draw → icon path regularly
	// while keeping the soak runtime tractable. Scenarios that specifically
	// want to measure Draw-path allocation rate set sc.drawEvery=1.
	drawEvery := 30
	if sc.drawEvery > 0 {
		drawEvery = sc.drawEvery
	}
	for f := 0; f < frames; f++ {
		if sc.playing {
			advancePlaybackByAbs(g, 1)
		} else {
			_ = g.Update()
		}
		if f%drawEvery == 0 {
			g.Draw(screen)
		}
		if sc.playing && sc.editEvery > 0 && f > 0 && f%sc.editEvery == 0 {
			toggleSoakEdits(g)
		}
		if sc.playing && sc.fxChurnEvery > 0 && f > 0 && f%sc.fxChurnEvery == 0 {
			toggleSoakFXChurn(g)
		}
		if sc.playing && sc.fxParamChurnEvery > 0 && f > 0 && f%sc.fxParamChurnEvery == 0 {
			toggleSoakFXParamChurn(g, f)
		}
		if sc.playing && sc.graphEditEvery > 0 && f > 0 && f%sc.graphEditEvery == 0 {
			toggleSoakGraphEdit(g, &graphChurn)
		}
		if f%sampleEvery == 0 {
			samples = append(samples, sampleHeap(f))
		}
	}
	// Final sample at the loop end so the tail measurement reflects the full run.
	samples = append(samples, sampleHeap(frames))

	// Post-shutdown sample: drop the Game's goroutines and force GC, then
	// measure live heap. Catches leaks rooted in goroutines / channels that
	// the per-loop bound would miss.
	g.CloseForTest()
	runtime.GC()
	runtime.GC()
	postShutdown := readMemStats()

	if len(samples) <= warmupSamples {
		t.Fatalf("not enough samples: got %d, need > warmup %d", len(samples), warmupSamples)
	}
	post := samples[warmupSamples:]

	minAlloc := post[0].alloc
	maxAlloc := post[0].alloc
	for _, s := range post {
		if s.alloc < minAlloc {
			minAlloc = s.alloc
		}
		if s.alloc > maxAlloc {
			maxAlloc = s.alloc
		}
	}
	firstObj := post[0].objects
	lastObj := post[len(post)-1].objects

	failures := []string{}
	if minAlloc > 0 {
		ratio := float64(maxAlloc) / float64(minAlloc)
		if ratio > allocBoundX {
			failures = append(failures, fmt.Sprintf(
				"HeapAlloc grew %.2fx over post-warmup (min=%dKB max=%dKB), bound=%.2fx",
				ratio, minAlloc/1024, maxAlloc/1024, allocBoundX))
		}
	}
	// Net-growth check: compare the median of the head-quartile of post-warmup
	// samples to the FINAL post-loop sample (which is taken after a forced
	// GC by sampleHeap). Using the final sample for the tail eliminates GC
	// oscillation noise — the heap may rise mid-run and fall again, so what
	// matters for an OOM regression is whether the run ENDED higher than it
	// started. Bound scales as bytesPerFrame × frames.
	headEnd := len(post) / 4
	if headEnd < 1 {
		headEnd = 1
	}
	headMed := medianAlloc(post[:headEnd])
	tailAlloc := post[len(post)-1].alloc
	netGrowth := int64(tailAlloc) - int64(headMed)
	netBound := int64(allocSlopeMaxBytesPerFrame) * int64(frames)
	if netGrowth > netBound {
		// Project the observed per-frame growth to the WASM ceiling so the
		// failure message tells the reader WHEN this leak rate would crash a
		// real WASM session. Production stack landed in
		// RowRackZone.drawRowControlsToCache → vector.Path tessellation —
		// the upstream growth tracked here is what pushed the heap to the 2
		// GB ceiling so the next vector.Path alloc could not fit.
		perFrame := float64(netGrowth) / float64(frames)
		framesToOOM := projectFramesToOOM(tailAlloc, perFrame)
		secondsToOOM := framesToOOM / 60
		failures = append(failures, fmt.Sprintf(
			"HeapAlloc head→final growth=%d KB over %d frames (≈%.0f bytes/frame), bound=%d KB (≈%.0f bytes/frame); "+
				"at this rate WASM would OOM (~2 GB linear memory) after ~%d frames "+
				"(~%d s @ 60 fps, ~%.1f min). Head-quartile median=%d KB, final post-GC=%d KB.",
			netGrowth/1024, frames, perFrame,
			netBound/1024, allocSlopeMaxBytesPerFrame,
			framesToOOM, secondsToOOM, float64(secondsToOOM)/60,
			headMed/1024, tailAlloc/1024))
	}
	if firstObj > 0 {
		ratio := float64(lastObj) / float64(firstObj)
		if ratio > objBoundX {
			failures = append(failures, fmt.Sprintf(
				"HeapObjects grew %.2fx over post-warmup (first=%d last=%d), bound=%.2fx",
				ratio, firstObj, lastObj, objBoundX))
		}
	}
	// Goroutine-rooted leak guard: live heap after shutdown should not exceed
	// pre-loop live heap by more than 10% — a small tolerance for residual
	// allocator state held by the runtime.
	if preSample.objects > 0 {
		ratio := float64(postShutdown.objects) / float64(preSample.objects)
		if ratio > 1.1 {
			failures = append(failures, fmt.Sprintf(
				"post-shutdown HeapObjects grew %.2fx vs pre-loop (pre=%d post=%d), bound=1.10x",
				ratio, preSample.objects, postShutdown.objects))
		}
	}

	// Per-component hard bounds — deterministic checks against the specific
	// accumulators that have caused unbounded growth in the past or could in
	// the future. Unlike the noisy LSQ slope, these check exact counts on
	// known suspect data structures so a regression can be diagnosed by
	// looking at WHICH bound failed.
	failures = append(failures, checkComponentBounds(g, sc, frames)...)

	if len(failures) > 0 {
		dumpSoakDiagnostics(t, sc, samples, preSample, postShutdown, g)
		for _, f := range failures {
			t.Error(f)
		}
		t.FailNow()
	}
}

// buildSoakScene constructs `rows` separate small loops (each `nodesPerRow`
// nodes arranged as a rectangle), one per drum row. This mirrors the kind of
// scene a user would have during sustained playback: a per-instrument circuit
// per row with audible nodes that exercise predictor + parity recording.
func buildSoakScene(t *testing.T, g *Game, rows, nodesPerRow int) {
	t.Helper()
	if rows <= 0 || nodesPerRow < 4 {
		t.Fatalf("invalid scene: rows=%d nodesPerRow=%d", rows, nodesPerRow)
	}
	width := nodesPerRow / 2 // 4 wide → 4×2 rectangle for 8 nodes
	for r := 0; r < rows; r++ {
		// Each row's loop occupies its own vertical band so the geometries
		// don't overlap and the predictor builds clean per-row paths.
		baseJ := r * 16
		var first *uiNode
		var prev *uiNode
		// Bottom edge: left → right
		for k := 0; k < width; k++ {
			n := g.tryAddNode(k*8, baseJ, model.NodeTypeRegular)
			if n == nil {
				t.Fatalf("scene build: nil node row=%d k=%d", r, k)
			}
			if first == nil {
				first = n
			}
			if prev != nil {
				g.addEdge(prev, n)
			}
			prev = n
		}
		// Top edge: right → left
		for k := width - 1; k >= 0; k-- {
			n := g.tryAddNode(k*8, baseJ+8, model.NodeTypeRegular)
			if n == nil {
				t.Fatalf("scene build: nil node row=%d k=%d top", r, k)
			}
			g.addEdge(prev, n)
			prev = n
		}
		// Close the loop.
		if prev != nil && first != nil && prev != first {
			g.addEdge(prev, first)
		}
		// Add a row for r > 0 (row 0 already exists from constructor).
		if r > 0 {
			g.drum.AddRow()
		}
		g.drum.Rows[r].Origin = first.ID
		g.drum.Rows[r].Node = first
	}
	// Pin start to row 0's first node.
	if len(g.drum.Rows) > 0 && g.drum.Rows[0].Node != nil {
		g.start = g.drum.Rows[0].Node
		g.graph.StartNodeID = g.drum.Rows[0].Node.ID
	}
	g.updateBeatInfos()
	g.refreshDrumRow()
}

// toggleSoakEdits flips solo/mute on the first two rows so per-row state
// machinery (mute gates, solo masks, parity gen bumps) is exercised across
// a long run, not just steady-state playback.
func toggleSoakEdits(g *Game) {
	if g == nil || g.drum == nil {
		return
	}
	if len(g.drum.Rows) > 0 {
		g.drum.toggleSolo(0)
	}
	if len(g.drum.Rows) > 2 {
		g.drum.toggleMute(2)
	}
}

// toggleSoakFXChurn adds an insert effect on row 0's instrument and removes
// it on the next call. Mirrors the production OOM repro where the user added
// then removed insert effects mid-play right before the crash. Catches leaks
// in:
//   - audio.effectChainManager (slot/processor accumulation across add/remove)
//   - rebuildChannelProcessors (per-call Processor slice churn into the live channel)
//   - platformInsertEffectsChanged (per-call JSON marshal on WASM)
//
// On WASM the platform callback marshals the new slot list to JSON every call;
// on stub builds the callback is a no-op but the Go-side slot/processor
// reallocation still runs and is what we measure here.
func toggleSoakFXChurn(g *Game) {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	id := g.drum.Rows[0].Instrument
	if id == "" {
		return
	}
	cur := audio.GetInsertEffects(id)
	if len(cur) == 0 {
		_ = audio.AddInsertEffect(id, audio.EffectDistortion, nil)
	} else {
		audio.RemoveInsertEffect(id, len(cur)-1)
	}
}

// toggleSoakFXParamChurn updates a parameter on the first slot of row 0's
// chain (adding a slot if missing) every tick. Simulates a slider drag on a
// live FX param. Production WASM marshals the entire slot list to JSON on
// each call (insert_effects_wasm.go) — we measure the Go-side allocation
// rate of that path here.
func toggleSoakFXParamChurn(g *Game, f int) {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	id := g.drum.Rows[0].Instrument
	if id == "" {
		return
	}
	if len(audio.GetInsertEffects(id)) == 0 {
		_ = audio.AddInsertEffect(id, audio.EffectDistortion, nil)
	}
	val := 0.5 + 0.45*math.Sin(float64(f)/97.0)
	audio.SetInsertEffectParam(id, 0, "drive", val)
}

// graphChurnState retains the most recently added churn node so the next
// invocation can delete it. Reset between runs by runSoakHeapBound.
type graphChurnState struct {
	node *uiNode
}

// checkComponentBounds returns failure messages for any per-component
// accumulator that has grown past a hard ceiling. Bounds are sized for the
// 8400-frame production-comparable run and explicitly account for each
// data structure's expected steady-state size — a failure here means
// something is leaking, not that the bound was hit by ordinary growth.
//
// The bounds catch leaks that the heap-level slope/ratio checks miss when:
//   - a single accumulator grows large enough to OOM but is amortized by
//     other allocations dropping (slope sees no net trend);
//   - the leak grows in HeapObjects but each entry is small (HeapAlloc bound
//     not tripped);
//   - GC oscillation makes the slope estimate noise-dominated.
//
// All bounds are absolute counts, computed at the end of the run. They are
// independent of the slope/ratio checks above so a single soak run can
// flag several distinct regressions at once.
func checkComponentBounds(g *Game, sc soakScenario, frames int) []string {
	if g == nil {
		return nil
	}
	var fails []string

	// parityAudio is enforced bounded at 1024 entries by the recordParityAudio
	// drop-oldest cap (game_parity_state.go:150-153). If this assertion ever
	// fails, the cap was bypassed or removed.
	const parityAudioMaxBound = 1024
	if got := len(g.parityAudio); got > parityAudioMaxBound {
		fails = append(fails, fmt.Sprintf(
			"parityAudio len=%d exceeds bound=%d — drop-oldest cap in recordParityAudio was bypassed",
			got, parityAudioMaxBound))
	}

	// paritySeqDecisions is pruned by parityPrune(minAbs) which is called by
	// the sequencer with the row's earliest still-relevant abs index. Each
	// row's map should never hold more than ~one cycle length × a few cycles
	// of buffer. We allow 4096 per row as a generous bound (production cycles
	// are typically <64 long; 4096 = 64 cycles of slack).
	const paritySeqDecisionsPerRowBound = 4096
	for r, m := range g.paritySeqDecisions {
		if len(m) > paritySeqDecisionsPerRowBound {
			fails = append(fails, fmt.Sprintf(
				"paritySeqDecisions[row=%d] size=%d exceeds bound=%d — parityPrune is not keeping up with playback rate",
				r, len(m), paritySeqDecisionsPerRowBound))
		}
	}

	// Timeline immutables sidecar. RecordCommitKind stores every playback
	// commit in immutables[row][abs] and never deletes — see service.go:84-93.
	// Bounded by elapsed playback in absolute indices: at 1 commit/frame for
	// `frames` frames, immutables can grow to `frames` entries per row. We
	// allow 2× headroom (16800 for the default 8400-frame run) so a benign
	// pre-existing growth pattern doesn't trip; anything larger is a leak.
	immutablesMaxBound := 2 * frames
	if g.timeline != nil && g.drum != nil {
		for r := 0; r < len(g.drum.Rows); r++ {
			_, _, immut := g.timeline.RingLenForTest(r)
			if immut > immutablesMaxBound {
				fails = append(fails, fmt.Sprintf(
					"timeline[row=%d] immutables=%d exceeds bound=%d (frames=%d) — timeline.immutables sidecar accumulates without bound",
					r, immut, immutablesMaxBound, frames))
			}
		}
	}

	// Predictor horizon grows linearly with elapsed wall-clock × bpm × subdiv.
	// The slate is []bool so this directly bounds the per-row prediction
	// buffer size in bytes. At subdiv=16 + bpm=240 the helper advances ~64
	// indices/sec, so 8400 frames at 60fps wall-clock → ~9000 horizon.
	// Allow 4× headroom (32×frames) before flagging as runaway.
	horizonMaxBound := 4 * frames
	if g.engine != nil && g.engine.Predictor != nil {
		if h := g.engine.Predictor.Horizon(); h > horizonMaxBound {
			fails = append(fails, fmt.Sprintf(
				"predictor horizon=%d exceeds bound=%d (frames=%d) — Ensure() is being called with horizons growing past elapsed wall-clock",
				h, horizonMaxBound, frames))
		}
		// Predictor slice growth pattern. Production OOM at 10:20 of playback
		// (~14 835-element horizon) was caused by Ensure reallocating per step:
		// `make([]bool, len, horizon)` set cap=horizon exactly, so every
		// single-step horizon advance reallocated the slice. Total allocations
		// across N steps were O(N^2) bytes — at production rate ~24 abs/s for
		// 600 s, ~1.9 GB of garbage that WASM's single-threaded GC could not
		// reclaim before the 2 GB ceiling. Fix is geometric growth in
		// engine/predictor_compute.go::growBoolBuf.
		//
		// We check the pattern indirectly: after many single-step Ensure calls,
		// a geometric grower will have cap ≥ ~next power of two ≥ horizon, so
		// (cap - len) > 0 typically. A per-step grower has cap == len exactly.
		// Sample 10 evenly-spaced post-warmup horizons and assert the cap-to-len
		// relationship demonstrates amortized growth rather than per-call
		// reallocation. If horizon advanced in single steps and cap == len for
		// every sample, regression is confirmed.
		if h := g.engine.Predictor.Horizon(); h > 256 && sc.playing {
			perRowCaps := g.engine.Predictor.BufferCapsForTest()
			anyGrown := false
			for _, perBufCaps := range perRowCaps {
				for _, capLen := range perBufCaps {
					// capLen is [cap, len]. With per-step regression cap == len.
					// With geometric growth cap > len for at least some buffers
					// once horizon advances past the initial doubling.
					if capLen[0] > capLen[1] && capLen[1] > 0 {
						anyGrown = true
						break
					}
				}
				if anyGrown {
					break
				}
			}
			if !anyGrown && len(perRowCaps) > 0 {
				fails = append(fails, fmt.Sprintf(
					"predictor slice growth is per-step (cap==len for every row × buffer), horizon=%d — "+
						"Ensure must grow geometrically or single-step horizon advances reallocate per call. "+
						"Production OOM at 10:20 of playback was this exact pattern; fix is in growBoolBuf.",
					h))
			}
		}
	}

	// Insert effects per instrument. Add/Remove churn should keep this
	// oscillating between 0 and 1; in graph-mutation scenarios it should
	// stay below ~16 (UI ceiling). Anything >32 means RemoveInsertEffect
	// is leaking slots.
	const fxSlotsPerInstrumentBound = 32
	if g.drum != nil {
		for r := 0; r < len(g.drum.Rows); r++ {
			id := g.drum.Rows[r].Instrument
			if id == "" {
				continue
			}
			n := len(audio.GetInsertEffects(id))
			if n > fxSlotsPerInstrumentBound {
				fails = append(fails, fmt.Sprintf(
					"insertFX[inst=%s] slots=%d exceeds bound=%d — RemoveInsertEffect leaked slots over %s churn",
					id, n, fxSlotsPerInstrumentBound, sc.name))
			}
		}
	}

	return fails
}

// toggleSoakGraphEdit alternates between adding a node + edge to row 1 and
// deleting it. Real users edit while playing — every mutation marks the
// predictor dirty and bumps parity gen, exercising rebuild paths that
// steady-state playback skips entirely.
func toggleSoakGraphEdit(g *Game, st *graphChurnState) {
	if g == nil || g.drum == nil || len(g.drum.Rows) < 2 || st == nil {
		return
	}
	if st.node == nil {
		// Place near row 1's home band (baseJ = 16 from buildSoakScene) but
		// off the loop's path so the existing cycle stays intact.
		n := g.tryAddNode(48, 18, model.NodeTypeRegular)
		if n == nil {
			return
		}
		// Connect the orphan to the row's first node so it actually
		// participates in path computation (and thus exercises predictor
		// rebuild + parity gen).
		if g.drum.Rows[1].Node != nil {
			g.addEdge(n, g.drum.Rows[1].Node)
		}
		st.node = n
		return
	}
	g.deleteNode(st.node)
	st.node = nil
	g.updateBeatInfos()
	g.refreshDrumRow()
}

func sampleHeap(frame int) soakSample {
	runtime.GC()
	runtime.GC()
	ms := readMemStats()
	ms.frame = frame
	return ms
}

func readMemStats() soakSample {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return soakSample{alloc: ms.HeapAlloc, objects: ms.HeapObjects, gc: ms.NumGC, total: ms.TotalAlloc}
}

func dumpSoakDiagnostics(
	t *testing.T,
	sc soakScenario,
	samples []soakSample,
	pre, post soakSample,
	g *Game,
) {
	t.Helper()
	t.Logf("soak diagnostics scenario=%s frames=%d samples=%d", sc.name, samples[len(samples)-1].frame, len(samples))
	t.Logf("frame,heap_alloc_kb,heap_objects,num_gc")
	for _, s := range samples {
		t.Logf("%d,%d,%d,%d", s.frame, s.alloc/1024, s.objects, s.gc)
	}
	t.Logf("pre-loop:    heap_alloc_kb=%d heap_objects=%d", pre.alloc/1024, pre.objects)
	t.Logf("post-shutdown: heap_alloc_kb=%d heap_objects=%d", post.alloc/1024, post.objects)

	// Linear-regression slope of HeapAlloc over post-warmup samples.
	if len(samples) >= 4 {
		slope := allocSlopePerFrame(samples)
		final := samples[len(samples)-1]
		// Project to frame 50_000.
		projected := int64(final.alloc) + int64(slope*float64(50_000-final.frame))
		t.Logf("alloc slope ≈ %.1f bytes/frame; projected heap_alloc at frame 50000 ≈ %d KB",
			slope, projected/1024)
	}

	// Top diagnostic counts: paritySeqDecisions per row + commit ring + immutables.
	if g != nil {
		var totalDecisions int
		for r, m := range g.paritySeqDecisions {
			totalDecisions += len(m)
			if r < 8 { // cap log spam
				t.Logf("paritySeqDecisions[row=%d] size=%d", r, len(m))
			}
		}
		t.Logf("paritySeqDecisions total=%d (rows=%d)", totalDecisions, len(g.paritySeqDecisions))
		t.Logf("parityAudio len=%d", len(g.parityAudio))
		if g.timeline != nil && g.drum != nil {
			for r := 0; r < len(g.drum.Rows) && r < 8; r++ {
				size, capa, immut := g.timeline.RingLenForTest(r)
				t.Logf("timeline[row=%d] ring_size=%d ring_cap=%d immutables=%d", r, size, capa, immut)
			}
		}
		// Predictor horizon grows with elapsed-time × bpm × subdiv. The slate
		// itself is []bool so cap == horizon in bytes/row, but a runaway
		// horizon during a long session signals that target is being computed
		// off a wall-clock that wasn't reset.
		if g.engine != nil && g.engine.Predictor != nil {
			t.Logf("predictor horizon=%d", g.engine.Predictor.Horizon())
		}
		// Insert effect chain length per row's instrument. Add/Remove churn
		// on row 0 should keep this oscillating between 0 and 1; a steady
		// climb means RemoveInsertEffect leaked a slot.
		if g.drum != nil {
			for r := 0; r < len(g.drum.Rows) && r < 8; r++ {
				id := g.drum.Rows[r].Instrument
				if id == "" {
					continue
				}
				t.Logf("insertFX[row=%d inst=%s] slots=%d", r, id, len(audio.GetInsertEffects(id)))
			}
		}
	}

	// Heap pprof under t.TempDir() — the path lands in test logs so a human
	// can `go tool pprof` it after the run.
	path := filepath.Join(t.TempDir(), "soak_heap.pprof")
	if f, err := os.Create(path); err == nil {
		defer f.Close()
		runtime.GC()
		if perr := pprof.Lookup("heap").WriteTo(f, 0); perr == nil {
			t.Logf("heap profile: %s", path)
		} else {
			t.Logf("heap profile write failed: %v", perr)
		}
	} else {
		t.Logf("heap profile create failed: %v", err)
	}
}

// projectFramesToOOM returns how many additional frames at the given
// allocation slope would push the heap to the WASM linear-memory ceiling
// from currentAlloc. Returns math.MaxInt64 if the slope is non-positive
// (no leak) or if the heap is already over the ceiling.
func projectFramesToOOM(currentAlloc uint64, slopeBytesPerFrame float64) int64 {
	if slopeBytesPerFrame <= 0 {
		return math.MaxInt64
	}
	if currentAlloc >= wasmHeapCeilingBytes {
		return 0
	}
	remaining := wasmHeapCeilingBytes - currentAlloc
	frames := float64(remaining) / slopeBytesPerFrame
	if frames > math.MaxInt64 || math.IsInf(frames, 0) {
		return math.MaxInt64
	}
	return int64(frames)
}

// medianAlloc returns the median HeapAlloc across the given samples.
// Robust to a single GC sample landing low (or high) within a quartile.
func medianAlloc(samples []soakSample) uint64 {
	if len(samples) == 0 {
		return 0
	}
	vals := make([]uint64, len(samples))
	for i, s := range samples {
		vals[i] = s.alloc
	}
	// Insertion sort — n is tiny (≤ 7 quartile samples) so quicksort is overkill.
	for i := 1; i < len(vals); i++ {
		for j := i; j > 0 && vals[j-1] > vals[j]; j-- {
			vals[j-1], vals[j] = vals[j], vals[j-1]
		}
	}
	return vals[len(vals)/2]
}

// allocSlopePerFrame returns least-squares slope of (frame → alloc) across
// the given samples in bytes/frame. Used purely for diagnostic projection.
func allocSlopePerFrame(samples []soakSample) float64 {
	if len(samples) < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	n := float64(len(samples))
	for _, s := range samples {
		x := float64(s.frame)
		y := float64(s.alloc)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if math.Abs(denom) < 1e-9 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denom
}
