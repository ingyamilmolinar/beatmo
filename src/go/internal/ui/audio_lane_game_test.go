//go:build test

package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// laneRun advances the fake audio clock in `step` increments for `secs`,
// running one real Game.Update per increment (the test path drives the
// sequencer — and with it the audio lane — from Update).
func laneRun(t *testing.T, g *Game, secs, step float64) {
	t.Helper()
	now := laneTestNow(g)
	for end := *now + secs; *now < end-1e-9; {
		advanceLaneClock(now, step)
		if err := g.Update(); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
}

// newLaneGame runs under the BROWSER profile (H = 1.0 s): the lane exists
// for the single-threaded WASM build, and the desktop horizon (0.25 s) is
// shorter than the stalls these tests inject.
func newLaneGame(t *testing.T) (*Game, *fakeSink) {
	t.Helper()
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := newLaneCircuitGame(t)
	sink := newFakeSink(laneTestNow(g))
	enableAudioLaneForTest(t, g, sink)
	return g, sink
}

// The whole point: while the main thread is stalled (no Update at all), the
// notes due during the stall are ALREADY committed to the engine.
func TestLaneStallToleranceGame(t *testing.T) {
	g, sink := newLaneGame(t)
	laneRun(t, g, 0.02, 0.004) // a few frames: the lane commits the horizon
	now := laneTestNow(g)
	stallFrom := *now
	advanceLaneClock(now, 0.6) // 600 ms stall: no Update
	g.laneSrc.refresh()
	c, ok := g.playbackSlate.laneContext()
	if !ok {
		t.Fatal("no lane context")
	}
	want := 0
	lo, hi, _ := g.laneSrc.Steps(stallFrom, *now)
	for abs := lo; abs <= hi; abs++ {
		if n, ok := g.playbackSlate.laneNoteAt(&c, 0, abs); ok && n.Kind == laneSound && n.Time > stallFrom+0.01 && n.Time <= *now {
			want++
		}
	}
	got := 0
	for _, n := range sink.heard() {
		if n.Time > stallFrom+0.01 && n.Time <= *now {
			got++
		}
	}
	if want == 0 {
		t.Fatal("setup: no notes due during the stall")
	}
	if got != want {
		t.Fatalf("notes due during a 600 ms stall: %d committed of %d", got, want)
	}
}

func TestLaneGameStopCancelsFuture(t *testing.T) {
	g, sink := newLaneGame(t)
	laneRun(t, g, 0.1, 0.004)
	if len(sink.sched) == 0 {
		t.Fatal("setup: nothing committed")
	}
	g.drum.stopPressed = true
	laneRun(t, g, 0.008, 0.004)
	if sink.all == 0 {
		t.Fatal("Stop must CancelAllFuture on the lane")
	}
	now := *laneTestNow(g)
	for _, n := range sink.sched {
		if n.Time > now {
			t.Fatalf("a future note survived Stop: %+v", n)
		}
	}
}

func TestLaneBPMChangeRecommitsAtNewTimes(t *testing.T) {
	g, sink := newLaneGame(t)
	laneRun(t, g, 0.1, 0.004)
	prev := g.state.AppliedBPM()
	if prev <= 0 {
		prev = g.drum.BPM()
	}
	g.drum.SetBPM(150)
	g.state.AnchorForBPMChange(prev, time.Now(), audio.Now())
	g.state.SetAppliedBPM(150)
	laneRun(t, g, 0.008, 0.004)
	now := *laneTestNow(g)
	c, _ := g.playbackSlate.laneContext()
	checked := 0
	for _, n := range sink.sched {
		if n.Time <= now+0.008 {
			continue
		}
		want := g.state.AudioTimeOfStep(n.Abs, c.div, 150) + c.offset
		if math.Abs(n.Time-want) > 1e-6 {
			t.Fatalf("note %+v kept an old-tempo time (want %.6f at 150 BPM)", n, want)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no future notes to check after the BPM change")
	}
}

func TestLaneModeSkipsLegacyDispatch(t *testing.T) {
	g, sink := newLaneGame(t)
	plays := 0
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) { plays++ })
	before := append([]int(nil), g.nextBeatIdxs...)
	laneRun(t, g, 1.0, 0.004)
	if plays != 0 {
		t.Fatalf("legacy dispatch still ran in lane mode: %d plays", plays)
	}
	if len(sink.sched) == 0 {
		t.Fatal("the lane committed nothing")
	}
	advanced := false
	for i := range g.nextBeatIdxs {
		if i >= len(before) || g.nextBeatIdxs[i] > before[i] {
			advanced = true
		}
	}
	if !advanced {
		t.Fatal("the UI lane (highlights / playhead) stopped advancing in lane mode")
	}
}

func TestLaneMuteNodeUsesStopAt(t *testing.T) {
	g, sink := newLaneGame(t)
	stops := 0
	audio.SetStopHook(func(instrument.ID) { stops++ })
	t.Cleanup(func() { audio.SetStopHook(nil) })
	laneRun(t, g, 1.5, 0.004)
	if len(sink.stopAts) == 0 {
		t.Fatal("the circuit's mute node never produced a lane StopAt")
	}
	for _, inst := range sink.stopAts {
		if instrument.ID(inst) != g.drum.Rows[0].Instrument {
			t.Fatalf("StopAt for %q, want the row instrument %q", inst, g.drum.Rows[0].Instrument)
		}
	}
	if stops != 0 {
		t.Fatalf("lane mode must not call the immediate audio.Stop (%d calls) — it would cut notes early", stops)
	}
}

// Trigger-driven UI (synth glow, sampler playhead) must light up when the
// note is DUE, not when the lane commits it up to the horizon early.
func TestLaneVoiceTriggerFiresAtGridTimeNotCommit(t *testing.T) {
	g, sink := newLaneGame(t)
	inst := g.drum.Rows[0].Instrument
	laneRun(t, g, 0.02, 0.004)
	var far laneNote
	for _, n := range sink.sched {
		if n.Time > *laneTestNow(g)+0.5 && (far.Inst == "" || n.Time < far.Time) {
			far = n
		}
	}
	if far.Inst == "" {
		t.Fatal("setup: nothing committed 0.5 s ahead")
	}
	stamp := audio.LastTriggerAt(inst)
	// Run up to just before the far note's grid moment: its trigger must not
	// have been stamped early by the commit.
	laneRun(t, g, far.Grid-*laneTestNow(g)-0.05, 0.004)
	early := audio.LastTriggerAt(inst)
	laneRun(t, g, 0.2, 0.004)
	if !audio.LastTriggerAt(inst).After(early) {
		t.Fatal("the UI lane never stamped the trigger when the beat came due")
	}
	_ = stamp
}

func TestLegacyPathUnchangedWhenLaneOff(t *testing.T) {
	g := newLaneCircuitGame(t) // lane NOT enabled: the default under go test
	if g.audioLaneOn() {
		t.Fatal("the lane must be off under go test unless a test enables it")
	}
	plays := make(chan struct{}, 8)
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) { plays <- struct{}{} })
	scheduleAbsForTest(g, 0)
	waitForChan(t, plays, 10000)
}

// A seek moves the abs↔time mapping without a new transport generation. Every
// note the lane committed under the OLD mapping must go: after the seek, each
// future committed voice must be exactly the note its (row, abs) makes under
// current state. Found by TestLaneHeardEqualsLegacyDispatched: the pre-seek
// timeline kept playing for a whole horizon, interleaved with the new one.
func TestLaneSeekCancelsTheOldTimeline(t *testing.T) {
	for _, beats := range []int{40, 0} { // forward far past the window; back to the start
		g, sink := newLaneGame(t)
		laneRun(t, g, 0.6, 0.004)
		g.Seek(beats)
		laneRun(t, g, 0.008, 0.004)
		now := *laneTestNow(g)
		c, ok := g.playbackSlate.laneContext()
		if !ok {
			t.Fatal("no lane context")
		}
		future := 0
		for vid, n := range sink.sched {
			if n.Time <= now+RuntimeProf().CancelGuardSec {
				continue
			}
			future++
			want, ok := g.playbackSlate.laneNoteAt(&c, n.Row, n.Abs)
			if !ok || want.Hash != n.Hash {
				t.Fatalf("seek to beat %d: vid %d (%+v) survived from the old timeline (now %.3f; current note ok=%v %+v)", beats, vid, n, now, ok, want)
			}
		}
		if future == 0 {
			t.Fatalf("seek to beat %d: nothing committed after the seek", beats)
		}
	}
}

// An edit made OUTSIDE Update (a JS-bridge call) must reach the lane at
// once: on WASM the sequencer ticker is a JS timer, so a UI stall right after
// the edit would hold the edit back from every note starting inside the
// stall (edit-storm e2e: stale notes up to 60 ms after their edit). A kick
// makes the sequencer goroutine runnable immediately, without the ticker.
func TestLaneKickTicksWithoutTheTicker(t *testing.T) {
	t.Setenv("SEQ_TICK_MS", "600000") // the ticker never fires during the test
	g, sink := newLaneGame(t)
	done := make(chan struct{})
	go func() { g.sequencerLoop(); close(done) }()
	t.Cleanup(func() { g.StopSequencerForTest(); <-done }) // idempotent: Close stops it again
	if len(sink.scheduled()) != 0 {
		t.Fatal("setup: the lane committed before any tick")
	}
	g.kickLane()
	deadline := time.Now().Add(2 * time.Second)
	for len(sink.scheduled()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if len(sink.scheduled()) == 0 {
		t.Fatal("a kick did not run the lane: edits wait for the next ticker tick")
	}
}

// A BACKWARD seek (and every undo/redo while playing: undoRestore re-imports
// and seeks back to the start of the current beat) replays abs values the lane
// already committed and played. Those old entries are history, not a reason
// to skip the cell: every replayed cell must be committed again.
func TestLaneBackwardSeekReplaysCells(t *testing.T) {
	for _, tc := range []struct {
		name     string
		played   float64
		seekBeat func(g *Game) int
	}{
		{"seek-to-start", 0.8, func(*Game) int { return 0 }},
		{"undo-to-beat-start", 2.4, func(g *Game) int { return g.playheadAbsSubdiv() / max1(g.grid.MaxDiv()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, sink := newLaneGame(t)
			laneRun(t, g, tc.played, 0.004)
			g.Seek(tc.seekBeat(g))
			from := *laneTestNow(g)
			laneRun(t, g, 0.7, 0.004)
			to := *laneTestNow(g)
			c, ok := g.playbackSlate.laneContext()
			if !ok {
				t.Fatal("no lane context")
			}
			g.laneSrc.refresh()
			lo, hi, _ := g.laneSrc.Steps(from, to)
			want := map[[2]int]bool{}
			for abs := lo; abs <= hi; abs++ {
				if n, ok := g.playbackSlate.laneNoteAt(&c, 0, abs); ok && n.Kind == laneSound && n.Time > from+0.03 && n.Time <= to {
					want[[2]int{0, abs}] = true
				}
			}
			got := 0
			for _, n := range sink.scheduled() {
				if n.Kind == laneSound && n.Time > from+0.03 && n.Time <= to && want[[2]int{n.Row, n.Abs}] {
					got++
				}
			}
			if len(want) == 0 {
				t.Fatal("setup: no notes due after the seek")
			}
			if got != len(want) {
				t.Fatalf("after a backward seek only %d of %d replayed notes were committed", got, len(want))
			}
		})
	}
}

// Play pressed while the audio clock still read 0 (no context yet / a context
// that has not rendered its first quantum — plausible when the first gesture
// IS Play) leaves no audio-clock anchor. The lane maps notes through that
// anchor, so it must anchor itself once the clock runs, not stay silent for
// the whole session.
func TestLaneAnchorsWhenPlayStartedBeforeTheAudioClock(t *testing.T) {
	g, sink := newLaneGame(t)
	g.state.SetAudioStartForTest(0)
	laneRun(t, g, 1.0, 0.004)
	if g.state.AudioStart() <= 0 {
		t.Fatal("still no audio-clock anchor after 1 s of play with a running clock")
	}
	if len(sink.scheduled()) == 0 {
		t.Fatal("the lane scheduled nothing: playback is silent without an audio-clock anchor")
	}
}
