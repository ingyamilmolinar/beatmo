//go:build test

package songrender

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestAllTemplates_RenderStubInvariants(t *testing.T) {
	for _, tpl := range assets.Templates() {
		stem := tpl.Genre
		t.Run(stem, func(t *testing.T) {
			audio.Reset()
			audio.ResetInstruments()
			r, err := Render(stem, 2, 44100)
			if err != nil {
				t.Fatalf("%s: render: %v", stem, err)
			}
			if r.Arrangement.TotalSamples <= 0 {
				t.Fatalf("%s: no samples", stem)
			}
			if len(r.Master) != r.Arrangement.TotalSamples {
				t.Errorf("%s: master len %d != totalSamples %d", stem, len(r.Master), r.Arrangement.TotalSamples)
			}
			// Master == Σ stems (stub contract).
			sum := make([]float64, len(r.Master))
			for _, s := range r.Stems {
				for i := range s {
					sum[i] += s[i]
				}
			}
			for i := range r.Master {
				if d := r.Master[i] - sum[i]; d > 1e-9 || d < -1e-9 {
					t.Fatalf("%s: master != Σ stems at %d", stem, i)
				}
			}
			// Renderer's whole job is to schedule audible notes — a silent template is a bug.
			if len(r.Arrangement.Notes) == 0 {
				t.Fatalf("%s: arrangement emitted 0 notes", stem)
			}
			peak := 0.0
			for _, s := range r.Stems {
				for _, v := range s {
					if v > peak {
						peak = v
					} else if -v > peak {
						peak = -v
					}
				}
			}
			if peak < 1e-6 {
				t.Errorf("%s: all stems silent (peak=%g)", stem, peak)
			}
		})
	}
}
