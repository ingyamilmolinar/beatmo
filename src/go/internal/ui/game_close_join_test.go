package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

// Close must JOIN bpmLoop and audioLoop within closeJoinTimeout, or goleak sees parked receivers.
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

	// A yield so the dump does not catch an already-returned goroutine mid-defer.
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

// Fixture cleanups sometimes call it twice.
func TestGameCloseIsIdempotent(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	g.Close()
	// Without the early-return guard this double-closes audioQuit and panics.
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
