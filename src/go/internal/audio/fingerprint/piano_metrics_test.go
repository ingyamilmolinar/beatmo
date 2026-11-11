package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// synthStiffString builds a decaying struck-string tone whose partials follow
// f_n = n·f0·√(1+B·n²) (B=0 → exact harmonics). Used to validate AnalyzePiano's
// inharmonicity fit.
func synthStiffString(sr int, f0, B float64, nPartials int) wave.Wave {
	n := sr // 1 s
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / float64(sr)
		env := math.Exp(-3 * t)
		s := 0.0
		for h := 1; h <= nPartials; h++ {
			fn := float64(h) * f0 * math.Sqrt(1+B*float64(h*h))
			s += (1.0 / float64(h)) * math.Sin(2*math.Pi*fn*t)
		}
		x[i] = s * env
	}
	return wave.Wave{Samples: x, SampleRate: sr}
}

func TestPianoInharmonicity(t *testing.T) {
	const sr = 44100
	// Exact harmonics → B ≈ 0.
	pmExact := AnalyzePiano(synthStiffString(sr, 262, 0.0, 8), 262)
	if pmExact.InharmonicityB > 0.0002 {
		t.Fatalf("exact-harmonic tone should have B≈0, got %.5f", pmExact.InharmonicityB)
	}
	// A stiff string (B=0.001) → clearly positive B, and the upper partials read
	// progressively SHARP (the piano signature).
	pmStiff := AnalyzePiano(synthStiffString(sr, 262, 0.001, 8), 262)
	if pmStiff.InharmonicityB < 0.0005 {
		t.Fatalf("stiff string (B=0.001) should measure B>0.0005, got %.5f", pmStiff.InharmonicityB)
	}
	if pmStiff.InharmonicityB <= pmExact.InharmonicityB {
		t.Fatalf("stiff (%.5f) should exceed exact (%.5f)", pmStiff.InharmonicityB, pmExact.InharmonicityB)
	}
	// Per-partial stretch increases with n for the stiff string.
	if len(pmStiff.StretchCents) >= 6 {
		h3, h6 := pmStiff.StretchCents[2], pmStiff.StretchCents[5]
		if !(h6 > h3 && h3 > 0) {
			t.Fatalf("stretch should rise with n: H3=%.1f H6=%.1f cents", h3, h6)
		}
	}
}

func TestPianoDecayAndAttack(t *testing.T) {
	const sr = 44100
	pm := AnalyzePiano(synthStiffString(sr, 262, 0.0005, 8), 262)
	// A single exponential decay → fast≈slow → TwoStageRatio near 1 (this synthetic
	// has no two-stage structure; a real piano's would exceed it — this pins the
	// baseline so the metric isn't fabricating a double decay).
	if pm.TwoStageRatio < 0.3 || pm.TwoStageRatio > 3.0 {
		t.Fatalf("single-exp decay should give TwoStageRatio ~1, got %.2f", pm.TwoStageRatio)
	}
	// A struck onset is fast.
	if pm.AttackMs > 50 {
		t.Fatalf("struck attack should be <50 ms, got %.1f", pm.AttackMs)
	}
}
