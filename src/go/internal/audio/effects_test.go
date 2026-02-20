//go:build !test

package audio

import (
	"math"
	"testing"
)

func TestDelayOffset(t *testing.T) {
	// Test that delay_process produces output offset by the delay length.
	sr := 44100
	initSendEffects(sr)
	if sendFX == nil {
		t.Fatal("send effects not initialized")
	}

	// Reset to clean state.
	resetSendEffects()

	// Feed an impulse through the delay.
	delaySamples := sr * 300 / 1000 // 300ms
	input := make([]float64, delaySamples+1000)
	input[0] = 1.0 // impulse at sample 0

	// Simulate processing by setting send level and calling processSends.
	SetDelaySend("test-delay", 1.0)
	SetReverbSend("test-delay", 0.0)

	instBufs := map[string][]float64{"test-delay": nil}
	activeInsts := []string{"test-delay"}
	masterBuf := make([]float64, blockSize)

	// Process in blocks, looking for the delayed impulse.
	foundDelay := false
	totalSamples := 0
	for block := 0; block < (delaySamples+1000)/blockSize; block++ {
		// Prepare input block.
		blockBuf := make([]float64, blockSize)
		for i := 0; i < blockSize && totalSamples+i < len(input); i++ {
			blockBuf[i] = input[totalSamples+i]
		}
		instBufs["test-delay"] = blockBuf

		// Clear master buf.
		for i := range masterBuf {
			masterBuf[i] = 0
		}

		sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

		// Check for non-zero output (the delayed impulse).
		for i := 0; i < blockSize; i++ {
			sampleIdx := totalSamples + i
			if sampleIdx > delaySamples-50 && sampleIdx < delaySamples+50 {
				if math.Abs(masterBuf[i]) > 0.01 {
					foundDelay = true
				}
			}
		}
		totalSamples += blockSize
	}

	if !foundDelay {
		t.Error("delay output: expected delayed impulse near sample offset, not found")
	}
}

func TestReverbDecay(t *testing.T) {
	// Test that reverb produces a decaying tail.
	sr := 44100
	initSendEffects(sr)
	if sendFX == nil {
		t.Fatal("send effects not initialized")
	}
	resetSendEffects()

	SetDelaySend("test-reverb", 0.0)
	SetReverbSend("test-reverb", 1.0)

	instBufs := map[string][]float64{"test-reverb": nil}
	activeInsts := []string{"test-reverb"}
	masterBuf := make([]float64, blockSize)

	// Feed an impulse.
	impulseBuf := make([]float64, blockSize)
	impulseBuf[0] = 1.0
	instBufs["test-reverb"] = impulseBuf

	for i := range masterBuf {
		masterBuf[i] = 0
	}
	sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

	// After the impulse, feed silence and measure tail energy.
	silenceBuf := make([]float64, blockSize)
	var energyEarly, energyLate float64

	// Early blocks (first 100ms).
	earlyBlocks := (sr / 10) / blockSize
	for block := 0; block < earlyBlocks; block++ {
		instBufs["test-reverb"] = silenceBuf
		for i := range masterBuf {
			masterBuf[i] = 0
		}
		sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)
		for i := 0; i < blockSize; i++ {
			energyEarly += masterBuf[i] * masterBuf[i]
		}
	}

	// Late blocks (500ms-600ms).
	skipBlocks := (sr / 2) / blockSize
	for block := 0; block < skipBlocks; block++ {
		instBufs["test-reverb"] = silenceBuf
		for i := range masterBuf {
			masterBuf[i] = 0
		}
		sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)
	}
	lateBlocks := (sr / 10) / blockSize
	for block := 0; block < lateBlocks; block++ {
		instBufs["test-reverb"] = silenceBuf
		for i := range masterBuf {
			masterBuf[i] = 0
		}
		sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)
		for i := 0; i < blockSize; i++ {
			energyLate += masterBuf[i] * masterBuf[i]
		}
	}

	// Reverb tail should decay: early energy should be greater than late energy.
	if energyEarly <= 0 {
		t.Error("reverb: no energy in early tail")
	}
	if energyEarly > 0 && energyLate >= energyEarly {
		t.Errorf("reverb tail not decaying: early=%f late=%f", energyEarly, energyLate)
	}
}

func TestSendLevels(t *testing.T) {
	SetDelaySend("test-inst", 0.5)
	SetReverbSend("test-inst", 0.3)

	if d := DelaySend("test-inst"); math.Abs(d-0.5) > 0.001 {
		t.Errorf("DelaySend: expected 0.5, got %f", d)
	}
	if r := ReverbSend("test-inst"); math.Abs(r-0.3) > 0.001 {
		t.Errorf("ReverbSend: expected 0.3, got %f", r)
	}

	// Clamping.
	SetDelaySend("test-inst", -1)
	if d := DelaySend("test-inst"); d != 0 {
		t.Errorf("DelaySend: expected 0 for negative, got %f", d)
	}
	SetReverbSend("test-inst", 2)
	if r := ReverbSend("test-inst"); r != 1 {
		t.Errorf("ReverbSend: expected 1 for >1, got %f", r)
	}
}
