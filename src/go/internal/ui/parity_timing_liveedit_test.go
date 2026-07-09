package ui

import (
	"testing"
	"time"
)

// This file is the TDD repro + regression suite for C1 (final whole-branch
// review): the timing gate's grace/gen-filter suppression spine was applied
// to Stage D only, while Stages A/B/C's absMax gate statistic is a
// session-monotonic running max (schedule_metrics.go) that never resets in
// production. A live circuit/rule edit (or BPM re-anchor) during playback can
// stall scheduling long enough to latch a large Stage A/B/C spike; the grace
// window (bumpParityGen, game_mutation_gate.go) suppressed the CHECK
// (parityTimingCheck's parityInGrace() gate) but not the OBSERVATION, so the
// latched spike survives grace expiry and panics the NEXT parityTimingCheck
// even though the edit itself was completely benign. Design §3.4 of
// docs/superpowers/specs/2026-07-10-parity-timing-sync-design.md calls the
// stale-gen/grace filter "universal suppression across all stages"; this file
// proves it wasn't, and pins the fix (schedMetrics.ResetPeaks() wired into
// bumpParityGen + grace-guarded A/B observation call sites).

// TestParityTimingLiveEditLatchedSpikeClearedByEdit is the deterministic
// repro for fix part (b): a directly-latched Stage-C overdue spike (0.5s, far
// beyond desktop Overdue.AbsMax=0.020s) makes parityTimingCheck panic even
// though the "healthy" reservoir window is otherwise clean and well past
// MinSamples. This is exactly the shape of a post-edit catch-up burst: one
// huge absMax value latched into the session-monotonic running max.
//
// A real production live-edit (g.bumpParityGen, the single choke-point every
// structural mutation routes through per game_mutation_gate.go) must clear
// that latched peak so the NEXT check — which runs after grace has expired
// and therefore evaluates for real — does not inherit a pre-edit or
// during-edit spike that has nothing to do with post-edit audio health.
//
// Under `go test`, parityInGrace() returns false unless
// SetParityGraceForceForTest(true) is armed (game_parity_state.go), so the
// post-edit g.parityTimingCheck() call below is NOT masked by grace — it is
// a genuine four-stage evaluation. If it still panics, the latched peak
// survived the edit; the fix is schedMetrics.ResetPeaks() wired into
// bumpParityGen.
func TestParityTimingLiveEditLatchedSpikeClearedByEdit(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	// Feed a healthy window past MinSamples (64 desktop) for all four stages,
	// mirroring TestParityTimingStressSyntheticFeedNoFalsePositive's healthy
	// magnitudes, so the gate is actually exercising a populated window (not
	// a MinSamples no-op) when the spike lands.
	//
	// Stage C (overdue) additionally needs a realistic-sized population of
	// routine, tiny overdue "blips" (lag ~10us) BEFORE the injected spike.
	// ResetPeaks (by design/spec) clears only the session-monotonic MAX
	// fields, not the running sums/reservoirs that feed AvgLag/LagP99 — so
	// those statistics deliberately keep integrating across an edit. In a
	// real long-running session the overdue-lag population is large enough
	// that one rare edit-induced spike barely moves the mean/P99 (this is
	// exactly why the bug report calls out the absMax branch specifically,
	// "fires regardless of MinSamples", as the vector — not the mean/P99
	// branches). A too-small pre-spike population would make the single
	// spike dominate AvgLag/LagP99 after the reset and produce a spurious
	// mean/p99 breach that isn't representative of the real bug or the real
	// fix; 2000 tiny-lag samples keeps AvgLag/LagP99 comfortably under
	// desktop Overdue.MeanMax=0.0005s/P99Max=0.006s even with the 0.5s spike
	// mixed in (avg ≈ (2000*0.00001 + 0.5)/2001 ≈ 0.00026s).
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

	// Simulate the catch-up burst latching a huge Stage-C overdue spike: a
	// scheduling stall (edit held seqMu, or a BPM re-anchor rewound
	// seqNextIdxs) makes the audio bridge dispatch a beat 0.5s late.
	spikeNow := baseNow + 2000*0.001 + 1.0
	g.schedMetrics.Observe(spikeNow, spikeNow-0.5) // lead = -0.5s -> lag = 0.5s
	g.schedMetrics.ObserveEmitDrift(0.05)          // emission also drifted late -> confirmed audible lateness (fatal)

	snapBefore := g.schedMetrics.Snapshot()
	if snapBefore.MaxLag < 0.5 {
		t.Fatalf("setup: expected latched MaxLag>=0.5, got %v", snapBefore.MaxLag)
	}

	// RED: prove the bug exists. The latched spike alone (absMax dropout
	// class, fires regardless of MinSamples per evalTimingStage) must panic
	// parityTimingCheck right now, before any edit has happened.
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })

	// Now simulate a live structural edit — the real production entrypoint
	// every circuit/rule/BPM/etc. mutation routes through.
	g.bumpParityGen("test-edit", structuralMutationOptions{})

	// GREEN (post-fix): the edit's ResetPeaks() call clears the latched
	// Stage A/B/C/D peaks, so MaxLag reads back to 0 and the next check
	// (which, under `go test`, evaluates for real since grace is bypassed by
	// default) does not panic.
	snapAfter := g.schedMetrics.Snapshot()
	if snapAfter.MaxLag != 0 {
		t.Fatalf("expected MaxLag reset to 0 after live edit, got %v", snapAfter.MaxLag)
	}

	g.parityTimingCheck() // must NOT panic
}

// TestParityTimingBPMReanchorClearsLatchedSpike is the regression test for
// the SECOND schedMetrics.ResetPeaks() call site (C1 fix, fix part (b)'s
// sibling): the BPM re-anchor branch inside seqScheduleTime
// (game_sequencer_schedule.go:67-89, guarded by
// `g.Playing() && bpm != g.prevBPM && g.prevBPM > 0`). A live BPM change
// reaches this branch via the async applied-BPM path (bpmCh/bpmAck ->
// g.state.SetAppliedBPM, driven by bpmLoop) — NOT via bumpParityGen/the
// mutation gate — so it re-anchors seqNextIdxs from nextBeatIdxs (the same
// catch-up-burst shape a structural edit produces) and must clear the
// timing-gate's latched peaks itself rather than relying on
// TestParityTimingLiveEditLatchedSpikeClearedByEdit's bumpParityGen path.
//
// This test drives the REAL production entrypoint — g.seqScheduleTime()
// (game_sequencer_schedule.go:13), the exact function the audioLoop ticker
// (game_audio_loop.go:222) and sequencerLoop (game_sequencer_loop.go:34)
// call every tick, and the same function many other suites call directly to
// exercise scheduling deterministically (e.g. mute_node_audio_test.go,
// row_delete_playback_parity_test.go, pause_resume_sync_test.go) — by
// diverging g.state.AppliedBPM() from g.prevBPM while Playing() and then
// invoking it, rather than calling schedMetrics.ResetPeaks() directly.
func TestParityTimingBPMReanchorClearsLatchedSpike(t *testing.T) {
	g := newTestGameForParity(t)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	// Build a real circuit and warm up seqNextIdxs/nextBeatIdxs/seqPathSnap
	// via the same real, Update()-driven path
	// TestParityTimingGraceSuppressesStageAObservation uses — seqScheduleTime
	// early-returns before ever reaching the BPM branch if any of those are
	// unpopulated/length-mismatched. The parity timing gate is intentionally
	// NOT armed yet during warmup (armed below, after settling) so any
	// incidental Stage-A observation from cold-start scheduling can't panic
	// this setup phase.
	const rows = 4
	loadStressCircuitForTest(t, g, rows, 120)
	g.SetPlaying(true)
	advanceFrames(g, 5)

	// Pin a known-clean BPM baseline explicitly rather than relying on
	// whatever the warmup's async bpmCh/bpmAck exchange settled on, and pin
	// PlayStart/AudioStart to zero-value so (a) parityTimingCheck's
	// wall-clock warmup guard (schedWarmupSec=0.5s, game_parity_timing.go:115)
	// doesn't mask evaluation below, matching
	// TestParityTimingLiveEditLatchedSpikeClearedByEdit's technique of never
	// setting PlayStart, and (b) seqScheduleTime's own dtSec/target
	// computation (lines 90-104) stays near zero so the re-anchor's
	// legitimate small catch-up burst can't itself latch a fresh Stage-A
	// spike large enough to confuse the GREEN assertion below.
	const startBPM = 120
	g.SetAppliedBPMForTest(startBPM)
	g.prevBPM = startBPM
	g.SetPlayStartForTest(time.Time{})
	g.SetAudioStartForTest(0)

	// Clean slate: clear any peak noise the warmup's real scheduling left
	// behind so the controlled feed below is the only latched peak.
	g.schedMetrics.ResetPeaks()

	enableParityTimingForTest(t, g)

	// Feed a healthy window past MinSamples plus a latched huge overdue
	// spike, mirroring TestParityTimingLiveEditLatchedSpikeClearedByEdit's
	// proven-safe construction (identical magnitudes/rationale).
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

	// RED: prove the hazard exists before any re-anchor fires. The latched
	// spike alone must panic parityTimingCheck right now.
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })

	// Drive the REAL BPM re-anchor branch: diverge the applied BPM (the
	// value bpmLoop writes back via g.state.SetAppliedBPM once
	// g.engine.SetBPM finishes applying — SetAppliedBPMForTest calls the
	// identical setter, game_transport_helpers.go:55) from g.prevBPM while
	// Playing(), then call the exact production function
	// (game_sequencer_schedule.go:13) containing the branch. This is NOT a
	// direct schedMetrics.ResetPeaks() call.
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

	// GREEN (post-fix): the re-anchor's ResetPeaks() call clears the latched
	// Stage A/B/C/D peaks, so MaxLag reads back to 0.
	snapAfter := g.schedMetrics.Snapshot()
	if snapAfter.MaxLag != 0 {
		t.Fatalf("expected MaxLag reset to 0 after BPM re-anchor, got %v", snapAfter.MaxLag)
	}

	// Force a real (non-skipped) evaluation again: AnchorForBPMChange
	// unconditionally re-stamps PlayStart to "now" as part of the re-anchor
	// (internal/gamestate/state.go AnchorForBPMChange), which would
	// otherwise re-trip the wall-clock warmup guard and make the next
	// parityTimingCheck() a silent no-op rather than a genuine assertion.
	g.SetPlayStartForTest(time.Time{})

	g.parityTimingCheck() // must NOT panic
}

// TestParityTimingGraceSuppressesStageAObservation is the regression test
// for fix part (a): while a live edit's grace window is active
// (g.parityInGrace() == true), the Stage-A observation call site in
// seqScheduleTime (game_sequencer_schedule.go, guards
// g.schedMetrics.ObserveSeqFireLate) must be skipped entirely — not just the
// downstream check — so a scheduling catch-up burst produced BY the edit
// never enters the reservoir/peak in the first place. This drives real,
// Update()-driven scheduling (the same advancePlaybackByAbs path
// TestParityTimingStressRealPlaybackNoFalsePositive uses to exercise Stage A
// for real) with grace forced on for the whole drive, and asserts the
// SeqFire window does not move.
func TestParityTimingGraceSuppressesStageAObservation(t *testing.T) {
	g := newTestGameForParity(t)
	restore := SetParityGraceForceForTest(true)
	t.Cleanup(restore)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	const rows = 4
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	// Grant a grace window that comfortably outlives the drive below so
	// every real seqScheduleTime call along the way sees parityInGrace()==true.
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

	// Sanity: once grace lifts, the same real path resumes recording, proving
	// the zero counts above were caused by the guard, not by scheduling never
	// reaching the observation site at all.
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
