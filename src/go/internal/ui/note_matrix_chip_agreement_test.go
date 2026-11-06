package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestPickerAndChipAgreeAcrossTheRoster is the guard for the bug that making
// note display universal would otherwise have shipped.
//
// nodeNoteLabel used to name a note via hzToNote, which CLAMPS to the piano
// range (MIDI 21..108); the note-matrix picker labelled its cells from raw
// MIDI and never clamped. No instrument that displayed a note before reached
// the clamp, so the two agreed by luck. Universal display breaks that luck: a
// kick's range is MIDI 9..57 and the deep kick's is 4..52, so the chip would
// have shown "A0" for every cell in the bottom octave while the picker offered
// A-1, G#-1, G-1 — the picker promising a note the chip then denies.
//
// This walks every instrument's whole selectable pitch range and asserts the
// two surfaces name each cell identically.
func TestPickerAndChipAgreeAcrossTheRoster(t *testing.T) {
	checked := 0
	for _, id := range roster.All {
		offset, pitched := audio.InstrumentPitchInfo(id)
		if !pitched {
			continue
		}
		m := newNoteMatrixModel(offset)
		for col := 0; col < m.Cols(); col++ {
			for row := 0; row < noteMatrixRows; row++ {
				pitch, ok := m.PitchAt(col, row)
				if !ok {
					continue // dimmed cell, outside the selectable run
				}
				want := m.NoteLabelAt(col, row)
				got := midiNoteName(nodePitchMIDI(pitch, offset))
				if got != want {
					t.Errorf("%s: cell(col=%d,row=%d) pitch %+.0f — picker says %q, chip says %q",
						id, col, row, pitch, want, got)
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no cells checked; the roster walk is not exercising anything")
	}
	t.Logf("checked %d picker cells against the chip label", checked)
}

// TestDeepInstrumentsKeepDistinctNoteNames pins the clamp fix directly: the
// bottom of a deep kick's range must be twelve DIFFERENT note names, not twelve
// copies of "A0".
func TestDeepInstrumentsKeepDistinctNoteNames(t *testing.T) {
	for _, id := range []instrument.ID{roster.Kick, roster.KickDeep, roster.SubBass} {
		offset, pitched := audio.InstrumentPitchInfo(id)
		if !pitched {
			t.Fatalf("%s: expected a note name", id)
		}
		seen := map[string]bool{}
		for p := model.NodePitchMin; p < model.NodePitchMin+12; p++ {
			seen[midiNoteName(nodePitchMIDI(p, offset))] = true
		}
		if len(seen) != 12 {
			t.Errorf("%s: the bottom octave collapsed to %d distinct names, want 12 — the piano-range clamp is back",
				id, len(seen))
		}
	}
}
