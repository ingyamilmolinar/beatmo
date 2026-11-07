package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// What reads as a real piano: inharmonicity (stiff strings stretch partials sharp, f_n = n·f0·√(1 + B·n²); B=0 is the
// synth/organ tell), a two-stage decay (prompt sound then aftersound) and a hammer attack.

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

// f0 ≤ 0 is detected.
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

// s_n² = 1 + B·n², so B is the amplitude-weighted least-squares slope of (s_n² − 1) on n².
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
	// Only STRONG, plausible partials: a weak high partial grabs a neighbouring peak and, weighted by n², would corrupt B.
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

// Early (prompt) vs late (aftersound) RMS decay, plus the attack time.
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
