//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// A seek is a transport discontinuity, exactly like Stop→Play: everything the
// parity comparator recorded about the superseded pass (audio events, seq
// decisions, timing traces, dropped-highlight marks) describes cells that are
// about to be REPLAYED (backward seek) or never reached (forward seek). SeekSteps
// wiped the highlights (resetHighlights) but left the audio-event log in place,
// so every cell between a backward seek's target and the old playhead read
// "audio fired, no highlight" until the playhead re-passed it — the
// `[parity] [highlight_vs_audio] … src=refresh` storm the default circuit logs
// after every rewind / timeline go-to while playing (TestImportStopReplayClearsParity
// is the Stop→Play sibling of this guard).
//
// Every seek path a user has is driven through the real request queue +
// Game.Update (drainSeekRequest → SeekSteps), lane on (browser) and off (desktop
// legacy dispatch). The RecordedAt freshness grace is pinned 1 s ahead so the
// test sees what a real session sees: the scan lands long after the pre-seek
// events were stamped (the harness otherwise finishes inside 120 ms of wall time
// and the grace hides the stale events).
func TestSeekWhilePlayingLeavesNoStaleParity(t *testing.T) {
	type seekFn func(g *Game)
	seeks := []struct {
		name string
		do   seekFn
	}{
		// Transport "rewind" button: one whole beat back, beat-snapped.
		{"rewind", func(g *Game) { g.drum.RequestSeekDeltaBeats(-1, hooks.SeekTransport) }},
		// Timeline go-to (wheel / ribbon / counter): absolute target behind the playhead.
		{"goto-back", func(g *Game) {
			g.drum.RequestSeekSteps(g.playheadAbsSubdiv()-3*g.grid.MaxDiv()/2, hooks.SeekRibbon)
		}},
		// Fast-forward: must stay clean too (nothing replayed, old cells are past).
		{"fast-forward", func(g *Game) { g.drum.RequestSeekDeltaBeats(+1, hooks.SeekTransport) }},
		{"goto-forward", func(g *Game) {
			g.drum.RequestSeekSteps(g.playheadAbsSubdiv()+2*g.grid.MaxDiv(), hooks.SeekRibbon)
		}},
	}
	for _, lane := range []bool{true, false} {
		for _, sk := range seeks {
			name := sk.name + "/legacy"
			if lane {
				name = sk.name + "/lane"
			}
			t.Run(name, func(t *testing.T) {
				g := newSeekParityGame(t, lane)
				restore := SetParityRecordNowForTest(func() time.Time { return time.Now().Add(time.Second) })
				defer restore()

				laneRun(t, g, 2.0, 0.004)
				if got := g.ParityMismatchSnapshot(); len(got) != 0 {
					t.Fatalf("setup: parity already reports %d mismatches before any seek (first: %+v)", len(got), got[0])
				}
				if !parityHasAudio(g) {
					t.Fatal("setup: no parity audio events were recorded — the checker would be vacuous")
				}

				before := g.playheadAbsSubdiv()
				sk.do(g)
				laneRun(t, g, 0.004, 0.004) // the Update that drains the request
				after := g.playheadAbsSubdiv()
				if after == before {
					t.Fatalf("setup: seek did not move the playhead (%d)", before)
				}
				scans := g.parityScanCallsForTest
				laneRun(t, g, 1.0, 0.004)
				if g.parityScanCallsForTest == scans {
					t.Fatal("setup: parityScan never ran after the seek")
				}

				if got := g.ParityMismatchSnapshot(); len(got) != 0 {
					kinds := map[string]int{}
					for _, m := range got {
						kinds[m.Kind]++
					}
					t.Fatalf("seek %d→%d left stale parity state: %d mismatches %v (first: row=%d abs=%d kind=%s)",
						before, after, len(got), kinds, got[0].Row, got[0].Abs, got[0].Kind)
				}
			})
		}
	}
}

// newSeekParityGame is the lane test circuit (one looping row, fake audio clock)
// with parity in LOG mode so the test can read the ring instead of dying on the
// first panic, and the ring cleared of the play-start transient.
func newSeekParityGame(t *testing.T, lane bool) *Game {
	t.Helper()
	var g *Game
	if lane {
		g, _ = newLaneGame(t)
	} else {
		restore := SetRuntimeProfileForTest(browserRuntimeProfile())
		t.Cleanup(restore)
		g = newLaneCircuitGame(t)
	}
	g.parityWatch.Store(parityWatchLog)
	g.ClearParityMismatches()
	return g
}

func parityHasAudio(g *Game) bool {
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	return len(g.parityAudio) > 0
}

// The audio lane ticks at the END of a sequencer pass, so on the pass that
// first reaches a cell (play start; equally a seek or a new row) the UI lane
// gets there before the lane has committed it. The UI lane used to read the
// lane's ledger right then, find it empty, and record nothing: the first note
// of every playback stayed "audible but never enqueued" — an audio_missing for
// a note the engine was in fact playing — and never lit its trigger.
func TestLanePlayStartObservesTheFirstCell(t *testing.T) {
	g, sink := newLaneGame(t)
	g.parityWatch.Store(parityWatchLog)
	g.ClearParityMismatches()
	restore := SetParityRecordNowForTest(func() time.Time { return time.Now().Add(time.Second) })
	defer restore()

	laneRun(t, g, 0.004, 0.004) // the first pass: UI lane fires abs 0, then the lane ticks
	heard := false
	for _, n := range sink.scheduled() {
		if n.Row == 0 && n.Abs == 0 && n.Kind == laneSound {
			heard = true
		}
	}
	if !heard {
		t.Fatal("setup: the lane did not commit abs 0 — nothing to observe")
	}
	g.parityMu.Lock()
	dec, ok := g.paritySeqDecisions[0][0]
	recorded := false
	for _, ev := range g.parityAudio {
		if ev.Row == 0 && ev.Abs == 0 {
			recorded = true
		}
	}
	g.parityMu.Unlock()
	if !ok || !dec.Audible {
		t.Fatalf("setup: no audible decision for abs 0 (%+v, ok=%v)", dec, ok)
	}
	if !dec.Enqueued || !recorded {
		t.Fatalf("abs 0 is committed to the engine but parity observed enqueued=%v recorded=%v", dec.Enqueued, recorded)
	}
	laneRun(t, g, 0.2, 0.004)
	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" {
			t.Fatalf("audio_missing for row=%d abs=%d although the lane played it", m.Row, m.Abs)
		}
	}
}

// RewindTo moves a backward seek's superseded commits (abs >= target) into the
// timeline's cold ARCHIVE — retained, by the full-history rule — and the
// Service's lookup falls back to that archive. Parity read it through that
// lookup, so every not-yet-replayed cell after a rewind presented the ABANDONED
// pass's Playback commit as live immutable history. Without an edit the two
// passes agree and nothing shows; edit the circuit, then rewind, and the
// abandoned pass contradicts the predictor: timeline_vs_pred(_immutable) for
// every cell between the new playhead and the old one.
func TestBackwardSeekAfterEditDoesNotCompareAbandonedPass(t *testing.T) {
	for _, lane := range []bool{true, false} {
		name := "legacy"
		if lane {
			name = "lane"
		}
		t.Run(name, func(t *testing.T) {
			g := newSeekParityGame(t, lane)
			restore := SetParityRecordNowForTest(func() time.Time { return time.Now().Add(time.Second) })
			defer restore()

			laneRun(t, g, 2.0, 0.004)
			// Delete a node on the loop: from here the predictor plays a
			// different pattern than the pass the timeline recorded.
			var victim *uiNode
			for _, n := range g.nodes {
				if n != g.start && n.I == 6 {
					victim = n
				}
			}
			if victim == nil {
				t.Fatal("setup: no node at i=6")
			}
			g.deleteNodeInternal(victim, true)
			laneRun(t, g, 0.1, 0.004)
			g.ClearParityMismatches()

			div := g.grid.MaxDiv()
			before := g.playheadAbsSubdiv()
			target := before - 2*div
			played := map[int]bool{} // the pass the seek is about to abandon
			for abs := target; abs < before; abs++ {
				if v, _, k, ok := g.timelineCommittedWithKind(0, abs); ok && k == timeline.CommitKindPlayback {
					played[abs] = v
				}
			}
			g.drum.RequestSeekSteps(target, hooks.SeekRibbon) // timeline go-to, two beats back
			laneRun(t, g, 0.004, 0.004)
			if after := g.playheadAbsSubdiv(); after >= before {
				t.Fatalf("setup: seek did not go back (%d → %d)", before, after)
			}
			// Non-vacuity: the abandoned pass disagrees with what the predictor
			// now plays there — the comparison parity must NOT make.
			disagree := 0
			for abs, v := range played {
				if v != g.engine.Predictor.VisibleAt(0, abs) {
					disagree++
				}
			}
			if disagree == 0 {
				t.Fatal("setup: the edit did not make the abandoned pass disagree with the predictor — test is vacuous")
			}
			laneRun(t, g, 0.6, 0.004)
			for _, m := range g.ParityMismatchSnapshot() {
				t.Errorf("rewind after an edit: %s row=%d abs=%d expected=%v actual=%v detail=%s", m.Kind, m.Row, m.Abs, m.Expected, m.Actual, m.Detail)
			}
			// What the grid DRAWS for the replayed past: the played snapshot
			// froze the replay (the predictor's pass), not the abandoned one —
			// CrossingCell prefers a Playback commit, so an abandoned commit
			// answering the lookup was frozen into the past for good.
			tl := g.timelineService()
			wrong, checked := 0, 0
			for abs := target; abs < g.rowPastExclusive(0) && abs < before; abs++ {
				v, _, ok := tl.Played(0, abs)
				if !ok {
					continue
				}
				checked++
				if want, was := g.engine.Predictor.VisibleAt(0, abs), played[abs]; v != want && v == was {
					wrong++
				}
			}
			if checked == 0 {
				t.Fatal("setup: no replayed cell was frozen")
			}
			if wrong > 0 {
				t.Errorf("%d of %d replayed cells froze the ABANDONED pass into the drawn past", wrong, checked)
			}
		})
	}
}

// With the audio lane on, every note is committed to the engine up to the
// horizon ahead; the UI lane reaching a cell late moves a highlight, never a
// sound. Stage A ("seqfire": how late the sequencer fires a cell vs its grid
// time) therefore measures UI-lane lag, not audio timing — on WASM the
// sequencer shares the main thread, so one ~17 ms frame puts its p99 over the
// browser bound and the gate dumped "[PARITY] scheduler vs DrumView mismatch"
// by itself, mid-playback, with every audio stage clean. Audio timing in lane
// mode is the lane's own Stage C (lead at commit) and Stage D (start vs
// requested): those stay gated.
func TestLaneModeSeqFireIsNotAnAudioTimingBreach(t *testing.T) {
	for _, lane := range []bool{false, true} {
		g := newSeekParityGame(t, lane)
		enableParityTimingForTest(t, g)
		g.parityTimingWatch = parityWatchLog
		parityTimingFatalEnabled.Store(false)
		g.SetPlayStartForTest(time.Now().Add(-time.Minute)) // past the settle guard
		g.schedMetrics.Reset()
		clear(g.timingWarnedThisWindow)
		for i := 0; i < 200; i++ {
			g.schedMetrics.ObserveSeqFireLate(0.004)
		}
		g.schedMetrics.ObserveSeqFireLate(0.068) // one stalled frame: the user's absmax
		g.parityTimingCheck()
		_, breached := g.timingWarnedThisWindow["seqfire/absmax"]
		if !lane && !breached {
			t.Fatal("control: legacy dispatch must still gate Stage A (the injection never reached the gate)")
		}
		if lane && breached {
			t.Fatal("lane mode reported a UI-lane frame stall as an audio-timing breach")
		}
	}
}

// A main-thread stall longer than the audio lane's horizon (seen in the
// browser suite under load: ~32 frames drawn in ~32 s of playback). When the
// sequencer resumes, the UI lane fires the CURRENT cell (it never replays
// further back), but the lane walks only from now − LateToleranceSec: a note
// that late is dropped by policy, not played. Parity then saw "audible, never
// enqueued" — audio_missing — and, armed, panicked the WASM runtime. A policy
// drop is the lane's documented behaviour under starvation (the legacy path
// exempts its own downstream drops the same way); it must not read as a
// scheduler bug. Real misses (the lane SHOULD have committed and did not)
// stay reported: TestLanePlayStartObservesTheFirstCell.
func TestLaneStarvationLateDropIsNotAudioMissing(t *testing.T) {
	drops := uint64(0)
	for _, stall := range []float64{1.2, 1.5, 1.8, 3.0} {
		g, _ := newLaneGame(t) // parity armed (panic) by default
		// The browser case had 150 ms cells (4/beat at 100 BPM): the cell
		// just behind the playhead is far past the lane's 25 ms late
		// tolerance when a stall makes the lane discover it. Same shape
		// here with a slow tempo (32 cells/beat at 20 BPM ≈ 94 ms cells).
		g.SetPlayingForTest(false)
		g.drum.SetBPM(20)
		g.state.SetAppliedBPM(20)
		g.prevBPM = 20
		g.SetPlayingForTest(true)
		g.state.StartClock(time.Now(), *laneTestNow(g))
		restore := SetParityRecordNowForTest(func() time.Time { return time.Now().Add(time.Second) })
		laneRun(t, g, 1.0, 0.004)

		advanceLaneClock(laneTestNow(g), stall) // the stall: no Update for this long
		g.state.SetPlayStart(g.state.PlayStart().Add(-time.Duration(stall * float64(time.Second))))
		before := g.audioLane.Stats().DroppedLate
		// The sequencer ticker (a JS timer on WASM) fires before the next
		// frame's Update has moved the UI playhead: one pass with the stale
		// nextBeatIdxs floor, as in the browser.
		g.playbackSlate.seqScheduleTime()
		laneRun(t, g, 0.3, 0.004)
		restore()
		drops += g.audioLane.Stats().DroppedLate - before
		if got := g.ParityMismatchSnapshot(); len(got) != 0 {
			t.Fatalf("stall %.1fs: %d mismatches (first %s row=%d abs=%d)", stall, len(got), got[0].Kind, got[0].Row, got[0].Abs)
		}
	}
	if drops == 0 {
		t.Fatal("setup: no stall made the lane drop a note late — test is vacuous")
	}
}
