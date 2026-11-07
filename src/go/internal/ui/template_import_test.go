package ui

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Templates are JSON files with no generator at runtime, so the ACTUAL embedded bytes are imported.

func TestTemplates_ImportRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	for _, tp := range assets.Templates() {
		tp := tp
		t.Run(tp.Genre, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(640, 480)
			if err := g.Import(tp.Bytes); err != nil {
				t.Fatalf("%s: import: %v", tp.Genre, err)
			}
			g.updateBeatInfos()
			g.refreshDrumRow()

			if len(g.drum.Rows) == 0 {
				t.Fatalf("%s: no rows after import", tp.Genre)
			}
			// Row 0 origin is promoted to StartNodeID.
			if g.graph.StartNodeID == model.InvalidNodeID {
				t.Fatalf("%s: StartNodeID unset after import", tp.Genre)
			}
			// Every row uses an available instrument and has a non-empty beat path.
			for i, r := range g.drum.Rows {
				if !g.drum.IsInstrumentAvailable(r.Instrument) {
					t.Fatalf("%s row %d: instrument %q not available", tp.Genre, i, r.Instrument)
				}
				if len(g.beatInfosByRow[i]) == 0 {
					t.Fatalf("%s row %d (%s): empty beat path", tp.Genre, i, r.Instrument)
				}
			}
			// Any row, not row 0: a faithful transcription can have its first part rest at the opening (handel-water-horn's
			// Horn 1 is silent for ten bars). A truly dead circuit already failed above.
			anyStep := false
			for _, r := range g.drum.Rows {
				for _, v := range r.Steps {
					if v {
						anyStep = true
						break
					}
				}
				if anyStep {
					break
				}
			}
			if !anyStep {
				t.Fatalf("%s: no row produced any visible steps in the opening window", tp.Genre)
			}
		})
	}
}

// audio.Play silently drops an unregistered id.
func TestTemplates_AllInstrumentsRegistered(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	registered := map[string]bool{}
	for _, id := range audio.Instruments() {
		registered[string(id)] = true
	}
	for _, tp := range assets.Templates() {
		var f projectFile
		if err := json.Unmarshal(tp.Bytes, &f); err != nil {
			t.Fatalf("%s: unmarshal: %v", tp.Genre, err)
		}
		for _, in := range f.Instruments {
			// Instance variants ("organ-2") are promoted onto their base by import, as here.
			audio.EnsureInstanceInstrument(in.ID)
			registered[string(in.ID)] = registered[string(in.ID)] || slices.Contains(audio.Instruments(), in.ID)
			if !registered[string(in.ID)] {
				t.Errorf("%s: instrument %q is not a registered builtin (audio.Play would silently drop it)", tp.Genre, in.ID)
			}
		}
	}
}
