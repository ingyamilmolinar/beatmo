package ui

import (
	"encoding/json"
	"slices"
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
			// The circuit must actually play: at least one row shows a visible
			// step in the opening import window. We deliberately do NOT require
			// row 0 specifically — row 0 is just the first-listed part, and a
			// faithful multi-part transcription can have that part rest at the
			// opening. handel-water-horn is the canonical case: "Horn 1" (row 0)
			// is silent for the first 10 bars (first score onset at step 120),
			// so on the square-perimeter layout its first audible node lands at
			// beat 480 — past the ~250-wide import window — while the trumpets
			// hit the downbeat. The circuit plays; only the top row is quiet at
			// the very start. (Every row already asserted a non-empty beat path
			// above, so a truly dead circuit still fails.)
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
			// Instance variants ("organ-2") aren't pre-registered builtins; import
			// promotes them to first-class instruments aliased onto their base.
			// Mirror that promotion (import.go does the same) before the check.
			audio.EnsureInstanceInstrument(in.ID)
			registered[in.ID] = registered[in.ID] || slices.Contains(audio.Instruments(), in.ID)
			if !registered[in.ID] {
				t.Errorf("%s: instrument %q is not a registered builtin (audio.Play would silently drop it)", tp.Genre, in.ID)
			}
		}
	}
}
