//go:build !test

package audio

import (
	"math"
	"sync"
	"testing"
)

func initSendFXForTest(t *testing.T) {
	t.Helper()
	sr := 44100
	initSendEffects(sr)
	if sendFX == nil {
		t.Fatal("send effects not initialized")
	}
	resetSendEffects()
}

func TestSendEffectsPerInstrumentIsolation(t *testing.T) {
	initSendFXForTest(t)

	SetDelaySend("inst-a", 1.0)
	SetDelaySend("inst-b", 0.0)
	SetReverbSend("inst-a", 0.0)
	SetReverbSend("inst-b", 0.0)

	bufA := make([]float64, blockSize)
	bufB := make([]float64, blockSize)
	for i := range bufA {
		bufA[i] = 0.5
		bufB[i] = 0.5
	}

	instBufs := map[string][]float64{"inst-a": bufA, "inst-b": bufB}
	activeInsts := []string{"inst-a", "inst-b"}
	masterBuf := make([]float64, blockSize)

	sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

	// inst-a has delay=1.0 so it should contribute; inst-b has delay=0 so it shouldn't.
	// The delay output is the immediate pass-through of the input, so first block should have signal.
	hasSignal := false
	for _, v := range masterBuf {
		if math.Abs(v) > 0.001 {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		t.Error("expected signal in masterBuf from inst-a's delay send")
	}
}

func TestSendEffectsZeroSendSkipsProcessing(t *testing.T) {
	initSendFXForTest(t)

	SetDelaySend("zero-inst", 0.0)
	SetReverbSend("zero-inst", 0.0)

	inputBuf := make([]float64, blockSize)
	for i := range inputBuf {
		inputBuf[i] = 1.0
	}

	instBufs := map[string][]float64{"zero-inst": inputBuf}
	activeInsts := []string{"zero-inst"}
	masterBuf := make([]float64, blockSize)

	sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

	for i, v := range masterBuf {
		if v != 0 {
			t.Errorf("sample[%d] = %f, want 0 (all sends at 0)", i, v)
			break
		}
	}
}

func TestSendEffectsMultiInstrumentAccumulation(t *testing.T) {
	initSendFXForTest(t)

	ids := []string{"acc-a", "acc-b", "acc-c"}
	instBufs := map[string][]float64{}
	for _, id := range ids {
		SetDelaySend(id, 1.0)
		SetReverbSend(id, 0.0)
		buf := make([]float64, blockSize)
		for i := range buf {
			buf[i] = 0.3
		}
		instBufs[id] = buf
	}

	masterBuf := make([]float64, blockSize)
	sendFX.processSends(instBufs, ids, blockSize, masterBuf)

	// All 3 instruments send signal through delay; output should be nonzero.
	hasSignal := false
	for _, v := range masterBuf {
		if math.Abs(v) > 0.001 {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		t.Error("expected accumulated signal from 3 instruments")
	}
}

func TestSendEffectsConcurrentLevelUpdates(t *testing.T) {
	initSendFXForTest(t)

	id := "conc-send"
	SetDelaySend(id, 0.5)
	SetReverbSend(id, 0.5)

	inputBuf := make([]float64, blockSize)
	for i := range inputBuf {
		inputBuf[i] = 0.5
	}
	instBufs := map[string][]float64{id: inputBuf}
	activeInsts := []string{id}
	masterBuf := make([]float64, blockSize)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				SetDelaySend(id, float64(i%100)/100.0)
				SetReverbSend(id, float64(i%100)/100.0)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			for j := range masterBuf {
				masterBuf[j] = 0
			}
			sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)
		}
	}()
	wg.Wait()
}

func TestSendEffectsDelayAndReverbSimultaneous(t *testing.T) {
	initSendFXForTest(t)

	id := "both-fx"
	SetDelaySend(id, 0.5)
	SetReverbSend(id, 0.5)

	inputBuf := make([]float64, blockSize)
	for i := range inputBuf {
		inputBuf[i] = 0.8
	}
	instBufs := map[string][]float64{id: inputBuf}
	activeInsts := []string{id}
	masterBuf := make([]float64, blockSize)

	sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

	hasSignal := false
	for _, v := range masterBuf {
		if math.Abs(v) > 0.001 {
			hasSignal = true
			break
		}
	}
	if !hasSignal {
		t.Error("expected signal from combined delay + reverb sends")
	}
}

func TestSendEffectsResetClearsState(t *testing.T) {
	initSendFXForTest(t)

	id := "reset-state"
	SetDelaySend(id, 1.0)
	SetReverbSend(id, 1.0)

	// Feed loud signal.
	loudBuf := make([]float64, blockSize)
	for i := range loudBuf {
		loudBuf[i] = 1.0
	}
	instBufs := map[string][]float64{id: loudBuf}
	activeInsts := []string{id}
	masterBuf := make([]float64, blockSize)

	for b := 0; b < 100; b++ {
		for i := range masterBuf {
			masterBuf[i] = 0
		}
		sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)
	}

	// Reset and feed silence.
	resetSendEffects()
	silenceBuf := make([]float64, blockSize)
	instBufs[id] = silenceBuf
	for i := range masterBuf {
		masterBuf[i] = 0
	}
	sendFX.processSends(instBufs, activeInsts, blockSize, masterBuf)

	for i, v := range masterBuf {
		if math.Abs(v) > 0.01 {
			t.Errorf("sample[%d] = %f after reset+silence, want ≈ 0", i, v)
			break
		}
	}
}

func TestSendLevelsDefaultZero(t *testing.T) {
	// Fresh instrument should have 0 send levels.
	freshID := "fresh-send-inst"
	d := DelaySend(freshID)
	r := ReverbSend(freshID)
	if d != 0 {
		t.Errorf("DelaySend default = %f, want 0", d)
	}
	if r != 0 {
		t.Errorf("ReverbSend default = %f, want 0", r)
	}
}

func TestSendEffectsLevelClamping(t *testing.T) {
	id := "clamp-send"

	SetDelaySend(id, -1)
	if d := DelaySend(id); d != 0 {
		t.Errorf("DelaySend(-1) = %f, want 0", d)
	}

	SetDelaySend(id, 2)
	if d := DelaySend(id); d != 1 {
		t.Errorf("DelaySend(2) = %f, want 1", d)
	}

	SetReverbSend(id, -0.5)
	if r := ReverbSend(id); r != 0 {
		t.Errorf("ReverbSend(-0.5) = %f, want 0", r)
	}

	SetReverbSend(id, 1.5)
	if r := ReverbSend(id); r != 1 {
		t.Errorf("ReverbSend(1.5) = %f, want 1", r)
	}
}
