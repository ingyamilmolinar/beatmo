package ui

import (
	"strconv"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// warmupDeadlineMS bounds how long the readiness gate may hold playback. The
// gate MUST never deadlock: a backend that cannot make progress (no render
// worker, WASM module unavailable, throttled background tab) opens the gate
// anyway and playback proceeds with the existing serveBestAvailable /
// neighbor-substitution behaviour — degraded, not blocked.
const warmupDeadlineMS int64 = 15000

// warmBackend renders voices into the platform's voice cache. Cached must use
// the SAME key computation the dispatch path uses, or the diff will re-render
// voices that are already warm (or, worse, skip ones that are not).
type warmBackend interface {
	Cached(inst instrument.ID, pitch float64) bool
	Warm(inst instrument.ID, pitch float64, done func())
}

// warmupController gates playback on a warm voice cache after a wholesale
// cache-invalidating event (import, sample-rate change). A BPM change is not
// one: a voice is render.seconds long at every tempo.
type warmupController struct {
	g  *Game
	be warmBackend

	mu        sync.Mutex
	active    bool
	ended     bool // finish() latch: blocks a second onEnd for this session (Cancel after natural completion/timeout, or a repeat Cancel)
	total     int
	completed int
	skipped   int
	startMs   int64
	elapsedMs float64
	timedOut  bool
	// epoch increments on every Begin(). itemDone callbacks capture the epoch
	// they were issued under and are dropped if it no longer matches — without
	// this, a stale completion from a superseded session (Begin() called again
	// before the previous session's Warm() callbacks all fired, e.g. import
	// immediately followed by a sample-rate change) credits the NEW session's counters
	// and can open the gate before the new circuit's voices are actually warm.
	// That is a silent correctness bug (wrong-pitch/stuttering playback), not
	// a cosmetic one — never remove this guard to "simplify" itemDone.
	epoch uint64

	// src is the hooks.Source of the ACTION that started this warm session,
	// captured once in Begin(). It cannot be derived at emit time: finish() is
	// reached from Begin's fast path, from itemDone() on the feeder GOROUTINE
	// (no user stack at all), from Tick()'s deadline on a frame tick, and from
	// Cancel() — four different depths, so no fixed CaptureSource skip is
	// correct for onEnd. Capturing at the trigger is the only stable answer.
	src hooks.Source

	// onBegin/onUpdate/onEnd drive the progress toast (Task 8). nil-safe.
	onBegin  func(total int, src hooks.Source)
	onUpdate func(done, total int)
	onEnd    func(elapsedMs float64, rendered, skipped int, timedOut bool, src hooks.Source)
}

func newWarmupController(g *Game, be warmBackend) *warmupController {
	return &warmupController{g: g, be: be}
}

// Begin rebuilds the warm set, diffs it against the cache, and starts warming
// whatever is missing. A fully cached circuit opens the gate synchronously.
func (w *warmupController) Begin() {
	if w == nil || w.g == nil || w.be == nil {
		return
	}
	set := w.g.buildWarmSet()

	todo := make([]warmItem, 0, set.Len())
	skipped := 0
	for _, it := range set.items {
		if w.be.Cached(it.inst, it.pitch) {
			skipped++
			continue
		}
		todo = append(todo, it)
	}

	w.mu.Lock()
	w.epoch++
	ep := w.epoch
	w.total = len(todo)
	w.completed = 0
	w.skipped = skipped
	w.timedOut = false
	w.ended = false
	w.startMs = time.Now().UnixMilli()
	w.active = len(todo) > 0
	// Capture the triggering action's source ONCE, here, and carry it through
	// every finish() path (fast-path below, itemDone on the feeder goroutine,
	// Tick's deadline, Cancel) — see the src field's doc comment for why no
	// fixed skip at the emit site can be correct. skip arithmetic: inside
	// Begin, skip 0 is Begin itself, 1 is Begin's caller (beginWarmup, added
	// by a later task), 2 is that wrapper's caller — the actual import /
	// sample-rate site. CaptureSource returns the zero Source (never panics)
	// if the stack is shorter than requested; callers already handle that.
	w.src = hooks.CaptureSource(2)
	src := w.src
	begin, onBegin := w.active, w.onBegin
	w.mu.Unlock()

	if !begin {
		// Everything already cached: no toast, no wait.
		w.finish(false)
		return
	}
	if onBegin != nil {
		onBegin(len(todo), src)
	}
	// Feed the backend OFF the caller's goroutine. Desktop's backend blocks on a
	// bounded queue (SubmitBlocking — by design, the gated set must never be
	// dropped), and Begin runs on the UI goroutine at import time, so feeding
	// inline would freeze the frame for the whole warm duration and the progress
	// toast could never animate. The feeder re-checks the epoch each iteration so
	// a superseded session (a second Begin) stops submitting instead of racing
	// the new one for queue slots. Everything Ready()/Progress() depend on
	// (active, total, completed, epoch, startMs, ended) is already set
	// synchronously above — only the feeding loop itself moves off-thread.
	go func() {
		for _, it := range todo {
			w.mu.Lock()
			stale := ep != w.epoch
			w.mu.Unlock()
			if stale {
				return
			}
			w.be.Warm(it.inst, it.pitch, func() { w.itemDone(ep) })
		}
	}()
}

// itemDone credits one completed render against the session identified by ep.
// A callback from a superseded session (ep != current epoch) is dropped —
// see the epoch field's doc comment for why this must never be relaxed.
func (w *warmupController) itemDone(ep uint64) {
	w.mu.Lock()
	if !w.active || ep != w.epoch {
		w.mu.Unlock()
		return
	}
	w.completed++
	done, total := w.completed, w.total
	onUpdate := w.onUpdate
	complete := done >= total
	w.mu.Unlock()

	if onUpdate != nil {
		onUpdate(done, total)
	}
	if complete {
		w.finish(false)
	}
}

// Tick enforces the deadline. nowMs is milliseconds since Begin for tests; pass
// 0 in production and the controller reads the wall clock.
func (w *warmupController) Tick(nowMs int64) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.active {
		w.mu.Unlock()
		return
	}
	elapsed := nowMs
	if elapsed == 0 {
		elapsed = time.Now().UnixMilli() - w.startMs
	}
	over := elapsed > warmupDeadlineMS
	w.mu.Unlock()
	if over {
		w.finish(true)
	}
}

// Cancel opens the gate immediately (user abort). Outstanding items stay queued
// in the backend as ordinary background warms.
func (w *warmupController) Cancel() { w.finish(false) }

func (w *warmupController) finish(timedOut bool) {
	w.mu.Lock()
	if w.ended || w.startMs == 0 {
		w.mu.Unlock()
		return
	}
	w.ended = true
	w.active = false
	w.timedOut = timedOut
	if w.startMs > 0 {
		w.elapsedMs = float64(time.Now().UnixMilli() - w.startMs)
	}
	onEnd := w.onEnd
	// src is read under the same lock that Begin() writes it under: finish()
	// can run on the feeder goroutine (via itemDone), so this is a real
	// synchronization point, not a formality — an unsynchronized read here
	// would be a genuine data race against Begin()'s write.
	elapsed, rendered, skipped, src := w.elapsedMs, w.completed, w.skipped, w.src
	w.mu.Unlock()
	if onEnd != nil {
		onEnd(elapsed, rendered, skipped, timedOut, src)
	}
}

// Ready reports whether playback may start.
func (w *warmupController) Ready() bool {
	if w == nil {
		return true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.active
}

// Progress reports (completed, total) for the progress toast.
func (w *warmupController) Progress() (int, int) {
	if w == nil {
		return 0, 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.completed, w.total
}

// BenchStats feeds the [bench_json] line (Task 1).
func (w *warmupController) BenchStats() (int, int, float64) {
	if w == nil {
		return 0, 0, 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.total + w.skipped, w.skipped, w.elapsedMs
}

// wireWarmupNotifications connects the controller to the progress toast, the
// notification ring, and the hooks bus.
//
// Deliberately asymmetric: START shows the toast and publishes a hooks event
// but pushes NOTHING into the notification ring (it would spam the Logs panel
// on every import). COMPLETION pushes exactly one durable, translatable line.
func (g *Game) wireWarmupNotifications() {
	if g == nil || g.warmup == nil || g.drum == nil {
		return
	}
	w := g.warmup
	w.onBegin = func(total int, src hooks.Source) {
		g.drum.logsToast.beginTask(i18n.KeyNotifCircuitLoading, total)
		_, skipped, _ := w.BenchStats()
		emitWarmupStarted(src, total, skipped)
	}
	w.onUpdate = func(done, total int) {
		g.drum.logsToast.updateTask(done)
	}
	w.onEnd = func(elapsedMS float64, rendered, skipped int, timedOut bool, src hooks.Source) {
		g.drum.logsToast.endTask()
		total := rendered
		if d, tt := w.Progress(); tt > 0 {
			total, rendered = tt, d
		}
		// pushed tracks whether a notification actually went into the ring —
		// the fully-cached fast path calls finish() (and thus onEnd) without
		// ever calling onBegin, so total==0 and timedOut==false here. Publishing
		// EventWarmupComplete in that case would be an orphan "circuit ready —
		// 0 sounds" line for a session the user never saw start. Gate the hooks
		// event on the same predicate as the notification so start/complete
		// stay paired: neither fires, or both do.
		pushed := false
		if timedOut {
			g.drum.notifyErrorKey(i18n.KeyNotifCircuitWarmupTimedOut,
				strconv.Itoa(rendered), strconv.Itoa(total))
			pushed = true
		} else if total > 0 {
			g.drum.notifyInfoKey(i18n.KeyNotifCircuitReady,
				strconv.Itoa(rendered), strconv.FormatFloat(elapsedMS, 'f', 0, 64),
				strconv.Itoa(skipped))
			pushed = true
		}
		if pushed {
			emitWarmupComplete(src, hooks.WarmupPayload{
				Total: total, Rendered: rendered, Cached: skipped,
				ElapsedMS: elapsedMS, TimedOut: timedOut,
			})
		}
	}
}

// beginWarmup trips the readiness gate after a wholesale cache invalidation.
// reason is for the debug log only. Single instrument switches and one-off
// param edits deliberately do NOT come through here — they keep the existing
// non-blocking background warm plus the serveBestAvailable fallback.
func (g *Game) beginWarmup(reason string) {
	if g == nil || g.warmup == nil {
		return
	}
	g.logger.Debugf("[warmup] begin (%s)", reason)
	g.warmup.Begin()
}

// warmupSampleRateCheckInterval throttles the per-frame sample-rate poll to
// once every N calls. On desktop audio.SampleRate() is a single read of a
// cached int and effectively free either way; on js it is
// js.Global().Get("__audioCtxSR") + Truthy + Float — 2-3 WASM<->JS boundary
// crossings, on the platform with the tightest documented frame budget
// (PERF_BROWSER_UPDATE_MAX_MS). A sample-rate change is a once-per-session
// event (AudioContext creation on the unlock gesture), so polling every
// single frame buys nothing. The counter starts the check on the very FIRST
// call (so a fresh Game captures the initial rate immediately, no warm-up
// delay) and then every warmupSampleRateCheckInterval calls thereafter.
const warmupSampleRateCheckInterval = 30

// checkWarmupSampleRate trips the gate when the observed sample rate changes.
// The voice cache key carries sampleRate, so a rate change re-keys EVERYTHING.
// On browser the rate is unknown until the AudioContext exists, so a
// pre-unlock import warms at the assumed rate and goes cold the moment the
// real context appears. Called every frame from game_update.go, immediately
// before g.warmup.Tick — see warmupSampleRateCheckInterval for why the actual
// audio.SampleRate() read is throttled rather than per-frame.
func (g *Game) checkWarmupSampleRate() {
	if g == nil {
		return
	}
	g.warmupSampleRateCheckCounter++
	if (g.warmupSampleRateCheckCounter-1)%warmupSampleRateCheckInterval != 0 {
		return
	}
	sr := audio.SampleRate()
	if sr <= 0 || sr == g.warmupSampleRate {
		return
	}
	if g.warmupSampleRate != 0 {
		g.beginWarmup("sample-rate")
	}
	g.warmupSampleRate = sr
}
