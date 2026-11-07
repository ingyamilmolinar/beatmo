package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestEmitInsertEffectMoved(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectMoved, func() {
		emitInsertEffectMoved(channel.Of(roster.Snare), 1, 0)
	})
	p, ok := e.Payload.(hooks.InsertEffectPayload)
	if !ok {
		t.Fatalf("payload type = %T; want InsertEffectPayload", e.Payload)
	}
	if p.Channel != channel.Of(roster.Snare) || p.FromSlot != 1 || p.ToSlot != 0 {
		t.Errorf("payload = %+v; want channel=snare from=1 to=0", p)
	}
}

func TestEmitInsertEffectToggled(t *testing.T) {
	e := awaitEmit(t, hooks.EventInsertEffectToggled, func() {
		emitInsertEffectToggled(channel.Of(roster.Kick), 2, false)
	})
	p, ok := e.Payload.(hooks.InsertEffectPayload)
	if !ok {
		t.Fatalf("payload type = %T; want InsertEffectPayload", e.Payload)
	}
	if p.Channel != channel.Of(roster.Kick) || p.Slot != 2 || p.Enabled != false {
		t.Errorf("payload = %+v; want channel=kick slot=2 enabled=false", p)
	}
}
