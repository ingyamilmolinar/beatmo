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
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// soakScenario describes a long-session workload to measure heap growth against.
type soakScenario struct {
	name string
	// playing controls whether the workload starts playback after build.
	playing bool
	// editEvery, when > 0, toggles row 0 solo + row 2 mute every N frames during play.
	editEvery int
	// 0 falls back to 30, which keeps the soak fast while still drawing the row controls.
	drawEvery int
	// Adds then removes an insert effect on row 0 every N frames: effect-chain, channel-processor and WASM marshal
	// leaks.
	fxChurnEvery int
	// A slider drag on row 0's first slot every N frames; the helper adds a slot on the first tick.
	fxParamChurnEvery int
	// Graph mutation, predictor rebuild and parity-gen churn that pure playback misses.
	graphEditEvery int
	// Draws the Synth tab every frame (EQPanelZone.Draw → drawSynthTab → Knob.Draw); otherwise only the row rack is
	// drawn.
	synthTabOpen bool
	// Rotates row 0's voice cache key every N frames through SetInstrumentParam and the bridge marshal; needs a
	// recipe-bound channel.
	paramChurnEvery int
}

// Browsers cap Go-WASM linear memory near 2 GB.
const wasmHeapCeilingBytes = 2_130_444_288

// soakSample captures a heap-size measurement at a given frame.
type soakSample struct {
	frame   int
	alloc   uint64
	objects uint64
	gc      uint32
	// TotalAlloc is never reset by GC, so it sees churn native GC reclaims but WASM's single-threaded GC cannot keep up
	// with.
	total uint64
}

// Three bounds on post-warm-up samples: the max/min HeapAlloc ratio (spikes), the least-squares HeapAlloc slope
// (steady leaks, robust to earlier tests' residue) and the HeapObjects ratio (per-abs map growth), plus live heap
// after shutdown (goroutine-rooted leaks). A failure writes a heap profile under t.TempDir().
func runSoakHeapBound(
	t *testing.T,
	sc soakScenario,
	frames, sampleEvery, warmupSamples int,
	allocBoundX, objBoundX float64,
) {
	t.Helper()
	// DrawHeavy redraws every frame and legitimately rebuilds caches, so it gets a higher budget.
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

	// withDefaultAudio first: withDefaultStart flips the global the second assertDefaultParityState checks.
	withDefaultAudio(t)
	withDefaultStart(t, false)

	// recordSeqDecision short-circuits with watch off and fatals off; watch=log records, fatals off keep synthetic
	// timing from panicking.
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
	// New() reads parityWatchDefault at construction; set it on the game too.
	g.parityWatch = parityWatchLog
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	// An abs advance rate comparable to the production crash.
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	g.drum.SetBPM(240)

	buildSoakScene(t, g, 6 /* rows */, 8 /* nodes per row */)

	screen := ebiten.NewImage(1280, 720)

	// Before play, so Draw goes through the synth panel the crash landed in rather than the row rack.
	if sc.synthTabOpen {
		// A recipe binding so the Synth tab has params to render; ResetInstrumentParams drops them at teardown.
		if len(g.drum.Rows) > 0 && g.drum.Rows[0].Instrument != "" {
			_ = audio.InstallInstrumentDocForTest(g.drum.Rows[0].Instrument, roster.Snare)
			t.Cleanup(func() { audio.ResetInstrumentParams(g.drum.Rows[0].Instrument) })
		}
		if len(g.drum.Rows) > 1 && g.drum.Rows[1].Instrument != "" {
			_ = audio.InstallInstrumentDocForTest(g.drum.Rows[1].Instrument, roster.Kick)
			t.Cleanup(func() { audio.ResetInstrumentParams(g.drum.Rows[1].Instrument) })
		}
		g.drum.eqPanelZone.SetActiveTab(TabSynth)
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
		// One Update lets the tree pick up the hit areas and build the knobs, or the first Draws see none.
		_ = g.Update()
	}

	if sc.playing {
		g.SetPlaying(true)
		// Let playback actually transition to running before sampling.
		advanceFrames(g, 5)
	}

	preSample := readMemStats()

	var graphChurn graphChurnState
	samples := make([]soakSample, 0, frames/sampleEvery+1)
	// Drawing every frame is faithful but slow under the stub; scenarios measuring the Draw path set drawEvery=1.
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
		if sc.synthTabOpen && sc.paramChurnEvery > 0 && f > 0 && f%sc.paramChurnEvery == 0 {
			toggleSoakSynthParamChurn(g, f)
		}
		if f%sampleEvery == 0 {
			samples = append(samples, sampleHeap(f))
		}
	}
	// Final sample at the loop end so the tail measurement reflects the full run.
	samples = append(samples, sampleHeap(frames))

	// Dropping the Game's goroutines then forcing GC exposes leaks rooted in goroutines and channels.
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
	// The head-quartile median against the FINAL post-GC sample: the heap may rise and fall mid-run, and what matters
	// is whether the run ENDED higher than it started.
	headEnd := len(post) / 4
	if headEnd < 1 {
		headEnd = 1
	}
	headMed := medianAlloc(post[:headEnd])
	tailAlloc := post[len(post)-1].alloc
	netGrowth := int64(tailAlloc) - int64(headMed)
	netBound := int64(allocSlopeMaxBytesPerFrame) * int64(frames)
	if netGrowth > netBound {
		// Projected to the WASM ceiling so the message says WHEN this rate would crash a real session.
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
	// The tolerance covers residual allocator state.
	if preSample.objects > 0 {
		ratio := float64(postShutdown.objects) / float64(preSample.objects)
		if ratio > 1.1 {
			failures = append(failures, fmt.Sprintf(
				"post-shutdown HeapObjects grew %.2fx vs pre-loop (pre=%d post=%d), bound=1.10x",
				ratio, preSample.objects, postShutdown.objects))
		}
	}

	// Exact counts on known suspect structures, so a failure names WHICH one grew.
	failures = append(failures, checkComponentBounds(g, sc, frames)...)

	if len(failures) > 0 {
		dumpSoakDiagnostics(t, sc, samples, preSample, postShutdown, g)
		for _, f := range failures {
			t.Error(f)
		}
		t.FailNow()
	}
}

// One small rectangular loop per row with audible nodes, as during sustained playback.
func buildSoakScene(t *testing.T, g *Game, rows, nodesPerRow int) {
	t.Helper()
	if rows <= 0 || nodesPerRow < 4 {
		t.Fatalf("invalid scene: rows=%d nodesPerRow=%d", rows, nodesPerRow)
	}
	width := nodesPerRow / 2 // 4 wide → 4×2 rectangle for 8 nodes
	for r := 0; r < rows; r++ {
		// Separate bands so the geometries don't overlap and each row gets a clean path.
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

// Mute gates, solo masks and parity-gen bumps across a long run, not only steady playback.
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

// Exercises slot/processor accumulation across add/remove; on stub builds the platform callback is a no-op but
// the Go-side reallocation still runs.
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

// A slider drag on a live FX param; adds a slot if missing.
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

// A synth-knob drag while playing: each call invalidates the voice cache, publishes a hook and notifies the
// platform. Two params, so each instrument's map holds more than one key.
func toggleSoakSynthParamChurn(g *Game, f int) {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	pitch := -12 + 24*math.Sin(float64(f)/73.0)
	decay := 0.25 + 0.75*math.Sin(float64(f)/41.0)
	if id := g.drum.Rows[0].Instrument; id != "" {
		audio.SetInstrumentParam(id, "pitch", pitch)
		audio.SetInstrumentParam(id, "amp_decay", decay)
	}
	if len(g.drum.Rows) > 1 {
		if id := g.drum.Rows[1].Instrument; id != "" {
			audio.SetInstrumentParam(id, "pitch", pitch*0.5)
		}
	}
}

// Reset between runs.
type graphChurnState struct {
	node *uiNode
}

// Absolute counts sized to each structure's expected steady state: they catch leaks the heap checks miss (one
// accumulator amortised by others, many tiny objects, GC noise), and several can fail in one run.
func checkComponentBounds(g *Game, sc soakScenario, frames int) []string {
	if g == nil {
		return nil
	}
	var fails []string

	// recordParityAudio's drop-oldest cap; a failure means it was bypassed.
	const parityAudioMaxBound = 1024
	if got := len(g.parityAudio); got > parityAudioMaxBound {
		fails = append(fails, fmt.Sprintf(
			"parityAudio len=%d exceeds bound=%d — drop-oldest cap in recordParityAudio was bypassed",
			got, parityAudioMaxBound))
	}

	// Pruned to the row's earliest relevant abs; generous slack over real cycle lengths.
	const paritySeqDecisionsPerRowBound = 4096
	for r, m := range g.paritySeqDecisions {
		if len(m) > paritySeqDecisionsPerRowBound {
			fails = append(fails, fmt.Sprintf(
				"paritySeqDecisions[row=%d] size=%d exceeds bound=%d — parityPrune is not keeping up with playback rate",
				r, len(m), paritySeqDecisionsPerRowBound))
		}
	}

	// Every playback commit is retained, so it grows with elapsed abs; 2× headroom over one per frame.
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

	// Grows with elapsed time × BPM × subdiv; 4× headroom before it counts as runaway.
	horizonMaxBound := 4 * frames
	if g.engine != nil && g.engine.Predictor != nil {
		if h := g.engine.Predictor.Horizon(); h > horizonMaxBound {
			fails = append(fails, fmt.Sprintf(
				"predictor horizon=%d exceeds bound=%d (frames=%d) — Ensure() is being called with horizons growing past elapsed wall-clock",
				h, horizonMaxBound, frames))
		}
		// Healthy: a geometric ramp (cap > len) before windowCap, or a plateau at windowCap. cap == len below windowCap
		// means Ensure reallocates per step.
		if h := g.engine.Predictor.Horizon(); h > 256 && sc.playing {
			perRowCaps := g.engine.Predictor.BufferCapsForTest()
			_, _, windowCap := g.engine.Predictor.WindowBoundsForTest()
			healthy := false
			for _, perBufCaps := range perRowCaps {
				for _, capLen := range perBufCaps {
					// capLen is [cap, len]. Either regime above is healthy.
					if capLen[0] > capLen[1] && capLen[1] > 0 {
						healthy = true
						break
					}
					if windowCap > 0 && capLen[1] == windowCap {
						healthy = true
						break
					}
				}
				if healthy {
					break
				}
			}
			if !healthy && len(perRowCaps) > 0 {
				fails = append(fails, fmt.Sprintf(
					"predictor slice growth is per-step (cap==len<windowCap for every row × buffer), horizon=%d windowCap=%d — "+
						"Ensure must grow geometrically up to windowCap then slide in place; per-step reallocation is the OOM regression.",
					h, windowCap))
			}
		}
	}

	// The stub returns 0/0 (the cache compiles only under !test && !js); checking it still exercises the hook, and
	// `make test-real` checks the real numbers.
	entries, totalBytes := audio.VoiceCacheStatsForTest()
	const voiceCacheEntriesBound = 256
	if entries > voiceCacheEntriesBound {
		fails = append(fails, fmt.Sprintf(
			"voice_cache entries=%d (bound=%d) totalBytes=%dKB — "+
				"voiceCacheKey churn from SetInstrumentParam outraced the cap; "+
				"see voice_cache.go:78 (silent drop on overflow)",
			entries, voiceCacheEntriesBound, totalBytes/1024))
	}

	// Every SetInstrumentParam on an unedited instrument makes its copy differ from the library; far more than the
	// ids in play means stale or synthesised ids.
	const paramsMgrBound = 64
	if n := audio.ModifiedInstrumentDocCountForTest(); n > paramsMgrBound {
		fails = append(fails, fmt.Sprintf(
			"modified instrument documents=%d (bound=%d) — "+
				"SetInstrumentParam ids are accumulating; ResetInstrumentParams "+
				"is not draining them",
			n, paramsMgrBound))
	}

	// WASM allocates a fresh slice on every snapshot; a sustained per-frame rate is pressure its GC cannot keep up
	// with. The stub keeps the counter at zero.
	calls, reads := audio.AnalyzerBridgeStats()
	if frames > 0 && reads/uint64(frames) > 20 {
		fails = append(fails, fmt.Sprintf(
			"analyzer bridge reads/frame=%d (calls=%d reads=%d frames=%d) — "+
				"per-frame []float64 allocation churn from ChannelAnalyzerSnapshot",
			reads/uint64(frames), calls, reads, frames))
	}

	// Churn keeps it between 0 and 1; well above the UI ceiling means RemoveInsertEffect leaks slots.
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

// Every edit marks the predictor dirty and bumps the parity generation, paths steady playback skips.
func toggleSoakGraphEdit(g *Game, st *graphChurnState) {
	if g == nil || g.drum == nil || len(g.drum.Rows) < 2 || st == nil {
		return
	}
	if st.node == nil {
		// Near row 1's band, off the loop's path, so the cycle stays intact.
		n := g.tryAddNode(48, 18, model.NodeTypeRegular)
		if n == nil {
			return
		}
		// Connected, so it joins path computation.
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
		// A runaway horizon means the target is computed off a wall clock that was not reset.
		if g.engine != nil && g.engine.Predictor != nil {
			t.Logf("predictor horizon=%d", g.engine.Predictor.Horizon())
		}
		// Read once at the end: the state the component bounds judged.
		vcEntries, vcBytes := audio.VoiceCacheStatsForTest()
		t.Logf("voice_cache entries=%d totalBytes=%dKB", vcEntries, vcBytes/1024)
		t.Logf("modified instrument documents=%d", audio.ModifiedInstrumentDocCountForTest())
		bridgeCalls, bridgeReads := audio.AnalyzerBridgeStats()
		t.Logf("analyzer bridge calls=%d reads=%d", bridgeCalls, bridgeReads)

		// A steady climb means RemoveInsertEffect leaked a slot.
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

	// Logged so a human can `go tool pprof` it.
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

// math.MaxInt64 for a non-positive slope or a heap already over the ceiling.
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

// Robust to one GC sample landing low or high.
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

// Diagnostic projection only.
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
