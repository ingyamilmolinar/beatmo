package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestViewModeEmits(t *testing.T) {
	ch := make(chan string, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventViewModeChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.ViewModePayload); ok {
			ch <- p.Mode
		}
	})
	defer u()

	emitViewMode("eq")

	select {
	case m := <-ch:
		if m != "eq" {
			t.Fatalf("mode %q", m)
		}
	case <-time.After(time.Second):
		t.Fatal("no view-mode event")
	}
}
