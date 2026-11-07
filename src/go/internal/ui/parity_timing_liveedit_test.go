package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Stages A/B/C keep session-monotonic peaks: an edit's grace window suppressed the CHECK but not the
// OBSERVATION, so a spike latched during a benign edit panicked the next check. The edit resets the peaks
// (bumpParityGen) and the A/B call sites skip observing under grace.

// A latched Stage-C spike panics a check even over an otherwise clean window; bumpParityGen, which every
// structural mutation goes through, must clear it. Grace is off under go test unless forced, so the post-edit
// check is a real evaluation.
func TestParityTimingLiveEditLatchedSpikeClearedByEdit(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// A populated window, plus many tiny overdue blips: ResetPeaks clears only the max fields, so the running
	// mean/p99 keep integrating across an edit, and a small population would let the one spike dominate them after
	// the reset and fake a different breach.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.ObserveBridge(0.0005)
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02) // healthy: lead +20ms, never overdue
		g.schedMetrics.ObserveEmitDrift(0.001)
	}
	baseNow := 2.0 // past schedWarmupSec (0.5s) relative to m.startAt (set on first Observe above)
	for i := 0; i < 2000; i++ {
		aNow := baseNow + float64(i)*0.001
		g.schedMetrics.Observe(aNow, aNow-0.00001) // routine tiny overdue blip: lag=10us
	}

	// A stall (an edit holding seqMu, a BPM re-anchor) dispatches a beat 0.5 s late.
	spikeNow := baseNow + 2000*0.001 + 1.0
	g.schedMetrics.Observe(spikeNow, spikeNow-0.5) // lead = -0.5s -> lag = 0.5s
	g.schedMetrics.ObserveEmitDrift(0.05)          // emission also drifted late -> confirmed audible lateness (fatal)

	snapBefore := g.schedMetrics.Snapshot()
	if snapBefore.MaxLag < 0.5 {
		t.Fatalf("setup: expected latched MaxLag>=0.5, got %v", snapBefore.MaxLag)
	}

	// The latched spike alone panics before any edit.
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })

	// The real entry point every structural mutation routes through.
	g.bumpParityGen("test-edit", structuralMutationOptions{})

	snapAfter := g.schedMetrics.Snapshot()
	if snapAfter.MaxLag != 0 {
		t.Fatalf("expected MaxLag reset to 0 after live edit, got %v", snapAfter.MaxLag)
	}

	g.parityTimingCheck() // must NOT panic
}

// A live BPM change reaches seqScheduleTime's re-anchor through the async applied-BPM path, not the mutation
// gate, so that branch must clear the latched peaks itself. Driven through the real seqScheduleTime.
func TestParityTimingBPMReanchorClearsLatchedSpike(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	// seqScheduleTime returns before the BPM branch unless seqNextIdxs / nextBeatIdxs / seqPathSnap are populated;
	// the gate is armed only after warm-up so cold-start scheduling can't panic the setup.
	const rows = 4
	loadStressCircuitForTest(t, g, rows, 120)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	// A pinned baseline BPM, and zero PlayStart/AudioStart so the warm-up guard doesn't mask the check and the
	// re-anchor's own small catch-up can't latch a fresh spike.
	const startBPM = 120
	g.SetAppliedBPMForTest(startBPM)
	g.prevBPM = startBPM
	g.SetPlayStartForTest(time.Time{})
	g.SetAudioStartForTest(0)

	// Only the controlled feed below may leave a latched peak.
	g.schedMetrics.ResetPeaks()

	enableParityTimingForTest(t, g)

	// The same safe construction as the edit test above.
	for i := 0; i < 200; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.ObserveBridge(0.0005)
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02) // healthy: lead +20ms, never overdue
		g.schedMetrics.ObserveEmitDrift(0.001)
	}
	baseNow := 2.0 // past schedWarmupSec (0.5s) relative to m.startAt (set on first Observe above)
	for i := 0; i < 2000; i++ {
		aNow := baseNow + float64(i)*0.001
		g.schedMetrics.Observe(aNow, aNow-0.00001) // routine tiny overdue blip: lag=10us
	}
	spikeNow := baseNow + 2000*0.001 + 1.0
	g.schedMetrics.Observe(spikeNow, spikeNow-0.5) // lead = -0.5s -> lag = 0.5s
	g.schedMetrics.ObserveEmitDrift(0.05)          // emission also drifted late -> confirmed audible lateness (fatal)

	snapBefore := g.schedMetrics.Snapshot()
	if snapBefore.MaxLag < 0.5 {
		t.Fatalf("setup: expected latched MaxLag>=0.5, got %v", snapBefore.MaxLag)
	}

	// The latched spike alone panics before any re-anchor.
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })

	// The applied BPM diverges from prevBPM while playing, then the real function runs: not a direct ResetPeaks.
	const newBPM = startBPM + 40
	g.SetAppliedBPMForTest(newBPM)
	if !g.Playing() {
		t.Fatalf("setup: expected Playing()==true")
	}
	if bpm := g.state.AppliedBPM(); bpm == g.prevBPM || g.prevBPM <= 0 {
		t.Fatalf("setup: expected AppliedBPM()=%d to diverge from prevBPM=%d>0 before driving the branch", bpm, g.prevBPM)
	}

	g.seqScheduleTime() // the real re-anchor entrypoint

	if g.prevBPM != newBPM {
		t.Fatalf("expected seqScheduleTime's re-anchor branch to advance prevBPM to %d, got %d (branch did not fire)", newBPM, g.prevBPM)
	}

	snapAfter := g.schedMetrics.Snapshot()
	if snapAfter.MaxLag != 0 {
		t.Fatalf("expected MaxLag reset to 0 after BPM re-anchor, got %v", snapAfter.MaxLag)
	}

	// AnchorForBPMChange re-stamps PlayStart, which would re-trip the warm-up guard and make the next check a no-op.
	g.SetPlayStartForTest(time.Time{})

	g.parityTimingCheck() // must NOT panic
}

// Under grace the Stage-A observation itself is skipped, so an edit's catch-up burst never enters the reservoir.
func TestParityTimingGraceSuppressesStageAObservation(t *testing.T) {
	g := newTestGameForParity(t)
	restore := SetParityGraceForceForTest(true)
	t.Cleanup(restore)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) {})

	const rows = 4
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	// Outlives the whole drive, so every seqScheduleTime call sees grace.
	g.parityGraceUntilNS.Store(time.Now().Add(time.Hour).UnixNano())
	if !g.parityInGrace() {
		t.Fatalf("setup: expected parityInGrace()==true with forced grace + future deadline")
	}

	before := g.schedMetrics.Snapshot()
	for i := 0; i < 300; i++ {
		advancePlaybackByAbs(g, 1)
	}
	after := g.schedMetrics.Snapshot()

	if after.SeqFireCount != before.SeqFireCount {
		t.Fatalf("Stage A observation recorded during grace: SeqFireCount %d -> %d", before.SeqFireCount, after.SeqFireCount)
	}
	if after.SeqFireMax != before.SeqFireMax {
		t.Fatalf("Stage A max moved during grace: SeqFireMax %v -> %v", before.SeqFireMax, after.SeqFireMax)
	}

	// Once grace lifts the same path records again, so the zero counts came from the guard.
	restore() // pop the force-grace override
	g.parityGraceUntilNS.Store(0)
	for i := 0; i < 50; i++ {
		advancePlaybackByAbs(g, 1)
	}
	afterGrace := g.schedMetrics.Snapshot()
	if afterGrace.SeqFireCount == after.SeqFireCount {
		t.Fatalf("expected Stage A observation to resume once grace lifted, SeqFireCount stayed %d", afterGrace.SeqFireCount)
	}
}
