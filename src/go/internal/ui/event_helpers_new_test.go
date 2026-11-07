package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Typed payloads reach subscribers, waited for up to 2 s.
func TestNewEmitHelpersPublish(t *testing.T) {
	e1 := awaitEmit(t, hooks.EventRowVolume, func() {
		emitRowVolume(2, 0.7)
	})
	p1, ok := e1.Payload.(hooks.RowChangePayload)
	if !ok || p1.Row != 2 || p1.Volume != 0.7 {
		t.Errorf("EventRowVolume payload = %+v (ok=%v); want row=2 volume=0.7", e1.Payload, ok)
	}
}
