package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestNewEmitHelpersPublish verifies that emitRowVolume and emitSendChanged
// publish to the global bus and deliver typed payloads to subscribers.
// Uses the awaitEmit helper (defined in event_helpers_test.go) which
// subscribes, triggers, and waits up to 2s for async delivery.
func TestNewEmitHelpersPublish(t *testing.T) {
	e1 := awaitEmit(t, hooks.EventRowVolume, func() {
		emitRowVolume(2, 0.7)
	})
	p1, ok := e1.Payload.(hooks.RowChangePayload)
	if !ok || p1.Row != 2 || p1.Volume != 0.7 {
		t.Errorf("EventRowVolume payload = %+v (ok=%v); want row=2 volume=0.7", e1.Payload, ok)
	}

	e2 := awaitEmit(t, hooks.EventSendChanged, func() {
		emitSendChanged("kick", "delay", 0.3)
	})
	p2, ok := e2.Payload.(hooks.SendPayload)
	if !ok || p2.Channel != "kick" || p2.Kind != "delay" || p2.Value != 0.3 {
		t.Errorf("EventSendChanged payload = %+v (ok=%v); want channel=kick kind=delay value=0.3", e2.Payload, ok)
	}
}
