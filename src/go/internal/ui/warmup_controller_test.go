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

// Tests the diff/progress/deadline logic without a real render.
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

// Begin() feeds the backend off the caller's goroutine, so a test that acts on the full request set must wait for
// it.
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

// A backend that never completes must still let the user press Play once the deadline passes.
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

// Natural completion then Cancel (Stop right after warm-up finished) fires onEnd once, not twice.
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

// Completes on its own goroutine, so completion really races the test's calls under -race; fakeWarmBackend
// completes synchronously and never exercises the lock discipline.
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

// Completions land on background goroutines while the test hammers Tick/Ready/Progress; Progress() never overshoots
// total, which a misplaced lock could produce. Run with -race.
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

// Synchronous on purpose, standing in for SubmitBlocking on a saturated queue: Begin() avoids blocking only if the
// controller feeds off-thread.
type slowWarmBackend struct {
	delay time.Duration
}

func (s *slowWarmBackend) Cached(inst instrument.ID, pitch float64) bool { return false }

func (s *slowWarmBackend) Warm(inst instrument.ID, pitch float64, done func()) {
	time.Sleep(s.delay)
	done()
}

// Begin() runs on the UI goroutine at import; a backend that blocks per item would otherwise freeze the frame for
// the whole warm-up and starve the progress toast.
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
	// Set SYNCHRONOUSLY although feeding is async, or Ready() could flip true before the feeder starts.
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

	// So no goroutine outlives the test.
	deadline := time.Now().Add(time.Duration(n)*be.delay + 2*time.Second)
	for time.Now().Before(deadline) {
		if g.warmup.Ready() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("warm-up with the slow backend never completed")
}

// A completion from a superseded session (Begin called again before the previous one's callbacks all fired) must
// never credit the new session or open the gate early: the epoch guard.
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
