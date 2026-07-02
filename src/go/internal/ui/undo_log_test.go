package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestUndoEmitsEvent(t *testing.T) {
	ch := make(chan string, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventUndo, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UndoPayload); ok {
			ch <- "undo:" + p.Label
		}
	})
	defer u()
	r := hooks.GlobalBus().Subscribe(hooks.EventRedo, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.UndoPayload); ok {
			ch <- "redo:" + p.Label
		}
	})
	defer r()

	emitUndo("add node")
	emitRedo("add node")

	deadline := time.After(time.Second)
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case s := <-ch:
			seen[s] = true
		case <-deadline:
			t.Fatalf("missing events, saw %v", seen)
		}
	}
}
