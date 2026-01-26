package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func (g *Game) Seek(beats int) {
	if beats < 0 {
		beats = 0
	}
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
		cur := g.elapsedBeats
		if div := g.grid.MaxDiv(); div > 0 {
			display := int(math.Round(g.state.LastDisplayBeat() * float64(div)))
			if display > cur {
				cur = display
			}
		}
		if len(g.nextBeatIdxs) > 0 {
			nb := g.nextBeatIdxs[0] - 1
			if nb < 0 {
				nb = 0
			}
			if nb > cur {
				cur = nb
			}
		}
		g.drum.TrackBeat(cur)
		return
	}
	if g.Paused() {
		g.applySeekState(g.state.PausedBeats(), false)
	}
}

func (g *Game) handlePlaybackTransition(prevPlaying bool) {
	curr := g.Playing()
	if curr == prevPlaying {
		return
	}
	g.logger.Infof("[GAME] Playing state changed: %t -> %t", prevPlaying, curr)
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
		g.logger.Infof("[GAME] Engine started.")
	} else {
		g.engine.Stop()
		g.logger.Infof("[GAME] Engine stopped.")
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
}
