package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// The single engine chokepoint for every navigation and undo restore. Caller holds seqMu.
func (g *Game) SeekSteps(steps int, origin hooks.SeekOrigin) {
	if steps < 0 {
		steps = 0
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	if maxSteps := g.drum.timelineBeats * div; maxSteps > 0 && steps > maxSteps {
		steps = maxSteps
	}
	emitSeek(steps/div, steps, origin)
	g.parityOnSeek()
	g.activePulses = nil
	g.activePulse = nil

	rows := len(g.drum.Rows)
	trim := steps - 1
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
		for row := 0; row < rows; row++ {
			// Archive superseded history and lower the freeze watermark.
			g.timelineRewindTo(row, steps)
			if g.frozenUpToByRow[row] > newLimit {
				g.frozenUpToByRow[row] = newLimit
			}
			maxKeep := g.frozenUpToByRow[row]
			if maxKeep < newLimit {
				maxKeep = newLimit
			}
			g.timelineTrimAfterMutable(row, maxKeep)
		}
	}

	// Rewind the predictor window to the seek target and re-simulate forward.
	if g.engine != nil && g.engine.Predictor != nil {
		// RebaseAt is O(steps), the dominant seek cost, and only needed below the evicted window; forward
		// and in-window seeks are already valid and Ensure(steps+1) suffices.
		if steps < g.engine.Predictor.WindowStart() {
			g.engine.Predictor.RebaseAt(steps)
		}
		g.engine.Predictor.Ensure(steps + 1)
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
	g.applySeekState(steps)
	g.resetOriginSequences()
	g.muteUntilByRow = make([]int, len(g.drum.Rows))
	// Reset sequencer counters to stay aligned with updated paths.
	g.seqNextIdxs = make([]int, len(g.drum.Rows))

	// With the lock on, a seek while STOPPED recenters the view (edge-triggered, so panning between
	// seeks stays free); while playing, updateDrumTracking already follows.
	if !g.Playing() {
		g.drum.TrackBeat(steps)
	}
}

// Takes seqMu itself (called after drum.Update released it): SeekSteps mutates state the
// sequencer guards with TryLock. timeline.mu and predictor.mu are never nested, seqMu outermost.
func (g *Game) drainSeekRequest() {
	value, mode, origin, ok := g.drum.TakeSeekRequest()
	if !ok {
		return
	}
	g.seqMu.Lock()
	defer g.seqMu.Unlock()
	div := max1(g.grid.MaxDiv())
	var steps int
	switch mode {
	case seekReqDeltaBeats:
		cur := g.playheadAbsSubdiv()
		snapped := int(math.Round(float64(cur)/float64(div))) * div // snap to whole beat first
		steps = snapped + value*div
	case seekReqDeltaSteps:
		steps = g.playheadAbsSubdiv() + value // no beat snap — subdivision granular
	default: // seekReqAbs
		steps = value
	}
	g.SeekSteps(steps, origin)
}

// Seek repositions by whole beats — the undo-restore entry point.
func (g *Game) Seek(beats int) {
	if beats < 0 {
		beats = 0
	}
	g.SeekSteps(beats*g.grid.MaxDiv(), hooks.SeekUndo)
}

func (g *Game) applySeekState(steps int) {
	g.elapsedBeats = steps
	div := max1(g.grid.MaxDiv())
	g.state.SeekTo(steps, div, time.Now(), audio.Now())
}

func (g *Game) setPrimaryStep(step int) {
	if g.Playing() {
		g.elapsedBeats = step
		g.state.SetLastStep(step)
		// Freeze what the passed cells show before anything later this frame changes live state.
		g.freezePlayedThroughPlayhead()
		return
	}
	g.applySeekState(step)
}

func (g *Game) updateDrumTracking() {
	if g.Playing() {
		if g.state.SeekFreezeFrames() > 0 {
			g.state.DecSeekFreezeFrames()
			return
		}
		// The same canonical playhead the ribbon cursor renders: mixing clocks made the view drift behind
		// the cursor.
		g.drum.TrackBeat(g.playheadAbsSubdiv())
		return
	}
	if g.Paused() {
		g.applySeekState(g.state.PausedBeats())
	}
}

// The one source shared by the ribbon cursor and the drum-view auto-scroll, so the two cannot
// desync. round(displayBeat() * div); the ±0.5-cell gap sits inside TrackBeat's ±1-cell dead zone.
func (g *Game) playheadAbsSubdiv() int {
	return g.PlayheadPosition().Steps
}

func (g *Game) handlePlaybackTransition(prevPlaying, prevPaused bool) {
	curr := g.Playing()
	if curr == prevPlaying {
		return
	}
	g.logger.Debugf("[game] playing state changed: %t -> %t", prevPlaying, curr)
	if curr {
		// Every FRESH start (not a resume) re-anchors all group rules at the session's starting position,
		// or a rule saved mid-session never ramps below its stale anchor next time. Runtime ramp state: no emit.
		if !prevPaused {
			g.graph.ReanchorAllGroupRules(g.playheadAbsSubdiv())
		}
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
