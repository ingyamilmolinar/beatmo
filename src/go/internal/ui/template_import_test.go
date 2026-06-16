package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Templates are authoritative JSON project files (no procedural generator), so
// these guards import the ACTUAL embedded bytes — the same strategy used for the
// startup demo (see startup_demo_quality_test.go). Each shipped template must
// import cleanly, attach known instruments, and produce a playable circuit.

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
			// The circuit must actually play: row 0 produces at least one visible step.
			anyStep := false
			for _, v := range g.drum.Rows[0].Steps {
				if v {
					anyStep = true
					break
				}
			}
			if !anyStep {
				t.Fatalf("%s: row 0 produced no visible steps", tp.Genre)
			}
		})
	}
}

// Every instrument id referenced by a template must be a registered builtin, or
// audio.Play silently drops it (the kick-punchy / unknown-id gotcha).
func TestTemplates_AllInstrumentsRegistered(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	registered := map[string]bool{}
	for _, id := range audio.Instruments() {
		registered[id] = true
	}
	for _, tp := range assets.Templates() {
		var f exportFile
		if err := json.Unmarshal(tp.Bytes, &f); err != nil {
			t.Fatalf("%s: unmarshal: %v", tp.Genre, err)
		}
		for _, in := range f.Instruments {
			if !registered[in.ID] {
				t.Errorf("%s: instrument %q is not a registered builtin (audio.Play would silently drop it)", tp.Genre, in.ID)
			}
		}
	}
}
