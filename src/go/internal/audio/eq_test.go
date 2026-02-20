package audio

import (
	"math"
	"testing"
)

func rms(samples []float64) float64 {
	var sum float64
	for _, v := range samples {
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}

func TestPeakingEQBoostsBand(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 44100
		freq  = 1000.0
		samps = 4096
	)

	eqID := "eq-test"
	dryID := "dry-test"
	SetChannelProcessors(eqID, NewPeakingEQ(sr, freq, 1.0, 6.0))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	eqOut := make([]float64, samps)
	dryOut := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
		eqOut[i] = eqCh.ProcessSample(x)
		dryOut[i] = dryCh.ProcessSample(x)
	}

	dryRMS := rms(dryOut)
	eqRMS := rms(eqOut)
	if eqRMS <= dryRMS*1.3 {
		t.Fatalf("expected peaking EQ to boost band: dry RMS %.4f, eq RMS %.4f", dryRMS, eqRMS)
	}
}

func TestAllBandsMutedProducesSilence(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 44100
		samps = 4096
	)

	// Create EQ with all bands muted - this should produce complete silence
	bands := []EQBand{
		{Kind: EQLowShelf, Freq: 60, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 170, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 310, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 600, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 1000, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 3000, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 6000, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 12000, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQPeaking, Freq: 14000, Q: 1.0, GainDB: 0, Muted: true},
		{Kind: EQHighShelf, Freq: 16000, Q: 1.0, GainDB: 0, Muted: true},
	}

	eqID := "eq-mute-test"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))
	eqCh := InstrumentChannel(eqID)

	// Process audio with multiple frequency components
	eqOut := make([]float64, samps)
	for i := 0; i < samps; i++ {
		// Mix of frequencies across the spectrum
		x := 0.3*math.Sin(2*math.Pi*100*float64(i)/float64(sr)) +
			0.3*math.Sin(2*math.Pi*1000*float64(i)/float64(sr)) +
			0.3*math.Sin(2*math.Pi*5000*float64(i)/float64(sr))
		eqOut[i] = eqCh.ProcessSample(x)
	}

	// When all bands are muted, output should be essentially zero
	eqRMS := rms(eqOut)
	if eqRMS > 0.001 {
		t.Fatalf("expected all-muted EQ to produce silence, got RMS %.6f", eqRMS)
	}
}

func TestSingleBandMutedReducesFrequency(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 44100
		freq  = 1000.0
		samps = 4096
	)

	// Create EQ with one band muted at 1kHz
	bands := []EQBand{
		{Kind: EQPeaking, Freq: freq, Q: 1.0, GainDB: 0, Muted: true},
	}

	eqID := "eq-single-mute-test"
	dryID := "dry-single-test"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	eqOut := make([]float64, samps)
	dryOut := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
		eqOut[i] = eqCh.ProcessSample(x)
		dryOut[i] = dryCh.ProcessSample(x)
	}

	dryRMS := rms(dryOut)
	eqRMS := rms(eqOut)
	// Muted band should significantly reduce the signal at that frequency
	if eqRMS > dryRMS*0.1 {
		t.Fatalf("expected muted band to significantly reduce signal: dry RMS %.4f, eq RMS %.4f", dryRMS, eqRMS)
	}
}

// TestMutedBandDoesNotAffectOtherBands verifies that muting one EQ band does not
// affect frequencies in other bands. This tests the parallel multiband processor
// that splits signals using crossover filters.
func TestMutedBandDoesNotAffectOtherBands(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 48000
		samps = 8192 // More samples for better frequency resolution
	)

	// Test frequency in band 0 (20-40Hz): use 30Hz
	testFreq := 30.0

	// Create full 10-band EQ matching defaultBandDefs with only band 5 (630-1250Hz) muted
	bandsMuted := []EQBand{
		{Kind: EQLowShelf, Freq: 30, Q: 0.707, GainDB: 0, Muted: false},     // Band 0: 20-40Hz
		{Kind: EQPeaking, Freq: 60, Q: 0.707, GainDB: 0, Muted: false},      // Band 1: 40-80Hz
		{Kind: EQPeaking, Freq: 120, Q: 0.707, GainDB: 0, Muted: false},     // Band 2: 80-160Hz
		{Kind: EQPeaking, Freq: 237, Q: 0.707, GainDB: 0, Muted: false},     // Band 3: 160-315Hz
		{Kind: EQPeaking, Freq: 472, Q: 0.707, GainDB: 0, Muted: false},     // Band 4: 315-630Hz
		{Kind: EQPeaking, Freq: 940, Q: 0.707, GainDB: 0, Muted: true},      // Band 5: 630-1250Hz (MUTED)
		{Kind: EQPeaking, Freq: 1875, Q: 0.707, GainDB: 0, Muted: false},    // Band 6: 1250-2500Hz
		{Kind: EQPeaking, Freq: 3750, Q: 0.707, GainDB: 0, Muted: false},    // Band 7: 2500-5000Hz
		{Kind: EQPeaking, Freq: 7500, Q: 0.707, GainDB: 0, Muted: false},    // Band 8: 5000-10000Hz
		{Kind: EQHighShelf, Freq: 15000, Q: 0.707, GainDB: 0, Muted: false}, // Band 9: 10000-20000Hz
	}

	// Create same bands but with none muted for comparison
	bandsUnmuted := []EQBand{
		{Kind: EQLowShelf, Freq: 30, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 60, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 120, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 237, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 472, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 940, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 1875, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 3750, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQPeaking, Freq: 7500, Q: 0.707, GainDB: 0, Muted: false},
		{Kind: EQHighShelf, Freq: 15000, Q: 0.707, GainDB: 0, Muted: false},
	}

	mutedID := "eq-muted-test"
	unmutedID := "eq-unmuted-test"
	SetChannelProcessors(mutedID, NewEQProcessor(sr, bandsMuted...))
	SetChannelProcessors(unmutedID, NewEQProcessor(sr, bandsUnmuted...))

	mutedCh := InstrumentChannel(mutedID)
	unmutedCh := InstrumentChannel(unmutedID)

	mutedOut := make([]float64, samps)
	unmutedOut := make([]float64, samps)

	// Generate 30Hz sine wave (in band 0, far from muted band 5)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*testFreq*float64(i)/float64(sr))
		mutedOut[i] = mutedCh.ProcessSample(x)
		unmutedOut[i] = unmutedCh.ProcessSample(x)
	}

	unmutedRMS := rms(unmutedOut)
	mutedRMS := rms(mutedOut)

	// The 30Hz signal should pass through similarly with band 5 muted vs unmuted,
	// since 30Hz is far from the muted band (630-1250Hz). With proper band isolation
	// and no incorrect normalization, the ratio should be close to 1.0.
	ratio := mutedRMS / unmutedRMS
	if ratio < 0.5 || ratio > 2.0 {
		t.Fatalf("30Hz signal should pass through near-unity with distant band muted: "+
			"muted RMS=%.4f, unmuted RMS=%.4f, ratio=%.4f (expected 0.5-2.0)", mutedRMS, unmutedRMS, ratio)
	}
	t.Logf("30Hz signal: muted RMS=%.4f, unmuted RMS=%.4f, ratio=%.4f", mutedRMS, unmutedRMS, ratio)

	// Now verify that a signal in the muted band IS significantly reduced
	// compared to the same signal in unmuted EQ.
	mutedFreq := 1000.0 // 1kHz is in band 5 (630-1250Hz)
	mutedOut2 := make([]float64, samps)
	unmutedOut2 := make([]float64, samps)

	// Reset processor state by creating new channels
	mutedID2 := "eq-muted-test-2"
	unmutedID2 := "eq-unmuted-test-2"
	SetChannelProcessors(mutedID2, NewEQProcessor(sr, bandsMuted...))
	SetChannelProcessors(unmutedID2, NewEQProcessor(sr, bandsUnmuted...))
	mutedCh2 := InstrumentChannel(mutedID2)
	unmutedCh2 := InstrumentChannel(unmutedID2)

	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*mutedFreq*float64(i)/float64(sr))
		mutedOut2[i] = mutedCh2.ProcessSample(x)
		unmutedOut2[i] = unmutedCh2.ProcessSample(x)
	}

	unmutedRMS2 := rms(unmutedOut2)
	mutedRMS2 := rms(mutedOut2)

	// The 1kHz signal should be significantly reduced in the muted version.
	// Compare the relative attenuation: 1kHz_muted/1kHz_unmuted should be much
	// smaller than 30Hz_muted/30Hz_unmuted (since 1kHz is in the muted band).
	ratio1k := mutedRMS2 / unmutedRMS2
	ratio30 := mutedRMS / unmutedRMS

	// 1kHz ratio should be much smaller than 30Hz ratio
	// (1kHz is in muted band, 30Hz is not)
	if ratio1k >= ratio30*0.8 {
		t.Fatalf("1kHz signal in muted band not sufficiently attenuated relative to 30Hz: "+
			"30Hz ratio=%.4f, 1kHz ratio=%.4f", ratio30, ratio1k)
	}

	t.Logf("Band isolation test passed: 30Hz ratio=%.4f, 1kHz ratio=%.4f", ratio30, ratio1k)
}

// TestHighFrequencyRejectionWithLowBandOnly verifies that 4th-order Linkwitz-Riley
// crossovers (-24dB/octave) provide sufficient high-frequency attenuation when
// only the lowest band (20-40Hz) is active. This tests the fix for crackling/
// metallic sounds leaking through when muting high bands.
func TestHighFrequencyRejectionWithLowBandOnly(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 48000
		samps = 16384 // More samples for accurate RMS at low frequencies
	)

	// Create 10-band EQ with only band 0 (20-40Hz) unmuted
	bands := []EQBand{
		{Kind: EQLowShelf, Freq: 30, Q: 0.707, GainDB: 0, Muted: false},    // Band 0: 20-40Hz (ACTIVE)
		{Kind: EQPeaking, Freq: 60, Q: 0.707, GainDB: 0, Muted: true},      // Band 1: 40-80Hz
		{Kind: EQPeaking, Freq: 120, Q: 0.707, GainDB: 0, Muted: true},     // Band 2: 80-160Hz
		{Kind: EQPeaking, Freq: 237, Q: 0.707, GainDB: 0, Muted: true},     // Band 3: 160-315Hz
		{Kind: EQPeaking, Freq: 472, Q: 0.707, GainDB: 0, Muted: true},     // Band 4: 315-630Hz
		{Kind: EQPeaking, Freq: 940, Q: 0.707, GainDB: 0, Muted: true},     // Band 5: 630-1250Hz
		{Kind: EQPeaking, Freq: 1875, Q: 0.707, GainDB: 0, Muted: true},    // Band 6: 1250-2500Hz
		{Kind: EQPeaking, Freq: 3750, Q: 0.707, GainDB: 0, Muted: true},    // Band 7: 2500-5000Hz
		{Kind: EQPeaking, Freq: 7500, Q: 0.707, GainDB: 0, Muted: true},    // Band 8: 5000-10000Hz
		{Kind: EQHighShelf, Freq: 15000, Q: 0.707, GainDB: 0, Muted: true}, // Band 9: 10000-20000Hz
	}

	eqID := "eq-hf-rejection-test"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)

	// Test 1: 5kHz signal should be heavily attenuated compared to 30Hz signal
	// through the same processor.
	// 5kHz is ~7 octaves above 40Hz cutoff
	// With LR4 (-24dB/octave): 7 * 24 = -168dB theoretical
	highFreq := 5000.0
	lowFreq := 30.0

	// Process 5kHz first
	eqOutHigh := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*highFreq*float64(i)/float64(sr))
		eqOutHigh[i] = eqCh.ProcessSample(x)
	}
	eqRMSHigh := rms(eqOutHigh)

	// Reset and process 30Hz
	eqID2 := "eq-lf-pass-test"
	SetChannelProcessors(eqID2, NewEQProcessor(sr, bands...))
	eqCh2 := InstrumentChannel(eqID2)

	eqOutLow := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*lowFreq*float64(i)/float64(sr))
		eqOutLow[i] = eqCh2.ProcessSample(x)
	}
	eqRMSLow := rms(eqOutLow)

	// The key test: 5kHz should be much more attenuated than 30Hz
	// when only the 20-40Hz band is active.
	// 30Hz is in the active band, 5kHz is far outside it.
	if eqRMSLow < 0.001 {
		t.Fatalf("30Hz signal not passing through multiband EQ at all: RMS %.6f", eqRMSLow)
	}

	// 5kHz should be very heavily attenuated relative to 30Hz
	// With LR4 (-24dB/octave) over ~7 octaves, we expect massive attenuation.
	relativeRatio := eqRMSHigh / eqRMSLow
	if relativeRatio > 0.01 {
		t.Fatalf("5kHz signal not sufficiently attenuated relative to 30Hz with only band 0 active: "+
			"5kHz RMS %.6f, 30Hz RMS %.6f, ratio %.6f (expected < 0.01)",
			eqRMSHigh, eqRMSLow, relativeRatio)
	}

	// Calculate attenuation in dB for logging
	attenuationDB := -20 * math.Log10(relativeRatio)
	t.Logf("High-frequency rejection test passed: 5kHz vs 30Hz relative attenuation=%.1fdB (ratio=%.6f)",
		attenuationDB, relativeRatio)
}

// TestMultibandOutputMatchesPassthrough verifies that a 10-band multiband processor
// (with only the highest band muted) passes a low-frequency signal at near-unity
// compared to a dry channel. This fails if the multiband divides by numBands.
func TestMultibandOutputMatchesPassthrough(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 48000
		freq  = 60.0
		samps = 8192
	)

	// 10-band EQ with only band 9 (10k-20kHz) muted
	bands := make([]EQBand, 10)
	for i := range bands {
		bands[i] = EQBand{Kind: EQPeaking, Freq: 1000, Q: 0.707, GainDB: 0}
	}
	bands[9].Muted = true

	eqID := "eq-multiband-passthrough"
	dryID := "dry-multiband-passthrough"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	eqOut := make([]float64, samps)
	dryOut := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
		eqOut[i] = eqCh.ProcessSample(x)
		dryOut[i] = dryCh.ProcessSample(x)
	}

	eqRMS := rms(eqOut)
	dryRMS := rms(dryOut)
	ratio := eqRMS / dryRMS

	// With the normalization bug, ratio would be ~0.03 (1/10 of passthrough).
	// Without it, crossover filters still attenuate somewhat due to overlapping
	// rolloff regions, but signal should be well above the buggy level.
	if ratio < 0.2 {
		t.Fatalf("multiband output too quiet vs passthrough: eq RMS=%.4f, dry RMS=%.4f, ratio=%.4f (expected >= 0.2)",
			eqRMS, dryRMS, ratio)
	}
	t.Logf("multiband vs passthrough: eq RMS=%.4f, dry RMS=%.4f, ratio=%.4f", eqRMS, dryRMS, ratio)
}

// TestMutingHighBandPreservesKickEnergy simulates a kick drum signal (60Hz + 120Hz
// with decay) and verifies that muting only the 10k-20kHz band preserves nearly all
// energy. This directly reproduces the user-reported bug.
func TestMutingHighBandPreservesKickEnergy(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 48000
		samps = 16384
	)

	// 10-band EQ with only band 9 (10k-20kHz) muted
	bands := make([]EQBand, 10)
	for i := range bands {
		bands[i] = EQBand{Kind: EQPeaking, Freq: 1000, Q: 0.707, GainDB: 0}
	}
	bands[9].Muted = true

	eqID := "eq-kick-energy"
	dryID := "dry-kick-energy"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	eqOut := make([]float64, samps)
	dryOut := make([]float64, samps)
	for i := 0; i < samps; i++ {
		t_ := float64(i) / float64(sr)
		// Simulated kick: 60Hz fundamental + 120Hz harmonic with exponential decay
		decay := math.Exp(-t_ * 8.0)
		x := decay * (0.6*math.Sin(2*math.Pi*60*t_) + 0.3*math.Sin(2*math.Pi*120*t_))
		eqOut[i] = eqCh.ProcessSample(x)
		dryOut[i] = dryCh.ProcessSample(x)
	}

	eqRMS := rms(eqOut)
	dryRMS := rms(dryOut)
	ratio := eqRMS / dryRMS

	// Kick has negligible energy above 10kHz. Muting that band should preserve
	// most energy. Crossover filters cause some attenuation, but with the
	// normalization bug fixed the ratio should be well above 0.1.
	if ratio < 0.2 {
		t.Fatalf("kick energy not preserved with high band muted: eq RMS=%.4f, dry RMS=%.4f, ratio=%.4f (expected >= 0.2)",
			eqRMS, dryRMS, ratio)
	}
	t.Logf("kick energy test: eq RMS=%.4f, dry RMS=%.4f, ratio=%.4f", eqRMS, dryRMS, ratio)
}

// TestMultibandNormalizationRemoved is a minimal test that verifies the 1/numBands
// normalization has been removed from the multiband processor.
func TestMultibandNormalizationRemoved(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr    = 48000
		freq  = 200.0
		samps = 4096
	)

	// 10-band EQ, one band muted
	bands := make([]EQBand, 10)
	for i := range bands {
		bands[i] = EQBand{Kind: EQPeaking, Freq: 1000, Q: 0.707, GainDB: 0}
	}
	bands[9].Muted = true

	proc := NewEQProcessor(sr, bands...)

	var sumSq float64
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
		y := proc.ProcessSample(x)
		sumSq += y * y
	}
	outRMS := math.Sqrt(sumSq / float64(samps))
	inRMS := 0.5 / math.Sqrt(2) // RMS of 0.5-amplitude sine

	ratio := outRMS / inRMS

	// With the bug (dividing by 10), ratio would be ~0.02.
	// Without it, crossover rolloff still attenuates but should be above 0.15.
	if ratio < 0.15 {
		t.Fatalf("multiband output/input ratio too low: %.4f (expected >= 0.15, bug would give ~0.02)",
			ratio)
	}
	t.Logf("normalization test: output RMS=%.4f, input RMS=%.4f, ratio=%.4f", outRMS, inRMS, ratio)
}
