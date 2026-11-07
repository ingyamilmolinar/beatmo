//go:build !test && !js

package synthmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestRenderShippedIncludesDocumentFX: for an instrument whose document
// carries FX, RenderShipped is the PLAYED sound (FX applied, normalised),
// which the raw RenderWithParams is not — the bug this exists to fix.
func TestRenderShippedIncludesDocumentFX(t *testing.T) {
	var id string
	for _, cand := range roster.All {
		if d, ok := audio.ShippedDoc(cand); ok && len(d.FX) > 0 {
			id = string(cand)
			break
		}
	}
	if id == "" {
		t.Fatal("no shipped instrument carries FX")
	}
	sr := 48000
	got, err := RenderShipped(instrument.ID(id), 0, sr, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := audio.EnsureInstrumentDoc(instrument.ID(id))
	want := audio.RenderAsShipped(d, sr, 0)
	for i := 0; i < len(got.Samples) && i < len(want); i++ {
		if math.Float64bits(got.Samples[i]) != math.Float64bits(want[i]) {
			t.Fatalf("%s sample %d: %v want %v", id, i, got.Samples[i], want[i])
		}
	}
	if len(got.Samples) != sr {
		t.Fatalf("len %d, want %d (1 s, zero-padded past the voice)", len(got.Samples), sr)
	}
	raw, err := RenderWithParams(instrument.ID(id), 0, sr, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	same := true
	for i := range raw.Samples {
		if raw.Samples[i] != got.Samples[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("%s: RenderShipped equals the raw render — FX/normalisation not applied", id)
	}
}
