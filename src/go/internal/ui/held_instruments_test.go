package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestHeldSetFollowsTheRows(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 2)
	t.Cleanup(audio.ResetHeldInstruments)
	_ = g.Update()
	for _, id := range g.rowInstruments() {
		if !audio.ChannelHeld(channel.Of(id)) {
			t.Fatalf("%s plays on a row but is not held", id)
		}
	}
	if audio.ChannelHeld(channel.Of(roster.Cowbell)) {
		t.Fatal("an instrument no row plays is held")
	}
	g.drum.selRow = 1
	g.drum.SetInstrument(roster.Cowbell)
	_ = g.Update()
	if !audio.ChannelHeld(channel.Of(roster.Cowbell)) {
		t.Fatal("the row's new instrument is not held after the next frame")
	}
	if audio.ChannelHeld(channel.Of(roster.Kick)) {
		t.Fatal("the row's previous instrument is still held")
	}
}
