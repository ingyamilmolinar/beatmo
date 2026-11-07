//go:build !test

package synthmatch

import (
	"math"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// A node at semitone p must sound at 220·2^((p+offset)/12), measured off the rendered audio of the REAL C DSP: green
// unit tests alone left voices 65 cents flat and an octave up.

const a3Hz = 220.0

// YIN rightly locks onto the 2nd harmonic of a weak fundamental, so whole octaves fold to 0 and only the fractional
// part, what a listener hears as dissonance, remains. Register is checked exactly from the declared offsets.
func foldCents(c float64) float64 {
	f := math.Mod(c, 1200)
	if f >= 600 {
		f -= 1200
	}
	if f < -600 {
		f += 1200
	}
	return f
}

// Struck bars and bowls whose inharmonic partials dominate by design, so a fundamental tracker cannot grade them.
var inharmonicByDesign = map[instrument.ID]string{
	roster.Kalimba: "osc_enabled:0 — the tine's measured inharmonic partials " +
		"(3.44/4.03/4.53x f0, gains up to 0.62) deliberately dominate the " +
		"Karplus-Strong pluck (0.4) that carries the fundamental",
	roster.SingingBowl: "gen1/gen2 sit at 2.34x/4.10x f0 — a real bowl's partials " +
		"are inharmonic, so its struck pitch is not its fundamental",
}

// A quarter tone (50) is unmistakably out; 25 leaves room for intended vibrato and ensemble detune.
const detuneTolCents = 25.0

// Non-octave intervals too, so a per-octave wrap cannot hide inside the fold.
var probePitches = []int{-12, -5, 0, 4, 7, 12}

func measureDetune(t *testing.T, inst string) (median float64, measured int) {
	t.Helper()
	offset, _ := audio.InstrumentPitchInfo(instrument.ID(inst))
	var errs []float64
	for _, p := range probePitches {
		// A cello is SUPPOSED to sound an octave below node pitch 0.
		want := a3Hz * math.Pow(2, (float64(p)+offset)/12)
		w, err := RenderInstrument(instrument.ID(inst), float64(p), 44100, 1.5)
		if err != nil || w.Len() == 0 {
			continue
		}
		est := fingerprint.DetectF0Detailed(w)
		// An ABSOLUTE frequency must clear every gate, not just confidence: below YIN's 50 Hz floor a kick pins at the floor at
		// confidence 1.0 and would read as a tuning fault.
		if usable, _ := fingerprint.UsableAsPitch(w, est); !usable || est.Confidence < 0.5 {
			continue
		}
		errs = append(errs, foldCents(1200*math.Log2(est.Hz/want)))
	}
	if len(errs) == 0 {
		return math.NaN(), 0
	}
	sort.Float64s(errs)
	return errs[len(errs)/2], len(errs)
}

// Measures the real DSP rather than trusting the declared params.
func TestEveryPitchedInstrumentSoundsInTune(t *testing.T) {
	if testing.Short() {
		t.Skip("renders ~90 instruments through the C DSP; skipped under -short")
	}
	for _, inst := range audio.Instruments() {
		if _, pitched := audio.InstrumentPitchInfo(inst); !pitched {
			continue
		}
		inst := inst
		t.Run(string(inst), func(t *testing.T) {
			med, n := measureDetune(t, string(inst))
			if n == 0 {
				// Not a silent pass: a pitched instrument that cannot be graded is worth knowing.
				t.Skipf("no confident f0 at any probe pitch — cannot grade tuning")
			}
			if why, ok := inharmonicByDesign[inst]; ok {
				t.Logf("inharmonic by design (%+.0f cents off nominal): %s", med, why)
				return
			}
			if math.Abs(med) > detuneTolCents {
				t.Errorf("sounds %+.1f cents from nominal (median over %d pitches); "+
					"tolerance is %.0f. The node pitch a score asks for is not the "+
					"pitch this instrument produces, so it will beat against every "+
					"in-tune instrument in the circuit.", med, n, detuneTolCents)
			}
		})
	}
}

// Every tuning claim rests on DetectF0; tones of known frequency are the only ground truth independent of the synth.
func TestDetectF0IsAccurateOnKnownTones(t *testing.T) {
	const sr = 44100
	for _, harmonics := range []int{1, 5, 12} {
		for semis := -24; semis <= 24; semis += 3 {
			want := a3Hz * math.Pow(2, float64(semis)/12)
			n := int(float64(sr) * 1.5)
			s := make([]float64, n)
			for i := range s {
				tt := float64(i) / sr
				v := 0.0
				// A bare sine cannot expose the octave ambiguity real timbres provoke.
				for h := 1; h <= harmonics; h++ {
					v += math.Sin(2*math.Pi*want*float64(h)*tt) / float64(h)
				}
				s[i] = v * 0.5
			}
			got := fingerprint.DetectF0(wave.Wave{Samples: s, SampleRate: sr})
			if got <= 0 {
				t.Errorf("harmonics=%d semis=%+d: no f0 detected for a pure %.2f Hz tone", harmonics, semis, want)
				continue
			}
			if cents := math.Abs(1200 * math.Log2(got/want)); cents > 15 {
				t.Errorf("harmonics=%d semis=%+d: detected %.2f Hz for a %.2f Hz tone (%.1f cents off)",
					harmonics, semis, got, want, cents)
			}
		}
	}
}
