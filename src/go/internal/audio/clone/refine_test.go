package clone

import (
	"bytes"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const refineRate = 48000

// modelRender is the -tags test RenderFunc: the document's modes, summed in Go.
func modelRender(pitch float64, n int) RenderFunc {
	return func(d instrument.Doc) []float64 { return Synthesize(DocModes(d, pitch, refineRate), refineRate, n) }
}

func refineFixture(t *testing.T) (*LossRef, Analysis, instrument.Doc, float64) {
	t.Helper()
	truth := eightModes()
	for i := range truth {
		truth[i].Amp *= 0.4 // the eight amplitudes sum past full scale
	}
	x := modalSignal(truth, refineRate, 0.05, 1.2, 1e-6, 1)
	p, err := Prepare(x, refineRate, PrepareOptions{Label: "refine"})
	if err != nil {
		t.Fatal(err)
	}
	// Doc₀ from a WRONG analysis: everything above 1.5 kHz 6 dB hot, every
	// decay 1.5× fast.
	bad := append([]Mode(nil), truth...)
	for i := range bad {
		if bad[i].Freq > 1500 {
			bad[i].Amp *= 2
		}
		bad[i].Alpha *= 1.5
	}
	a := analysisOf(bad, PitchInfo{Pitched: true, F0Hz: 261.63})
	a.Residual = Residual{}
	d, rep, err := Invert(a, InvertOptions{ID: "refine"})
	if err != nil {
		t.Fatal(err)
	}
	return NewLossRef(p, 1.0), a, d, rep.FitPitch
}

func TestRefineRecoversAPerturbedModel(t *testing.T) {
	ref, a, d0, pitch := refineFixture(t)
	res, err := Refine(d0, ref, a, RefineOptions{Render: modelRender(pitch, refineRate), Pitch: pitch, Gens: 80, Seed: 1,
		// No prior: this fixture's analysis is deliberately WRONG, and the
		// prior's job is to hold the search near the analysis.
		Weights: LossWeights{MRSTFT: 1, Envelope: 1}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dims %d evals %d: total %.3f → %.3f (MR-STFT %.3f → %.3f, env %.2f → %.2f dB) in %.0f ms",
		len(res.Dims), res.Evals, res.Before.Total, res.After.Total, res.Before.MRSTFT, res.After.MRSTFT,
		res.Before.Envelope, res.After.Envelope, res.TotalMs)
	if res.Kept || !(res.After.MRSTFT < 0.5*res.Before.MRSTFT) {
		t.Fatalf("refine did not halve the spectral loss: %+v → %+v", res.Before, res.After)
	}
}

func TestRefineIsDeterministicAcrossWorkerCounts(t *testing.T) {
	ref, a, d0, pitch := refineFixture(t)
	run := func(workers int) []byte {
		res, err := Refine(d0, ref, a, RefineOptions{Render: modelRender(pitch, refineRate/2), Pitch: pitch, Gens: 4, Seed: 9, Workers: workers})
		if err != nil {
			t.Fatal(err)
		}
		b, err := instrument.MarshalFile(res.Doc)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	if one, many := run(1), run(6); !bytes.Equal(one, many) {
		t.Fatal("1 worker and 6 workers refined to different documents")
	}
}

func TestRefineKeepsDoc0WhenItCannotImprove(t *testing.T) {
	ref, a, d0, pitch := refineFixture(t)
	// A render that ignores the document: no candidate can beat Doc₀.
	fixed := modelRender(pitch, refineRate)(d0)
	res, err := Refine(d0, ref, a, RefineOptions{Render: func(instrument.Doc) []float64 { return fixed }, Pitch: pitch, Gens: 3, Seed: 2,
		Weights: LossWeights{MRSTFT: 1, Envelope: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Kept || res.After.Total != res.Before.Total {
		t.Fatalf("kept=%v before %.6f after %.6f", res.Kept, res.Before.Total, res.After.Total)
	}
	if math.IsNaN(res.Before.Total) {
		t.Fatal("NaN loss")
	}
}
