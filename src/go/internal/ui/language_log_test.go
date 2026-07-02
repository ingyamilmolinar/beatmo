package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestLanguageChangeEmitsEvent(t *testing.T) {
	got := make(chan hooks.LanguagePayload, 1)
	unsub := hooks.GlobalBus().Subscribe(hooks.EventLanguageChanged, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.LanguagePayload); ok {
			got <- p
		}
	})
	defer unsub()

	emitLanguageChanged("en", "es")

	select {
	case p := <-got:
		if p.Old != "en" || p.New != "es" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		// hooks.Bus is async; poll briefly.
		t.Fatal("no EventLanguageChanged delivered")
	}
}
