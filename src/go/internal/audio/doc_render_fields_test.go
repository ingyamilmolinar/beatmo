//go:build !test && !js

package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestRenderInstrumentWithFieldsIsTheDocumentRender(t *testing.T) {
	resetLibraryForTest(t)
	sr, n := proofSR, 4800
	plain, err := RenderInstrumentWithFields(roster.Kick, sr, n, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := InstrumentDoc(roster.Kick)
	if want := renderBlockForTest(instrument.Flatten(d, BuiltinModuleRegistry()), n); !slices.Equal(plain, want) {
		t.Fatal("no overrides must render the document itself")
	}
	moved, err := RenderInstrumentWithFields(roster.Kick, sr, n, map[string]float64{"post_pitch": 7})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(plain, moved) {
		t.Fatal("an override must reach the render")
	}
	if d2, _ := InstrumentDoc(roster.Kick); InstrumentDocModified(roster.Kick) || len(instrument.Diff(d, d2)) != 0 {
		t.Fatal("rendering with overrides must not edit the project copy")
	}
	if _, err := RenderInstrumentWithFields(roster.Kick, sr, n, map[string]float64{"no_such_field": 1}); err == nil {
		t.Fatal("an unknown field must be an error")
	}
	if _, err := RenderInstrumentWithFields(roster.Kick, sr, n, map[string]float64{"fm_op2_depth": 3}); err != nil {
		t.Fatalf("a field the kick has no module for is skipped, not an error: %v", err)
	}
}
