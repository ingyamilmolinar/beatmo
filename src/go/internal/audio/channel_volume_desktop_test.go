//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestDesktopChannelVolumeApplied guards that per-instrument channel volume
// attenuates desktop live audio. The desktop mixer never calls Channel.
// ProcessBlockLocal for instrument channels — it applies the channel gain inside
// ProcessInsertsBlockLocal (Phase 2 of processBlock, engine_stop.go) — so this
// pins that the atomic set by SetChannelVolume actually reaches the output.
// Halving the channel volume must halve the output RMS.
func TestDesktopChannelVolumeApplied(t *testing.T) {
	pinSampleRate(t, 44100)
	pinMasterVolume(t, 1.0)
	// SetChannelVolume mutates global channel state; restore defaults afterwards.
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

	render := func(chVol float64) float64 {
		ResetInstruments()
		SetChannelVolume("kick", chVol)
		m := newTestMixer()
		v := newRecipeAwareVoice("kick", bpm, sampleRate)
		if v == nil {
			t.Fatal("nil kick voice")
		}
		m.Schedule("kick", v, 0, -1, -1)
		return rms(renderMixer(m, sampleRate/2))
	}

	full := render(1.0)
	half := render(0.5)
	ratio := half / full
	t.Logf("channelVol 1.0 rms=%.6f  0.5 rms=%.6f  ratio=%.4f", full, half, ratio)
	if math.Abs(ratio-0.5) > 0.02 {
		t.Errorf("desktop channel volume not applied as ~0.5: ratio=%.4f", ratio)
	}
}
