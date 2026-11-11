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

// game_playback_tuning_test.go — the retuned instruments must reach the GAME,
// not just the analysis tools.
//
// Every measurement in this work went through synthmatch.RenderInstrument,
// which resolves the recipe and calls recipe.Render. That is not the path the
// game plays: playback goes through the instrument table's baked render
// (bakedModularRender) and scales by ConfigForInstrument().Amplitude, which
// comes from the MEASURED loudness table. Either could diverge from what the
// tools measure -- and one of them had, silently: the loudness table was stale
// for seven instruments, pinning tubular-bells 6.4 dB below everything else in
// the mix while every level-matched tool reported it fine.
//
// So these assert the tuning through the production path, on the specific
// properties that were changed.

// playbackRender renders an instrument the way the game does: the one-shot raw
// capture, which shares bakedModularRender with playback.
func playbackRender(t *testing.T, id instrument.ID) wave.Wave {
	t.Helper()
	buf, sr := RenderInstrumentOneShotRaw(id)
	if len(buf) == 0 {
		t.Fatalf("%s: production render produced no samples", id)
	}
	s := make([]float64, len(buf))
	for i, v := range buf {
		s[i] = float64(v)
	}
	return wave.Wave{Samples: s, SampleRate: sr, Label: string(id)}
}

// TestGamePlaybackHasTheVibratoFix proves the waveguide vibrato reaches the
// production render, not only synthmatch's.
func TestGamePlaybackHasTheVibratoFix(t *testing.T) {
	for _, tc := range []struct {
		inst     instrument.ID
		wantRate float64
	}{
		{roster.Cello, 4.0}, {roster.Violin, 3.6}, {roster.Viola, 4.4}, {roster.DoubleBass, 3.8},
	} {
		w := playbackRender(t, tc.inst)
		fp := fingerprint.FromWave(w, string(tc.inst))
		if fp.Vibrato == nil || !fp.Vibrato.Detected {
			t.Errorf("%s: no vibrato in the PRODUCTION render — the fix reaches the analysis "+
				"path but not the game", tc.inst)
			continue
		}
		if math.Abs(fp.Vibrato.RateHz-tc.wantRate) > 0.6 {
			t.Errorf("%s: production vibrato rate %.2f Hz, want ~%.1f", tc.inst, fp.Vibrato.RateHz, tc.wantRate)
		}
		// Jitter is measured on a LONGER render of the same production
		// parameters. Cycle-to-cycle irregularity needs several vibrato cycles
		// to estimate, and the one-shot capture is 1 s -- four cycles at 4 Hz,
		// where the estimator legitimately returns 0. The two paths are gated
		// as identical by TestModularNoEditFastPathMatchesRecipe, so this is
		// the same instrument measured over a window that can resolve the
		// property.
		if j := longRenderJitter(t, tc.inst, 4.0); j <= 0.01 {
			t.Errorf("%s: vibrato jitter %.4f over a 4 s render — metronomic in the game", tc.inst, j)
		}
	}
}

// TestGamePlaybackHasTheKalimbaGainFix proves genN_gain reaches the production
// render. The kalimba seed pulls its Karplus-Strong pluck back to 0.4 so the
// additive bar partials dominate; before the fix that had no effect and the
// listener heard "a plucked string" instead of a wooden bar.
func TestGamePlaybackHasTheKalimbaGainFix(t *testing.T) {
	w := playbackRender(t, roster.Kalimba)
	fp := fingerprint.FromWave(w, "kalimba")
	ratios := fp.PartialRatioProfile.Ratios
	if len(ratios) == 0 {
		t.Fatal("kalimba: no partials measured in the production render")
	}
	// With the KS pluck at full level its fundamental dominates and the lowest
	// measured partial sits near 1.7x f0. With it pulled back, the bar partials
	// lead and the series starts at ~3x.
	if ratios[0] < 2.0 {
		t.Errorf("kalimba: lowest partial at %.2fx f0 — the Karplus-Strong pluck is still "+
			"dominating in the PRODUCTION render, so gen1_gain=0.4 is not reaching playback "+
			"(partials: %v)", ratios[0], ratios)
	}
}

// The level half (ConfigForInstrument == the document's render.amplitude) is
// TestConfigForInstrumentUsesTheDocumentAmplitude (loudness_amp_test.go).

// TestGamePlaybackTubularBellsIsNoLongerQuiet pins the specific regression the
// stale table was hiding: tubular-bells was pinned at 0.4122 against a measured
// 0.8608, i.e. it played 6.4 dB below every other instrument in the game.
func TestGamePlaybackTubularBellsIsNoLongerQuiet(t *testing.T) {
	amp := ConfigForInstrument(roster.TubularBells).Amplitude
	if amp < 0.7 {
		t.Errorf("tubular-bells playback amplitude %.4f — its document amplitude is stale again; "+
			"it was 0.4122 (6.4 dB below the rest of the mix) before being re-measured. "+
			"Run `make measure-loudness`.", amp)
	}
}

// longRenderJitter renders an instrument's production parameters for secs
// seconds and returns the detected vibrato jitter.
func longRenderJitter(t *testing.T, id instrument.ID, secs float64) float64 {
	t.Helper()
	const sr = 44100
	n := int(secs * sr)
	buf := renderFieldsForTest(t, id, sr, n, nil)
	s := make([]float64, len(buf))
	for i, v := range buf {
		s[i] = float64(v)
	}
	fp := fingerprint.FromWave(wave.Wave{Samples: s, SampleRate: sr}, string(id))
	if fp.Vibrato == nil || !fp.Vibrato.Detected {
		return 0
	}
	return fp.Vibrato.Jitter
}
