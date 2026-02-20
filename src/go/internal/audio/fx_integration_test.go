package audio

import (
	"math"
	"testing"
)

// sineSamples generates n samples of a sine wave at the given frequency and sample rate.
func sineSamples(freq float64, sr, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.Sin(2 * math.Pi * freq * float64(i) / float64(sr))
	}
	return out
}

// rmsEnergy computes the RMS energy of a signal.
func rmsEnergy(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += s * s
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// processThrough runs samples through the channel's ProcessSampleLocal.
func processThrough(ch *Channel, input []float64) []float64 {
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = ch.ProcessSampleLocal(x)
	}
	return out
}

// TestChainMultiEffectModifiesAudio verifies that stacking distortion→delay→reverb
// through a channel produces output that differs from the dry signal.
func TestChainMultiEffectModifiesAudio(t *testing.T) {
	resetChains(t)
	id := "integ-multi"
	ch := InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, map[string]float64{"drive": 10, "tone": 4000, "mix": 1})
	AddInsertEffect(id, EffectDelay, map[string]float64{"time": 100, "feedback": 0.3, "mix": 0.5})
	AddInsertEffect(id, EffectReverb, map[string]float64{"room": 0.5, "damping": 0.5, "mix": 0.5})

	const sr = 44100
	input := sineSamples(440, sr, sr/2) // 0.5 second

	wet := processThrough(ch, input)

	// Also process with a clean channel (no effects).
	resetChains(t)
	ch2 := InstrumentChannel("integ-dry")
	dry := processThrough(ch2, input)

	wetRMS := rmsEnergy(wet)
	dryRMS := rmsEnergy(dry)

	if wetRMS == 0 {
		t.Fatal("wet signal is silent")
	}
	if dryRMS == 0 {
		t.Fatal("dry signal is silent")
	}

	// The outputs should differ since effects modify the signal.
	diff := math.Abs(wetRMS - dryRMS)
	if diff < 0.001 {
		t.Errorf("expected measurable difference between wet (%.4f) and dry (%.4f) RMS", wetRMS, dryRMS)
	}
}

// TestChainParameterChangesAffectOutput verifies that increasing distortion drive
// changes the output energy. Tanh saturation with higher drive pushes small inputs
// closer to ±1, raising RMS for sub-1.0 inputs.
func TestChainParameterChangesAffectOutput(t *testing.T) {
	resetChains(t)
	id := "integ-param"
	ch := InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, map[string]float64{"drive": 1, "tone": 8000, "mix": 1})

	const sr = 44100
	// Use a quiet signal (amplitude 0.3) so drive increase has visible effect.
	input := make([]float64, sr/4)
	for i := range input {
		input[i] = 0.3 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}

	lowDriveOut := processThrough(ch, input)
	lowRMS := rmsEnergy(lowDriveOut)

	// Increase drive to 20.
	SetInsertEffectParam(id, 0, "drive", 20)

	// Reset filter state so comparison is clean.
	ResetAllInsertEffects()

	highDriveOut := processThrough(ch, input)
	highRMS := rmsEnergy(highDriveOut)

	if highRMS <= lowRMS {
		t.Errorf("expected higher drive to increase energy: low=%.4f high=%.4f", lowRMS, highRMS)
	}
}

// TestChainOrderAffectsOutput verifies that distortion→filter and filter→distortion
// produce different results. Distortion-first generates harmonics then filter removes
// them; filter-first removes highs before distortion, producing fewer harmonics.
func TestChainOrderAffectsOutput(t *testing.T) {
	resetChains(t)
	id := "integ-order"
	ch := InstrumentChannel(id)

	// Distortion first, then LP filter at 500Hz.
	AddInsertEffect(id, EffectDistortion, map[string]float64{"drive": 15, "tone": 8000, "mix": 1})
	AddInsertEffect(id, EffectFilter, map[string]float64{"mode": 0, "cutoff": 500, "q": 0.707, "mix": 1})

	const sr = 44100
	// Broadband signal: 200Hz + 5kHz.
	input := make([]float64, sr/4)
	for i := range input {
		t := float64(i) / float64(sr)
		input[i] = 0.5*math.Sin(2*math.Pi*200*t) + 0.5*math.Sin(2*math.Pi*5000*t)
	}

	order1Out := processThrough(ch, input)
	order1RMS := rmsEnergy(order1Out)

	// Move filter to slot 0 (filter→distortion).
	MoveInsertEffect(id, 1, 0)
	ResetAllInsertEffects()

	order2Out := processThrough(ch, input)
	order2RMS := rmsEnergy(order2Out)

	diff := math.Abs(order1RMS - order2RMS)
	if diff < 0.001 {
		t.Errorf("expected different energies for different chain orders: order1=%.4f order2=%.4f", order1RMS, order2RMS)
	}
}

// TestChainToggleBypassesEffect verifies that disabling a LP filter lets high
// frequencies pass through, while enabling it attenuates them.
func TestChainToggleBypassesEffect(t *testing.T) {
	resetChains(t)
	id := "integ-toggle"
	ch := InstrumentChannel(id)

	// LP filter at 200Hz — should strongly attenuate 5kHz.
	AddInsertEffect(id, EffectFilter, map[string]float64{"mode": 0, "cutoff": 200, "q": 0.707, "mix": 1})

	const sr = 44100
	input := sineSamples(5000, sr, sr/4)

	enabledOut := processThrough(ch, input)
	enabledRMS := rmsEnergy(enabledOut)

	// Disable the filter.
	ToggleInsertEffect(id, 0, false)

	disabledOut := processThrough(ch, input)
	disabledRMS := rmsEnergy(disabledOut)

	// With filter disabled, energy should be much higher.
	if disabledRMS <= enabledRMS*2 {
		t.Errorf("expected bypassed energy (%.4f) >> enabled energy (%.4f)", disabledRMS, enabledRMS)
	}
}

// TestChannelProcessorChainIncludesInserts verifies that a delay effect wired into
// the channel produces an echo at the expected position.
func TestChannelProcessorChainIncludesInserts(t *testing.T) {
	resetChains(t)
	id := "integ-echo"
	ch := InstrumentChannel(id)

	// 100ms delay, no feedback, 100% wet.
	AddInsertEffect(id, EffectDelay, map[string]float64{"time": 100, "feedback": 0, "mix": 1})

	const sr = 44100
	expectSample := int(0.1 * float64(sr)) // ~4410

	// Send impulse through channel, then silence.
	output := make([]float64, sr/2)
	output[0] = ch.ProcessSampleLocal(1.0)
	for i := 1; i < len(output); i++ {
		output[i] = ch.ProcessSampleLocal(0)
	}

	// Find peak near expected delay position.
	peakIdx := 0
	peakVal := 0.0
	searchStart := expectSample - 50
	searchEnd := expectSample + 50
	if searchStart < 0 {
		searchStart = 0
	}
	if searchEnd > len(output) {
		searchEnd = len(output)
	}
	for i := searchStart; i < searchEnd; i++ {
		if math.Abs(output[i]) > peakVal {
			peakVal = math.Abs(output[i])
			peakIdx = i
		}
	}

	if peakVal < 0.1 {
		t.Fatalf("no echo detected near sample %d (peak=%.4f)", expectSample, peakVal)
	}
	if math.Abs(float64(peakIdx-expectSample)) > 50 {
		t.Errorf("echo at sample %d, expected near %d", peakIdx, expectSample)
	}
}

// TestChainAllSixEffectsProcess adds all 6 effect types and processes a sine through
// the channel. Verifies no NaN/Inf and the output is non-silent.
func TestChainAllSixEffectsProcess(t *testing.T) {
	resetChains(t)
	id := "integ-all6"
	ch := InstrumentChannel(id)

	AddInsertEffect(id, EffectDistortion, nil)
	AddInsertEffect(id, EffectDelay, nil)
	AddInsertEffect(id, EffectReverb, nil)
	AddInsertEffect(id, EffectChorus, nil)
	AddInsertEffect(id, EffectBitcrusher, nil)
	AddInsertEffect(id, EffectFilter, nil)

	slots := GetInsertEffects(id)
	if len(slots) != 6 {
		t.Fatalf("expected 6 effects, got %d", len(slots))
	}

	const sr = 44100
	input := sineSamples(440, sr, sr) // 1 second

	output := processThrough(ch, input)

	hasNaN := false
	hasInf := false
	nonZero := false
	for _, s := range output {
		if math.IsNaN(s) {
			hasNaN = true
		}
		if math.IsInf(s, 0) {
			hasInf = true
		}
		if math.Abs(s) > 1e-10 {
			nonZero = true
		}
	}
	if hasNaN {
		t.Error("output contains NaN")
	}
	if hasInf {
		t.Error("output contains Inf")
	}
	if !nonZero {
		t.Error("output is silent with all 6 effects")
	}
}

// TestChainDelayTimingViaParam verifies that changing the delay time parameter
// moves the echo to the expected position.
func TestChainDelayTimingViaParam(t *testing.T) {
	resetChains(t)
	id := "integ-delay-timing"
	ch := InstrumentChannel(id)

	// 50ms delay, no feedback, 100% wet.
	AddInsertEffect(id, EffectDelay, map[string]float64{"time": 50, "feedback": 0, "mix": 1})

	const sr = 44100

	// Send impulse and find echo.
	findEchoPeak := func() int {
		n := sr / 2
		out := make([]float64, n)
		out[0] = ch.ProcessSampleLocal(1.0)
		for i := 1; i < n; i++ {
			out[i] = ch.ProcessSampleLocal(0)
		}
		peakIdx := 0
		peakVal := 0.0
		for i := 10; i < n; i++ { // skip first few samples
			if math.Abs(out[i]) > peakVal {
				peakVal = math.Abs(out[i])
				peakIdx = i
			}
		}
		return peakIdx
	}

	peak50 := findEchoPeak()

	// Change to 200ms.
	SetInsertEffectParam(id, 0, "time", 200)
	ResetAllInsertEffects()

	peak200 := findEchoPeak()

	expect50 := int(0.05 * float64(sr)) // ~2205
	expect200 := int(0.2 * float64(sr)) // ~8820

	if math.Abs(float64(peak50-expect50)) > 100 {
		t.Errorf("50ms echo at sample %d, expected near %d", peak50, expect50)
	}
	if math.Abs(float64(peak200-expect200)) > 100 {
		t.Errorf("200ms echo at sample %d, expected near %d", peak200, expect200)
	}
	if peak200 <= peak50 {
		t.Errorf("200ms echo (%d) should be later than 50ms echo (%d)", peak200, peak50)
	}
}

// TestChainFilterCutoffViaParam verifies that lowering the LP filter cutoff
// attenuates a 1kHz signal.
func TestChainFilterCutoffViaParam(t *testing.T) {
	resetChains(t)
	id := "integ-filter-cutoff"
	ch := InstrumentChannel(id)

	// LP filter at 10kHz — should pass 1kHz easily.
	AddInsertEffect(id, EffectFilter, map[string]float64{"mode": 0, "cutoff": 10000, "q": 0.707, "mix": 1})

	const sr = 44100
	input := sineSamples(1000, sr, sr/4)

	highCutoffOut := processThrough(ch, input)
	highRMS := rmsEnergy(highCutoffOut)

	// Lower cutoff to 200Hz — should strongly attenuate 1kHz.
	SetInsertEffectParam(id, 0, "cutoff", 200)

	// Reset filter state for a clean comparison.
	ResetAllInsertEffects()

	lowCutoffOut := processThrough(ch, input)
	lowRMS := rmsEnergy(lowCutoffOut)

	if lowRMS >= highRMS*0.5 {
		t.Errorf("expected 200Hz cutoff to attenuate 1kHz signal significantly: highCutoff=%.4f lowCutoff=%.4f", highRMS, lowRMS)
	}
}
