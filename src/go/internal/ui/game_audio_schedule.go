package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const (
	// maxHighlightSeconds caps visual highlight duration to prevent long samples
	// (e.g., sub-bass at 2.0s) from keeping highlights on for multiple beats.
	maxHighlightSeconds = 0.20 // 200ms max visual highlight

	// minHighlightSeconds ensures very short samples are still visible.
	minHighlightSeconds = 0.05 // 50ms minimum
)

func (s *playbackSlate) queueSoundParams(id instrument.ID, vol, pitch, dur float64) {
	g := s.g
	// Queue non-blockingly; if the buffer is saturated, drop the oldest
	// and enqueue the latest so playback remains responsive while tests
	// can also assert non-blocking behavior.
	n := audio.Now()
	// Allow volumes above 1.0 (200%+) for intentional boosts; only clamp
	// negative values to zero.
	if vol < 0 {
		vol = 0
	}
	req := soundReq{id: id, vol: vol, pitch: pitch, dur: dur, enqAt: time.Now(), gen: g.audioGen.Load(), parityGen: g.parityGen.Load(), row: -1, abs: -1}
	if n > 0 {
		req.hasWhen = true
		req.when = n
	}
	g.logger.Tracef("[audio/queue] id=%s vol=%.3f when=%v", id, vol, req.when)
	g.perf.onAudioEnq()
	sendLatest(g.audioCh, req, &g.perf.aDrops)
}

// rowStructGenLoad returns the current per-row structural generation for row,
// or 0 for out-of-range rows (which then rely on global-transport gating only).
func (s *playbackSlate) rowStructGenLoad(row int) uint64 {
	g := s.g
	if row < 0 || row >= len(g.rowStructGen) {
		return 0
	}
	return g.rowStructGen[row].Load()
}

// bumpRowStructGen advances the structural generation of a single row so that
// its already-scheduled in-flight beats are dropped by the audioLoop filter.
func (s *playbackSlate) bumpRowStructGen(row int) {
	g := s.g
	if row < 0 || row >= len(g.rowStructGen) {
		return
	}
	g.rowStructGen[row].Add(1)
}

// queueSoundAtParams schedules with explicit timestamp seconds.
// gridSec is the predictor's trigger time without the lookahead cushion (0 =
// unknown); it only feeds timing measurement, never the schedule.
func (s *playbackSlate) queueSoundAtParams(row, abs int, id instrument.ID, vol, pitch, dur, whenSec, gridSec float64) {
	g := s.g
	if vol < 0 {
		vol = 0
	}
	req := soundReq{id: id, vol: vol, pitch: pitch, dur: dur, when: whenSec, hasWhen: true, grid: gridSec, enqAt: time.Now(), gen: g.audioGen.Load(), rowGen: g.rowStructGenLoad(row), parityGen: g.parityGen.Load(), row: row, abs: abs}
	// Mark the seq decision as having reached the audio pipeline so parity's
	// audio_missing check does not flag in-flight/dropped audio as a scheduler
	// bug (see paritySeqDecision.Enqueued). Sequencer reqs only (row >= 0).
	g.markSeqAudioEnqueued(row, abs)
	g.logger.Tracef("[audio/queue] id=%s vol=%.3f when=[%.6f]", id, vol, whenSec)
	g.perf.onAudioEnq()
	sendLatest(g.audioCh, req, &g.perf.aDrops)
}

// audioChNearFull returns true if the audio channel is above 75% capacity.
// Used by seqScheduleTime to skip the entire tick when the audioLoop
// goroutine hasn't drained the channel yet.
func (s *playbackSlate) audioChNearFull() bool {
	g := s.g
	return len(g.audioCh) >= cap(g.audioCh)*3/4
}

func (s *playbackSlate) runtimeAudioLookahead() float64 {
	g := s.g
	look := g.audioLookaheadSec
	if look < 0 {
		look = 0
	}
	if !g.Playing() {
		if look > 0.15 {
			return 0.06
		}
		return look
	}
	// When audioLookaheadSec is explicitly 0, respect it without adding
	// performance-based extras. Tests use setAudioLookahead(0) to get
	// immediate highlight visibility in syncAndFindHighlights().
	if look == 0 {
		return 0
	}
	ps := g.perf.snapshot()
	extra := 0.0
	if ps.AudioQLatMax > 18 {
		extra = math.Max(extra, 0.03)
	} else if ps.AudioQLatMax > 12 {
		extra = math.Max(extra, 0.02)
	}
	if ps.DrawAvgMS > 18.0 || ps.UpdateMaxMS > 25.0 {
		extra = math.Max(extra, 0.03)
	} else if ps.DrawAvgMS > 14.0 || ps.UpdateMaxMS > 18.0 {
		extra = math.Max(extra, 0.02)
	} else if ps.DrawAvgMS > 11.0 || ps.UpdateAvgMS > 8.0 {
		extra = math.Max(extra, 0.01)
	}
	look += extra
	if look > 0.15 {
		look = 0.15
	}
	// Ratchet within a transport session: never SHRINK mid-play. The cushion
	// is added to every beat's `when`, so each change shifts all later notes
	// by the delta — the extras above follow draw/update timings, and a UI
	// hovering around a threshold made the audio grid wobble (measured with
	// the playback probe: 20 ms tempo hiccups while nothing about the circuit
	// changed). Growing still costs one shift, but at most three per session;
	// the floor resets on the next transport generation (Stop/Pause/seek).
	if gen := g.audioGen.Load(); gen != s.lookaheadGen {
		s.lookaheadGen = gen
		s.lookaheadFloor = 0
	}
	if look < s.lookaheadFloor {
		look = s.lookaheadFloor
	}
	s.lookaheadFloor = look
	return look
}

// scheduleSound applies groove (swing and micro-delay) and enqueues the sound.
func (s *playbackSlate) scheduleSound(row, idx int, info model.BeatInfo, inst instrument.ID, vol, pitch, dur, baseNow float64, sequencer bool) {
	g := s.g
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
	// Backstop the graph-change enumeration: warm the exact post-effect pitch
	// (includes group transpose / logic decisions) off the dispatch goroutine.
	// Only for real sequencer beats — fire-and-forget UI sounds (sequencer=false)
	// play immediately and don't need pre-warming. Reached only for non-mute
	// regular beats (mute nodes returned above).
	// A chord cell sounds one voice per layer (nil = one voice, untouched).
	layers := s.nodeLayersAt(info)
	if sequencer && g.voicePrewarmer != nil {
		if layers == nil {
			g.voicePrewarmer.warmPitch(inst, pitch)
		}
		for _, l := range layers {
			g.voicePrewarmer.warmPitch(inst, pitch+l.Semis)
		}
	}
	when := baseNow
	if !(when > 0) {
		when = audio.Now()
	}
	d := g.nodeNudgeSecondsAt(row, idx, info) // apply per-node groove; fallback to previous node
	when += d
	grid := when
	if look := g.runtimeAudioLookahead(); look > 0 {
		when += look
	}
	// NOTE: the dispatch-time clamp lives in audioLoop, not here, because
	// audio.Now() advances between scheduleSound and audioLoop dispatch by
	// the Stage B (bridge) latency (~1-30ms in WASM). Clamping here would
	// still let overdue events sneak through when bridge latency exceeds
	// the safety margin.
	parityRow, parityAbs := -1, -1
	if sequencer {
		parityRow, parityAbs = row, idx
		// Stage-D drift measures scheduler-INTENDED vs actually-produced: how
		// faithfully the audio device emitted the sample at the moment we asked
		// it to. The intended moment is 'when' — the fully-resolved schedule
		// time AFTER the deliberate groove nudge + lookahead cushion — NOT the
		// bare grid time baseNow. Passing baseNow folds the whole lookahead
		// (desktop ~20ms, more under adaptive headroom) into every drift
		// reading, so Stage D would report a systematic ~lookahead bias and
		// breach EmitDrift.MeanMax on normal playback even when the device is
		// emitting perfectly on schedule. drift = emittedWall - when isolates
		// true device jitter, which is what the gate is meant to catch.
		g.openTimingTrace(row, idx, when, g.audioGen.Load(), g.parityGen.Load())
	}
	if layers == nil {
		g.queueSoundAtParams(parityRow, parityAbs, inst, vol, pitch, dur, when, grid)
	}
	for _, l := range layers {
		g.queueSoundAtParams(parityRow, parityAbs, inst, vol*l.Gain, pitch+l.Semis, dur, when, grid)
	}
	s.scheduleSoundUI(row, idx, info, inst, pitch, dur, when)
}

// scheduleSoundUI is the visual half of scheduling a beat: highlight duration
// matched to the audio length, the node's highlight window on the audio
// clock, and the node animation. The audio lane calls it alone (its audio
// was committed ahead); the legacy path calls it after queueing the sound.
func (s *playbackSlate) scheduleSoundUI(row, idx int, info model.BeatInfo, inst instrument.ID, pitch, dur, when float64) {
	g := s.g
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
func (s *playbackSlate) expectedHighlightSeconds(inst instrument.ID, pitch, dur float64) float64 {
	g := s.g
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

func (g *Game) queueSoundParams(id instrument.ID, vol, pitch, dur float64) {
	g.playbackSlate.queueSoundParams(id, vol, pitch, dur)
}

func (g *Game) rowStructGenLoad(row int) uint64 { return g.playbackSlate.rowStructGenLoad(row) }

func (g *Game) bumpRowStructGen(row int) { g.playbackSlate.bumpRowStructGen(row) }

func (g *Game) queueSoundAtParams(row, abs int, id instrument.ID, vol, pitch, dur, whenSec, gridSec float64) {
	g.playbackSlate.queueSoundAtParams(row, abs, id, vol, pitch, dur, whenSec, gridSec)
}

func (g *Game) audioChNearFull() bool { return g.playbackSlate.audioChNearFull() }

func (g *Game) runtimeAudioLookahead() float64 { return g.playbackSlate.runtimeAudioLookahead() }

func (g *Game) scheduleSoundUI(row, idx int, info model.BeatInfo, inst instrument.ID, pitch, dur, when float64) {
	g.playbackSlate.scheduleSoundUI(row, idx, info, inst, pitch, dur, when)
}

func (g *Game) scheduleSound(row, idx int, info model.BeatInfo, inst instrument.ID, vol, pitch, dur, baseNow float64, sequencer bool) {
	g.playbackSlate.scheduleSound(row, idx, info, inst, vol, pitch, dur, baseNow, sequencer)
}

func (g *Game) expectedHighlightSeconds(inst instrument.ID, pitch, dur float64) float64 {
	return g.playbackSlate.expectedHighlightSeconds(inst, pitch, dur)
}
