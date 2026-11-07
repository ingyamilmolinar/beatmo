package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// hzToNote clamps to the piano range while the picker labels raw MIDI: a kick reaches MIDI 9 and the deep kick
// 4, so a clamped chip would read "A0" across the bottom octave the picker offers as A-1, G#-1….
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

// Twelve DIFFERENT names, not twelve copies of "A0".
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
