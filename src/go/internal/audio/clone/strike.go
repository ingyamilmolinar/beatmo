package clone

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Strike is the fitted excitation (spec §2c / §3c): a raised-cosine contact
// force of duration TcSec plus a hardness tilt (dB/oct re 1 kHz).
type Strike struct {
	TcSec            float64 `json:"tc_s"`
	HardnessDBPerOct float64 `json:"hardness_db_per_oct"`
	GainDB           float64 `json:"gain_db"`        // fitted mode level at 1 kHz before the force spectrum
	ClickLevelDB     float64 `json:"click_level_db"` // residual vs reference over the contact (from MeasureResidual)
	FitRMSdB         float64 `json:"fit_rms_db"`
}

// StrikeSpectrum is the smooth ENVELOPE of the raised-cosine contact force
// spectrum |sinc(x)/(1 − x²)| (x = f·tc, force 0.5·(1 − cos 2πt/tc) on
// [0, tc]): 1/√(1 + (π x³)²) — 1 at DC, the exact −18 dB/oct asymptote
// 1/(π x³), and no nulls. The ideal spectrum's nulls made the inverse
// ill-posed: a mode next to one needed ×1000 storage, and the fit could put
// every mode on a sidelobe peak (Tc = 25 ms at +44 dB gain on an 8-mode round
// trip); a real mallet has no exact zeros. Mirrors modal_strike_spectrum.
func StrikeSpectrum(f, tc float64) float64 { return audio.ModalStrikeSpectrum(f, tc) }

// HardnessGain is the hardness tilt as a linear gain: h dB per octave re 1 kHz.
func HardnessGain(f, h float64) float64 {
	return math.Pow(10, h*math.Log2(f/1000)/20)
}

const (
	maxHardness = 24   // dB/oct, the knob's bound
	strikeTieDB = 0.05 // a longer contact must improve the fit by this much
)

// EstimateStrike fits dB(amp_i) ≈ G + dB(S(f_i; Tc)) + H·log2(f_i/1 kHz)
// across the modes, energy-weighted: a log grid over Tc ∈ [0.05 ms, 50 ms]
// (241 points, 2.9 % apart) with a weighted line fit for (G, H) at each Tc,
// |H| within the knob's bound. (S used to be floored at −60 dB against the
// ideal spectrum's nulls; on the smooth envelope that floor only made a kink a
// long contact could exploit.)
func EstimateStrike(modes []Mode) Strike {
	if len(modes) == 0 {
		return Strike{}
	}
	n := len(modes)
	u := make([]float64, n)
	a := make([]float64, n)
	w := make([]float64, n)
	for i, m := range modes {
		u[i] = math.Log2(m.Freq / 1000)
		a[i] = dB(m.Amp)
		w[i] = m.Energy()
	}
	best := Strike{FitRMSdB: math.MaxFloat64}
	for i := 0; i <= 240; i++ {
		tc := 0.05e-3 * math.Pow(10, float64(i)/80)
		r := make([]float64, n)
		for k, m := range modes {
			r[k] = a[k] - dB(StrikeSpectrum(m.Freq, tc))
		}
		g, h, cost := weightedLine(u, r, w)
		if math.Abs(h) > maxHardness {
			continue
		}
		// Above the corner the −18 dB/oct roll-off and the tilt trade off
		// almost exactly, so a longer contact must fit NOTICEABLY better to
		// win: ties go to the hardest mallet (smallest stored amplitudes).
		if cost < best.FitRMSdB-strikeTieDB {
			best = Strike{TcSec: tc, HardnessDBPerOct: h, GainDB: g, FitRMSdB: cost}
		}
	}
	return best
}

// weightedLine fits r ≈ g + h·u by weighted least squares and returns the
// weighted RMS residual. One distinct u (or one point) fits a constant.
func weightedLine(u, r, w []float64) (g, h, rmsErr float64) {
	var sw, su, sr, suu, sur float64
	for i := range u {
		sw += w[i]
		su += w[i] * u[i]
		sr += w[i] * r[i]
		suu += w[i] * u[i] * u[i]
		sur += w[i] * u[i] * r[i]
	}
	if sw == 0 {
		return 0, 0, 0
	}
	den := sw*suu - su*su
	if math.Abs(den) < 1e-12*sw*sw {
		g = sr / sw
	} else {
		h = (sw*sur - su*sr) / den
		g = (sr - h*su) / sw
	}
	var e float64
	for i := range u {
		d := r[i] - g - h*u[i]
		e += w[i] * d * d
	}
	return g, h, math.Sqrt(e / sw)
}
