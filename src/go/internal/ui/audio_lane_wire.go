package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// audioLaneOn reports whether the audio lane (not the legacy grid-time
// dispatch) owns audio scheduling right now.
func (g *Game) audioLaneOn() bool { return g.audioLane != nil && g.laneOn.Load() }

// laneTick runs at the end of every seqScheduleTime pass, under seqMu (all
// drive points: the 4 ms sequencer goroutine, the frame-end drive, audioLoop).
func (s *playbackSlate) laneTick() {
	g := s.g
	if !g.audioLaneOn() || g.laneSrc == nil {
		return
	}
	// Play pressed while the audio clock still read 0 (no context yet, or one
	// that has not rendered its first quantum) left no audio-clock anchor, and
	// every lane note is mapped through it: anchor now, continuing from the
	// wall-clock position, instead of committing nothing for the session.
	if g.Playing() && !g.Paused() && g.state.AudioStart() <= 0 {
		if an := audio.Now(); an > 0 {
			bpm := g.state.AppliedBPM()
			if bpm <= 0 {
				bpm = g.bpm
			}
			g.state.AnchorForBPMChange(bpm, time.Now(), an)
		}
	}
	g.laneSrc.refresh()
	g.audioLane.Tick()
}

// kickLane makes the sequencer goroutine run a pass NOW instead of at its next
// ticker tick: on WASM the ticker is a JS timer, so an edit made outside
// Update (a JS-bridge call) followed by a main-thread stall would otherwise
// miss every note starting inside the stall. Non-blocking; a pending kick
// absorbs the next one.
func (g *Game) kickLane() {
	if g.laneKick == nil || !g.audioLaneOn() {
		return
	}
	select {
	case g.laneKick <- struct{}{}:
	default:
	}
}

// laneKickPass is what a kick runs: the audio-lane reconcile alone (under
// seqMu, like every laneTick), not a whole sequencer pass — a kick fires on
// every bridge call, and the UI lane / parity work of seqScheduleTime keeps
// its own cadence.
func (g *Game) laneKickPass() {
	g.seqMu.Lock()
	defer g.seqMu.Unlock()
	g.laneTick()
}

// initAudioLane builds the lane from the RuntimeProfile. Off under go test
// (legacy tests assert synchronous playFn dispatch) unless a test enables it.
func (g *Game) initAudioLane() {
	p := RuntimeProf()
	if g.laneWake == nil { // New allocates both before starting audioLoop / sequencerLoop
		g.laneWake = make(chan struct{}, 1)
		g.laneKick = make(chan struct{}, 1)
	}
	g.laneSink = &outboxSink{wake: g.laneWake}
	g.laneSrc = g.newGameLaneSource()
	g.audioLane = newAudioLane(g.laneSrc, g.laneSink, laneConfig{
		Horizon: p.AudioHorizonSec, Guard: p.CancelGuardSec, LateTol: p.LateToleranceSec,
		FullReconcile: p.LaneFullReconcileSec, VoiceRecommitMin: p.LaneVoiceRecommitMinSec,
		VoiceNear: p.LaneVoiceNearSec,
	})
	g.laneOn.Store(p.AudioLaneEnabled && !runningUnderGoTest())
}

// resetAudioLane cancels every not-yet-started lane voice (Stop / Pause /
// media pause / Close).
func (g *Game) resetAudioLane() {
	if g.audioLane == nil || !g.laneOn.Load() {
		return
	}
	g.seqMu.Lock()
	g.audioLane.Reset()
	g.seqMu.Unlock()
}

// observeLaneCell is the UI lane's parity/visual observation of an audible
// cell it just passed: the lane is the source of truth for what was committed
// to the engine, so the cell counts as enqueued only if the lane holds a voice
// for it. The lane ticks at the END of the sequencer pass, so on the pass that
// first reaches a cell the lane has not walked yet (play start, a seek, a new
// row) the ledger is legitimately still empty: defer those to
// observePendingLaneCells instead of recording "never enqueued" — reading the
// ledger one tick early made the first note of every playback an
// audio_missing (and skipped its trigger light-up). seqMu held.
func (g *Game) observeLaneCell(row, idx int) {
	if g.observeLaneCellNow(row, idx) {
		return
	}
	g.lanePendingObs = append(g.lanePendingObs, [2]int{row, idx})
}

func (g *Game) observeLaneCellNow(row, idx int) bool {
	n, ok := g.audioLane.Committed(row, idx)
	if !ok || n.Kind != laneSound {
		return false
	}
	g.markSeqAudioEnqueued(row, idx)
	g.recordParityAudio(row, idx, n.Time, n.Inst, n.Vol, n.Pitch, n.Dur, g.audioGen.Load(), g.parityGen.Load())
	audio.RecordVoiceTrigger(n.Inst) // trigger-driven UI lights up on the beat, not at commit
	return true
}

// observePendingLaneCells re-observes, after this pass's laneTick, the cells
// observeLaneCell deferred. A cell the lane DROPPED by its late policy (a
// main-thread stall longer than the horizon made it reach the cell past
// LateToleranceSec) was handed off and deliberately not played: it counts as
// enqueued — the legacy path exempts its own downstream drops the same way —
// and nothing is recorded as heard. One that is STILL uncommitted stays
// unobserved: a genuine miss parity must report. seqMu held (deferred in
// seqScheduleTime).
func (g *Game) observePendingLaneCells() {
	if len(g.lanePendingObs) == 0 {
		return
	}
	if g.audioLaneOn() {
		for _, c := range g.lanePendingObs {
			if g.observeLaneCellNow(c[0], c[1]) {
				continue
			}
			if g.audioLane.DroppedLate(c[0], c[1]) {
				g.markSeqAudioEnqueued(c[0], c[1])
			}
		}
	}
	g.lanePendingObs = g.lanePendingObs[:0]
}
