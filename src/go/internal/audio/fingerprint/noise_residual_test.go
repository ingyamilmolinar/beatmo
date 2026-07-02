package fingerprint

import (
	"testing"
)

func TestNoiseProfile_SeparatesResidual(t *testing.T) {
	sr := 48000
	pure := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	noisy := synthNoisyTone(sr, 2.0, 440, []float64{1, .5, .25}, 0.15)
	p := computeNoiseProfile(sustainWindow(pure), 440, DefaultAnalysisConfig())
	n := computeNoiseProfile(sustainWindow(noisy), 440, DefaultAnalysisConfig())
	if n.ResidualRatio <= p.ResidualRatio {
		t.Fatalf("noisy residual %.3f not > pure %.3f", n.ResidualRatio, p.ResidualRatio)
	}
	if n.ResidualRatio < 0 || n.ResidualRatio > 1 {
		t.Fatalf("ResidualRatio out of [0,1]: %.3f", n.ResidualRatio)
	}
}

func TestNoiseProfile_PureToneNearZeroResidual(t *testing.T) {
	sr := 48000
	pure := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	p := computeNoiseProfile(sustainWindow(pure), 440, DefaultAnalysisConfig())
	if p.ResidualRatio > 0.15 {
		t.Fatalf("pure tone ResidualRatio should be low, got %.3f", p.ResidualRatio)
	}
}

func TestNoiseProfile_BandLevelsLength(t *testing.T) {
	sr := 48000
	w := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	cfg := DefaultAnalysisConfig()
	p := computeNoiseProfile(sustainWindow(w), 440, cfg)
	if len(p.ResidualBandLevels) != len(cfg.Bands) {
		t.Fatalf("ResidualBandLevels len=%d, want %d", len(p.ResidualBandLevels), len(cfg.Bands))
	}
}

func TestNoiseProfile_SilentInput(t *testing.T) {
	sr := 48000
	w := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	// Zero out all samples to simulate silence.
	for i := range w.Samples {
		w.Samples[i] = 0
	}
	cfg := DefaultAnalysisConfig()
	p := computeNoiseProfile(w, 440, cfg)
	if p.ResidualRatio != 0 {
		t.Fatalf("silent input ResidualRatio should be 0, got %.3f", p.ResidualRatio)
	}
	if p.ResidualCentroidHz != 0 {
		t.Fatalf("silent input ResidualCentroidHz should be 0, got %.3f", p.ResidualCentroidHz)
	}
	if len(p.ResidualBandLevels) != len(cfg.Bands) {
		t.Fatalf("silent: ResidualBandLevels len=%d, want %d", len(p.ResidualBandLevels), len(cfg.Bands))
	}
}

func TestNoiseProfile_ResidualCentroidHigherForNoise(t *testing.T) {
	sr := 48000
	// Broadband noise should have a higher residual centroid than a pure tone.
	pure := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	noisy := synthNoisyTone(sr, 2.0, 440, []float64{1, .5, .25}, 0.3)
	p := computeNoiseProfile(sustainWindow(pure), 440, DefaultAnalysisConfig())
	n := computeNoiseProfile(sustainWindow(noisy), 440, DefaultAnalysisConfig())
	if n.ResidualCentroidHz <= p.ResidualCentroidHz {
		t.Fatalf("noisy residual centroid %.1f Hz not > pure %.1f Hz", n.ResidualCentroidHz, p.ResidualCentroidHz)
	}
}

func TestNoiseProfile_RatioInRange(t *testing.T) {
	sr := 48000
	noisy := synthNoisyTone(sr, 2.0, 440, []float64{1, .5, .25}, 0.5)
	cfg := DefaultAnalysisConfig()
	np := computeNoiseProfile(sustainWindow(noisy), 440, cfg)
	if np.ResidualRatio < 0 || np.ResidualRatio > 1 {
		t.Fatalf("ResidualRatio %.3f out of [0,1]", np.ResidualRatio)
	}
}
