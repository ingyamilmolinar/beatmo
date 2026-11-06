//go:build test

package ui

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// fakeWarmBackend records warm requests and completes them on demand, so the
// controller's diff/progress/deadline logic is tested without a real render.
type fakeWarmBackend struct {
	mu      sync.Mutex
	cached  map[warmItem]bool
	asked   []warmItem
	pending []func()
}

func newFakeWarmBackend() *fakeWarmBackend {
	return &fakeWarmBackend{cached: map[warmItem]bool{}}
}

func (f *fakeWarmBackend) Cached(inst instrument.ID, pitch float64) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cached[warmItem{inst: inst, pitch: pitch}]
}

func (f *fakeWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	f.mu.Lock()
	f.asked = append(f.asked, warmItem{inst: inst, pitch: pitch})
	f.pending = append(f.pending, done)
	f.mu.Unlock()
}

// completeAll fires every queued completion callback.
func (f *fakeWarmBackend) completeAll() {
	f.mu.Lock()
	todo := f.pending
	f.pending = nil
	f.mu.Unlock()
	for _, d := range todo {
		d()
	}
}

// waitForAskedCount blocks until the fake backend has recorded at least n
// Warm() requests, or fails the test after a short timeout. Needed because
// Begin() feeds the backend off the caller's goroutine (see the "feed OFF
// the caller's goroutine" fix in warmup_controller.go): the asked/pending
// slices populate asynchronously relative to Begin() returning, so a test
// that wants to act on the full request set (complete it, capture it as
// "stale", etc.) must synchronize on it explicitly rather than assuming
// Begin() has already fed every item by the time it returns.
func waitForAskedCount(t *testing.T, f *fakeWarmBackend, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := len(f.asked)
		f.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d Warm() request(s) to be fed", n)
}

func newWarmupTestGame(t *testing.T) (*Game, *fakeWarmBackend) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.updateBeatInfos()
	be := newFakeWarmBackend()
	g.warmup = newWarmupController(g, be)
	return g, be
}

// TestWarmupNotReadyUntilEveryItemCompletes is the core gate invariant.
func TestWarmupNotReadyUntilEveryItemCompletes(t *testing.T) {
	g, be := newWarmupTestGame(t)
	g.warmup.Begin()
	done, total := g.warmup.Progress()
	if total == 0 {
		t.Fatal("warm-up began with an empty set")
	}
	if done == total {
		t.Fatal("warm-up reported complete before any item finished")
	}
	if g.warmup.Ready() {
		t.Fatal("Ready() true while items are outstanding")
	}
	waitForAskedCount(t, be, total)
	be.completeAll()
	g.warmup.Tick(1)
	if !g.warmup.Ready() {
		t.Fatal("Ready() false after every item completed")
	}
}

// TestWarmupSkipsAlreadyCachedItems is the cache-hit requirement: an item the
// backend already holds must never be re-rendered.
func TestWarmupSkipsAlreadyCachedItems(t *testing.T) {
	g, be := newWarmupTestGame(t)
	set := g.buildWarmSet()
	for _, it := range set.items {
		be.cached[it] = true // pretend the whole circuit is already warm
	}
	g.warmup.Begin()
	if len(be.asked) != 0 {
		t.Fatalf("re-warmed %d already-cached items, want 0", len(be.asked))
	}
	if !g.warmup.Ready() {
		t.Fatal("gate did not open immediately for a fully cached circuit")
	}
	_, skipped, _ := g.warmup.BenchStats()
	if skipped != set.Len() {
		t.Errorf("BenchStats skippedCached = %d, want %d", skipped, set.Len())
	}
}

// TestWarmupDeadlineOpensTheGate pins the never-deadlock rule: a backend that
// never completes must still let the user press Play once the deadline passes.
func TestWarmupDeadlineOpensTheGate(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	g.warmup.Begin() // fake backend never calls done()
	if g.warmup.Ready() {
		t.Fatal("Ready() true before any completion")
	}
	g.warmup.Tick(warmupDeadlineMS + 1)
	if !g.warmup.Ready() {
		t.Fatal("gate stayed shut past the deadline — playback is deadlocked")
	}
}

// TestWarmupCancelOpensTheGate pins that a user abort releases the gate.
func TestWarmupCancelOpensTheGate(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	g.warmup.Begin()
	g.warmup.Cancel()
	if !g.warmup.Ready() {
		t.Fatal("gate stayed shut after Cancel")
	}
}

// TestWarmupFinishFiresOnEndOnce pins Finding 1: natural completion followed
// by a Cancel() (an ordinary sequence — the user presses Stop just after
// warm-up finished on its own) must fire onEnd exactly once, not twice.
func TestWarmupFinishFiresOnEndOnce(t *testing.T) {
	g, be := newWarmupTestGame(t)
	var mu sync.Mutex
	ends := 0
	g.warmup.onEnd = func(elapsedMs float64, rendered, skipped int, timedOut bool, src hooks.Source) {
		mu.Lock()
		ends++
		mu.Unlock()
	}
	g.warmup.Begin()
	_, total := g.warmup.Progress()
	waitForAskedCount(t, be, total)
	be.completeAll() // natural completion -> fires onEnd once
	g.warmup.Tick(1)
	if !g.warmup.Ready() {
		t.Fatal("expected Ready() after natural completion")
	}
	g.warmup.Cancel() // must be a no-op: session already ended
	g.warmup.Cancel() // and idempotent under repeat calls too
	mu.Lock()
	got := ends
	mu.Unlock()
	if got != 1 {
		t.Fatalf("onEnd fired %d times, want 1", got)
	}
}

// concurrentWarmBackend is a REAL concurrent backend: Warm completes on its
// own goroutine after a short delay, so completion races against the test
// goroutine's Tick/Ready/Progress/Cancel calls for real (unlike
// fakeWarmBackend, whose done() fires synchronously on the caller's
// goroutine and therefore never exercises warmupController's lock
// discipline under -race).
type concurrentWarmBackend struct {
	mu     sync.Mutex
	cached map[warmItem]bool
}

func newConcurrentWarmBackend() *concurrentWarmBackend {
	return &concurrentWarmBackend{cached: map[warmItem]bool{}}
}

func (c *concurrentWarmBackend) Cached(inst instrument.ID, pitch float64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cached[warmItem{inst: inst, pitch: pitch}]
}

func (c *concurrentWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	go func() {
		time.Sleep(2 * time.Millisecond)
		done()
	}()
}

// TestWarmupConcurrentCompletionIsRaceFree exercises invariant 2 (Warm's done
// callback may fire from a worker goroutine) for real: completions land on
// background goroutines while the test goroutine hammers Tick/Ready/Progress
// concurrently. Run with -race; also asserts Progress() never overshoots
// total, which a missing/misplaced lock could produce.
func TestWarmupConcurrentCompletionIsRaceFree(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	be := newConcurrentWarmBackend()
	g.warmup = newWarmupController(g, be)
	g.warmup.Begin()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				g.warmup.Tick(1)
				done, total := g.warmup.Progress()
				if total > 0 && done > total {
					t.Errorf("Progress() overshoot: done=%d total=%d", done, total)
				}
				_ = g.warmup.Ready()
			}
		}
	}()

	ready := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if g.warmup.Ready() {
			ready = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(stop)
	wg.Wait()
	if !ready {
		t.Fatal("warm-up never became ready — concurrent completion path is stuck")
	}
}

// slowWarmBackend's Warm blocks on the CALLING goroutine for delay before
// calling done — it stands in for the real desktop backend's SubmitBlocking
// against a bounded queue, which blocks its feeder for roughly the whole warm
// duration when the queue is saturated. It is intentionally synchronous (no
// internal goroutine) so the only way Begin() can avoid blocking the test
// goroutine is for warmupController itself to feed off-thread.
type slowWarmBackend struct {
	delay time.Duration
}

func (s *slowWarmBackend) Cached(inst instrument.ID, pitch float64) bool { return false }

func (s *slowWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	time.Sleep(s.delay)
	done()
}

// TestWarmupBeginFeedsBackendOffCallerGoroutine pins the non-blocking-caller
// contract: Begin() must return promptly even against a backend whose Warm
// blocks per item, because desktop's real backend blocks on a bounded
// SubmitBlocking queue and Begin() runs on the UI goroutine at import time —
// blocking it would freeze the frame for the whole warm duration and starve
// the progress toast (a later task) of any chance to paint or animate.
//
// Verified to FAIL without the fix: temporarily reverting the feeder in
// warmup_controller.go back to the inline `for _, it := range todo { ...
// w.be.Warm(...) }` loop makes Begin() block for len(todo)*delay, which for
// any non-empty warm set clearly exceeds the 100ms bound below.
func TestWarmupBeginFeedsBackendOffCallerGoroutine(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	set := g.buildWarmSet()
	n := set.Len()
	if n == 0 {
		t.Fatal("warm set is empty; this test needs at least one item to prove non-blocking feeding")
	}

	be := &slowWarmBackend{delay: 300 * time.Millisecond}
	g.warmup = newWarmupController(g, be)

	start := time.Now()
	g.warmup.Begin()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Fatalf("Begin() blocked the caller for %v (n=%d items x %v delay); "+
			"the feeder must run off the caller's goroutine", elapsed, n, be.delay)
	}
	// The gate-correctness state must already be set SYNCHRONOUSLY, even
	// though feeding itself is async — otherwise Ready() could flip true
	// between Begin() returning and the feeder starting.
	if g.warmup.Ready() {
		t.Fatal("Ready() true immediately after Begin() with unfinished slow items")
	}
	done, total := g.warmup.Progress()
	if total != n {
		t.Fatalf("Progress() total = %d, want %d (must be set synchronously in Begin())", total, n)
	}
	if done != 0 {
		t.Fatalf("Progress() done = %d, want 0 immediately after Begin()", done)
	}

	// Drain: let the slow backend actually finish so no goroutine outlives the
	// test (t.Cleanup(g.CloseForTest) runs via newWarmupTestGame).
	deadline := time.Now().Add(time.Duration(n)*be.delay + 2*time.Second)
	for time.Now().Before(deadline) {
		if g.warmup.Ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("warm-up with the slow backend never completed")
}

// TestWarmupStaleCompletionsDoNotCreditNewSession pins Finding 2: a Warm()
// completion callback issued by a superseded session (Begin() called again
// before the prior session's callbacks all fired — e.g. import immediately
// followed by a BPM change) must never credit the new session's counters or
// open the gate early.
//
// Verified to fail without the epoch guard: temporarily reverting itemDone to
// check only `!w.active` (dropping the `ep != w.epoch` clause) makes this
// test fail, because the single stale callback below is then enough to
// satisfy the second session's total=1 and flip Ready() true immediately.
func TestWarmupStaleCompletionsDoNotCreditNewSession(t *testing.T) {
	g, be := newWarmupTestGame(t)

	g.warmup.Begin()
	_, total1 := g.warmup.Progress()
	waitForAskedCount(t, be, total1)
	be.mu.Lock()
	stale := be.pending
	be.pending = nil
	be.mu.Unlock()
	if len(stale) == 0 {
		t.Fatal("expected at least one pending item from the first Begin()")
	}

	g.warmup.Begin() // supersede the first session before its callbacks fire
	_, total2 := g.warmup.Progress()
	if total2 == 0 {
		t.Fatal("second Begin() produced an empty set")
	}

	for _, d := range stale {
		d() // fire the FIRST session's callbacks, now stale
	}

	if g.warmup.Ready() {
		t.Fatal("stale completions from a superseded session opened the gate early")
	}
	done, total := g.warmup.Progress()
	if done != 0 {
		t.Fatalf("stale completions credited the new session: done=%d total=%d, want done=0", done, total)
	}
}
