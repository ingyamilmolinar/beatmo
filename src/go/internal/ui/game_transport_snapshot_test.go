package ui

import (
	"testing"
)

func TestTransportSnapshotTracksState(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	snap := g.transportSnapshot()
	if snap.Playing || snap.Paused {
		t.Fatalf("unexpected playing=%v paused=%v", snap.Playing, snap.Paused)
	}

	g.SetPlaying(true)
	g.state.SetAppliedBPM(150)
	g.state.SetLastStep(32)
	g.state.SetPausedBeats(16)
	g.state.SetSeekFreezeFrames(3)

	snap = g.transportSnapshot()
	if !snap.Playing {
		t.Fatalf("expected playing state")
	}
	if snap.AppliedBPM != 150 {
		t.Fatalf("unexpected bpm %d", snap.AppliedBPM)
	}
	if snap.LastStep != 32 {
		t.Fatalf("unexpected last step %d", snap.LastStep)
	}
	if snap.PausedBeats != 16 {
		t.Fatalf("unexpected paused beats %d", snap.PausedBeats)
	}
	if snap.SeekFreezeFrames != 3 {
		t.Fatalf("unexpected freeze frames %d", snap.SeekFreezeFrames)
	}
}
