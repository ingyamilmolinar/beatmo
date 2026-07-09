package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// nodeNoteLabel returns the musical note name (e.g. "C#4") a node will sound,
// derived programmatically from the node's semitone pitch plus the current
// synth/sampler pitch offset of its row's instrument. Returns "" for unpitched
// (percussion) instruments — where a note label would mislead — or when the
// node has no resolvable row.
//
// The node-pitch-0 convention is A3 = 220 Hz, so the effective frequency is
// 220·2^((nodePitch+offset)/12); hzToNote maps that to a name. Because it reads
// live audio state (audio.InstrumentPitchInfo is zero-allocation and only holds
// short RLocks), it is safe to call from Draw every frame and the label tracks
// pitch, synth, and sampler edits with no cached state to invalidate.
func (g *Game) nodeNoteLabel(nodeID model.NodeID, nodePitch float64) string {
	row, ok := g.nodeRows[nodeID]
	if !ok || row < 0 || row >= len(g.drum.Rows) {
		return ""
	}
	offset, pitched := audio.InstrumentPitchInfo(g.drum.Rows[row].Instrument)
	if !pitched {
		return ""
	}
	return hzToNote(220 * math.Pow(2, (nodePitch+offset)/12))
}
