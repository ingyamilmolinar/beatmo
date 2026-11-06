package ui

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// maxWarmSetItems bounds the gated warm set so a pathological circuit cannot
// make the readiness gate effectively infinite. Overflow items are NOT dropped
// — they fall through to the existing background prewarm after the gate opens.
// 512 is ~5x the largest built-in template's 91 pairs (beach-gaelic).
//
// var, not const: a test seam so the cap can be lowered to exercise
// truncation against a small fixture without needing a 512-pair circuit,
// mirroring the existing test-seam pattern (nowNS in notification_store.go,
// drawRect in drawing.go).
var maxWarmSetItems = 512

// warmItem is one distinct voice the circuit can trigger.
type warmItem struct {
	inst  instrument.ID
	pitch float64 // rounded to 0.5 st (roundWarmPitch)
}

// warmSet is the deterministic, de-duplicated working set of a loaded circuit.
// Order is stable: rows in row order, pitches in first-appearance order, so a
// truncated set always warms the same prefix.
type warmSet struct {
	items     []warmItem
	bpm       int
	truncated bool // the cap was hit; the tail warms in background post-gate
}

func (s warmSet) Len() int { return len(s.items) }

// buildWarmSet enumerates every (instrument, pitch) the loaded circuit can
// trigger.
//
// It deliberately reuses the SAME walk the incremental prewarmer uses
// (beatInfosByRow + evalNodeParamsOnly) rather than re-deriving pitch: that
// function is the only correct computation (node pitch + model.GroupEffect
// windowed group transpose + the ±24 clamp), and a second derivation would
// drift from dispatch. The ONLY difference from warmFromBeatInfos is the
// horizon — callers must have ensured beatInfosByRow spans a full loop first.
//
// The ±1 pitch neighborhood is deliberately NOT included: it exists so a
// one-tick pitch EDIT lands warm, it is not needed for playback correctness,
// and including it triples the gate wait (91 → 273 items on beach-gaelic).
// The neighborhood keeps warming in the background via warmFromBeatInfos.
// rowInstruments lists the distinct instruments the rows play, in row order:
// the warm set's instruments and the held set (audio.SetHeldInstruments).
func (g *Game) rowInstruments() []instrument.ID {
	if g == nil || g.drum == nil {
		return nil
	}
	var out []instrument.ID
	for _, r := range g.drum.Rows {
		if r.Instrument != "" && !slices.Contains(out, r.Instrument) {
			out = append(out, r.Instrument)
		}
	}
	return out
}

// syncHeldInstruments hands the rows' instruments to the audio layer when the
// set changed (spec 2026-10-31 §1.2).
func (g *Game) syncHeldInstruments() {
	ids := g.rowInstruments()
	if g.heldSynced && slices.Equal(ids, g.heldInstruments) {
		return
	}
	g.heldInstruments, g.heldSynced = ids, true
	audio.SetHeldInstruments(ids)
}

func (g *Game) buildWarmSet() warmSet {
	out := warmSet{bpm: g.bpm}
	if g == nil || g.drum == nil {
		return out
	}
	seen := make(map[warmItem]struct{})
	rows := g.beatInfosByRow
	for row := range rows {
		if row >= len(g.drum.Rows) {
			break
		}
		inst := g.drum.Rows[row].Instrument
		if inst == "" {
			continue
		}
		for idx, info := range rows[row] {
			if !info.NodeType.Fires() {
				continue // mute/other node types never schedule a voice
			}
			_, pitch, _ := g.evalNodeParamsOnly(row, idx, info)
			it := warmItem{inst: inst, pitch: roundWarmPitch(pitch)}
			if _, dup := seen[it]; dup {
				continue
			}
			seen[it] = struct{}{}
			if len(out.items) >= maxWarmSetItems {
				out.truncated = true
				return out
			}
			out.items = append(out.items, it)
		}
	}
	return out
}
