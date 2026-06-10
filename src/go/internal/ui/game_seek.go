package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func (g *Game) Seek(beats int) {
	if beats < 0 {
		beats = 0
	}
	emitSeek(beats)
	g.resetHighlights()
	g.activePulses = nil
	g.activePulse = nil
	// Convert beat count to internal subdivision steps
	steps := beats * g.grid.MaxDiv()
	trim := steps - 1
	rows := len(g.drum.Rows)
	if rows > 0 {
		if len(g.frozenUpToByRow) != rows {
			old := g.frozenUpToByRow
			g.frozenUpToByRow = make([]int, rows)
			for i := range g.frozenUpToByRow {
				g.frozenUpToByRow[i] = -1
			}
			copy(g.frozenUpToByRow, old)
		}
		newLimit := trim
		if newLimit < -1 {
			newLimit = -1
		}
		for i := 0; i < rows; i++ {
			if g.frozenUpToByRow[i] > newLimit {
				g.frozenUpToByRow[i] = newLimit
			}
		}
		for row := 0; row < rows; row++ {
			maxKeep := g.frozenUpToByRow[row]
			if maxKeep < newLimit {
				maxKeep = newLimit
			}
			g.timelineTrimAfterMutable(row, maxKeep)
		}
	}
	if g.Playing() {
		for row := range g.drum.Rows {
			g.spawnPulseFromRow(row, steps)
		}
	} else {
		for i := range g.nextBeatIdxs {
			g.nextBeatIdxs[i] = steps
		}
	}
	g.applySeekState(steps, g.Playing())
	g.resetOriginSequences()
	g.muteUntilByRow = make([]int, len(g.drum.Rows))
	// Reset sequencer counters to stay aligned with updated paths.
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
}

func (g *Game) applySeekState(steps int, anchorClocks bool) {
	g.elapsedBeats = steps
	div := max1(g.grid.MaxDiv())
	if div <= 0 {
		div = 1
	}
	beat := float64(steps) / float64(div)
	g.state.SetBeatBase(beat)
	g.state.SetLastBeat(beat)
	g.state.SetLastDisplayBeat(beat)
	g.state.SetLastProg(0)
	g.state.SetLastStep(steps)
	if g.Paused() || !g.Playing() {
		g.state.SetPausedBeats(steps)
	}
	if anchorClocks {
		now := time.Now()
		g.state.SetPlayStart(now)
		if n := audio.Now(); n > 0 {
			g.state.SetAudioStart(n)
		} else {
			g.state.SetAudioStart(0)
		}
		g.state.ClearJustResumed()
	}
}

func (g *Game) setPrimaryStep(step int) {
	if g.Playing() {
		g.elapsedBeats = step
		g.state.SetLastStep(step)
		return
	}
	g.applySeekState(step, false)
}

func (g *Game) updateDrumTracking() {
	if g.Playing() {
		if g.state.SeekFreezeFrames() > 0 {
			g.state.DecSeekFreezeFrames()
			return
		}
		// Single source of truth: feed TrackBeat the same canonical
		// playhead the mini-timeline cursor renders. Sampling separate
		// clocks here (g.elapsedBeats, LastDisplayBeat, nextBeatIdxs[0])
		// and MAX-ing them produced the symptom in screenshot.png — the
		// drum-view window drifted behind the cursor whenever wall-clock
		// interpolation got ahead of (or behind) the sequencer fires.
		g.drum.TrackBeat(g.playheadAbsSubdiv())
		return
	}
	if g.Paused() {
		g.applySeekState(g.state.PausedBeats(), false)
	}
}

// playheadAbsSubdiv is the canonical "current absolute subdivision being
// played" — the single source of truth shared by the mini-timeline cursor
// (which renders g.displayBeat()) and the drum-view auto-scroll (which
// consumes this integer). Both must derive from the same clock or the
// orange viewRect and the cursor visibly desync, exactly the bug the user
// reported in screenshot.png.
//
// Derivation: round(displayBeat() * div). displayBeat() is monotonic and
// already combines sequencer-tick progress with smooth wall/audio-clock
// interpolation, so the conversion to subdivisions inherits all of those
// invariants. The ±0.5-cell quantisation gap between the smooth cursor and
// this integer is the irreducible cost of drum.Offset being an int, and
// is well below TrackBeat's ±1-cell recenter dead-zone.
func (g *Game) playheadAbsSubdiv() int {
	div := max1(g.grid.MaxDiv())
	return int(math.Round(g.displayBeat() * float64(div)))
}

func (g *Game) handlePlaybackTransition(prevPlaying, prevPaused bool) {
	curr := g.Playing()
	if curr == prevPlaying {
		return
	}
	g.logger.Debugf("[game] playing state changed: %t -> %t", prevPlaying, curr)
	if curr {
		if !g.Paused() && len(g.activePulses) == 0 {
			for row := range g.drum.Rows {
				g.spawnPulseFromRow(row, g.nextBeatIdxs[row])
			}
			if g.activePulse == nil && len(g.activePulses) > 0 {
				g.activePulse = g.activePulses[0]
			}
		}
		g.engine.Start()
		g.state.SetPaused(false)
		g.logger.Debugf("[game] engine started")
	} else {
		g.engine.Stop()
		g.logger.Debugf("[game] engine stopped")
		for len(g.audioCh) > 0 {
			<-g.audioCh
		}
		g.clearParityState()
		if !g.Paused() {
			g.setPrimaryStep(0)
			g.activePulses = nil
			g.activePulse = nil
			g.resetHighlights()
		}
	}
	g.drum.SetPlaying(curr)
	notifyMediaSessionState()
	// Distinguish four narrative transitions so INFO reads naturally:
	//   prevPaused=true,  curr=true  → "resumed" (was paused, now playing)
	//   prevPaused=false, curr=true  → "play started" (fresh start)
	//   curr=false, currently paused → "paused" (suspended, not stopped)
	//   curr=false, not paused        → "play stopped"
	switch {
	case curr && prevPaused:
		hooks.PublishWithSource(hooks.EventResumed, nil, hooks.CaptureSource(0))
	case curr:
		hooks.PublishWithSource(hooks.EventPlayStart, nil, hooks.CaptureSource(0))
	case !curr && g.Paused():
		hooks.PublishWithSource(hooks.EventPaused, nil, hooks.CaptureSource(0))
	default:
		hooks.PublishWithSource(hooks.EventPlayStop, nil, hooks.CaptureSource(0))
	}
}
