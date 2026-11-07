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

// A seed parameter that never reaches the DSP fails silently (no error, no audible clue), so this asserts the EFFECT:
// the physical-model branch runs ahead of the generic vibrato, so the waveguides apply it themselves.

// 1.5 s holds several cycles of a 3-5 Hz vibrato.
func renderModularProbe(t *testing.T, seed RecipeParams) []float32 {
	t.Helper()
	return renderModularSeed(t, seed, 1.5)
}

// A copy, so probes cannot share mutated state.
func withParam(seed RecipeParams, key string, v float64) RecipeParams {
	out := make(RecipeParams, len(seed)+1)
	for k, val := range seed {
		out[k] = val
	}
	out[key] = v
	return out
}

func probeVibrato(t *testing.T, inst instrument.ID) (rateHz, extentCents float64, ok bool) {
	t.Helper()
	// The analysis rate: a ~2.5-cent vibrato sits at the detector's 2-cent floor, where the rate matters.
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

// These bypass the generic vibrato path and apply their own.
var waveguideOscTypes = []int{7, 8, 9, 10, 11}

func isWaveguide(t int) bool {
	for _, w := range waveguideOscTypes {
		if w == t {
			return true
		}
	}
	return false
}

// A no-op vibrato renders byte-identical audio.
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

// So applying vibrato cannot perturb instruments that do not ask for it.
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

// At the configured RATE, not merely a different output.
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
		// A deliberately shallow vibrato at the detector's floor: whether it should be deeper is an ear question.
		if extent < 1.5 {
			t.Errorf("%s: vibrato extent %.1f cents did not register", tc.inst, extent)
		}
	}
}

// Unison stays unreachable on a waveguide by decision (N limit cycles is N× the real-time cost); no document may
// configure it there and believe it works.
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

// A constant-rate vibrato is the most recognisable "LFO, not a player" tell; a jitter of exactly zero is
// self-evidently synthetic, no reference needed.
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

// Cycle-to-cycle rate irregularity.
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
