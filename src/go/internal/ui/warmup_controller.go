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

// The gate must never deadlock: a backend that cannot progress opens it anyway and playback degrades, never blocks.
const warmupDeadlineMS int64 = 15000

// Cached must use dispatch's key, or warm voices re-render (or cold ones are skipped).
type warmBackend interface {
	Cached(inst instrument.ID, pitch float64) bool
	Warm(inst instrument.ID, pitch float64, done func())
}

// After a wholesale invalidation (import, sample-rate change); a tempo change is not one.
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
	// A completion from a superseded session must not credit the new one and open the gate before its voices are warm
	// (wrong-pitch, stuttering playback). Never remove this guard.
	epoch uint64

	// The ACTION's source, captured once in Begin: finish() is reached from four call depths (one on a goroutine with
	// no user stack), so no fixed skip at the emit site is right.
	src hooks.Source

	// onBegin/onUpdate/onEnd drive the progress toast (Task 8). nil-safe.
	onBegin  func(total int, src hooks.Source)
	onUpdate func(done, total int)
	onEnd    func(elapsedMs float64, rendered, skipped int, timedOut bool, src hooks.Source)
}

func newWarmupController(g *Game, be warmBackend) *warmupController {
	return &warmupController{g: g, be: be}
}

// A fully cached circuit opens the gate synchronously.
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
	// skip 2: Begin's caller is beginWarmup, whose caller is the real import / sample-rate site. A short stack yields
	// the zero Source.
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
	// Off the caller's goroutine: the desktop backend blocks, and inline feeding would freeze the frame and the toast.
	// The feeder re-checks the epoch so a superseded session stops submitting.
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

// A superseded session's callback is dropped.
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

// nowMs is for tests; 0 reads the wall clock.
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

// Outstanding items stay queued as ordinary background warms.
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
	// Under the lock Begin writes it under: finish() can run on the feeder goroutine.
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

// START shows the toast and publishes an event but logs NOTHING (it would spam Logs on every import); COMPLETION
// pushes exactly one durable line.
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
		// The cached fast path finishes without ever beginning; gating the event on the same predicate keeps start and
		// complete paired, never an orphan "ready — 0 sounds".
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

// Instrument switches and one-off edits do NOT come here; they keep the non-blocking background warm.
func (g *Game) beginWarmup(reason string) {
	if g == nil || g.warmup == nil {
		return
	}
	g.logger.Debugf("[warmup] begin (%s)", reason)
	g.warmup.Begin()
}

// On js the rate read costs 2–3 JS crossings and it changes once a session (the unlock gesture), so it is polled
// every N calls, starting on the very first.
const warmupSampleRateCheckInterval = 30

// The cache key carries the rate, so a change re-keys everything; on the browser it is unknown until the
// AudioContext exists, so a pre-unlock import goes cold.
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
