//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

// A seek is a transport discontinuity like Stop→Play: what parity recorded about the superseded pass describes
// cells about to be replayed or never reached. Every user seek runs through drainSeekRequest, lane on and off,
// with the freshness grace pinned ahead so the pre-seek events read as old, as in a real session.
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

// LOG mode so the ring can be read instead of dying on the first panic; the play-start transient is cleared.
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
	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()
	return g
}

func parityHasAudio(g *Game) bool {
	g.parityMu.Lock()
	defer g.parityMu.Unlock()
	return len(g.parityAudio) > 0
}

// The lane ticks at the END of a pass, so the UI lane reaches a new cell before the lane commits it; it must still
// observe the note, or the first note of every playback reads as audio_missing and never lights.
func TestLanePlayStartObservesTheFirstCell(t *testing.T) {
	g, sink := newLaneGame(t)
	g.parityWatch = parityWatchLog
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

// RewindTo retains a backward seek's superseded commits, and a lookup that falls back to them presents the
// ABANDONED pass as live history: after an edit it contradicts the predictor for every cell up to the old playhead.
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
			// The predictor now plays a different pattern than the recorded pass.
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
			// Non-vacuity: the abandoned pass disagrees with what the predictor plays there.
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
			// The drawn past must freeze the replay, not the abandoned pass: CrossingCell prefers a Playback commit.
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

// With the lane on, a late UI lane moves a highlight, never a sound, so Stage A measures UI lag, not audio; one
// WASM frame would breach it. The lane's own Stage C and D stay gated.
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

// After a stall longer than the horizon the lane walks only from now − LateToleranceSec, dropping older notes by
// policy; that drop is not audio_missing. Real misses stay reported.
func TestLaneStarvationLateDropIsNotAudioMissing(t *testing.T) {
	drops := uint64(0)
	for _, stall := range []float64{1.2, 1.5, 1.8, 3.0} {
		g, _ := newLaneGame(t) // parity armed (panic) by default
		// A slow tempo makes the cell behind the playhead far older than the late tolerance when the lane finds it.
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
		// The sequencer timer fires before the next Update moves the UI playhead: one pass on the stale floor.
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
