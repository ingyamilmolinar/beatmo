package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

func TestChannelEQSnapshot(t *testing.T) {
	const channelID = "test.eqsnap"
	t.Cleanup(func() { ClearChannelProcessors(channel.Of(channelID)) })

	got := ChannelEQSnapshot(channel.Of(channelID))
	if got.ID != "" || got.SampleRate != 0 || len(got.Bands) != 0 {
		t.Errorf("empty snapshot: got %+v; want zero value", got)
	}

	SetChannelEQ(channel.Of(channelID), 48000,
		EQBand{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 3},
		EQBand{Kind: EQHighShelf, Freq: 8000, Q: 0.7, GainDB: -2},
	)
	got = ChannelEQSnapshot(channel.Of(channelID))
	if got.ID != channelID {
		t.Errorf("ID = %q; want %q", got.ID, channelID)
	}
	if got.SampleRate != 48000 {
		t.Errorf("SampleRate = %d; want 48000", got.SampleRate)
	}
	if len(got.Bands) != 2 {
		t.Fatalf("Bands length = %d; want 2", len(got.Bands))
	}
	if got.Bands[0].GainDB != 3 || got.Bands[1].GainDB != -2 {
		t.Errorf("Bands gains = %+v; want [3, -2]", got.Bands)
	}

	got.Bands[0].GainDB = 99
	again := ChannelEQSnapshot(channel.Of(channelID))
	if again.Bands[0].GainDB != 3 {
		t.Errorf("snapshot aliased internal slice: re-read gain = %v; want 3", again.Bands[0].GainDB)
	}
}
