package ui

import (
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// THE node pitch → note conversion for the badge, chip and picker: the picker labels from (col,row)
// and the chip from a pitch, and they must name the same note for the same cell.
func nodePitchMIDI(nodePitch, instrumentOffset float64) int {
	return int(math.Round(float64(a3MIDI) + nodePitch + instrumentOffset))
}

// Unlike hzToNote, never clamped to the piano range: offsets reach far below A0, and clamping showed
// a kick's whole bottom octave as "A0" while the picker named A-1, G#-1, G-1.
func midiNoteName(midi int) string {
	// Floor division, so octave numbering stays monotonic through MIDI 0.
	pc := ((midi % 12) + 12) % 12
	oct := int(math.Floor(float64(midi)/12)) - 1
	return noteNames[pc] + strconv.Itoa(oct)
}

// "" only for unpitched noise or a node with no row. Reads live audio state (zero-alloc, short
// RLocks), so it is safe in Draw and tracks edits with no cache to invalidate.
func (g *Game) nodeNoteLabel(nodeID model.NodeID, nodePitch float64) string {
	row, ok := g.nodeRows[nodeID]
	if !ok || row < 0 || row >= len(g.drum.Rows) {
		return ""
	}
	offset, pitched := audio.InstrumentPitchInfo(g.drum.Rows[row].Instrument)
	if !pitched {
		return ""
	}
	return midiNoteName(nodePitchMIDI(nodePitch, offset))
}
