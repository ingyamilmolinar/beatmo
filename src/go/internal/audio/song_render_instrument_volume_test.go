//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The arrangement already folds instrument volume into each note's Gain; setting the channel too would square it, so
// halving volume must cost 6 dB, not 12.
func TestSongRenderInstrumentVolumeAppliedOnce(t *testing.T) {
	pinSampleRate(t, 44100)
	// It mutates global channel, effect and recipe state.
	t.Cleanup(ResetInstruments)

	rms := func(x []float64) float64 {
		if len(x) == 0 {
			return 0
		}
		var s float64
		for _, v := range x {
			s += v * v
		}
		return math.Sqrt(s / float64(len(x)))
	}

	render := func(v float64) float64 {
		ResetInstruments()
		insts := []SongRenderInstrument{{ID: roster.Kick, Volume: v}}
		notes := []SongRenderNote{{InstID: roster.Kick, AtSample: 0, Pitch: 0, Gain: v, DurSec: 0.2}}
		master, _, err := RenderSongOffline(insts, notes, sampleRate, sampleRate/2)
		if err != nil {
			t.Fatalf("RenderSongOffline: %v", err)
		}
		return rms(master)
	}

	full := render(1.0)
	half := render(0.5)
	if full <= 0 {
		t.Fatalf("full-volume render is silent (rms=%.6f)", full)
	}
	ratio := half / full
	t.Logf("inst.Volume 1.0 rms=%.6f  0.5 rms=%.6f  ratio=%.4f", full, half, ratio)
	if math.Abs(ratio-0.5) > 0.02 {
		t.Errorf("instrument volume applied %.4f× per halving (want 0.5); a ratio near 0.25 means it is squared (double-applied)", ratio)
	}
}
