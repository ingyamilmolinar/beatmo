//go:build !test && !js

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

// sounding_pitch_test.go closes the loop the other guards each cover half of.
//
//	TestTemplateIsDerivedFromScore   score  -> template node pitch   (symbolic)
//	TestEveryPitchedInstrumentSoundsInTune  instrument -> audio      (acoustic)
//	THIS FILE                        template node -> AUDIO HERTZ    (both)
//
// Reading a committed template off disk and measuring what it actually sounds
// is the only check that spans the whole pipeline, and it is the one that was
// missing while three separate pitch defects shipped green: a contrabass
// octave-corrected for an offset its instrument does not have, a soprano whose
// real +1 octave was corrected nowhere (Ave Maria's vocal line topped out at
// F6), and two voices sitting ~65 cents flat.
//
// Requires the real C DSP, hence //go:build !test — under the stubbed fast path
// RenderInstrument is a pure-Go approximation and would prove nothing about
// what ships.

// pitchCenterOverrides silence the PERIODIC pitch modulation — vibrato, the
// pitch LFO, and the unison/ensemble spread — so what is measured is the pitch
// the note is CENTRED on rather than wherever its vibrato happened to be when
// the window closed. The melodic seeds carry up to 30 cents of vibrato depth
// (ens_vib_depth) plus a deep LFO, which showed up as ±40-cent readings that
// swung both ways from note to note: excursion, not mistuning.
//
// The pitch ENVELOPE is deliberately left ON. It is a one-way offset, not an
// oscillation, and it is exactly how voice-ahh/voice-opera came to sit ~65
// cents flat (pitchenv_amt -2.0 held over a 0.7 s decay). Disabling it here
// would make this test blind to the very defect it exists to catch.
var pitchCenterOverrides = map[string]float64{
	"lfo_enabled":        0,
	"ens_vib_depth":      0,
	"ens_scatter":        0,
	"ens_humanize":       0,
	"unison_detune":      0,
	"unison_drift_depth": 0,
}

// soundingTolCents matches synthmatch's bar: a quarter tone is 50 cents and is
// unmistakably wrong, so 25 leaves room for intentional vibrato/ensemble
// detune while still catching a real mistuning.
const soundingTolCents = 25.0

// foldCents folds a cents error into [-600,+600) so YIN's legitimate
// octave ambiguity on weak-fundamental timbres (bassoon, harpsichord) cannot
// masquerade as a tuning fault. Whole-octave placement is verified
// symbolically by TestTemplateIsDerivedFromScore instead, which is exact.
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
				// Keyed by BASE id: a template's second copy of an instrument
				// is minted "voice-ahh--2", and an exemption that only matched
				// the plain id would silently not apply to it -- the same
				// instance blindness that made the retired recipe lookup answer ""
				// here until it learned to strip the suffix.
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
				// This comparison treats est.Hz as an ABSOLUTE frequency, so it
				// must clear every gate that qualifies one rather than
				// confidence alone -- see fingerprint.UsableAsPitch, which
				// spells out why none of the three is redundant.
				//
				// Confidence does not cover the detector's LOWER SEARCH BOUND,
				// and the gap is live here: the low piano notes these templates
				// write (piano-grand at node pitch -26..-28, notated 49.0/46.3/
				// 43.7 Hz) sit below fingerprint.F0SearchMinHz, so YIN pins them
				// at exactly 50.00 Hz and reports it with full confidence. The
				// test then blamed the circuit for a note the detector simply
				// cannot see. An unmeasurable note is skipped, not guessed at;
				// TestTemplateIsDerivedFromScore verifies those pitches
				// symbolically and exactly.
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

// renderAsPlayed renders a node the way the engine plays it
// (playParamsIdentity): a pitch-aware (melodic) recipe is re-rendered AT the
// node pitch; every other voice — drums, FM — is rendered once and resampled
// by 2^(pitch/12). A drum row's tom heights and a timpani part on a drum voice
// therefore only sound pitched through the resampling, and rendering them at
// pitch instead measured an unmoved drum.
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

// inharmonicByDesignTemplates mirrors synthmatch's exemption list: voices whose
// seeds deliberately let a partial other than the fundamental dominate, so a
// fundamental-tracking detector cannot grade them. Kept as its own map (rather
// than exported across packages) so adding an exemption here is a visible,
// justified edit in the file that depends on it.
//
// An entry here asserts the detector cannot READ the pitch, never that the
// instrument may be out of tune. Before adding one, measure the spectrum at the
// notated frequency: if the fundamental is ABSENT the voice really is mistuned
// and belongs in a bug, not in this map. That is the same discriminator that
// separated the real bowed-string mode lock from the guitar/organ/harpsichord
// false alarms.
var inharmonicByDesignTemplates = map[instrument.ID]string{
	roster.Kalimba:     "inharmonic tine partials dominate the fundamental by design",
	roster.SingingBowl: "bowl partials sit at 2.34x/4.10x f0 by design",
	// Measured, not assumed: rendering voice-ahh at node pitch -9 (notated
	// 130.81 Hz) puts real energy AT 130.81 Hz, 8.2 dB below the third
	// harmonic, which is the loudest partial and what YIN therefore reports
	// (392.46 Hz = exactly 3x130.81, a harmonic series built on the correct
	// fundamental). At node pitch +3 the same voice reads 784.84 = 3x261.63
	// with the fundamental 14.3 dB down. A sung "ahh" is formant-shaped, so
	// its first formant sitting on the third partial is the instrument working
	// as designed -- the voice is IN TUNE and the detector simply cannot say so.
	//
	// foldCents cannot absorb this the way it absorbs the bassoon/harpsichord
	// octave ambiguity: a twelfth is 1902 cents, which folds to -498, not 0.
	// Widening the fold to arbitrary integer ratios would also swallow a
	// genuine mode lock a twelfth high -- exactly the bowed-string bug this
	// suite is meant to catch -- so the per-instrument exemption is the only
	// honest place to record it.
	roster.VoiceAhh: "sung-vowel formant puts the 3rd harmonic above the fundamental " +
		"(present at -8.2 dB); YIN reports 3x f0",
	// Measured 2026-10-02, the same way: at node pitch +12 (notated 440 Hz) the
	// accordion's partials read 63.5 / 58.3 / 67.5 / 54.4 / 51.9 dB at 1x..5x —
	// the fundamental is present, 4.0 dB under the third partial, and YIN
	// reports 1320 Hz = 3 x 440 (a twelfth: -498 cents after folding). The
	// same ratio holds at node pitch 0. A reed timbre with a strong third
	// partial, in tune; it entered the templates with the vallenato and
	// merengue grooves.
	roster.Accordion: "reed timbre puts the 3rd harmonic 4 dB above the fundamental " +
		"(present); YIN reports 3x f0",
	// Tuned cello: the fundamental is present at every template pitch, the 3rd
	// partial 7-17 dB above it, no sub-octave (<= -40 dB); YIN reads exactly 3x.
	roster.Cello: "bowed body resonance puts the 3rd harmonic 7-17 dB above the " +
		"fundamental (present); YIN reports 3x f0",
	// A detector artifact of THIS test's settings, not of the voice:
	// pitchCenterOverrides zeroes unison_detune, which leaves a supersaw's
	// stacked saws in a fixed phase relation — a comb YIN reads at exactly
	// 3.5 x f0. The spectrum at node pitch +12 (440 Hz) peaks AT the notated
	// fundamental (52.2 dB; 880 Hz 46.2, 1320 Hz 42.7, and 1540 Hz, where YIN
	// claims the pitch, -5.1 dB). Unmodified, the voice passes
	// TestEveryPitchedInstrumentSoundsInTune.
	roster.LeadSupersaw: "zeroed unison detune phase-combs the stacked saws; YIN reads " +
		"3.5x f0 while the spectrum peaks at the notated fundamental",
}
