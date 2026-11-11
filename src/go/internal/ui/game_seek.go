package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// SeekSteps repositions the playhead to absolute subdivision `steps`. It is
// the single engine chokepoint for all user navigation (ribbon, FF/RW,
// counter, keyboard, JS) and for undo restore. Must be called with seqMu held.
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
		// RebaseAt re-walks [0, steps) per row to rebuild the position-dependent
		// context maps — O(steps) and the dominant seek cost (see
		// bench-results/seek_navigation_bench.md). It is only REQUIRED for a
		// backward seek below the evicted window (steps < WindowStart), where the
		// buffers were dropped and must be reconstructed. For a forward or
		// in-window seek the buffers [WindowStart, WindowEnd) are already valid
		// and Ensure(steps+1) — which extends from WindowEnd using context that
		// is already correct at the frontier — suffices. Gating removes the
		// O(steps) re-walk from forward seeks.
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

	// Tracking lock: when the drum-view lock (FollowPlayback) is on, a seek/jump
	// while STOPPED recenters the view on the new position so it follows the
	// playback pointer. This is edge-triggered by the pointer change (not a
	// per-frame lock), so manual panning between seeks stays free. During
	// playback the per-frame updateDrumTracking already follows the playhead.
	// TrackBeat self-gates on FollowPlayback(), so this no-ops when the lock is
	// off.
	if !g.Playing() {
		g.drum.TrackBeat(steps)
	}
}

// drainSeekRequest applies any queued DrumView seek request. It is called from
// Game.Update AFTER the drum.Update seqMu window has been released, so it
// acquires seqMu itself: SeekSteps mutates seqNextIdxs / timeline / predictor
// state shared with the background sequencer goroutine (which guards those with
// seqMu.TryLock in seqScheduleTime). Holding seqMu here excludes that tick for
// the duration of the seek. SeekSteps does not take seqMu, so there is no
// re-entrant deadlock; within it timeline.mu and predictor.mu are each taken
// and released independently (never nested with each other), with seqMu as the
// outer lock — the same shape the sequencer uses, so no ordering conflict.
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
		// The playhead just moved: freeze what the passed cells show now,
		// before anything later in the frame can change live state.
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
		g.applySeekState(g.state.PausedBeats())
	}
}

// playheadAbsSubdiv is the canonical "current absolute subdivision being
// played" — the single source of truth shared by the mini-timeline cursor
// (which renders g.displayBeat()) and the drum-view auto-scroll (which
// consumes this integer). Both must derive from the same clock or the
// orange viewRect and the cursor visibly desync, exactly the bug the user
// reported in screenshot.png.
//
// Derivation: PlayheadPosition().Steps == round(displayBeat() * div).
// displayBeat() is monotonic and already combines sequencer-tick progress with
// smooth wall/audio-clock interpolation, so the conversion to subdivisions
// inherits all of those invariants. The ±0.5-cell quantisation gap between the
// smooth cursor and this integer is the irreducible cost of drum.Offset being
// an int, and is well below TrackBeat's ±1-cell recenter dead-zone.
//
// Routed through PlayheadPosition so the step the seek path consumes and the
// beat/second the readouts print come from the same resolved clock sample.
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
		// Session-relative anchors (task-rule-replay): every FRESH playback
		// start — stop->play, NOT resume-from-pause (prevPaused distinguishes
		// the two; a resume must keep the mid-session anchor so the
		// task-rule-continuity in-place-edit guarantee holds) — re-anchors
		// every group rule to the session's starting absolute subdivision
		// index (0 after a stop, or the seek position when playback starts
		// from a seek — playheadAbsSubdiv is the same clock ruleAnchorIdxNow
		// reads, sampled here BEFORE any scheduling advance this frame).
		// Without this, a rule saved mid-session N (anchored well after abs
		// 0) silently fails to ramp for the entire region below that stale
		// anchor on session N+1, since GroupEffect's anchor=min(anchor,absIdx)
		// collapse forces steps=0 until the playhead re-passes it. This is
		// the single chokepoint shared by the transport play button and the
		// JS startPlay export (both set dv.playPressed, consumed by
		// Game.Update's PlayPressed() handling, which runs earlier in the
		// same Update() call that ends with this transition check). No
		// emits/undo: this is runtime ramp state, not a document edit — see
		// ReanchorAllGroupRules' doc comment.
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
