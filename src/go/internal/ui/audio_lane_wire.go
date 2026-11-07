package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (g *Game) audioLaneOn() bool { return g.audioLane != nil && g.laneOn.Load() }

// Under seqMu, at the end of every pass from every drive point.
func (s *playbackSlate) laneTick() {
	g := s.g
	if !g.audioLaneOn() || g.laneSrc == nil {
		return
	}
	// Play pressed while the audio clock read 0 left no anchor, and every lane note maps through it:
	// anchor now from the wall-clock position.
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

// On WASM the ticker is a JS timer, so an edit from the bridge followed by a stall would miss notes
// inside it. Non-blocking; a pending kick absorbs the next.
func (g *Game) kickLane() {
	if g.laneKick == nil || !g.audioLaneOn() {
		return
	}
	select {
	case g.laneKick <- struct{}{}:
	default:
	}
}

// Only the reconcile: kicks fire on every bridge call, and the UI lane keeps its own cadence.
func (g *Game) laneKickPass() {
	g.seqMu.Lock()
	defer g.seqMu.Unlock()
	g.playbackSlate.laneTick()
}

// Off under go test, where legacy tests assert synchronous dispatch, unless a test enables it.
func (g *Game) initAudioLane() {
	p := RuntimeProf()
	if g.laneWake == nil { // New allocates both before starting audioLoop / sequencerLoop
		g.laneWake = make(chan struct{}, 1)
		g.laneKick = make(chan struct{}, 1)
	}
	g.laneSink = &outboxSink{wake: g.laneWake}
	g.laneSrc = g.playbackSlate.newGameLaneSource()
	g.audioLane = newAudioLane(g.laneSrc, g.laneSink, laneConfig{
		Horizon: p.AudioHorizonSec, Guard: p.CancelGuardSec, LateTol: p.LateToleranceSec,
		FullReconcile: p.LaneFullReconcileSec, VoiceRecommitMin: p.LaneVoiceRecommitMinSec,
		VoiceNear: p.LaneVoiceNearSec,
	})
	g.laneOn.Store(p.AudioLaneEnabled && !runningUnderGoTest())
}

func (g *Game) resetAudioLane() {
	if g.audioLane == nil || !g.laneOn.Load() {
		return
	}
	g.seqMu.Lock()
	g.audioLane.Reset()
	g.seqMu.Unlock()
}

// Enqueued only if the lane holds a voice for it. On the pass that first reaches a cell the lane
// ticks afterwards, so defer to observePendingLaneCells instead of recording a false miss. seqMu held.
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

// A cell the lane dropped as late counts as enqueued (handed off, deliberately silent); one still
// uncommitted stays unobserved, a real miss. seqMu held.
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
