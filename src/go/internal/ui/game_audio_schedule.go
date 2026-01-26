package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

const (
	// maxHighlightSeconds caps visual highlight duration to prevent long samples
	// (e.g., sub-bass at 2.0s) from keeping highlights on for multiple beats.
	maxHighlightSeconds = 0.20 // 200ms max visual highlight

	// minHighlightSeconds ensures very short samples are still visible.
	minHighlightSeconds = 0.05 // 50ms minimum
)

func (g *Game) queueSoundParams(id string, vol, pitch, dur float64) {
	// Queue non-blockingly; if the buffer is saturated, drop the oldest
	// and enqueue the latest so playback remains responsive while tests
	// can also assert non-blocking behavior.
	n := audio.Now()
	// Allow volumes above 1.0 (200%+) for intentional boosts; only clamp
	// negative values to zero.
	if vol < 0 {
		vol = 0
	}
	req := soundReq{id: id, vol: vol, pitch: pitch, dur: dur, enqAt: time.Now(), gen: g.audioGen.Load(), row: -1, abs: -1}
	if n > 0 {
		req.hasWhen = true
		req.when = n
	}
	g.logger.Tracef("[AUDIO/QUEUE] id=%s vol=%.3f when=%v", id, vol, req.when)
	g.perf.onAudioEnq()
	sendLatest(g.audioCh, req)
}

// queueSoundAtParams schedules with explicit timestamp seconds.
func (g *Game) queueSoundAtParams(row, abs int, id string, vol, pitch, dur, whenSec float64) {
	if vol < 0 {
		vol = 0
	}
	req := soundReq{id: id, vol: vol, pitch: pitch, dur: dur, when: whenSec, hasWhen: true, enqAt: time.Now(), gen: g.audioGen.Load(), row: row, abs: abs}
	g.logger.Tracef("[AUDIO/QUEUE] id=%s vol=%.3f when=[%.6f]", id, vol, whenSec)
	g.perf.onAudioEnq()
	sendLatest(g.audioCh, req)
}

func (g *Game) runtimeAudioLookahead() float64 {
	look := g.audioLookaheadSec
	if look < 0 {
		look = 0
	}
	if !g.Playing() {
		if look > 0.12 {
			return 0.12
		}
		return look
	}
	s := g.perf.snapshot()
	extra := 0.0
	if s.AudioQLatMax > 18 {
		extra = math.Max(extra, 0.06)
	} else if s.AudioQLatMax > 12 {
		extra = math.Max(extra, 0.04)
	}
	if s.DrawAvgMS > 18.0 || s.UpdateMaxMS > 25.0 {
		extra = math.Max(extra, 0.06)
	} else if s.DrawAvgMS > 14.0 || s.UpdateMaxMS > 18.0 {
		extra = math.Max(extra, 0.04)
	} else if s.DrawAvgMS > 11.0 || s.UpdateAvgMS > 8.0 {
		extra = math.Max(extra, 0.02)
	}
	look += extra
	if look > 0.12 {
		look = 0.12
	}
	return look
}

// scheduleSound applies groove (swing and micro-delay) and enqueues the sound.
func (g *Game) scheduleSound(row, idx int, info model.BeatInfo, inst string, vol, pitch, dur, baseNow float64, sequencer bool) {
	if info.NodeType == model.NodeTypeMute {
		g.ensureGateSlices()
		if row < len(g.muteUntilByRow) {
			hold := g.muteHoldSteps(row, idx, info)
			until := idx + hold + 1
			if until > g.muteUntilByRow[row] {
				g.muteUntilByRow[row] = until
			}
		}
		audio.Stop(inst)
		if row >= len(g.lastFiredNodeByRow) {
			g.lastFiredNodeByRow = make([]model.NodeID, len(g.drum.Rows))
		}
		g.lastFiredNodeByRow[row] = info.NodeID
		g.setLastTriggered(row, info.NodeID, false)
		return
	}
	when := baseNow
	if !(when > 0) {
		when = audio.Now()
	}
	d := g.nodeNudgeSecondsAt(row, idx, info) // apply per-node groove; fallback to previous node
	when += d
	if look := g.runtimeAudioLookahead(); look > 0 {
		when += look
	}
	parityRow, parityAbs := -1, -1
	if sequencer {
		parityRow, parityAbs = row, idx
	}
	g.queueSoundAtParams(parityRow, parityAbs, inst, vol, pitch, dur, when)
	// Adjust highlight durations to match audio length as closely as possible.
	if sec := g.expectedHighlightSeconds(inst, pitch, dur); sec > 0 {
		// Timeline highlight uses frames based on seconds
		frames := int64(sec * ebitenTPS)
		if frames < 1 {
			frames = 1
		}
		g.highlightVisual(row, idx, info, frames)
		// Node highlight window in wall-clock seconds (audio timeline).
		// Use 'when' as start so highlight only appears when audio plays.
		g.setNodeHighlightUntil(info.NodeID, when, when+sec)
		// Set node animation level directly to ensure simpleDraw ring highlight
		// works in WASM, where the hlCh channel may drop events under load.
		g.nodeAnimSet(info.NodeID, 1)
	}
}

// expectedHighlightSeconds estimates the number of seconds a highlight should
// persist, capped to provide a short visual "punch" regardless of audio length.
// The highlight indicates "a note fired" not "the note is still playing".
func (g *Game) expectedHighlightSeconds(inst string, pitch, dur float64) float64 {
	base := audio.SampleSeconds(inst)
	if base <= 0 {
		// Fallback to a fraction of a beat if unknown: quarter-beat.
		sec := (60.0 / float64(g.drum.bpm)) * 0.25
		if sec > maxHighlightSeconds {
			sec = maxHighlightSeconds
		}
		if sec < minHighlightSeconds {
			sec = minHighlightSeconds
		}
		return sec
	}
	if dur <= 0 {
		dur = 1
	}
	rate := math.Pow(2, pitch/12.0) / dur
	if rate <= 0 {
		rate = 1
	}
	sec := base / rate

	// Cap to maximum highlight duration for visual "punch"
	if sec > maxHighlightSeconds {
		sec = maxHighlightSeconds
	}
	// Ensure minimum visibility
	if sec < minHighlightSeconds {
		sec = minHighlightSeconds
	}
	return sec
}
