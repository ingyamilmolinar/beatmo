package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// swellSine ramps linearly from 0 to full over swellSec, then decays with tau —
// a slow attack (in contrast to expDecaySine, which is full amplitude at t=0).
func swellSine(sr int, dur, f0, swellSec, tau float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		var amp float64
		if t < swellSec {
			amp = t / swellSec
		} else {
			amp = math.Exp(-(t - swellSec) / tau)
		}
		s[i] = amp * math.Sin(2*math.Pi*f0*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestLogAttackTime_SharpVsSwell(t *testing.T) {
	const sr = 44100
	sharp := KickAnalyze(expDecaySine(sr, 0.4, 100, 0.05)) // full amplitude at t=0 → very sharp
	swell := KickAnalyze(swellSine(sr, 0.4, 100, 0.05, 0.1))

	if sharp.LogAttackTime >= swell.LogAttackTime {
		t.Errorf("sharp LogAttackTime (%.4f) must be MORE negative than swell (%.4f)", sharp.LogAttackTime, swell.LogAttackTime)
	}
}

func TestTemporalCentroid_FrontLoadedVsLongTail(t *testing.T) {
	const sr = 44100
	frontLoaded := KickAnalyze(expDecaySine(sr, 0.6, 100, 0.03)) // fast decay, energy early
	longTail := KickAnalyze(expDecaySine(sr, 0.6, 100, 0.15))    // slow decay, energy spread late

	if frontLoaded.TemporalCentroidSec >= longTail.TemporalCentroidSec {
		t.Errorf("front-loaded TemporalCentroidSec (%.4f) must be SMALLER than long-tail (%.4f)",
			frontLoaded.TemporalCentroidSec, longTail.TemporalCentroidSec)
	}
	if frontLoaded.TemporalCentroidSec <= 0 || longTail.TemporalCentroidSec <= 0 {
		t.Errorf("temporal centroids must be positive: front=%.4f long=%.4f",
			frontLoaded.TemporalCentroidSec, longTail.TemporalCentroidSec)
	}
}

func TestKickDistance_AttackTerm(t *testing.T) {
	const sr = 44100
	sharp := KickAnalyze(expDecaySine(sr, 0.6, 100, 0.03))
	swell := KickAnalyze(swellSine(sr, 0.6, 100, 0.05, 0.15))

	d := KickDistance(sharp, swell)
	if d.Attack <= 0 {
		t.Errorf("KickDistance Attack term = %.4f, want clearly > 0", d.Attack)
	}
	// Identity + symmetry.
	if id := KickDistance(sharp, sharp); id.Attack != 0 {
		t.Errorf("KickDistance(x,x).Attack = %g, want 0", id.Attack)
	}
	if dr := KickDistance(swell, sharp); math.Abs(dr.Attack-d.Attack) > 1e-12 {
		t.Errorf("Attack term not symmetric: %g vs %g", d.Attack, dr.Attack)
	}
}

// --- combined identity with ALL new terms ------------------------------------

func TestKickDistance_IdentityWithFluxInharmAttack(t *testing.T) {
	const sr = 44100
	w := KickAnalyze(tremoloSine(sr, 0.5, 200, 0.25, 8, 0.5))
	d := KickDistance(w, w)
	if d.Total > 1e-9 || d.Flux != 0 || d.Inharmonicity != 0 || d.Attack != 0 {
		t.Errorf("KickDistance(x,x): Total=%g Flux=%g Inharmonicity=%g Attack=%g, want all ~0",
			d.Total, d.Flux, d.Inharmonicity, d.Attack)
	}
}
