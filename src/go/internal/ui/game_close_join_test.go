package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestGameCloseJoin verifies that Game.Close() actually joins the bpmLoop
// and audioLoop goroutines (via g.bgWG) before returning. The previous
// implementation closed the channels and returned immediately, leaving the
// receivers parked just long enough that goleak.VerifyTestMain panicked.
//
// We assert two properties:
//  1. Close() returns within the documented closeJoinTimeout budget.
//  2. After Close() returns, none of the package's named loops appear in a
//     fresh goroutine stack dump.
func TestGameCloseJoin(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)

	t0 := time.Now()
	done := make(chan struct{})
	go func() {
		g.Close()
		close(done)
	}()

	select {
	case <-done:
		if d := time.Since(t0); d > closeJoinTimeout {
			t.Fatalf("Close() took %s, want <= %s", d, closeJoinTimeout)
		}
	case <-time.After(closeJoinTimeout + 100*time.Millisecond):
		t.Fatalf("Close() did not return within %s; bgWG.Wait() never finished", closeJoinTimeout+100*time.Millisecond)
	}

	// Give the runtime a moment to settle the now-dead goroutines so the
	// stack dump doesn't catch them mid-defer. This isn't waiting for an
	// unblock — they already returned — just a yield to the scheduler.
	runtime.Gosched()
	time.Sleep(10 * time.Millisecond)

	dump := make([]byte, 64*1024)
	dump = dump[:runtime.Stack(dump, true)]
	stacks := string(dump)
	for _, name := range []string{
		"internal/ui.(*Game).bpmLoop",
		"internal/ui.(*Game).audioLoop",
		"core/engine.(*Engine).run",
	} {
		if strings.Contains(stacks, name) {
			t.Errorf("goroutine %q is still alive after Close() returned:\n%s", name, snippetAround(stacks, name))
		}
	}
}

// TestGameCloseIsIdempotent confirms the multi-call safety contract on
// Game.Close — t.Cleanup paths in fixtures occasionally double-invoke it.
func TestGameCloseIsIdempotent(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	g.Close()
	// A second call must be a no-op; absent the early-return guard this
	// would double-close audioQuit and panic.
	g.Close()
}

func snippetAround(stacks, needle string) string {
	idx := strings.Index(stacks, needle)
	if idx < 0 {
		return ""
	}
	start := idx
	if start > 200 {
		start = idx - 200
	} else {
		start = 0
	}
	end := idx + 400
	if end > len(stacks) {
		end = len(stacks)
	}
	return stacks[start:end]
}
