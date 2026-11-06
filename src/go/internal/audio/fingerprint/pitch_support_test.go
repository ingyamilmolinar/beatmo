package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// pitch_support_test.go — the pitched-vs-unpitched verdict against SYNTHETIC
// signals of known character.
//
// The roster tests (cmd/gen-fundamentals) check the verdict against the
// instruments that happen to ship today. These check it against signal classes
// chosen so that the right answer is known by construction, including the ones
// no shipped instrument produces: DC, impulses, chirps, clipping, denormals,
// NaN. A detector that is only exercised by its own corpus is only known to
// work on its own corpus.

const supportTestSR = 48000

// synth builds a wave from a per-sample function over dur seconds.
func synth(sr int, dur float64, f func(t float64) float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		s[i] = f(float64(i) / float64(sr))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// harmonics builds a tone at f0 with the given harmonic numbers and amplitudes.
// Passing numbers that skip 1 gives a missing-fundamental tone.
func harmonics(sr int, dur, f0 float64, numbers, amps []float64) wave.Wave {
	return synth(sr, dur, func(t float64) float64 {
		v := 0.0
		for i, k := range numbers {
			v += amps[i] * math.Sin(2*math.Pi*f0*k*t)
		}
		return v
	})
}

// TestAnalyzeF0Support_PitchedSignals: every signal here has a genuine pitch at
// 220 Hz, and the verdict must say so. The set walks from the easiest case to
// the ones that have historically fooled pitch detectors.
func TestAnalyzeF0Support_PitchedSignals(t *testing.T) {
	cases := []struct {
		name string
		w    wave.Wave
	}{
		{"pure sine", synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Sin(2 * math.Pi * 220 * t)
		})},
		{"full harmonic series", harmonics(supportTestSR, 1.5, 220,
			[]float64{1, 2, 3, 4, 5}, []float64{1, .6, .4, .25, .15})},
		{"missing fundamental (2,3,4)", harmonics(supportTestSR, 1.5, 220,
			[]float64{2, 3, 4}, []float64{1, .7, .5})},
		{"odd harmonics only (square-ish)", harmonics(supportTestSR, 1.5, 220,
			[]float64{1, 3, 5, 7}, []float64{1, .33, .2, .14})},
		{"inharmonic bell with fundamental present", harmonics(supportTestSR, 1.5, 220,
			[]float64{1, 2.76, 5.40, 8.93}, []float64{1, .8, .6, .4})},
		{"struck decay envelope", synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Exp(-3*t) * (math.Sin(2*math.Pi*220*t) + .5*math.Sin(2*math.Pi*440*t))
		})},
		{"vibrato (6 Hz, +/-30 cents)", synth(supportTestSR, 1.5, func(t float64) float64 {
			// Integrated phase, so the instantaneous frequency really is
			// modulated rather than the phase merely wobbling.
			return math.Sin(2*math.Pi*220*t + 0.03*math.Sin(2*math.Pi*6*t))
		})},
		{"hard-clipped sine", synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Max(-0.7, math.Min(0.7, 2*math.Sin(2*math.Pi*220*t)))
		})},
		{"quiet (-60 dBFS) sine", synth(supportTestSR, 1.5, func(t float64) float64 {
			return 0.001 * math.Sin(2*math.Pi*220*t)
		})},
		{"tone plus broadband noise at 0 dB SNR", func() wave.Wave {
			rng := rand.New(rand.NewSource(1))
			return synth(supportTestSR, 1.5, func(t float64) float64 {
				return math.Sin(2*math.Pi*220*t) + (rng.Float64()*2 - 1)
			})
		}()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			est := DetectF0Detailed(c.w)
			sup := AnalyzeF0Support(c.w, est.Hz)
			if !sup.Supported {
				t.Errorf("pitched signal reported UNSUPPORTED (f0=%.2f conf=%.3f lowestPartial=%.2fx)",
					est.Hz, est.Confidence, sup.LowestPartialRatio)
			}
			if sup.LowestPartialRatio <= 0 {
				t.Errorf("no significant partial found (f0=%.2f)", est.Hz)
			}
		})
	}
}

// TestAnalyzeF0Support_UnpitchedSignals: nothing here has a pitch, so whatever
// number the detector returns must not be usable as one. "cymbal-like" is the
// case this gate exists for — a dense inharmonic cluster high above whatever
// subharmonic the octave guard settles on.
func TestAnalyzeF0Support_UnpitchedSignals(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	cases := []struct {
		name string
		w    wave.Wave
	}{
		{"white noise", synth(supportTestSR, 1.5, func(float64) float64 {
			return rng.Float64()*2 - 1
		})},
		{"cymbal-like dense inharmonic cluster", harmonics(supportTestSR, 1.5, 100,
			// Every partial is 30x-70x the nominal f0: there is no energy
			// anywhere near 100 Hz, exactly the ride/crash shape.
			[]float64{31.7, 38.3, 44.1, 52.9, 61.3, 70.7},
			[]float64{1, .9, .85, .8, .7, .6})},
		{"band-limited high noise", func() wave.Wave {
			r2 := rand.New(rand.NewSource(11))
			prev := 0.0
			return synth(supportTestSR, 1.5, func(float64) float64 {
				x := r2.Float64()*2 - 1
				hp := x - prev // crude high-pass: emphasises the top octaves
				prev = x
				return hp
			})
		}()},
		{"impulse train with random spacing", func() wave.Wave {
			r3 := rand.New(rand.NewSource(13))
			next := 0
			i := 0
			return synth(supportTestSR, 1.5, func(float64) float64 {
				i++
				if i >= next {
					next = i + 200 + r3.Intn(3000)
					return 1
				}
				return 0
			})
		}()},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			est := DetectF0Detailed(c.w)
			usable, sup := UsableAsPitch(c.w, est)
			if usable {
				t.Errorf("unpitched signal reported USABLE as a pitch (f0=%.2f conf=%.3f lowestPartial=%.2fx)",
					est.Hz, est.Confidence, sup.LowestPartialRatio)
			}
		})
	}
}

// TestAnalyzeF0Support_DegenerateInputs: none of these may panic, and none may
// report a usable pitch. They are the inputs a caller reaches by accident —
// an empty buffer, a muted channel, a DC-offset track, a corrupted sample.
func TestAnalyzeF0Support_DegenerateInputs(t *testing.T) {
	cases := []struct {
		name string
		w    wave.Wave
		f0   float64
	}{
		{"nil samples", wave.Wave{Samples: nil, SampleRate: supportTestSR}, 220},
		{"empty samples", wave.Wave{Samples: []float64{}, SampleRate: supportTestSR}, 220},
		{"one sample", wave.Wave{Samples: []float64{1}, SampleRate: supportTestSR}, 220},
		{"three samples (below the transform minimum)",
			wave.Wave{Samples: []float64{1, -1, 1}, SampleRate: supportTestSR}, 220},
		{"zero sample rate", wave.Wave{Samples: make([]float64, 4096), SampleRate: 0}, 220},
		{"digital silence", synth(supportTestSR, 1.5, func(float64) float64 { return 0 }), 220},
		{"pure DC", synth(supportTestSR, 1.5, func(float64) float64 { return 0.5 }), 220},
		{"single impulse", func() wave.Wave {
			w := synth(supportTestSR, 1.5, func(float64) float64 { return 0 })
			w.Samples[0] = 1
			return w
		}(), 220},
		{"NaN samples", synth(supportTestSR, 0.5, func(float64) float64 { return math.NaN() }), 220},
		{"Inf samples", synth(supportTestSR, 0.5, func(float64) float64 { return math.Inf(1) }), 220},
		{"denormal-magnitude samples", synth(supportTestSR, 0.5, func(t float64) float64 {
			return 1e-300 * math.Sin(2*math.Pi*220*t)
		}), 220},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// A panic here is the failure; the assertions are the contract.
			_ = AnalyzeF0Support(c.w, c.f0)
			est := DetectF0Detailed(c.w)
			if usable, sup := UsableAsPitch(c.w, est); usable {
				t.Errorf("degenerate input reported usable as a pitch (f0=%.2f lowest=%.2fx)",
					est.Hz, sup.LowestPartialRatio)
			}
		})
	}
}

// TestUsableAsPitch_GatesAreIndependent proves each of the three gates catches
// something the others do not, so none can be dropped as redundant. Each case
// names the ONE gate that must reject it.
func TestUsableAsPitch_GatesAreIndependent(t *testing.T) {
	// Reliable() rejects this, support does not: white noise still peak-picks
	// partials, and its lowest can land inside the ratio bound by chance.
	noise := func() wave.Wave {
		rng := rand.New(rand.NewSource(3))
		return synth(supportTestSR, 1.5, func(float64) float64 { return rng.Float64()*2 - 1 })
	}()
	if est := DetectF0Detailed(noise); est.Reliable() {
		t.Errorf("white noise passed Reliable() at confidence %.3f", est.Confidence)
	}

	// F0AtSearchFloor() rejects this, and NEITHER other gate does: a 30 Hz tone
	// is perfectly periodic and perfectly supported — it is simply below what
	// the detector can see, so it pins at the floor and reports ~50 Hz with
	// high confidence. This is the gate that is easiest to believe redundant.
	low := synth(supportTestSR, 1.5, func(t float64) float64 {
		return math.Sin(2*math.Pi*30*t) + 0.5*math.Sin(2*math.Pi*60*t)
	})
	lowEst := DetectF0Detailed(low)
	if !lowEst.F0AtSearchFloor() {
		t.Errorf("a 30 Hz tone did not trip F0AtSearchFloor (reported %.2f Hz)", lowEst.Hz)
	}
	if usable, _ := UsableAsPitch(low, lowEst); usable {
		t.Errorf("a sub-floor tone was reported usable at %.2f Hz", lowEst.Hz)
	}

	// AnalyzeF0Support() rejects this, and neither other gate does: a dense
	// inharmonic cluster is periodic enough for YIN and sits well inside the
	// search range. This is the cymbal case.
	cymbal := harmonics(supportTestSR, 1.5, 100,
		[]float64{31.7, 38.3, 44.1, 52.9, 61.3, 70.7},
		[]float64{1, .9, .85, .8, .7, .6})
	cEst := DetectF0Detailed(cymbal)
	cSup := AnalyzeF0Support(cymbal, cEst.Hz)
	if cSup.Supported {
		t.Errorf("a dense inharmonic cluster was reported spectrally supported (lowest partial %.2fx)",
			cSup.LowestPartialRatio)
	}
}

// TestAnalyzeF0Support_UsesTheSustainWindow pins that support is measured over
// the same window the f0 came from.
//
// wave.MagnitudeSpectrum reads from the START of whatever it is handed, so a
// naive implementation measures the ATTACK — which for a struck instrument is
// broadband noise that looks nothing like the tone that follows. This builds
// exactly that signal: a noise burst, then a clean 220 Hz tone. Judged on its
// attack it has no pitch; judged on its sustain it plainly does.
func TestAnalyzeF0Support_UsesTheSustainWindow(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	attackSec := SustainSkipSec * 0.9
	w := synth(supportTestSR, 2.0, func(t float64) float64 {
		if t < attackSec {
			return rng.Float64()*2 - 1
		}
		return math.Sin(2 * math.Pi * 220 * t)
	})

	sup := AnalyzeF0Support(w, 220)
	if !sup.Supported {
		t.Errorf("a noise attack followed by a clean tone was reported unsupported "+
			"(lowest partial %.2fx) — support is being measured over the attack, not the sustain",
			sup.LowestPartialRatio)
	}
}

// TestAnalyzeF0Support_SeriesDeviationIsReportedNotGated pins the design
// decision recorded in the file doc: struck metal is genuinely inharmonic, so
// SeriesDeviation cannot separate a glockenspiel from a cymbal and must not be
// part of the verdict. A bell-like tone WITH its fundamental present has a high
// deviation and must still be supported.
func TestAnalyzeF0Support_SeriesDeviationIsReportedNotGated(t *testing.T) {
	bell := harmonics(supportTestSR, 1.5, 220,
		[]float64{1, 5.4, 10.4}, []float64{1, .8, .6}) // glockenspiel-shaped
	sup := AnalyzeF0Support(bell, 220)
	if !sup.Supported {
		t.Fatalf("an inharmonic bell with its fundamental present was rejected (lowest %.2fx)",
			sup.LowestPartialRatio)
	}
	if sup.SeriesDeviation <= 0.5 {
		t.Errorf("SeriesDeviation = %.3f, want a high value for a bell — the field is not being reported",
			sup.SeriesDeviation)
	}
}

// TestAnalyzeF0Support_RatioScalesWithF0 checks the measure is a RATIO and not
// an absolute frequency: the same spectral shape at two different pitches must
// give the same verdict and the same lowest-partial ratio.
func TestAnalyzeF0Support_RatioScalesWithF0(t *testing.T) {
	for _, f0 := range []float64{110, 220, 440, 880} {
		w := harmonics(supportTestSR, 1.5, f0, []float64{2, 3, 4}, []float64{1, .7, .5})
		sup := AnalyzeF0Support(w, f0)
		if !sup.Supported {
			t.Errorf("f0=%.0f: missing-fundamental tone unsupported (lowest %.2fx)", f0, sup.LowestPartialRatio)
		}
		if math.Abs(sup.LowestPartialRatio-2) > 0.1 {
			t.Errorf("f0=%.0f: lowest partial ratio = %.2f, want ~2 (the ratio must not depend on absolute pitch)",
				f0, sup.LowestPartialRatio)
		}
	}
}

// TestAnalyzeF0Support_RejectsDegenerateF0Argument covers the other half of the
// input space: a fine signal with a nonsense f0 handed in.
//
// It is kept apart from the degenerate-WAVE table deliberately. Those rows
// assert that UsableAsPitch says no, which re-detects f0 from the wave — a
// correct answer of "yes" for a clean 220 Hz sine, whatever f0 the caller
// passed alongside it. Mixing the two made this test fail for a reason that had
// nothing to do with the code.
func TestAnalyzeF0Support_RejectsDegenerateF0Argument(t *testing.T) {
	good := synth(supportTestSR, 1.5, func(t float64) float64 {
		return math.Sin(2 * math.Pi * 220 * t)
	})
	for _, f0 := range []float64{0, -1, -220, math.NaN(), math.Inf(1), math.Inf(-1)} {
		sup := AnalyzeF0Support(good, f0)
		if sup.Supported {
			t.Errorf("f0=%v reported supported on a clean tone: %+v", f0, sup)
		}
	}

	// A f0 the signal does not contain must also be rejected, even though it is
	// a perfectly ordinary number: 7000 Hz is nowhere in a 220 Hz sine.
	if sup := AnalyzeF0Support(good, 7000); sup.Supported {
		t.Errorf("an f0 absent from the signal reported supported: %+v", sup)
	}
}
