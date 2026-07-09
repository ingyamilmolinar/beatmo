package ui

import (
	"context"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synth_stage_thumbs.go — per-chip preview cache for the living pipeline chip
// strip (wave 2). Renders one short preview PER CHIP with every stage AFTER
// that chip forced disabled (enableParam -> 0), so the resulting waveform
// depicts "what the signal sounds like up to and including this stage" —
// the classic modular-synth signal-chain thumbnail. Task 15 draws these as
// faint min/max watermarks inside each chip; this file is data-only.
//
// Concurrency mirrors synthMirror.request/takePending exactly: `ensure` is a
// cheap hash+count gate on the UI goroutine; on a real change it captures the
// chip data (count + per-chip override maps — chip state must NOT be read from
// the worker) and stashes a render job that a shared 1-worker pool executes
// off-thread. Stale-while-revalidate: waveFor keeps returning the previous
// waves until the job lands, so a live knob drag costs the UI goroutine one
// string compare per frame instead of ~10 synchronous preview renders
// (measured 3.9-5.2ms/frame — the documented "synth param re-render storm"
// failure shape). The pool is SHARED with synthMirror (same *async.Pool, no
// new registry name, no extra Release discipline); a nil pool leaves jobs
// pending for drainForTest (deterministic tests).
type synthStageThumbs struct {
	mu      sync.Mutex
	hash    string
	wantN   int    // chip count of the newest request (async-completion gate)
	waves   [][]float64
	pending func()      // the render job, swapped to nil by takePending
	pool    *async.Pool // shared with synthMirror; nil => drainForTest-only
}

// synthStageDownstreamOverride builds an override map that disables every
// chip strictly AFTER chipIdx that carries a non-empty enableParam, so a
// preview render for chipIdx depicts the signal up to and including that
// stage but none of the stages after it. Chips at or before chipIdx (i.e.
// upstream stages and the stage itself) are left untouched. Returns a fresh
// map on every call — callers (renderers) must never share or mutate it.
func (dv *DrumView) synthStageDownstreamOverride(chipIdx int) map[string]float64 {
	ov := make(map[string]float64)
	if dv == nil {
		return ov
	}
	for i, c := range dv.instEditorChips {
		if i <= chipIdx {
			continue
		}
		if c.enableParam == "" {
			continue
		}
		ov[c.enableParam] = 0
	}
	return ov
}

// ensure schedules an off-thread re-render of every chip's thumbnail when the
// params hash OR the chip count changed since the newest request; a no-op
// (one mutex + two compares) otherwise. The count gate matters twice over:
// the first ensure can run on a frame where the synth layout hasn't populated
// instEditorChips yet (observed in the real app: ensure fired once with 0
// chips), and gating on the STORED wantN — not len(t.waves) — keeps the
// pending-but-not-landed state from resubmitting the same job every frame.
func (t *synthStageThumbs) ensure(dv *DrumView, instID, hash string) {
	if t == nil || dv == nil || instID == "" {
		return
	}
	n := len(dv.instEditorChips)
	t.mu.Lock()
	if hash == t.hash && t.wantN == n {
		t.mu.Unlock()
		return
	}
	// Capture chip-derived inputs on the caller (UI) goroutine, under the
	// lock — the render job runs on a pool worker and must not touch dv.
	overrides := make([]map[string]float64, n)
	for i := 0; i < n; i++ {
		overrides[i] = dv.synthStageDownstreamOverride(i)
	}
	t.hash = hash
	t.wantN = n
	job := func() {
		waves := make([][]float64, n)
		for i := 0; i < n; i++ {
			waves[i] = audio.RenderInstrumentPreviewWithOverrides(instID, overrides[i], 150)
		}
		t.mu.Lock()
		// Commit only when this job is still the newest request — a later
		// ensure (new hash or chip count) supersedes this render.
		if t.hash == hash && t.wantN == n {
			t.waves = waves
		}
		t.mu.Unlock()
	}
	t.pending = job
	pool := t.pool
	t.mu.Unlock()

	// Submit a wrapper that runs the pending job exactly once. If drainForTest
	// (or a later ensure) already took/replaced it, the wrapper is a no-op.
	if pool != nil {
		_ = pool.Submit(func(ctx context.Context) { t.takePending() })
	}
}

// ensureNow renders synchronously on the caller goroutine, bypassing the pool.
// Used by scene Setup (which runs in the production binary with no test seam)
// so deterministic watermark PCM exists at screenshot-capture time. Mirrors
// synthMirror.renderNow.
func (t *synthStageThumbs) ensureNow(dv *DrumView, instID, hash string) {
	if t == nil || dv == nil || instID == "" {
		return
	}
	n := len(dv.instEditorChips)
	waves := make([][]float64, n)
	for i := 0; i < n; i++ {
		waves[i] = audio.RenderInstrumentPreviewWithOverrides(instID, dv.synthStageDownstreamOverride(i), 150)
	}
	t.mu.Lock()
	t.hash = hash
	t.wantN = n
	t.waves = waves
	t.pending = nil
	t.mu.Unlock()
}

// takePending atomically swaps the pending job to nil and runs it (if any),
// guaranteeing it executes exactly once across the pool worker and
// drainForTest. Mirrors synthMirror.takePending.
func (t *synthStageThumbs) takePending() {
	t.mu.Lock()
	job := t.pending
	t.pending = nil
	t.mu.Unlock()
	if job != nil {
		job()
	}
}

// drainForTest runs the pending render job synchronously on the caller
// goroutine. Deterministic regardless of whether the pool worker has run yet.
func (t *synthStageThumbs) drainForTest() { t.takePending() }

// waveForRef returns the cached thumbnail for chip i DIRECTLY (no copy) under
// the lock (nil when absent or i out of range). Immutability contract:
// published thumbnail slices are never mutated in place — every render job
// builds a fresh `waves` slice-of-slices and commits it wholesale, so a
// caller-held reference stays valid; callers must treat it as read-only. The
// per-frame chip-strip watermark uses this instead of waveFor because copying
// each ~7k-float stage wave per chip per frame blew the Synth-tab frame byte
// budget (synth_live_param_edit_alloc_test.go). Stale-while-revalidate applies
// exactly as for waveFor.
func (t *synthStageThumbs) waveForRef(i int) []float64 {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if i < 0 || i >= len(t.waves) {
		return nil
	}
	return t.waves[i]
}

// waveFor returns a copy of the cached thumbnail for chip i (nil when absent
// or i is out of range). Prefer waveForRef on per-frame paths (published
// slices are immutable). Stale-while-revalidate: while a re-render is pending
// or in flight this keeps returning the PREVIOUS waves, so the chip strip
// never flickers empty mid-drag.
func (t *synthStageThumbs) waveFor(i int) []float64 {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if i < 0 || i >= len(t.waves) {
		return nil
	}
	src := t.waves[i]
	if src == nil {
		return nil
	}
	out := make([]float64, len(src))
	copy(out, src)
	return out
}

// lazyStageThumbs creates dv.stageThumbs on first use, sharing synthMirror's
// 1-worker preview pool (same *async.Pool — no new registry name, so the
// mirror's existing closeForTest/Release discipline covers it).
func (dv *DrumView) lazyStageThumbs() *synthStageThumbs {
	if dv.stageThumbs == nil {
		if dv.synthMirror == nil {
			dv.synthMirror = newSynthMirror()
		}
		dv.stageThumbs = &synthStageThumbs{pool: dv.synthMirror.previewPool()}
	}
	return dv.stageThumbs
}

// ensureStageThumbs lazily creates dv.stageThumbs and schedules a hash-gated
// off-thread re-render for instID. Task 15's drawSynthTab calls this before
// drawSynthChipStrip; on idle/drag frames it costs one string compare.
func (dv *DrumView) ensureStageThumbs(instID string) {
	if dv == nil || instID == "" {
		return
	}
	dv.lazyStageThumbs().ensure(dv, instID, dv.synthParamsHash(instID))
}

// ensureStageThumbsNow renders the stage thumbnails synchronously (bypassing
// the pool) so scene Setup produces deterministic watermarks at capture time.
// Sibling of renderSynthMirrorNow — call them together in scene setups.
func (dv *DrumView) ensureStageThumbsNow(instID string) {
	if dv == nil || instID == "" {
		return
	}
	dv.lazyStageThumbs().ensureNow(dv, instID, dv.synthParamsHash(instID))
}
