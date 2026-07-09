package ui

import (
	"context"
	"hash/fnv"
	"math"
	"strconv"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// synthMirrorPreviewMs is the rendered-note duration the CHECKSUM/responsiveness
// path uses (a full note). The on-screen "Your sound" trace uses the much
// shorter synthMirrorWaveMs so a few wave cycles are legible instead of a solid
// block of 50+ cycles crammed into the pane.
const synthMirrorPreviewMs = 250

// The right-pane "Your sound" trace renders a fixed number of clean wave cycles
// (RenderInstrumentPreviewWave) so the SHAPE — sine vs saw vs square, filter
// rounding, drive edge — reads at a glance for ANY pitch, instead of a solid
// block of 50+ cycles. Cycle-normalized, so it tracks timbre knobs (osc / FM /
// filter / drive), not pitch/envelope (those have their own per-knob pictures).
const (
	synthMirrorWaveCycles = 4
	synthMirrorWavePts    = 128
)

// synth_mirror.go — live "mirror" of the synth's final output for the Synth
// tab right pane. On knob release (debounced + coalesced by a params hash),
// the REAL note is re-rendered off the UI goroutine via a 1-worker async pool
// and cached. The previous render is retained as a faint ghost so the kid can
// see what changed.
//
// Concurrency: all mutable state lives under mu. The render closure runs on a
// pool worker (or synchronously in tests via drainForTest); both paths funnel
// through takePending so the job runs exactly once regardless of who wins the
// race.

const synthMirrorPoolName = "synth.preview"

// synthMirror caches one rendered note per params hash, retaining the prior
// render as a ghost. Cheap to construct; release the shared pool via
// closeForTest in any test that builds one (the ui suite is goleak-checked).
type synthMirror struct {
	mu       sync.Mutex
	lastHash string
	pcm      []float64
	ghost    []float64

	// Full-note slot: the whole rendered hit (RenderInstrumentPreview) for the
	// "Your sound" card, cached independently of the steady-wave slot.
	lastHashFull string
	pcmFull      []float64
	ghostFull    []float64

	ready       bool
	pending     func() // the steady-slot render job, swapped to nil by takePending
	pendingFull func() // the full-note render job, swapped to nil by takePendingFull
	pool        *async.Pool
	privatePool bool // true when pool is a private fallback (budget exhausted)
}

// newSynthMirror acquires the shared 1-worker preview pool from the default
// registry. The mirror is created LAZILY mid-session (when the Synth tab is
// first built), so registry-budget exhaustion must NOT crash the app: like
// eventlogger.format and hooks.fanout, fall back to a private pool when the
// shared budget is full. Multiple instances share the registry name; each must
// release it via closeForTest in tests.
func newSynthMirror() *synthMirror {
	opts := async.Options{
		MaxConcurrent: 1,
		QueueSize:     8,
		Name:          synthMirrorPoolName,
	}
	pool, err := async.DefaultRegistry().Get(synthMirrorPoolName, opts)
	if err != nil {
		// Registry budget exhausted — fall back to a private pool so the Synth
		// tab's live preview still works rather than panicking the UI thread.
		opts.Name = synthMirrorPoolName + ".fallback"
		return &synthMirror{pool: async.NewPool(context.Background(), opts), privatePool: true}
	}
	return &synthMirror{pool: pool}
}

// request schedules a re-render for inst keyed by hash. Identical hashes are
// coalesced (cache hit, no work). On a new hash the current pcm is demoted to
// ghost and the render job is submitted to the pool.
func (m *synthMirror) request(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	if hash == m.lastHash {
		m.mu.Unlock()
		return
	}
	m.lastHash = hash
	m.ghost = m.pcm
	m.pcm = nil
	job := func() {
		out := render(inst)
		m.mu.Lock()
		m.pcm = out
		m.ready = true
		m.mu.Unlock()
	}
	m.pending = job
	m.mu.Unlock()

	// Submit a wrapper that runs the pending job exactly once. If drainForTest
	// (or a later request) already took it, the wrapper is a no-op.
	_ = m.pool.Submit(func(ctx context.Context) { m.takePending() })
}

// previewPool exposes the mirror's shared 1-worker preview pool so sibling
// caches (synthStageThumbs) can reuse it instead of registering a new pool
// name — keeps Release/goleak discipline in one owner (closeForTest).
func (m *synthMirror) previewPool() *async.Pool { return m.pool }

// takePending atomically swaps the pending job to nil and runs it (if any),
// guaranteeing it executes exactly once across the pool worker and drainForTest.
func (m *synthMirror) takePending() {
	m.mu.Lock()
	job := m.pending
	m.pending = nil
	m.mu.Unlock()
	if job != nil {
		job()
	}
}

// snapshot returns the live + ghost PCM for the Draw path (copies under lock).
func (m *synthMirror) snapshot() (pcm, ghost []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pcm != nil {
		pcm = make([]float64, len(m.pcm))
		copy(pcm, m.pcm)
	}
	if m.ghost != nil {
		ghost = make([]float64, len(m.ghost))
		copy(ghost, m.ghost)
	}
	return pcm, ghost
}

// consumeReady returns whether a render completed since the last call and
// clears the flag (one-shot). The Update path uses it to mark a redraw.
func (m *synthMirror) consumeReady() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.ready
	m.ready = false
	return r
}

// renderLive renders synchronously on the caller goroutine, but ONLY when the
// hash changed since the last render — so the Synth tab can call it every frame
// to keep "Your sound" updating in real time as a knob is dragged, at the cost
// of a cheap hash compare when nothing changed. The prior render is kept as the
// ghost on a real change.
func (m *synthMirror) renderLive(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	if hash == m.lastHash && m.pcm != nil {
		m.mu.Unlock()
		return
	}
	m.lastHash = hash
	m.ghost = m.pcm
	m.pcm = render(inst)
	m.ready = true
	m.pending = nil
	m.mu.Unlock()
}

// renderNow fills the mirror synchronously on the caller goroutine, bypassing
// the pool. Used by scene Setup (no test seam) so a deterministic PCM exists at
// screenshot-capture time.
func (m *synthMirror) renderNow(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	m.lastHash = hash
	m.ghost = m.pcm
	m.pcm = render(inst)
	m.ready = true
	m.pending = nil
	m.mu.Unlock()
}

// renderLiveFull is renderLive over the full-note slot: synchronous,
// hash-gated, prior render demoted to ghostFull on a real change.
func (m *synthMirror) renderLiveFull(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	if hash == m.lastHashFull && m.pcmFull != nil {
		m.mu.Unlock()
		return
	}
	m.lastHashFull = hash
	m.ghostFull = m.pcmFull
	m.pcmFull = render(inst)
	m.ready = true
	m.mu.Unlock()
}

// renderNowFull fills the full-note slot synchronously for scene Setup.
func (m *synthMirror) renderNowFull(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	m.lastHashFull = hash
	m.ghostFull = m.pcmFull
	m.pcmFull = render(inst)
	m.ready = true
	m.mu.Unlock()
}

// requestFull schedules a debounced, off-thread re-render of the FULL-note slot
// keyed by hash. It mirrors request over the Full fields with ONE deliberate
// divergence — stale-while-revalidate, exactly like synthStageThumbs: on a new
// hash the current pcmFull is demoted to ghostFull but is NOT niled, so the old
// wave keeps drawing until the new render lands and a live knob drag never
// flickers the card empty. The render runs on a pool worker; the commit guards
// against a superseding newer hash by writing pcmFull only when m.lastHashFull
// still equals this job's hash. A nil pool (pool-free tests) skips the submit
// and leaves the job pending for drainFullForTest.
func (m *synthMirror) requestFull(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	if hash == m.lastHashFull {
		m.mu.Unlock()
		return
	}
	m.lastHashFull = hash
	m.ghostFull = m.pcmFull
	// Stale-while-revalidate: keep pcmFull drawing until the new render lands.
	job := func() {
		out := render(inst)
		m.mu.Lock()
		// Commit only when this job is still the newest request — a later
		// requestFull (new hash) supersedes this render.
		if m.lastHashFull == hash {
			m.pcmFull = out
			m.ready = true
		}
		m.mu.Unlock()
	}
	m.pendingFull = job
	pool := m.pool
	m.mu.Unlock()

	// Submit a wrapper that runs the pending job exactly once. If drainFullForTest
	// (or a later requestFull) already took/replaced it, the wrapper is a no-op.
	if pool != nil {
		_ = pool.Submit(func(ctx context.Context) { m.takePendingFull() })
	}
}

// takePendingFull atomically swaps the pending full-note job to nil and runs it
// (if any), guaranteeing it executes exactly once across the pool worker and
// drainFullForTest. Mirrors takePending over the Full slot.
func (m *synthMirror) takePendingFull() {
	m.mu.Lock()
	job := m.pendingFull
	m.pendingFull = nil
	m.mu.Unlock()
	if job != nil {
		job()
	}
}

// snapshotFullRef returns the full-note live + ghost PCM slices DIRECTLY (no
// copy) under the lock. Immutability contract: published render slices are
// never mutated in place — every render assigns a FRESH slice (m.pcmFull =
// render(...)), so a caller-held reference stays valid forever; callers must
// treat the slices as read-only. The per-frame Draw path uses this instead of
// snapshotFull because copying the up-to-~48k-float full note every frame blew
// the Synth-tab frame byte budget (synth_live_param_edit_alloc_test.go).
func (m *synthMirror) snapshotFullRef() (pcm, ghost []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pcmFull, m.ghostFull
}

// snapshotFull returns copies of the full-note live + ghost PCM. Prefer
// snapshotFullRef on per-frame paths (published slices are immutable).
func (m *synthMirror) snapshotFull() (pcm, ghost []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pcmFull != nil {
		pcm = make([]float64, len(m.pcmFull))
		copy(pcm, m.pcmFull)
	}
	if m.ghostFull != nil {
		ghost = make([]float64, len(m.ghostFull))
		copy(ghost, m.ghostFull)
	}
	return pcm, ghost
}

// --- test helpers ---

// drainForTest runs the pending render job synchronously on the caller
// goroutine. Deterministic regardless of whether the pool worker has run yet.
func (m *synthMirror) drainForTest() { m.takePending() }

// drainFullForTest runs the pending full-note render job synchronously on the
// caller goroutine. Deterministic regardless of whether the pool worker ran yet.
func (m *synthMirror) drainFullForTest() { m.takePendingFull() }

func (m *synthMirror) pcmForTest() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pcm
}

func (m *synthMirror) ghostForTest() []float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ghost
}

// closeForTest releases the shared pool name so the goleak-checked ui suite
// stays clean. Mirrors the async test-discipline note in CLAUDE.md. A private
// fallback pool (budget was exhausted at construction) isn't registry-tracked,
// so close it directly instead of releasing the name.
func (m *synthMirror) closeForTest() {
	if m.privatePool {
		_ = m.pool.Close()
		return
	}
	_ = async.DefaultRegistry().Release(synthMirrorPoolName)
}

// --- DrumView wiring ---

// synthParamsHash returns a stable identity for the instrument's merged
// params: the recipe id plus the two params revisions (per-instrument +
// recipe-defaults). Any mutation that changes the merged output bumps a
// revision, so identical values produce an identical hash and the mirror
// caches re-renders / coalesces no-op knob releases exactly as before — but
// the per-frame gate is now O(1) instead of merging + sorting + formatting
// the ~1030-param modular schema every frame (the Synth-tab GC-churn hang;
// see synth_live_param_edit_alloc_test.go).
func (dv *DrumView) synthParamsHash(instID string) string {
	return audio.RecipeForInstrument(instID) + ":" +
		strconv.FormatUint(audio.InstrumentParamsRev(instID), 10) + ":" +
		strconv.FormatUint(audio.RecipeDefaultsRev(), 10)
}

// requestSynthMirror schedules a debounced, cached, off-thread re-render of the
// real note for instID into the right-pane mirror. Lazily creates the mirror.
func (dv *DrumView) requestSynthMirror(instID string) {
	if dv == nil || instID == "" {
		return
	}
	if dv.synthMirror == nil {
		dv.synthMirror = newSynthMirror()
	}
	dv.synthMirror.request(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreviewWave(i, nil, synthMirrorWaveCycles, synthMirrorWavePts)
	})
}

// synthMirrorPCMLen returns the current mirror PCM sample count (0 when no
// render has completed). Used by the synthMirrorPCMLen JS debug export so a
// browser test can verify the WASM↔audio mirror path produced a non-empty
// render without inspecting DSP internals.
func (dv *DrumView) synthMirrorPCMLen() int {
	if dv == nil || dv.synthMirror == nil {
		return 0
	}
	pcm, _ := dv.synthMirror.snapshot()
	return len(pcm)
}

// synthMirrorPCMChecksum returns a stable 32-bit fingerprint of the current
// mirror PCM CONTENT (0 when empty). Used by the synthMirrorPCMChecksum JS debug
// export so a browser test can prove the WASM mirror REACTS to a param change
// (the fingerprint differs) — not merely that it produced bytes (length, which a
// frozen render would also pass). Samples are rounded to 1e-6 before hashing so
// the digest is robust to negligible float jitter.
func (dv *DrumView) synthMirrorPCMChecksum() int {
	if dv == nil || dv.synthMirror == nil {
		return 0
	}
	pcm, _ := dv.synthMirror.snapshot()
	if len(pcm) == 0 {
		return 0
	}
	h := fnv.New32a()
	var b [8]byte
	for _, v := range pcm {
		bits := math.Float64bits(math.Round(v*1e6) / 1e6)
		for i := 0; i < 8; i++ {
			b[i] = byte(bits >> (8 * i))
		}
		_, _ = h.Write(b[:])
	}
	return int(h.Sum32())
}

// renderSynthMirrorNow fills the mirror synchronously (bypassing the pool) so a
// deterministic PCM exists at screenshot-capture time. Used by scene Setup,
// which runs in the production binary with no test seam.
func (dv *DrumView) renderSynthMirrorNow(instID string) {
	if dv == nil || instID == "" {
		return
	}
	if dv.synthMirror == nil {
		dv.synthMirror = newSynthMirror()
	}
	dv.synthMirror.renderNow(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreviewWave(i, nil, synthMirrorWaveCycles, synthMirrorWavePts)
	})
	dv.synthMirror.renderNowFull(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
}

// updateSynthFullNoteLive schedules a debounced OFF-THREAD re-render of the
// full-note "Your sound" card. Hash-gated like updateSynthMirrorLive, so idle
// frames cost one string compare; on a real change the ~48k-sample full-note
// render runs on the shared preview pool instead of blocking the UI goroutine
// every drag frame. Stale-while-revalidate: the previous trace (plus a ghost of
// the value before the change) keeps drawing until the new render lands.
func (dv *DrumView) updateSynthFullNoteLive(instID string) {
	if dv == nil || instID == "" {
		return
	}
	if dv.synthMirror == nil {
		dv.synthMirror = newSynthMirror()
	}
	dv.synthMirror.requestFull(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
}

// updateSynthMirrorLive re-renders the on-screen "Your sound" wave in real time
// — called every frame from drawSynthTab. It is hash-gated, so a frame where no
// knob moved costs only a hash compare; a knob drag re-renders the short wave
// window each frame so the trace follows the gesture live.
func (dv *DrumView) updateSynthMirrorLive(instID string) {
	if dv == nil || instID == "" {
		return
	}
	if dv.synthMirror == nil {
		dv.synthMirror = newSynthMirror()
	}
	dv.synthMirror.renderLive(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreviewWave(i, nil, synthMirrorWaveCycles, synthMirrorWavePts)
	})
}
