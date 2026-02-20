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

// initMediaSessionExports registers mediaSessionPause and mediaSessionPlay as
// JS globals. These are called directly from the browser's media session action
// handlers and must change game state immediately (not via flags) because
// requestAnimationFrame does not fire when the browser is backgrounded.
func (g *Game) initMediaSessionExports() {
	js.Global().Set("mediaSessionPause", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		if !g.Playing() {
			return nil
		}
		g.logger.Infof("[GAME] mediaSessionPause")

		// Bump audio generation so the sequencer drops stale events.
		g.audioGen.Add(1)

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
		g.logger.Infof("[GAME] mediaSessionPlay")

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
			beat := float64(g.elapsedBeats) / float64(div)
			g.state.SetBeatBase(beat)
			g.state.SetLastDisplayBeat(beat)
			g.state.SetLastBeat(beat)
			g.state.SetLastStep(g.elapsedBeats)

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
			g.state.SetPausedBeats(g.elapsedBeats)
			g.lastFrame = time.Now()
			g.frozenUpToByRow = make([]int, len(g.drum.Rows))
			for i := range g.frozenUpToByRow {
				g.frozenUpToByRow[i] = -1
			}
		} else {
			g.state.SetLastProg(0)
			g.syncUIToTime()
		}

		g.engine.Start()
		g.drum.SetPlaying(true)
		notifyMediaSessionState()
		return nil
	}))
}
