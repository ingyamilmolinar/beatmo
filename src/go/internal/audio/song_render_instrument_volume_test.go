//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestSongRenderInstrumentVolumeAppliedOnce guards against double-applying the
// per-instrument volume in the offline song-render path. songrender/arrangement.go
// already folds inst.Volume into every note's Gain; RenderSongOffline must NOT
// also set the channel volume to inst.Volume, or the gain is squared (0.5 → 0.25).
//
// We simulate what render_prod.go passes: node volume 1.0, so note.Gain == inst.Volume.
// Halving inst.Volume must halve the output RMS (−6 dB), not quarter it (−12 dB).
func TestSongRenderInstrumentVolumeAppliedOnce(t *testing.T) {
	pinSampleRate(t, 44100)
	// RenderSongOffline mutates global channel/effect/recipe state; restore the
	// default instrument set afterwards so this test can't poison a later one.
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
		insts := []SongRenderInstrument{{ID: "kick", Volume: v}}
		notes := []SongRenderNote{{InstID: "kick", AtSample: 0, Pitch: 0, Gain: v, DurSec: 0.2}}
		master, _, err := RenderSongOffline(insts, notes, 120, sampleRate, sampleRate/2)
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
