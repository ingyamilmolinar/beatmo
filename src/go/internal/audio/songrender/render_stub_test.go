//go:build test

package songrender

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestRenderStub_MasterEqualsSumOfStems(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	r, err := Render("bach-toccata", 2, 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Master) == 0 || len(r.Stems) == 0 {
		t.Fatalf("empty render: master=%d stems=%d", len(r.Master), len(r.Stems))
	}
	// Master must equal the exact element-wise sum of all stems (stub contract).
	sum := make([]float64, len(r.Master))
	for _, s := range r.Stems {
		for i := range s {
			if i < len(sum) {
				sum[i] += s[i]
			}
		}
	}
	for i := range r.Master {
		if d := r.Master[i] - sum[i]; d > 1e-9 || d < -1e-9 {
			t.Fatalf("master != sum of stems at %d: %v vs %v", i, r.Master[i], sum[i])
		}
	}
	// Non-trivial: at least one stem has signal.
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
		t.Errorf("all stems silent (peak=%g) — stub synth not wired", peak)
	}
}

func TestRenderStub_Deterministic(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	a, _ := Render("bach-toccata", 1, 44100)
	b, _ := Render("bach-toccata", 1, 44100)
	if len(a.Master) != len(b.Master) {
		t.Fatalf("len differ %d vs %d", len(a.Master), len(b.Master))
	}
	for i := range a.Master {
		if a.Master[i] != b.Master[i] {
			t.Fatalf("non-deterministic master at %d", i)
		}
	}
}
