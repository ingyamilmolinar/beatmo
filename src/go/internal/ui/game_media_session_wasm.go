//go:build js && !test

package ui

import (
	"math"
	"syscall/js"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/gamestate"
)

func notifyMediaSessionState() {
	fn := js.Global().Get("updateMediaSessionState")
	if fn.Truthy() {
		fn.Invoke()
	}
}

// These change state directly, not via flags: requestAnimationFrame does not fire while the
// browser is backgrounded.
func (g *Game) initMediaSessionExports() {
	js.Global().Set("mediaSessionPause", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if !g.Playing() {
			return nil
		}
		g.logger.Debugf("[game] mediaSessionPause")

		// Bump audio generation so the sequencer drops stale events.
		g.audioGen.Add(1)
		g.resetAudioLane() // cancel every lane voice not started yet

		// Drain pending audio events.
		for {
			select {
			case <-g.audioCh:
			default:
				goto drained
			}
		}
	drained:

		// Pause at current position (preserves elapsedBeats for resume).
		div := max1(g.grid.MaxDiv())
		g.state.Pause(gamestate.PauseInput{
			Beats:      g.elapsedBeats,
			GridDiv:    div,
			DisplayDiv: div,
		})
		g.setPrimaryStep(g.elapsedBeats)
		g.engine.Stop()
		g.drum.SetPlaying(false)
		g.clearParityState()
		notifyMediaSessionState()
		return nil
	}))

	js.Global().Set("mediaSessionPlay", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if g.Playing() || g.start == nil {
			return nil
		}
		// The readiness gate covers every play, including media keys and headset buttons, which bypass
		// the playPressed chokepoint; otherwise they start on a cold voice cache.
		if !g.playbackReady() {
			return nil
		}
		g.logger.Debugf("[game] mediaSessionPlay")

		audio.Resume()
		now := time.Now()
		audioNow := audio.Now()
		resumeKind := g.state.Resume(now, audioNow)
		g.resetHighlights()
		g.activePulses = nil
		g.activePulse = nil

		resumeFromOffset := (resumeKind == gamestate.ResumeKindResume) || g.elapsedBeats > 0
		if resumeFromOffset {
			for row := range g.drum.Rows {
				g.spawnPulseFromRow(row, g.elapsedBeats)
			}
			if g.activePulse == nil && len(g.activePulses) > 0 {
				g.activePulse = g.activePulses[0]
			}
			div := max1(g.grid.MaxDiv())
			g.state.SetPosition(g.elapsedBeats, div)

			// Align sequencer indices.
			if len(g.seqNextIdxs) != len(g.drum.Rows) {
				g.seqNextIdxs = make([]int, len(g.drum.Rows))
			}
			div2 := max1(g.grid.MaxDiv())
			bpm := g.state.AppliedBPM()
			if bpm <= 0 {
				bpm = g.bpm
			}
			target := g.elapsedBeats
			if bpm > 0 {
				target = int(math.Floor(g.state.BeatBase() * float64(div2)))
			}
			for row := range g.drum.Rows {
				next := g.elapsedBeats
				if row < len(g.nextBeatIdxs) {
					next = g.nextBeatIdxs[row]
				}
				t := target + 1
				if next > t {
					t = next
				}
				g.seqNextIdxs[row] = t
			}
			g.state.SetSeekFreezeFrames(0)
			g.lastFrame = time.Now()
			g.frozenUpToByRow = make([]int, len(g.drum.Rows))
			for i := range g.frozenUpToByRow {
				g.frozenUpToByRow[i] = -1
			}
		} else {
			g.state.SetLastProg(0)
			g.syncUIToTime()
		}

		// This bypasses handlePlaybackTransition, so mirror its re-anchor: anything but a resume ramps
		// saved rules from this session's origin.
		if resumeKind != gamestate.ResumeKindResume {
			g.graph.ReanchorAllGroupRules(g.playheadAbsSubdiv())
		}
		g.engine.Start()
		g.drum.SetPlaying(true)
		notifyMediaSessionState()
		return nil
	}))
}
