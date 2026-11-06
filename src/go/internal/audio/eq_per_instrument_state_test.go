package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Phase 5 contract: EQ state is per-channel. Setting EQ on one channel and
// then triggering an insert-chain rebuild on a DIFFERENT channel must not
// disturb the first channel's EQ.
//
// Pre-Phase-5, rebuildChannelProcessors read a process-wide lastSetEQ and
// gated on `lastSetEQ.ID == thisChannel`. Setting EQ on instrument A, then
// adding/removing an insert effect on instrument B, would either:
//   - lose A's EQ from the channel (rebuildChannelProcessors finds
//     lastSetEQ.ID != A and skips the EQ append), OR
//   - silently apply B's-most-recent EQ to A.
//
// Both modes are present in the pre-Phase-5 codebase depending on edit
// order. Per-channel storage makes the invariant structural.
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

	// Set EQ on instrument B. lastSetEQ now points at B (legacy compat),
	// but instA's per-channel record must be untouched.
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
