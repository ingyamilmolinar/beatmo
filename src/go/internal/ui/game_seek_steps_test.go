package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestSeekStepsStoppedSetsPosition(t *testing.T) {
	g := newGameWithStart(t) // internal package-ui constructor with a start node (playable)
	div := g.grid.MaxDiv()
	g.SeekSteps(4*div, seekOriginRibbon) // beat 4
	if got := g.playheadAbsSubdiv(); got != 4*div {
		t.Fatalf("playhead=%d want %d", got, 4*div)
	}
}

func TestSeekStepsEmitsEventWithOrigin(t *testing.T) {
	g := newGameWithStart(t)
	// hooks.Bus is ASYNC (worker pool, no ordering) — observe via a buffered
	// channel + timeout rather than a synchronous count.
	ch := make(chan hooks.SeekPayload, 4)
	unsub := hooks.Subscribe(hooks.EventSeek, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.SeekPayload); ok {
			select {
			case ch <- p:
			default:
			}
		}
	})
	defer unsub()
	div := g.grid.MaxDiv()
	g.SeekSteps(6*div, seekOriginKeyboard)
	select {
	case p := <-ch:
		if p.Origin != seekOriginKeyboard || p.Steps != 6*div {
			t.Fatalf("payload=%+v want origin=keyboard steps=%d", p, 6*div)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventSeek observed within 1s")
	}
}

func TestSeekStepsClampsNegative(t *testing.T) {
	g := newGameWithStart(t)
	g.SeekSteps(-100, seekOriginJS)
	if got := g.playheadAbsSubdiv(); got != 0 {
		t.Fatalf("playhead=%d want 0 (clamped)", got)
	}
}

func TestSeekStepsPlayingReanchorsClocks(t *testing.T) {
	g := newGameWithStart(t)
	g.updateBeatInfos()
	g.state.SetPlaying(true)
	div := g.grid.MaxDiv()
	before := time.Now()
	g.SeekSteps(3*div, seekOriginTransport)
	if got := g.playheadAbsSubdiv(); got != 3*div {
		t.Fatalf("playhead=%d want %d", got, 3*div)
	}
	if g.state.PlayStart().Before(before) {
		t.Fatalf("expected playStart to be re-anchored to now, got %v (before %v)", g.state.PlayStart(), before)
	}
}
