package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestScrollGestureEndEmits(t *testing.T) {
	ch := make(chan string, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventScroll, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ScrollPayload); ok {
			ch <- p.Surface
		}
	})
	defer u()

	emitScroll("inst-menu")

	select {
	case s := <-ch:
		if s != "inst-menu" {
			t.Fatalf("surface %q", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no scroll event")
	}
}

// A committed scroll publishes exactly one EventScroll; a tap publishes none.
func TestScrollHandleTouchEndEmitsOnCommittedScroll(t *testing.T) {
	ch := make(chan string, 8)
	u := hooks.GlobalBus().Subscribe(hooks.EventScroll, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ScrollPayload); ok {
			ch <- p.Surface
		}
	})
	defer u()

	sb := NewScrollBehavior(ScrollbarStyle{}, 20)
	sb.Surface = "test-surface"
	// Give the vertical scroller enough content to scroll.
	sb.VS.Total = 50
	sb.VS.Visible = 10

	// Well past the 10 px dead zone.
	sb.HandleTouchBegin(100, 100)
	sb.HandleTouchMove(100, 130) // +30 px vertical — commits direction
	sb.HandleTouchEnd()

	select {
	case s := <-ch:
		if s != "test-surface" {
			t.Fatalf("committed scroll: got surface %q, want %q", s, "test-surface")
		}
	case <-time.After(time.Second):
		t.Fatal("committed scroll: no EventScroll received within 1s")
	}

	// Drain any extras (there should be none, but guard the tap case).
	drain := func() {
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
	drain()

	// --- Case 2: tap (begin+end, no movement) must NOT emit ---
	sb.HandleTouchBegin(100, 100)
	sb.HandleTouchEnd()

	select {
	case s := <-ch:
		t.Fatalf("tap: unexpected EventScroll with surface %q", s)
	case <-time.After(100 * time.Millisecond):
		// correct — no event
	}
}

// Nothing is emitted at a boundary, where ScrollBy is a no-op.
func TestScrollHandleWheelEmitsOnMove(t *testing.T) {
	ch := make(chan string, 8)
	u := hooks.GlobalBus().Subscribe(hooks.EventScroll, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ScrollPayload); ok {
			ch <- p.Surface
		}
	})
	defer u()

	sb := NewScrollBehavior(ScrollbarStyle{}, 20)
	sb.Surface = "wheel-surface"
	// Total=50, Visible=10 → max First=40; First starts at 0.
	sb.VS.Total = 50
	sb.VS.Visible = 10

	// Scroll down one step (steps<0 → ScrollBy(1) → First 0→1 → moved).
	sb.HandleWheel(-1)

	select {
	case s := <-ch:
		if s != "wheel-surface" {
			t.Fatalf("HandleWheel move: got surface %q, want wheel-surface", s)
		}
	case <-time.After(time.Second):
		t.Fatal("HandleWheel move: no EventScroll received")
	}

	// Pin to max and try to scroll further — ScrollBy is a no-op → no emit.
	sb.VS.First = sb.VS.Total - sb.VS.Visible // = 40 (max)
	sb.HandleWheel(-1)                        // ScrollBy(1) clamped → no change

	select {
	case s := <-ch:
		t.Fatalf("HandleWheel no-op: unexpected EventScroll with surface %q", s)
	case <-time.After(100 * time.Millisecond):
		// correct — no event
	}
}
