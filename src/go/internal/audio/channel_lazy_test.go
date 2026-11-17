package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Spec 2026-10-31 §1.2: boot, reset, registration and reads build no channel;
// the first use does.
func TestResetAndRegisterCreateNoInstrumentChannel(t *testing.T) {
	withDefaultAudio(t)
	ResetInstruments()
	t.Cleanup(ResetInstruments)
	if ids := InstrumentChannelIDs(); len(ids) != 0 {
		t.Fatalf("reset built %d instrument channels, want none", len(ids))
	}
	id := instrument.ID("lazy-channel-probe")
	Register(id, nil)
	t.Cleanup(func() { Unregister(id) })
	_ = ChannelVolume(channel.Of(id))
	if ids := InstrumentChannelIDs(); len(ids) != 0 {
		t.Fatalf("registering / reading built channels %v, want none", ids)
	}
	_ = channelForInstrument(channel.Of(id)) // a voice mixes through it
	if ids := InstrumentChannelIDs(); !slices.Contains(ids, channel.Of(id)) {
		t.Fatalf("first use built no channel: %v", ids)
	}
}
