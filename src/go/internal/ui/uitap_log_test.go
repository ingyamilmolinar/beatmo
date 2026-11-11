package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Pressing a Button (press edge) emits exactly one EventUITap with its label.
func TestButtonPressEmitsUITap(t *testing.T) {
	ch := make(chan hooks.UITapPayload, 4)
	unsub := hooks.GlobalBus().Subscribe(hooks.EventUITap, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UITapPayload); ok {
			ch <- p
		}
	})
	defer unsub()

	b := NewButton("Play", ButtonStyle{}, func() {})
	b.ResetPress()
	b.PressFromTree(true)  // press edge → tap
	b.PressFromTree(false) // release → no second tap

	select {
	case p := <-ch:
		if p.Label != "Play" {
			t.Fatalf("got label %q", p.Label)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventUITap")
	}
	// Ensure release did not emit a second tap.
	select {
	case p := <-ch:
		t.Fatalf("unexpected second tap: %+v", p)
	case <-time.After(150 * time.Millisecond):
	}
}
