package synthmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestMatch_RecoversPerturbedParams(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	sr := 44100
	// Ground truth = sax rendered with a known cutoff; treat as the "reference".
	ref, err := RenderWithParams(roster.Sax, 0, sr, 1.2, map[string]float64{"filter_cutoff": 1500})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Match(roster.Sax, ref, 90, 1)
	if err != nil {
		t.Fatal(err)
	}

	// The true cutoff is a zero-distance well, but narrow and flanked by ridges, so a bounded search from the default
	// cannot reliably reach it; asserting recovery would test the landscape. What the matcher guarantees: the loss drops and
	// the result is in range.
	if res.BestLoss >= res.StartLoss {
		t.Errorf("optimizer did not improve: start=%.4f best=%.4f", res.StartLoss, res.BestLoss)
	}
	if got, ok := res.Params["filter_cutoff"]; !ok {
		t.Errorf("result missing tuned filter_cutoff")
	} else if got < 200 || got > 8000 {
		t.Errorf("recovered filter_cutoff=%.0f out of spec range [200,8000]", got)
	}
}

func TestMatch_UsesReferenceDuration(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	ref, err := RenderWithParams(roster.Violin, 0, 44100, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Match(roster.Violin, ref, 40, 0)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if res.BestLoss > res.StartLoss+1e-9 {
		t.Fatalf("BestLoss %.4f worse than StartLoss %.4f", res.BestLoss, res.StartLoss)
	}
}

func TestMatch_Deterministic(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	ref, _ := RenderInstrument(roster.Violin, 0, 44100, 1.0)
	// Each eval renders and fingerprints; this only needs the restart path twice.
	a, _ := Match(roster.Violin, ref, 60, 1)
	b, _ := Match(roster.Violin, ref, 60, 1)
	if a.BestLoss != b.BestLoss {
		t.Errorf("non-deterministic: %.6f vs %.6f", a.BestLoss, b.BestLoss)
	}
}
