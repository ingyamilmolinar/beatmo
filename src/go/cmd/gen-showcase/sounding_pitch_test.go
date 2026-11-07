//go:build !test

package main

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Template node → audio hertz, the only check spanning the whole pipeline (the symbolic and per-instrument guards each
// cover half). !test: the stubbed render would prove nothing about what ships.

// Silences the PERIODIC pitch modulation (vibrato, pitch LFO, unison spread) so the note's centre is measured, not its
// excursion. The pitch ENVELOPE stays on: a one-way offset is exactly a mistuning this test must see.
var pitchCenterOverrides = map[string]float64{
	"lfo_enabled":        0,
	"ens_vib_depth":      0,
	"ens_scatter":        0,
	"ens_humanize":       0,
	"unison_detune":      0,
	"unison_drift_depth": 0,
}

// A quarter tone is unmistakably wrong; 25 leaves room for intentional vibrato / ensemble detune.
const soundingTolCents = 25.0

// Folded so YIN's octave ambiguity on weak-fundamental timbres cannot pass for a tuning fault; whole-octave placement
// is checked exactly and symbolically elsewhere.
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

// TestTemplateNodesSoundAtTheirNotatedPitch renders each committed template's
// distinct (instrument, node pitch) pairs through the real DSP and measures the
// result.
func TestTemplateNodesSoundAtTheirNotatedPitch(t *testing.T) {
	if testing.Short() {
		t.Skip("renders every template's distinct voices through the C DSP")
	}
	for _, spec := range templateSpecs() {
		spec := spec
		t.Run(spec.Stem, func(t *testing.T) {
			d := readTemplate(t, spec.Stem)
			if len(d.Instruments) == 0 || len(d.Nodes) == 0 {
				t.Fatalf("template %s has no instruments or nodes", spec.Stem)
			}

			// A node belongs to the instrument whose origin most recently
			// precedes it — the same row layout the exporter writes.
			rowOf := func(nodeIdx int) int {
				row := 0
				for i, in := range d.Instruments {
					if nodeIdx >= in.Origin {
						row = i
					}
				}
				return row
			}

			type voice struct {
				inst  instrument.ID
				pitch float64
			}
			seen := map[voice]bool{}
			checked := 0
			for i, n := range d.Nodes {
				if n.Type != "regular" {
					continue
				}
				v := voice{d.Instruments[rowOf(i)].ID, n.Pitch}
				if seen[v] {
					continue
				}
				seen[v] = true

				offset, pitched := audio.InstrumentPitchInfo(v.inst)
				if !pitched {
					continue // percussion carries no note to verify
				}
				// Keyed by BASE id, so a template's "voice-ahh--2" is exempt too.
				if _, inharmonic := inharmonicByDesignTemplates[v.inst.Base()]; inharmonic {
					continue
				}
				// What the template + engine together promise this node sounds.
				want := 220 * math.Pow(2, (v.pitch+offset)/12)
				w, err := renderAsPlayed(v.inst, v.pitch)
				if err != nil || w.Len() == 0 {
					t.Errorf("%s pitch %+.0f: render failed: %v", v.inst, v.pitch, err)
					continue
				}
				est := fingerprint.DetectF0Detailed(w)
				// est.Hz is used as an ABSOLUTE frequency, so it must clear every gate (UsableAsPitch): the low piano notes sit below
				// F0SearchMinHz, where YIN pins at 50.00 Hz with full confidence. An unmeasurable note is skipped, not guessed.
				if usable, _ := fingerprint.UsableAsPitch(w, est); !usable || est.Confidence < 0.5 {
					continue // no trustworthy f0 to grade (short/noisy voice)
				}
				checked++
				if cents := foldCents(1200 * math.Log2(est.Hz/want)); math.Abs(cents) > soundingTolCents {
					t.Errorf("%s node pitch %+.0f sounds %+.1f cents off: measured %.2f Hz, "+
						"notated %.2f Hz. The circuit does not play the note the score wrote.",
						v.inst, v.pitch, cents, est.Hz, want)
				}
			}
			if checked == 0 {
				t.Errorf("no pitched voice could be measured — the test proved nothing")
			}
		})
	}
}

// renderAsPlayed: a melodic voice is rendered AT the node pitch; every other voice once and resampled by 2^(pitch/12),
// so tom heights and timpani sound pitched only through the resampling.
func renderAsPlayed(inst instrument.ID, pitch float64) (wave.Wave, error) {
	if audio.IsMelodicInstrument(inst) {
		return synthmatch.RenderWithParams(inst, pitch, 44100, 1.5, pitchCenterOverrides)
	}
	w, err := synthmatch.RenderWithParams(inst, 0, 44100, 1.5, pitchCenterOverrides)
	if err != nil {
		return w, err
	}
	step := math.Pow(2, pitch/12)
	var out []float64
	for pos := 0.0; ; pos += step {
		i := int(pos)
		if i+1 >= len(w.Samples) {
			break
		}
		f := pos - float64(i)
		out = append(out, w.Samples[i]*(1-f)+w.Samples[i+1]*f)
	}
	w.Samples = out
	return w, nil
}

// Voices whose design lets a partial other than the fundamental dominate. An entry asserts the detector cannot READ the
// pitch, never that it may be out of tune: measure the spectrum at the notated frequency first, and if the fundamental
// is ABSENT the voice is mistuned (a bug, not an entry).
var inharmonicByDesignTemplates = map[instrument.ID]string{
	roster.Kalimba:     "inharmonic tine partials dominate the fundamental by design",
	roster.SingingBowl: "bowl partials sit at 2.34x/4.10x f0 by design",
	// Measured: the fundamental is present ~8-14 dB under the 3rd harmonic, which YIN reports (a sung "ahh"'s first
	// formant). A twelfth folds to -498 cents, not 0, and widening the fold would swallow a real mode lock a twelfth high.
	roster.VoiceAhh: "sung-vowel formant puts the 3rd harmonic above the fundamental " +
		"(present at -8.2 dB); YIN reports 3x f0",
	// Measured: the fundamental is present 4 dB under the 3rd partial, which YIN reports (a twelfth) at every pitch.
	roster.Accordion: "reed timbre puts the 3rd harmonic 4 dB above the fundamental " +
		"(present); YIN reports 3x f0",
	// This test's zeroed unison_detune phase-combs the stacked saws, which YIN reads at 3.5 x f0; the spectrum peaks AT the
	// fundamental, and unmodified the voice passes TestEveryPitchedInstrumentSoundsInTune.
	roster.LeadSupersaw: "zeroed unison detune phase-combs the stacked saws; YIN reads " +
		"3.5x f0 while the spectrum peaks at the notated fundamental",
}
