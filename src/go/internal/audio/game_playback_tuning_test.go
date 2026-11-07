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

// The retuned instruments must reach the GAME's render, not only the analysis tools': a stale loudness table once
// left tubular-bells 6.4 dB below the mix while every level-matched tool reported it fine.

// The one-shot raw capture, which shares the render with playback.
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
		// Jitter needs several vibrato cycles; the 1 s capture holds four at 4 Hz, where the estimator rightly reads 0. The
		// longer render is the same production parameters.
		if j := longRenderJitter(t, tc.inst, 4.0); j <= 0.01 {
			t.Errorf("%s: vibrato jitter %.4f over a 4 s render — metronomic in the game", tc.inst, j)
		}
	}
}

// The kalimba pulls its Karplus-Strong pluck back to 0.4 so the additive bar partials dominate: a wooden bar, not a
// plucked string.
func TestGamePlaybackHasTheKalimbaGainFix(t *testing.T) {
	w := playbackRender(t, roster.Kalimba)
	fp := fingerprint.FromWave(w, "kalimba")
	ratios := fp.PartialRatioProfile.Ratios
	if len(ratios) == 0 {
		t.Fatal("kalimba: no partials measured in the production render")
	}
	// At full level the pluck's fundamental leads and the lowest partial sits near 1.7× f0; pulled back, the series starts
	// at ~3×.
	if ratios[0] < 2.0 {
		t.Errorf("kalimba: lowest partial at %.2fx f0 — the Karplus-Strong pluck is still "+
			"dominating in the PRODUCTION render, so gen1_gain=0.4 is not reaching playback "+
			"(partials: %v)", ratios[0], ratios)
	}
}

func TestGamePlaybackTubularBellsIsNoLongerQuiet(t *testing.T) {
	amp := ConfigForInstrument(roster.TubularBells).Amplitude
	if amp < 0.7 {
		t.Errorf("tubular-bells playback amplitude %.4f — its document amplitude is stale again; "+
			"it was 0.4122 (6.4 dB below the rest of the mix) before being re-measured. "+
			"Run `make measure-loudness`.", amp)
	}
}

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
