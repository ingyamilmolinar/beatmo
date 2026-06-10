package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestEmitInsertEffectMoved verifies that emitInsertEffectMoved publishes
// EventInsertEffectMoved with the correct channel/from/to payload.
// This is the emit helper wired at the desktop (drumview_fx_panel.go)
// move-up and move-down OnClick sites.
func TestEmitInsertEffectMoved(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectMoved, func() {
		emitInsertEffectMoved("snare", 1, 0)
	})
	p, ok := e.Payload.(hooks.InsertEffectPayload)
	if !ok {
		t.Fatalf("payload type = %T; want InsertEffectPayload", e.Payload)
	}
	if p.Channel != "snare" || p.FromSlot != 1 || p.ToSlot != 0 {
		t.Errorf("payload = %+v; want channel=snare from=1 to=0", p)
	}
}

// TestEmitInsertEffectToggled verifies that emitInsertEffectToggled publishes
// EventInsertEffectToggled with the correct channel/slot/enabled payload.
// This is the emit helper wired at the desktop (drumview_fx_panel.go)
// toggle OnClick site.
func TestEmitInsertEffectToggled(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectToggled, func() {
		emitInsertEffectToggled("kick", 2, false)
	})
	p, ok := e.Payload.(hooks.InsertEffectPayload)
	if !ok {
		t.Fatalf("payload type = %T; want InsertEffectPayload", e.Payload)
	}
	if p.Channel != "kick" || p.Slot != 2 || p.Enabled != false {
		t.Errorf("payload = %+v; want channel=kick slot=2 enabled=false", p)
	}
}
