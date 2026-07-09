package ui

import "testing"

// This file is Task 2.3 (Phase 2, final task) of the parity-timing-sync
// plan. It stress-tests the timing gate (parityTimingCheck,
// game_parity_timing.go) added in Phase 2: real playback + rapid live edits
// must never false-panic, synthetic drift must always be caught, and a
// pause/resume transition must not leave the gate in a bad state.
//
// The gate is dormant-by-default under `go test` (see init() in
// game_parity_timing.go), so every test below arms it explicitly via
// enableParityTimingForTest (parity_test_helpers_test.go) rather than the
// raw g.parityTimingWatch/SetParityTimingFatal poke the original brief
// sketched — the helper additionally restores both to their prior values on
// cleanup, which the raw poke does not, and would otherwise leak
// panic-arming into every test that runs afterward in the same binary.

// loadStressCircuitForTest builds a multi-row playable circuit at the given
// BPM. Thin wrapper over the existing soak-test scene builder
// (buildSoakScene, soak_heap_bound_helper_test.go) — reused rather than
// duplicated per the brief's explicit instruction.
func loadStressCircuitForTest(t *testing.T, g *Game, rows, bpm int) {
	t.Helper()
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdiv 16: %v", err)
	}
	buildSoakScene(t, g, rows, 8)
	g.drum.SetBPM(bpm)
}

// cycleInstrumentForTest performs a live instrument-change edit on the given
// row — a real production mutation (DrumView.CycleInstrument, already used
// directly in parity_runtime_mutation_matrix_test.go) that routes through
// the mutation gate and bumps g.parityGen / grants a grace window
// (game_mutation_gate.go).
func cycleInstrumentForTest(g *Game, row int) {
	if g == nil || g.drum == nil || len(g.drum.Rows) == 0 {
		return
	}
	row %= len(g.drum.Rows)
	if row < 0 {
		row += len(g.drum.Rows)
	}
	g.drum.selRow = row
	g.drum.CycleInstrument()
}

// TestParityTimingStressRealPlaybackNoFalsePositive drives real,
// Update()-driven deterministic scheduling (advancePlaybackByAbs, the same
// helper TestParityDecisionsBounded soaks 5000 abs on) across 3000 abs steps
// on an 8-row/240bpm circuit, with periodic live edits (instrument cycle,
// mute/solo toggle) that bump parityGen and grant a grace window.
//
// What this exercises for real: under `go test`, g.SetPlayFunc(...) makes
// the scheduler take the direct-dispatch test path
// (game_sequencer_schedule.go: "runningUnderGoTest() && g.playFn != nil &&
// g.scheduleHook == nil"), which bypasses audioLoop entirely — so Stage B
// (bridge, ObserveBridge from game_audio_loop.go) and Stage C (overdue,
// Observe from game_audio_loop.go) never receive samples on this path; their
// windows stay under MinSamples and evalTimingStage no-ops them (see
// game_parity_timing_types.go). Stage D (emitdrift) is likewise desktop-only
// production instrumentation not wired into this path in Phase 2. Stage A
// (seqfire, ObserveSeqFireLate) DOES fire for real here: it's observed
// directly inside seqScheduleTime on every burst iteration while Playing(),
// computed from the real backdated-PlayStart wall-clock delta
// (advancePlaybackByAbs -> setPlayStartForAbs) versus each beat's ideal
// time. So this test is a genuine (non-trivial) no-false-positive proof for
// Stage A under real scheduling + live-edit churn; Stages B/C/D are only
// meaningfully exercised by TestParityTimingStressSyntheticFeedNoFalsePositive
// below.
func TestParityTimingStressRealPlaybackNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	const rows = 8
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	for i := 0; i < 3000; i++ {
		advancePlaybackByAbs(g, 1)
		switch {
		case i%250 == 0:
			cycleInstrumentForTest(g, i%rows)
		case i%400 == 0:
			g.drum.toggleMute((i / 400) % rows)
		case i%600 == 0:
			g.drum.toggleSolo(0)
		}
		g.refreshDrumRow() // drives parityScan + parityTimingCheck
	}
	// Reaching here without panic proves the gate does not false-positive
	// under sustained real scheduling + live-edit churn.
}

// TestParityTimingStressSyntheticFeedNoFalsePositive makes the
// no-false-positive claim non-trivial for ALL FOUR stages (not just Stage A,
// see the real-playback test above): it feeds the reservoirs directly with
// realistic healthy samples (identical magnitudes to
// TestParityTimingCheckNoPanicUnderBounds in parity_timing_check_test.go,
// the Task 2.2 no-panic baseline) every iteration across 4000 iterations,
// interleaved with the same live-edit churn as the real-playback test above.
// Because grace defaults to bypassed under `go test`
// (parityInGrace/parityGraceForceForTest, game_parity_state.go) unless a
// test explicitly opts in via SetParityGraceForceForTest, every one of the
// 4000 g.parityTimingCheck() calls below runs the real four-stage
// evaluation — none of them are masked by a post-edit grace window. This is
// the actual proof that live edits interleaved with healthy timing data
// never trip the gate.
func TestParityTimingStressSyntheticFeedNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	const rows = 8
	loadStressCircuitForTest(t, g, rows, 240)
	g.SetPlaying(true)

	for i := 0; i < 4000; i++ {
		g.schedMetrics.ObserveSeqFireLate(0.0008)
		g.schedMetrics.ObserveBridge(0.0005)
		// Advance audioNow so Stage C clears its 0.5s warmup and genuinely
		// evaluates healthy (lead +0.02s, never overdue) — a constant audioNow
		// would keep Stage C warmup-starved and inert (all four stages must
		// actually reach evaluation for the no-false-positive claim to hold).
		aNow := float64(i) * 0.01
		g.schedMetrics.Observe(aNow, aNow+0.02)
		g.schedMetrics.ObserveEmitDrift(0.001)
		switch {
		case i%250 == 0:
			cycleInstrumentForTest(g, i%rows)
		case i%400 == 0:
			g.drum.toggleMute((i / 400) % rows)
		case i%600 == 0:
			g.drum.toggleSolo(0)
		}
		g.parityTimingCheck()
	}
}

// TestParityTimingStressCatchesInjectedDrift proves the positive case for the
// statistical-mean branch on the fatal Stage C (overdue): a systematically-late
// lag stream (mean 0.001s > desktop Overdue.MeanMax 0.0005s, see
// runtime_profile.go) reliably panics once MinSamples (64) is reached, even
// though no single sample individually breaches AbsMax (0.020s) — i.e. a
// genuine statistical-mean catch, not a dropout-class absmax catch (mirrors
// evalTimingStage's check order in game_parity_timing_types.go). Uses overdue
// rather than emitdrift because emitdrift is WarnOnly post-2026-07 calibration
// (it would log, not panic); Stage C is the authoritative audible-lateness gate
// and stays fatal.
func TestParityTimingStressCatchesInjectedDrift(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	g.schedMetrics.Observe(0.01, 0.01) // prime startAt (non-zero: 0 is the "unset" sentinel); dropped by warmup
	for i := 0; i < 200; i++ {
		aNow := 1.0 + float64(i)*0.001
		g.schedMetrics.Observe(aNow, aNow-0.001) // lag 1ms > Overdue.MeanMax 0.5ms, < AbsMax 20ms
	}
	g.schedMetrics.ObserveEmitDrift(0.05) // emission also drifted late -> confirms the overdue is fatal
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })
}

// TestParityTimingStressPauseResumeNoFalsePositive validates the pause/
// resume path: while Paused, parityTimingCheck no-ops unconditionally
// (g.Paused() guard, game_parity_timing.go), but resuming can produce a
// scheduling catch-up burst (seqScheduleTime's burst-limited backlog drain)
// whose lateness readings must still land inside healthy Stage A bounds.
// Drives real playback before, across, and after the pause using
// SetPausedForTest (the same real entry point game_play_function_test.go
// uses), plus a live edit right after resume.
func TestParityTimingStressPauseResumeNoFalsePositive(t *testing.T) {
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlayFunc(func(string, float64, ...float64) {})

	const rows = 4
	loadStressCircuitForTest(t, g, rows, 240)

	g.SetPlaying(true)
	advanceFrames(g, 5)

	for i := 0; i < 800; i++ {
		advancePlaybackByAbs(g, 1)
		g.refreshDrumRow()
	}

	// Pause mid-stream: gate must stay silent regardless of wall-clock drift
	// accrued while paused.
	g.SetPausedForTest(true)
	t.Cleanup(func() { g.SetPausedForTest(false) })
	for i := 0; i < 50; i++ {
		advancePlaybackByAbs(g, 1)
		g.refreshDrumRow()
	}

	// Resume and keep driving, including a live edit right at the seam where
	// a catch-up burst is most likely.
	g.SetPausedForTest(false)
	for i := 0; i < 800; i++ {
		advancePlaybackByAbs(g, 1)
		if i == 0 || i%150 == 0 {
			cycleInstrumentForTest(g, i%rows)
		}
		g.refreshDrumRow()
	}
}
