//go:build !test && !js

package synthmatch

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// mode_lock_test.go — does a self-oscillating voice sound its FUNDAMENTAL, or
// has it locked onto a higher mode of its own waveguide?
//
// This is the one pitch fault TestEveryPitchedInstrumentSoundsInTune above
// cannot see, by construction:
//
//   - its foldCents folds by 1200, so a note sounding EXACTLY an octave high
//     folds to 0 cents and reads as perfect;
//   - it takes the MEDIAN over six probe pitches, so a fault that affects a
//     minority of the range is voted away.
//
// Both were load-bearing. The bowed strings (osc_type 7) used to lock onto
// their 2nd/3rd mode over wide contiguous bands — cello a twelfth high across
// node pitch +1..+6, viola/violin/double-bass across -11..-7 — while that test
// stayed green, because a waveguide's higher modes are exact integer multiples
// of the fundamental and so are invisible to a cents comparison that folds
// octaves and then takes a median. The circuits built from a score leapt
// register mid-phrase, which is what made them sound dissonant rather than
// merely mistuned. Root cause was the bow ramp-in: a nonlinear stick-slip
// oscillator brought up to speed too fast settles on a higher limit cycle
// (see render_bowed_string in src/c/modular.c).
//
// The test does NOT compare frequencies, precisely because an integer-multiple
// error is what it is hunting. It asks a spectral question instead: is there
// real energy AT the notated fundamental? A detector latching a harmonic (which
// YIN legitimately does on weak-fundamental timbres) leaves the fundamental in
// the spectrum; a genuine mode lock does not.

// fundamentalEnergyRatio returns the energy at the notated fundamental (or the
// harmonics between it and detHz) relative to the energy at detHz. Near 1 means
// the fundamental is present and detHz is merely a harmonic the detector
// preferred; near 0 means the notated pitch is simply not in the audio.
func fundamentalEnergyRatio(w wave.Wave, want, detHz float64, sr int) float64 {
	best := binEnergy(w, want, sr)
	// Any harmonic of `want` strictly below detHz is equally good evidence that
	// `want` is the true fundamental (detHz being one of its harmonics).
	for h := 2.0; want*h < detHz-1; h++ {
		if e := binEnergy(w, want*h, sr); e > best {
			best = e
		}
	}
	return best / math.Max(binEnergy(w, detHz, sr), 1e-12)
}

// binEnergy is a Hann-windowed single-bin DFT over a steady-state segment,
// which keeps spectral leakage from a loud neighbour out of a quiet bin.
func binEnergy(w wave.Wave, f float64, sr int) float64 {
	s := w.Samples
	lo := len(s) / 5
	hi := lo + sr/2
	if hi > len(s) {
		hi = len(s)
	}
	if hi-lo < 1024 {
		return 0
	}
	s = s[lo:hi]
	n := len(s)
	var re, im float64
	for i, v := range s {
		win := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		a := 2 * math.Pi * f * float64(i) / float64(sr)
		re += v * win * math.Cos(a)
		im += v * win * math.Sin(a)
	}
	return math.Sqrt(re*re+im*im) / float64(n)
}

// selfOscillating lists the voices driven by a nonlinear self-oscillating
// physical model, which are the ones that CAN mode-lock. A static oscillator
// bank cannot: it plays whatever frequency it is handed.
var selfOscillating = []instrument.ID{roster.Violin, roster.Viola, roster.Cello, roster.CelloWarm, roster.DoubleBass}

// fundamentalPresenceFloor: below this the notated pitch carries no more energy
// than the noise floor. Measured spread when the fix is in place is 0.3-1.0+;
// when mode-locked it was 0.03-0.17. 0.25 sits in the empty gap between them.
const fundamentalPresenceFloor = 0.25

func TestSelfOscillatingVoicesHoldTheirFundamental(t *testing.T) {
	if testing.Short() {
		t.Skip("renders the physical-model voices across their range through the C DSP")
	}
	const sr = 44100
	for _, inst := range selfOscillating {
		inst := inst
		t.Run(string(inst), func(t *testing.T) {
			offset, pitched := audio.InstrumentPitchInfo(inst)
			if !pitched {
				t.Fatalf("%s is no longer pitched — update selfOscillating", inst)
			}
			checked := 0
			for p := -20; p <= 28; p++ {
				want := 220 * math.Pow(2, (float64(p)+offset)/12)
				w, err := RenderInstrument(inst, float64(p), sr, 1.2)
				if err != nil || w.Len() == 0 {
					continue
				}
				est := fingerprint.DetectF0Detailed(w)
				if est.Hz <= 0 || est.Confidence < 0.5 {
					continue
				}
				// Only an integer-multiple reading can be a mode lock; anything
				// else is ordinary mistuning, which the cents guard owns.
				m := est.Hz / want
				mi := math.Round(m)
				if mi < 2 || math.Abs(1200*math.Log2(m/mi)) > 60 {
					continue
				}
				checked++
				if rel := fundamentalEnergyRatio(w, want, est.Hz, sr); rel < fundamentalPresenceFloor {
					t.Errorf("node pitch %+d: locked onto mode x%.0f — notated %.2f Hz carries "+
						"only %.0f%% of the energy at the sounding %.2f Hz, so the fundamental "+
						"is absent, not merely weak. The voice plays a harmonic of its note.",
						p, mi, want, rel*100, est.Hz)
				}
			}
			t.Logf("%s: %d integer-multiple readings examined", inst, checked)
		})
	}
}
