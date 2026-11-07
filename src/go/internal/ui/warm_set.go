package ui

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Bounds the gate so a pathological circuit cannot hold it forever; the overflow warms in the background afterwards.
// A var so tests can lower it.
var maxWarmSetItems = 512

// warmItem is one distinct voice the circuit can trigger.
type warmItem struct {
	inst  instrument.ID
	pitch float64 // rounded to 0.5 st (roundWarmPitch)
}

// Stable order, so a truncated set always warms the same prefix.
type warmSet struct {
	items     []warmItem
	bpm       int
	truncated bool // the cap was hit; the tail warms in background post-gate
}

func (s warmSet) Len() int { return len(s.items) }

// The warm set reuses the prewarmer's own walk (beatInfosByRow + evalNodeParamsOnly), the only correct pitch
// computation; a second derivation would drift from dispatch. No ±1 neighbourhood: it would triple the wait.
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

// Only when the set changed.
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
