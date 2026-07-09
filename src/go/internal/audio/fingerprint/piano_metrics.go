package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// piano_metrics.go — piano-specific analysis. What makes a struck-string note
// read as a real PIANO rather than an electronic/organ tone:
//  1. INHARMONICITY — a real piano string is STIFF, so its partials are STRETCHED
//     sharp: f_n = n·f0·√(1 + B·n²). Exact integer harmonics (B=0) are the classic
//     synth/organ tell. B is the single strongest cue.
//  2. TWO-STAGE DECAY — the coupled unison strings give a fast "prompt" decay then
//     a slow "aftersound". A single exponential decay reads synthetic.
//  3. HAMMER ATTACK — a fast percussive strike with a brief broadband transient.
// These complement the general Fingerprint; use AnalyzePiano for a struck note.

const (
	pianoFFT       = 16384 // fine frequency resolution for measuring partial stretch
	pianoMaxN      = 12    // partials to fit B over
	pianoSpecSkipS = 0.06  // start the spectral window just after the strike transient
	pianoSpecLenS  = 0.5   // spectral window length
)

// PianoMetrics holds the struck-string descriptors.
type PianoMetrics struct {
	F0Hz           float64
	InharmonicityB float64   // stiff-string stretch coefficient (0 = exact harmonics / electronic)
	StretchCents   []float64 // per-partial stretch vs exact n·f0, in cents (index n-1)
	NumPartials    int
	DecayFastDBs   float64 // early decay rate dB/s (prompt sound)
	DecaySlowDBs   float64 // late decay rate dB/s (aftersound)
	TwoStageRatio  float64 // DecayFastDBs / DecaySlowDBs (>~1.5 = piano double-decay; ~1 = single exp)
	AttackMs       float64 // onset → envelope peak (hammer sharpness)
	CentroidHz     float64
}

// AnalyzePiano computes the struck-string metrics for a full note (attack +
// decay). f0 is the played fundamental; if ≤0 it is detected.
func AnalyzePiano(w wave.Wave, f0 float64) PianoMetrics {
	sr := w.SampleRate
	if sr <= 0 {
		sr = 44100
	}
	out := PianoMetrics{F0Hz: f0}
	if w.PeakSample() < 1e-9 {
		return out
	}
	w = PeakNormalize(w)
	x := w.Samples
	if f0 <= 0 {
		f0 = DetectF0(w)
		out.F0Hz = f0
	}
	if f0 <= 0 {
		return out
	}

	out.InharmonicityB, out.StretchCents, out.NumPartials, out.CentroidHz = pianoInharmonicity(x, sr, f0)
	out.DecayFastDBs, out.DecaySlowDBs, out.AttackMs = pianoDecay(x, sr)
	if out.DecaySlowDBs > 1e-6 {
		out.TwoStageRatio = out.DecayFastDBs / out.DecaySlowDBs
	}
	return out
}

// pianoInharmonicity fits the stiff-string stretch coefficient B from the
// measured partial frequencies over the post-attack spectral window. For each
// partial n, s_n = f_measured/(n·f0) and s_n² = 1 + B·n², so B is the
// amplitude-weighted least-squares slope of (s_n²−1) against n².
func pianoInharmonicity(x []float64, sr int, f0 float64) (B float64, stretch []float64, nUsed int, centroid float64) {
	a := secToSamples(pianoSpecSkipS, sr)
	b := a + secToSamples(pianoSpecLenS, sr)
	if b > len(x) {
		b = len(x)
	}
	if a >= b || b-a < 64 {
		a, b = 0, len(x)
	}
	mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: x[a:b], SampleRate: sr}, pianoFFT, wave.WindowHann)
	centroid = spectralCentroidHz(mag, binHz)
	// Candidate partials down to just under f0.
	peaks := peakPickPartials(mag, binHz, f0*0.75, 40)

	stretch = make([]float64, pianoMaxN)
	// Match each harmonic n to its strongest nearby peak.
	type match struct {
		n      int
		s, amp float64
	}
	var matches []match
	maxAmp := 0.0
	for n := 1; n <= pianoMaxN; n++ {
		target := float64(n) * f0
		band := target * (0.03 + 0.02*float64(n)) // widens with n for the stretch
		var best spectralPartial
		found := false
		for _, p := range peaks {
			if math.Abs(p.Freq-target) <= band && (!found || p.Amp > best.Amp) {
				best, found = p, true
			}
		}
		if !found || best.Amp <= 0 {
			continue
		}
		s := best.Freq / target
		stretch[n-1] = 1200 * math.Log2(s)
		matches = append(matches, match{n, s, best.Amp})
		if best.Amp > maxAmp {
			maxAmp = best.Amp
		}
	}
	// Origin-constrained weighted least-squares fit of y = B·n² (y = s²−1),
	// but ONLY over STRONG, non-outlier partials: a weak high partial is easily
	// mis-identified (grabs a neighbouring peak) and, weighted by n², would
	// dominate and corrupt B. Reject amp < 3% of the strongest partial and any
	// implausible stretch (>80 cents = a wrong peak, not string stiffness).
	var numB, denB float64
	for _, m := range matches {
		if m.amp < 0.03*maxAmp || math.Abs(stretch[m.n-1]) > 80 {
			continue
		}
		y := m.s*m.s - 1.0
		n2 := float64(m.n * m.n)
		numB += m.amp * y * n2
		denB += m.amp * n2 * n2
		nUsed++
	}
	if denB > 1e-12 {
		B = numB / denB
	}
	if B < 0 {
		B = 0
	}
	return B, stretch, nUsed, centroid
}

// pianoDecay measures the RMS-envelope decay in an early window (prompt sound)
// vs a late window (aftersound), and the attack time to the envelope peak.
func pianoDecay(x []float64, sr int) (fastDBs, slowDBs, attackMs float64) {
	win := secToSamples(0.02, sr)
	hop := secToSamples(0.005, sr)
	env := kickRMSEnvelope(x, win, hop)
	if len(env) < 8 {
		return 0, 0, 0
	}
	hopSec := float64(hop) / float64(sr)
	peakIdx, peak := 0, 0.0
	for i, e := range env {
		if e > peak {
			peak, peakIdx = e, i
		}
	}
	// attack: onset (2% of peak) → peak.
	startIdx := 0
	for i := 0; i <= peakIdx; i++ {
		if env[i] >= 0.02*peak {
			startIdx = i
			break
		}
	}
	attackMs = float64(peakIdx-startIdx) * hopSec * 1000

	// Early window: peak → +150 ms. Late window: +300 ms → +900 ms.
	slopeDBs := func(fromMs, toMs float64) float64 {
		i0 := peakIdx + int(fromMs/1000/hopSec)
		i1 := peakIdx + int(toMs/1000/hopSec)
		if i1 > len(env) {
			i1 = len(env)
		}
		if i1-i0 < 3 {
			return 0
		}
		// Least-squares slope of 20·log10(env) vs time.
		var st, sy, stt, sty float64
		n := 0.0
		for i := i0; i < i1; i++ {
			if env[i] <= 1e-9 {
				continue
			}
			t := float64(i-i0) * hopSec
			ydb := 20 * math.Log10(env[i])
			st += t
			sy += ydb
			stt += t * t
			sty += t * ydb
			n++
		}
		if n < 3 {
			return 0
		}
		denom := n*stt - st*st
		if math.Abs(denom) < 1e-12 {
			return 0
		}
		return -(n*sty - st*sy) / denom // positive dB/s of decay
	}
	fastDBs = slopeDBs(5, 150)
	slowDBs = slopeDBs(300, 900)
	return fastDBs, slowDBs, attackMs
}
