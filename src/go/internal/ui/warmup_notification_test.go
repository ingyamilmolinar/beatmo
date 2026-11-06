//go:build test

package ui

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestWarmupPushesExactlyOneNotification pins the anti-spam rule: START pushes
// nothing into the notification ring (the toast carries it), COMPLETION pushes
// exactly one durable line for the Logs panel.
func TestWarmupPushesExactlyOneNotification(t *testing.T) {
	g, be := newWarmupTestGame(t)
	g.wireWarmupNotifications()

	before := g.drum.notifStore.Len()
	g.warmup.Begin()
	_, total := g.warmup.Progress()
	if got := g.drum.notifStore.Len(); got != before {
		t.Errorf("warm-up start pushed %d notifications, want 0", got-before)
	}
	if !g.drum.logsToast.taskActive() {
		t.Fatal("progress toast not showing during warm-up")
	}
	waitForAskedCount(t, be, total)
	be.completeAll()
	g.warmup.Tick(1)
	if got := g.drum.notifStore.Len(); got != before+1 {
		t.Errorf("warm-up completion pushed %d notifications, want 1", got-before)
	}
	if g.drum.logsToast.taskActive() {
		t.Fatal("progress toast still showing after completion")
	}
	latest := g.drum.notifStore.Latest()
	if latest == nil || latest.isErr {
		t.Fatal("completion notification missing or marked as an error")
	}
}

// TestWarmupTimeoutPushesAnError pins that a gate that opened on the deadline
// tells the user, rather than silently degrading.
func TestWarmupTimeoutPushesAnError(t *testing.T) {
	g, _ := newWarmupTestGame(t)
	g.wireWarmupNotifications()

	g.warmup.Begin() // fakeWarmBackend never completes
	g.warmup.Tick(warmupDeadlineMS + 1)

	latest := g.drum.notifStore.Latest()
	if latest == nil || !latest.isErr {
		t.Fatal("timed-out warm-up did not push an error notification")
	}
	if !strings.Contains(strings.ToLower(latest.display()), "circuit") {
		t.Errorf("timeout notification %q does not mention the circuit", latest.display())
	}
}

// TestWarmupFullyCachedPathPublishesNothing pins Finding 2 (round 1 review):
// a fully-cached circuit never shows the toast (onBegin is never called), so
// it must not publish EventWarmupComplete either — an orphan "circuit ready —
// 0 sounds" event/notification for a session the user never saw start is
// exactly the noise the start/complete asymmetry exists to avoid. Start and
// complete must stay paired: neither fires, or both do.
func TestWarmupFullyCachedPathPublishesNothing(t *testing.T) {
	g, be := newWarmupTestGame(t)
	g.wireWarmupNotifications()
	set := g.buildWarmSet()
	for _, it := range set.items {
		be.cached[it] = true // pretend the whole circuit is already warm
	}

	before := g.drum.notifStore.Len()
	startCh := make(chan hooks.Event, 1)
	endCh := make(chan hooks.Event, 1)
	t.Cleanup(hooks.Subscribe(hooks.EventWarmupStarted, func(e hooks.Event) {
		select {
		case startCh <- e:
		default:
		}
	}))
	t.Cleanup(hooks.Subscribe(hooks.EventWarmupComplete, func(e hooks.Event) {
		select {
		case endCh <- e:
		default:
		}
	}))
	g.warmup.Begin()
	select {
	case e := <-startCh:
		t.Fatalf("unexpected EventWarmupStarted published for a fully-cached circuit (source=%+v)", e.Source)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case e := <-endCh:
		t.Fatalf("unexpected EventWarmupComplete published for a fully-cached circuit (source=%+v)", e.Source)
	case <-time.After(100 * time.Millisecond):
	}
	if !g.warmup.Ready() {
		t.Fatal("gate did not open immediately for a fully cached circuit")
	}
	if got := g.drum.notifStore.Len(); got != before {
		t.Errorf("fully-cached warm-up pushed %d notifications, want 0", got-before)
	}
}

// TestWarmupSourceAttributionCarriesAcrossFinishPaths pins Finding 1 (round 1
// review): the triggering action's hooks.Source is captured exactly once, in
// Begin(), and must be identical whichever of the three finish() paths that
// actually publish (natural completion via itemDone on the feeder goroutine,
// deadline timeout via Tick, and Cancel) closes the session — none of them
// may re-derive their own (wrong) source via a fixed CaptureSource skip.
func TestWarmupSourceAttributionCarriesAcrossFinishPaths(t *testing.T) {
	t.Run("natural completion", func(t *testing.T) {
		g, be := newWarmupTestGame(t)
		g.wireWarmupNotifications()
		startEvt := awaitEmit(t, hooks.EventWarmupStarted, func() {
			g.warmup.Begin()
		})
		_, total := g.warmup.Progress()
		waitForAskedCount(t, be, total)
		endEvt := awaitEmit(t, hooks.EventWarmupComplete, func() {
			be.completeAll()
			g.warmup.Tick(1)
		})
		if startEvt.Source != endEvt.Source {
			t.Errorf("natural-completion source %+v != start source %+v", endEvt.Source, startEvt.Source)
		}
	})

	t.Run("deadline timeout", func(t *testing.T) {
		g, _ := newWarmupTestGame(t)
		g.wireWarmupNotifications()
		startEvt := awaitEmit(t, hooks.EventWarmupStarted, func() {
			g.warmup.Begin() // fakeWarmBackend never completes
		})
		endEvt := awaitEmit(t, hooks.EventWarmupComplete, func() {
			g.warmup.Tick(warmupDeadlineMS + 1)
		})
		if startEvt.Source != endEvt.Source {
			t.Errorf("timeout source %+v != start source %+v", endEvt.Source, startEvt.Source)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		g, _ := newWarmupTestGame(t)
		g.wireWarmupNotifications()
		startEvt := awaitEmit(t, hooks.EventWarmupStarted, func() {
			g.warmup.Begin()
		})
		endEvt := awaitEmit(t, hooks.EventWarmupComplete, func() {
			g.warmup.Cancel()
		})
		if startEvt.Source != endEvt.Source {
			t.Errorf("cancel source %+v != start source %+v", endEvt.Source, startEvt.Source)
		}
	})
}

// warmCircuitForTest warms every voice the current circuit needs and returns
// once the warm-up has fully ended — its "Circuit ready" line included — so a
// test that then asserts on the NEWEST notification reads its own action's line.
//
// Import, undo/redo (a re-import) and a BPM change each start a warm-up. On a
// cold voice cache it renders off-thread and posts "Circuit ready" whenever it
// lands — often after the action's own notification, which Latest() then no
// longer sees. With the circuit already warm, those later warm-ups take the
// fully-cached fast path and post nothing. (Tests that skipped this passed only
// when an earlier test in the run happened to have warmed the same voice.)
//
// It waits on onEnd, not Ready(): finish() clears `active` BEFORE it calls
// onEnd, so Ready() turns true while the notification is still unposted.
func warmCircuitForTest(t *testing.T, g *Game) {
	t.Helper()
	w := g.warmup
	done := make(chan struct{})
	var once sync.Once
	w.mu.Lock()
	prev := w.onEnd
	w.onEnd = func(elapsedMs float64, rendered, skipped int, timedOut bool, src hooks.Source) {
		if prev != nil {
			prev(elapsedMs, rendered, skipped, timedOut, src)
		}
		once.Do(func() { close(done) })
	}
	w.mu.Unlock()
	t.Cleanup(func() {
		w.mu.Lock()
		w.onEnd = prev
		w.mu.Unlock()
	})
	g.beginWarmup("test")
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("circuit warm-up did not finish within 10s")
	}
}
