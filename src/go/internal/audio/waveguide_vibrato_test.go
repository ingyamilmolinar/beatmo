//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// waveguide_vibrato_test.go — every parameter a seed sets must actually reach
// the DSP.
//
// THE BUG THIS PINS
// -----------------
// modular.c's oscillator dispatch is a strict if/else-if chain, and the
// physical-model branch (osc_type 7-11) sits AHEAD of both the vibrato branch
// and the unison branch. Every waveguide voice returned before either could
// run, so `lfo_target:1` vibrato was silently dead on the whole bowed-string
// family -- cello, viola, violin and double-bass all configure it (3.6-4.4 Hz
// at 2.5-3.0 cents), all four documented the effect in their seed comments,
// and none of them produced any. The listener's verdicts on three of the four
// were "breathy and soft" / "needs refinement", which is what a dead-straight
// bowed tone sounds like.
//
// The failure mode is silent by construction: a seed parameter that goes
// nowhere produces no error, no warning and no audible clue -- it just does
// nothing, while the seed comment asserts otherwise. So this test asserts the
// EFFECT, not the plumbing.

// renderModularProbe renders a seed through the same recipe path the modular
// goldens use. 1.5 s is long enough for several cycles of a 3-5 Hz vibrato.
func renderModularProbe(t *testing.T, seed RecipeParams) []float32 {
	t.Helper()
	return renderModularSeed(t, seed, 1.5)
}

// withParam returns a copy of seed with one key overridden, so a table of
// probes cannot accidentally share mutated state.
func withParam(seed RecipeParams, key string, v float64) RecipeParams {
	out := make(RecipeParams, len(seed)+1)
	for k, val := range seed {
		out[k] = val
	}
	out[key] = v
	return out
}

// probeVibrato renders a shipped instrument through its real recipe and reports
// what the analysis battery detects.
func probeVibrato(t *testing.T, inst instrument.ID) (rateHz, extentCents float64, ok bool) {
	t.Helper()
	// 44100 deliberately, not renderModularSeed's 48000: that is the rate the
	// game actually renders at, and these voices' vibrato is only ~2.5 cents --
	// right at the detector's own 2-cent floor, where the rate matters.
	const sr = 44100
	n := int(2.5 * sr)
	buf := renderFieldsForTest(t, inst, sr, n, nil)
	samples := make([]float64, len(buf))
	for i, v := range buf {
		samples[i] = float64(v)
	}
	fp := fingerprint.FromWave(wave.Wave{Samples: samples, SampleRate: sr}, string(inst))
	if fp.Vibrato == nil || !fp.Vibrato.Detected {
		return 0, 0, false
	}
	return fp.Vibrato.RateHz, fp.Vibrato.ExtentCents, true
}

// waveguideOscTypes are the physical-model branches that bypass the generic
// vibrato path and therefore need their own application of it.
var waveguideOscTypes = []int{7, 8, 9, 10, 11}

func isWaveguide(t int) bool {
	for _, w := range waveguideOscTypes {
		if w == t {
			return true
		}
	}
	return false
}

// TestWaveguideVibratoActuallyModulatesPitch renders a waveguide voice with and
// without vibrato and requires the output to differ. A no-op vibrato produces
// byte-identical audio, which is exactly how this went unnoticed.
func TestWaveguideVibratoActuallyModulatesPitch(t *testing.T) {
	for _, ot := range waveguideOscTypes {
		base := RecipeParams{
			"osc_type": float64(ot), "osc_enabled": 1,
			"amp_attack": 0.01, "amp_decay": 0.1, "amp_sustain": 0.9, "amp_release": 0.1,
			"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.0,
		}
		dry := renderModularProbe(t, withParam(base, "lfo_depth", 0))
		wet := renderModularProbe(t, withParam(base, "lfo_depth", 0.25))

		if len(dry) != len(wet) {
			t.Fatalf("osc_type %d: length mismatch %d vs %d", ot, len(dry), len(wet))
		}
		identical := true
		for i := range dry {
			if dry[i] != wet[i] {
				identical = false
				break
			}
		}
		if identical {
			t.Errorf("osc_type %d: lfo_depth 0 vs 0.25 rendered byte-identical audio -- "+
				"vibrato is not reaching this waveguide", ot)
		}
	}
}

// TestWaveguideVibratoIsIdentityWhenUnconfigured is the other half: a voice that
// does NOT configure vibrato must be untouched, so the fix cannot perturb the
// instruments it was not meant to change.
func TestWaveguideVibratoIsIdentityWhenUnconfigured(t *testing.T) {
	for _, ot := range waveguideOscTypes {
		base := RecipeParams{
			"osc_type": float64(ot), "osc_enabled": 1,
			"amp_attack": 0.01, "amp_decay": 0.1, "amp_sustain": 0.9, "amp_release": 0.1,
		}
		a := renderModularProbe(t, base)
		b := renderModularProbe(t, withParam(base, "lfo_enabled", 0))
		for i := range a {
			if a[i] != b[i] {
				t.Errorf("osc_type %d: a voice with no vibrato configured changed at sample %d", ot, i)
				break
			}
		}
		// And it must not be silent, or the comparison above proves nothing.
		var peak float64
		for _, v := range a {
			if m := math.Abs(float64(v)); m > peak {
				peak = m
			}
		}
		if peak <= 1e-9 {
			t.Errorf("osc_type %d: probe rendered silence, so the identity check is vacuous", ot)
		}
	}
}

// TestBowedStringSeedsRenderDetectableVibrato is the end-to-end gate on the
// four shipped instruments: the vibrato they configure must appear in the
// audio at the configured RATE, not merely make the output different.
func TestBowedStringSeedsRenderDetectableVibrato(t *testing.T) {
	for _, tc := range []struct {
		inst     instrument.ID
		wantRate float64
	}{
		{roster.Cello, 4.0}, {roster.Violin, 3.6}, {roster.Viola, 4.4}, {roster.DoubleBass, 3.8},
	} {
		rate, extent, ok := probeVibrato(t, tc.inst)
		if !ok {
			t.Errorf("%s: configures vibrato but none is detectable in the render", tc.inst)
			continue
		}
		if math.Abs(rate-tc.wantRate) > 0.5 {
			t.Errorf("%s: vibrato rate %.2f Hz, want ~%.1f Hz", tc.inst, rate, tc.wantRate)
		}
		// These seeds ship a deliberately shallow ~2.5-cent vibrato, matched to
		// references that are fairly straight notes. That sits right at the
		// detector's own 2-cent floor, so this asserts only that it registers
		// at all -- whether it should be DEEPER is an ear question, not a
		// measurement one.
		if extent < 1.5 {
			t.Errorf("%s: vibrato extent %.1f cents did not register", tc.inst, extent)
		}
	}
}

// TestNoSeedConfiguresUnisonOnAWaveguide guards the OTHER half of the same
// dispatch bug, which is NOT fixed: the unison branch is still unreachable for
// osc_type 7-11, because running N independent waveguide limit cycles is an
// N-fold cost in a real-time path and that is a deliberate decision, not an
// oversight. What must not happen is a seed quietly configuring unison there
// and its author believing it works -- as celloSeed did.
func TestNoSeedConfiguresUnisonOnAWaveguide(t *testing.T) {
	for _, id := range roster.All {
		m := MergedInstrumentParamsRO(id)
		if len(m) == 0 || m["osc_enabled"] == 0 {
			continue
		}
		if !isWaveguide(int(m["osc_type"])) {
			continue
		}
		if m["unison_voices"] >= 2 {
			t.Errorf("%s: osc_type %d is a waveguide, where the unison branch is unreachable, "+
				"but the seed sets unison_voices=%.0f -- those parameters do nothing. Remove them "+
				"or implement unison for the waveguide branch in modular.c",
				id, int(m["osc_type"]), m["unison_voices"])
		}
	}
}

// TestBowedStringVibratoIsNotMetronomic guards the regression that shipping the
// vibrato fix introduced.
//
// A vibrato at a mathematically constant rate is the single most recognisable
// "this is an LFO, not a player" tell. The first cut measured
// VibratoFingerprint.Jitter == 0.000 -- perfectly periodic -- against the corpus
// double-bass reference's 0.411, and the listener moved double-bass from "sounds
// like the real thing" to "has some vibrato or something that goes up and down
// in time" on the very next pass. Detecting it needs no reference audio: a
// jitter of exactly zero is self-evidently synthetic.
func TestBowedStringVibratoIsNotMetronomic(t *testing.T) {
	for _, inst := range []instrument.ID{roster.Cello, roster.Violin, roster.Viola, roster.DoubleBass} {
		_, _, ok := probeVibrato(t, inst)
		if !ok {
			t.Errorf("%s: no vibrato detected at all", inst)
			continue
		}
		jitter := probeVibratoJitter(t, inst)
		if jitter <= 0.01 {
			t.Errorf("%s: vibrato jitter %.4f -- a rate this constant reads as an LFO rather "+
				"than a player (the corpus double-bass reference measures 0.411)", inst, jitter)
		}
	}
}

// probeVibratoJitter renders a shipped instrument and returns the detected
// cycle-to-cycle rate irregularity.
func probeVibratoJitter(t *testing.T, inst instrument.ID) float64 {
	t.Helper()
	const sr = 44100
	n := int(2.5 * sr)
	buf := renderFieldsForTest(t, inst, sr, n, nil)
	samples := make([]float64, len(buf))
	for i, v := range buf {
		samples[i] = float64(v)
	}
	fp := fingerprint.FromWave(wave.Wave{Samples: samples, SampleRate: sr}, string(inst))
	if fp.Vibrato == nil || !fp.Vibrato.Detected {
		return 0
	}
	return fp.Vibrato.Jitter
}
