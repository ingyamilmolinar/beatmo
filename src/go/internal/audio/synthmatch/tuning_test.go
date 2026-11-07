//go:build !test && !js

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

// tuning_test.go — does every pitched instrument actually SOUND at the pitch
// the score asked for?
//
// Node pitch 0 is A3 = 220 Hz, and a node at semitone p must sound at
// 220·2^((p+offset)/12) where offset is audio.InstrumentPitchInfo's answer for
// that instrument. Nothing enforced that end to end before 2026-09-12, and
// three defects had accumulated underneath it: voice-ahh/voice-opera sat ~65
// cents flat (an over-long pitch-envelope scoop), voice-soprano rendered a
// full octave up uncompensated, and the score importer octave-corrected
// double-bass for an offset it does not have. Green unit tests all round — the
// pitch was simply never measured off the rendered audio.
//
// These tests run against the REAL C DSP (render_prod.go, //go:build !test),
// so they execute under `make test-real` / a plain `go test`, not the stubbed
// fast path where RenderInstrument is a pure-Go approximation.

const a3Hz = 220.0

// foldCents folds a cents error into [-600,+600).
//
// This is the crux of measuring tuning honestly. YIN legitimately locks onto
// the 2nd harmonic whenever an instrument's fundamental is weak (a bassoon, a
// harpsichord, a trumpet), so a RAW f0 comparison reports those as an octave
// out when they are perfectly in tune — a false positive that would make this
// test useless. Folding sends any whole-octave disagreement to 0 and leaves
// exactly the fractional part, which is what a listener hears as dissonance.
// Register errors are covered separately and exactly by
// TestInstrumentPitchInfoFoldsRecipeDefaults in internal/audio, which reads
// the declared offsets rather than guessing them from a spectrum.
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

// inharmonicByDesign lists instruments whose perceived pitch legitimately
// departs from their nominal fundamental, with the reason. These are struck
// bars and bowls whose seeds deliberately let inharmonic partials dominate, so
// a fundamental-tracking detector cannot grade them.
var inharmonicByDesign = map[instrument.ID]string{
	roster.Kalimba: "osc_enabled:0 — the tine's measured inharmonic partials " +
		"(3.44/4.03/4.53x f0, gains up to 0.62) deliberately dominate the " +
		"Karplus-Strong pluck (0.4) that carries the fundamental",
	roster.SingingBowl: "gen1/gen2 sit at 2.34x/4.10x f0 — a real bowl's partials " +
		"are inharmonic, so its struck pitch is not its fundamental",
}

// detuneTolCents is the pass bar. A quarter tone is 50 cents and is unmistakably
// out of tune; 25 keeps a comfortable margin while leaving room for the vibrato
// and ensemble detune the melodic seeds intentionally carry.
const detuneTolCents = 25.0

// probePitches spans two octaves either side of the A3 origin, including
// non-octave intervals so a per-octave wrap cannot hide inside the fold.
var probePitches = []int{-12, -5, 0, 4, 7, 12}

// measureDetune renders inst at each probe pitch and returns the median folded
// cents error plus the number of confidently-measured pitches.
func measureDetune(t *testing.T, inst string) (median float64, measured int) {
	t.Helper()
	offset, _ := audio.InstrumentPitchInfo(instrument.ID(inst))
	var errs []float64
	for _, p := range probePitches {
		// The instrument's own declared offset is part of the contract: a
		// cello is SUPPOSED to sound an octave below node pitch 0.
		want := a3Hz * math.Pow(2, (float64(p)+offset)/12)
		w, err := RenderInstrument(instrument.ID(inst), float64(p), 44100, 1.5)
		if err != nil || w.Len() == 0 {
			continue
		}
		est := fingerprint.DetectF0Detailed(w)
		// This test treats the detected f0 as an ABSOLUTE frequency — it
		// compares it against a nominal Hz — so it must clear every gate that
		// qualifies one, not just confidence.
		//
		// Confidence alone is not enough, and the gap is not theoretical: YIN
		// searches down to fingerprint.F0SearchMinHz (50 Hz), so an instrument
		// whose fundamental is below that pins AT the floor and reports it with
		// confidence 1.000. kick-808 (44 Hz) and kick-punchy (45 Hz) then read
		// +221 and +182 cents sharp — which is exactly 1200*log2(50/f0), the
		// detector's floor rather than any property of the instrument. Three of
		// the six probe pitches sit below the floor for those two, so the median
		// lands on a pinned value and the test reports a tuning fault in a
		// perfectly in-tune kick.
		//
		// UsableAsPitch is the conjunction of all three gates; see its doc for
		// why none is redundant.
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

// TestEveryPitchedInstrumentSoundsInTune is the end-to-end guard: render the
// real DSP and measure, rather than trusting the declared params.
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
				// Not a silent pass: an instrument that yields no confident
				// f0 at any probe pitch cannot be graded, and if it is marked
				// pitched that is itself worth knowing.
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

// TestDetectF0IsAccurateOnKnownTones guards the MEASURING INSTRUMENT itself.
// Every tuning claim above rests on DetectF0, so a regression in the detector
// would silently turn this file into a rubber stamp. Synthetic tones of known
// frequency are the only ground truth that does not depend on the synth.
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
				// A harmonic stack, not a bare sine: a single sine cannot
				// expose the octave ambiguity that real timbres provoke.
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
