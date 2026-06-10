//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Damping-branch coverage for the C send delay (effects.c): dampingHz <= 0
// selects the "no filtering" lpCoeff=1 branch in both delay_init and
// delay_set_damping_smooth, which no existing test reached.

func TestConfigureSendDelayDampingBranches(t *testing.T) {
	const sr = 44100
	initSendEffects(sr)
	if sendFX == nil {
		t.Fatal("send effects not initialized")
	}
	resetSendEffects()
	t.Cleanup(func() {
		resetSendEffects()
		// Restore the production defaults mutated by this test.
		ConfigureSendDelay(300, 0.35, 3000)
	})

	render := func(dampingHz float64) []float64 {
		resetSendEffects()
		ConfigureSendDelay(80, 0.3, dampingHz)
		SetDelaySend("damping-test", 1.0)
		SetReverbSend("damping-test", 0)
		defer SetDelaySend("damping-test", 0)

		// processSends works on mixer-sized blocks (its internal send
		// buffers are blockSize samples) — feed it like the mixer does.
		const blockSize = 64
		total := sr / 2
		in := make([]float64, total)
		in[0] = 1 // impulse
		out := make([]float64, total)
		instBufs := map[string][]float64{}
		active := []string{"damping-test"}
		for off := 0; off < total; off += blockSize {
			block := make([]float64, blockSize)
			copy(block, in[off:min(off+blockSize, total)])
			instBufs["damping-test"] = block
			master := make([]float64, blockSize)
			sendFX.processSends(instBufs, active, blockSize, master)
			copy(out[off:min(off+blockSize, total)], master)
		}
		return out
	}

	// dampingHz = 0 → lpCoeff = 1 (no filtering); echoes keep full bandwidth.
	undamped := render(0)
	// Heavy damping → audible LP on the repeats.
	damped := render(500)

	var undampedE, dampedE float64
	for i := range undamped {
		if math.IsNaN(undamped[i]) || math.IsInf(undamped[i], 0) {
			t.Fatalf("undamped[%d] not finite", i)
		}
		undampedE += undamped[i] * undamped[i]
		dampedE += damped[i] * damped[i]
	}
	if undampedE == 0 {
		t.Fatal("undamped delay returned silence")
	}
	if undampedE == dampedE {
		t.Fatal("damping had no effect on echo energy — LP branch not applied")
	}

	// Negative damping must take the same no-filtering branch without blowing up.
	neg := render(-100)
	for i := range neg {
		if math.IsNaN(neg[i]) || math.IsInf(neg[i], 0) {
			t.Fatalf("negative-damping out[%d] not finite", i)
		}
	}
}
