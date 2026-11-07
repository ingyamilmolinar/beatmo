//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The desktop mixer applies instrument channel gain inside ProcessInsertsBlockLocal, never ProcessBlockLocal, so
// this pins that SetChannelVolume reaches the output: half the volume, half the RMS.
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
		SetChannelVolume(channel.Of(roster.Kick), chVol)
		m := newTestMixer()
		v := newRecipeAwareVoice(roster.Kick, sampleRate)
		if v == nil {
			t.Fatal("nil kick voice")
		}
		m.Schedule(channel.Of(roster.Kick), v, 0, -1, -1)
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
