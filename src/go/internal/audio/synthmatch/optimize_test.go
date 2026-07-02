package synthmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestMatch_RecoversPerturbedParams(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	sr := 44100
	// Ground truth = sax rendered with a known cutoff; treat as the "reference".
	ref, err := RenderWithParams("sax", 0, sr, 1.2, map[string]float64{"filter_cutoff": 1500})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Match("sax", ref, 300, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.BestLoss >= res.StartLoss {
		t.Errorf("optimizer did not improve: start=%.4f best=%.4f", res.StartLoss, res.BestLoss)
	}
	if got := res.Params["filter_cutoff"]; math.Abs(got-1500) > 600 {
		t.Errorf("filter_cutoff=%.0f did not approach 1500", got)
	}
}

func TestMatch_UsesReferenceDuration(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	ref, err := RenderWithParams("violin", 0, 44100, 1.0, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Match("violin", ref, 40, 0)
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
	ref, _ := RenderInstrument("violin", 0, 44100, 1.0)
	a, _ := Match("violin", ref, 150, 1)
	b, _ := Match("violin", ref, 150, 1)
	if a.BestLoss != b.BestLoss {
		t.Errorf("non-deterministic: %.6f vs %.6f", a.BestLoss, b.BestLoss)
	}
}
