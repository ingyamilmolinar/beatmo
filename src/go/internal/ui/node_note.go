package ui

import (
	"math"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// nodePitchMIDI is the MIDI note a node sounds: the A3 = pitch-0 convention
// (a3MIDI) plus the node's semitone pitch plus the instrument's live offset.
//
// It is THE conversion for node pitch -> note, shared by the coordinate badge,
// the sidebar chip and the note-matrix picker. Sharing it is not tidiness: the
// picker labels a cell from (col,row) while the chip labels from a pitch, and
// the two must land on the same name for the same cell or the picker tells the
// user they picked a note the chip then denies.
func nodePitchMIDI(nodePitch, instrumentOffset float64) int {
	return int(math.Round(float64(a3MIDI) + nodePitch + instrumentOffset))
}

// midiNoteName renders a MIDI note as a name ("C#4").
//
// Unlike hzToNote it does NOT clamp to the piano range. The clamp is correct
// for a spectrum-cursor readout, which reads arbitrary FFT frequencies, and
// wrong here: instrument offsets now reach far below A0 (the deep kick's base
// is 42 Hz, and a node pitched down another two octaves from there lands at
// MIDI 4), so clamping would render every note in a kick's bottom octave as
// the same "A0" while the picker — which never clamped — labelled the same
// cells A-1, G#-1, G-1. Both surfaces resolve through this function so that
// disagreement cannot reappear.
func midiNoteName(midi int) string {
	// Floor division, so octave numbering stays monotonic through MIDI 0 rather
	// than folding at the sign boundary the way Go's truncating / and % do.
	pc := ((midi % 12) + 12) % 12
	oct := int(math.Floor(float64(midi)/12)) - 1
	return noteNames[pc] + strconv.Itoa(oct)
}

// nodeNoteLabel returns the musical note name (e.g. "C#4") a node will sound,
// derived programmatically from the node's semitone pitch plus the current
// synth/sampler pitch offset of its row's instrument. Returns "" only for the
// genuinely unpitched noise instruments (hi-hats, claps, shaker, whisper),
// where no fundamental exists to name, or when the node has no resolvable row.
//
// Every other instrument — kicks, snares, toms, congas, cowbell, the FM family
// and the whole melodic roster — names a note: audio.InstrumentPitchInfo
// resolves the instrument's sounding fundamental from the generated
// fundamentals table (measured by rendering the voice and analysing it, see
// cmd/gen-fundamentals) plus the delta of any live parameter edit.
//
// Because it reads live audio state (InstrumentPitchInfo is zero-allocation and
// only holds short RLocks), it is safe to call from Draw every frame and the
// label tracks pitch, synth, and sampler edits with no cached state to
// invalidate.
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
