package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// EQ state is per channel: an insert-chain rebuild on another channel must not touch it. A process-wide "last EQ"
// either dropped A's EQ or applied B's to it.
func TestEQPerInstrumentIsolation(t *testing.T) {
	// Set EQ on instrument A.
	bandsA := []EQBand{{Kind: EQPeaking, Freq: 500, Q: 1.0, GainDB: 6}}
	SetChannelEQ(channel.Of("instA"), 44100, bandsA...)

	// Verify it's recorded for A and only A.
	gotA := lookupChannelEQ(channel.Of("instA"))
	if gotA.ID != channel.Of("instA") || len(gotA.Bands) != 1 || gotA.Bands[0].GainDB != 6 {
		t.Fatalf("EQ for instA not recorded correctly: %+v", gotA)
	}
	gotB := lookupChannelEQ(channel.Of("instB"))
	if !gotB.ID.IsZero() || len(gotB.Bands) != 0 {
		t.Fatalf("instB has phantom EQ: %+v", gotB)
	}

	// A's per-channel record must be untouched.
	bandsB := []EQBand{{Kind: EQLowShelf, Freq: 200, Q: 0.7, GainDB: -3}}
	SetChannelEQ(channel.Of("instB"), 44100, bandsB...)

	gotA = lookupChannelEQ(channel.Of("instA"))
	if gotA.ID != channel.Of("instA") || len(gotA.Bands) != 1 || gotA.Bands[0].GainDB != 6 {
		t.Fatalf("instA's EQ leaked after instB SetChannelEQ: %+v", gotA)
	}
	gotB = lookupChannelEQ(channel.Of("instB"))
	if gotB.ID != channel.Of("instB") || len(gotB.Bands) != 1 || gotB.Bands[0].GainDB != -3 {
		t.Fatalf("instB's EQ not recorded: %+v", gotB)
	}

	// Clear instA's EQ — instB must survive.
	ClearChannelProcessors(channel.Of("instA"))
	gotA = lookupChannelEQ(channel.Of("instA"))
	if len(gotA.Bands) != 0 {
		t.Errorf("ClearChannelProcessors(instA) did not clear: %+v", gotA)
	}
	gotB = lookupChannelEQ(channel.Of("instB"))
	if len(gotB.Bands) != 1 || gotB.Bands[0].GainDB != -3 {
		t.Errorf("ClearChannelProcessors(instA) damaged instB: %+v", gotB)
	}

	// Cleanup.
	ClearChannelProcessors(channel.Of("instB"))
}
