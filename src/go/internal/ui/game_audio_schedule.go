package ui

import (
	"math"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const (
	// Caps highlight time so long samples do not stay lit for several beats.
	maxHighlightSeconds = 0.20 // 200ms max visual highlight

	// minHighlightSeconds ensures very short samples are still visible.
	minHighlightSeconds = 0.05 // 50ms minimum
)

func (s *playbackSlate) queueSoundParams(id instrument.ID, vol, pitch, dur float64) {
	g := s.g
	// Never blocks: on saturation drop the oldest so playback stays responsive.
	n := audio.Now()
	// Volumes above 1.0 are intentional boosts; only negatives clamp.
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

// 0 for out-of-range rows, which then rely on transport gating only.
func (s *playbackSlate) rowStructGenLoad(row int) uint64 {
	g := s.g
	if row < 0 || row >= len(g.rowStructGen) {
		return 0
	}
	return g.rowStructGen[row].Load()
}

func (s *playbackSlate) bumpRowStructGen(row int) {
	g := s.g
	if row < 0 || row >= len(g.rowStructGen) {
		return
	}
	g.rowStructGen[row].Add(1)
}

// gridSec (the trigger time without the cushion, 0 = unknown) only feeds measurement.
func (s *playbackSlate) queueSoundAtParams(row, abs int, id instrument.ID, vol, pitch, dur, whenSec, gridSec float64) {
	g := s.g
	if vol < 0 {
		vol = 0
	}
	req := soundReq{id: id, vol: vol, pitch: pitch, dur: dur, when: whenSec, hasWhen: true, grid: gridSec, enqAt: time.Now(), gen: g.audioGen.Load(), rowGen: g.rowStructGenLoad(row), parityGen: g.parityGen.Load(), row: row, abs: abs}
	// So parity's audio_missing check never flags in-flight audio as a scheduler bug.
	g.markSeqAudioEnqueued(row, abs)
	g.logger.Tracef("[audio/queue] id=%s vol=%.3f when=[%.6f]", id, vol, whenSec)
	g.perf.onAudioEnq()
	sendLatest(g.audioCh, req, &g.perf.aDrops)
}

// Above 75 %: seqScheduleTime skips the tick until audioLoop drains.
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
	// An explicit 0 stays 0 (tests rely on immediate highlights).
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
	// Ratchets: never shrinks mid-play. The cushion is added to every note's time, so a change shifts
	// all later notes; following UI timings made the rhythm wobble. Resets per transport generation.
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
	// Warm the exact post-effect pitch off the dispatch goroutine, sequencer beats only. A chord
	// cell sounds one voice per layer.
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
	// The dispatch clamp lives in audioLoop: audio.Now() advances by the bridge latency before then.
	parityRow, parityAbs := -1, -1
	if sequencer {
		parityRow, parityAbs = row, idx
		// Drift is measured against 'when' (after groove and cushion), not the grid time: the grid would
		// fold the whole lookahead into every reading.
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

// The lane calls this alone (its audio was committed ahead); legacy calls it after queueing.
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
		// Starts at 'when', so the highlight appears only when the audio plays.
		g.setNodeHighlightUntil(info.NodeID, when, when+sec)
		// Set directly: on WASM hlCh may drop events under load.
		g.nodeAnimSet(info.NodeID, 1)
	}
}

// Capped: a highlight means "a note fired", not "the note is still playing".
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
