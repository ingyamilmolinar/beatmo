package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestSearchEmits(t *testing.T) {
	ch := make(chan hooks.SearchPayload, 4)
	u := hooks.GlobalBus().Subscribe(hooks.EventSearchChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.SearchPayload); ok {
			ch <- p
		}
	})
	defer u()

	emitSearchChanged("inst-menu", "kick")

	select {
	case p := <-ch:
		if p.Query != "kick" || p.Surface != "inst-menu" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no search event")
	}
}
