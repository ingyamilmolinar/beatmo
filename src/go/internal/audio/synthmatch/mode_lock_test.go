//go:build !test

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

// A self-oscillating waveguide can lock onto a higher mode, an exact integer multiple of its fundamental, which the
// in-tune test cannot see (it folds octaves and takes a median). So this asks a spectral question instead of
// comparing frequencies: is there real energy AT the notated fundamental?

// Near 1: the fundamental is present and detHz is a harmonic the detector preferred; near 0: the notated pitch is not
// in the audio.
func fundamentalEnergyRatio(w wave.Wave, want, detHz float64, sr int) float64 {
	best := binEnergy(w, want, sr)
	// Any harmonic of want below detHz is equally good evidence.
	for h := 2.0; want*h < detHz-1; h++ {
		if e := binEnergy(w, want*h, sr); e > best {
			best = e
		}
	}
	return best / math.Max(binEnergy(w, detHz, sr), 1e-12)
}

// Hann-windowed so a loud neighbour does not leak into a quiet bin.
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

// Only a nonlinear self-oscillator CAN mode-lock; a static oscillator bank plays whatever it is handed.
var selfOscillating = []instrument.ID{roster.Violin, roster.Viola, roster.Cello, roster.CelloWarm, roster.DoubleBass}

// Sits in the empty gap between a present fundamental (0.3-1.0+) and a locked one (0.03-0.17).
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
				// Anything but an integer multiple is ordinary mistuning, which the cents guard owns.
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
