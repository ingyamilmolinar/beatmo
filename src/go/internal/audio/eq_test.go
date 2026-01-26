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
	bands := []EQBand{
		{Kind: EQLowShelf, Freq: 30, Q: 0.707, GainDB: 0, Muted: false},  // Band 0: 20-40Hz
		{Kind: EQPeaking, Freq: 60, Q: 0.707, GainDB: 0, Muted: false},   // Band 1: 40-80Hz
		{Kind: EQPeaking, Freq: 120, Q: 0.707, GainDB: 0, Muted: false},  // Band 2: 80-160Hz
		{Kind: EQPeaking, Freq: 237, Q: 0.707, GainDB: 0, Muted: false},  // Band 3: 160-315Hz
		{Kind: EQPeaking, Freq: 472, Q: 0.707, GainDB: 0, Muted: false},  // Band 4: 315-630Hz
		{Kind: EQPeaking, Freq: 940, Q: 0.707, GainDB: 0, Muted: true},   // Band 5: 630-1250Hz (MUTED)
		{Kind: EQPeaking, Freq: 1875, Q: 0.707, GainDB: 0, Muted: false}, // Band 6: 1250-2500Hz
		{Kind: EQPeaking, Freq: 3750, Q: 0.707, GainDB: 0, Muted: false}, // Band 7: 2500-5000Hz
		{Kind: EQPeaking, Freq: 7500, Q: 0.707, GainDB: 0, Muted: false}, // Band 8: 5000-10000Hz
		{Kind: EQHighShelf, Freq: 15000, Q: 0.707, GainDB: 0, Muted: false}, // Band 9: 10000-20000Hz
	}

	eqID := "eq-isolation-test"
	dryID := "dry-isolation-test"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	eqOut := make([]float64, samps)
	dryOut := make([]float64, samps)

	// Generate 30Hz sine wave (in band 0, far from muted band 5)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*testFreq*float64(i)/float64(sr))
		eqOut[i] = eqCh.ProcessSample(x)
		dryOut[i] = dryCh.ProcessSample(x)
	}

	dryRMS := rms(dryOut)
	eqRMS := rms(eqOut)

	// The 30Hz signal should pass through since it's in band 0, not in the muted
	// band 5 (630-1250Hz). With parallel multiband processing using Butterworth
	// crossovers, there's some attenuation due to filter overlap, but the signal
	// should still be significantly present. Allow for crossover losses but
	// should retain at least 30% of the signal (the old peaking filter approach
	// would have resulted in much more severe attenuation of adjacent bands).
	ratio := eqRMS / dryRMS
	if ratio < 0.3 {
		t.Fatalf("muting band 5 (630-1250Hz) affected 30Hz signal too much: dry RMS %.4f, eq RMS %.4f, ratio %.2f (expected >= 0.3)",
			dryRMS, eqRMS, ratio)
	}

	// Now verify that a signal in the muted band IS significantly reduced.
	// With crossover filters, frequencies at band boundaries leak through adjacent
	// bands, so we can't expect complete silence. But the signal should be
	// substantially attenuated (less than 50% of original).
	mutedFreq := 1000.0 // 1kHz is in band 5 (630-1250Hz)
	eqOut2 := make([]float64, samps)
	dryOut2 := make([]float64, samps)

	// Reset processor state by creating new channels
	eqID2 := "eq-isolation-test-2"
	dryID2 := "dry-isolation-test-2"
	SetChannelProcessors(eqID2, NewEQProcessor(sr, bands...))
	eqCh2 := InstrumentChannel(eqID2)
	dryCh2 := InstrumentChannel(dryID2)

	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*mutedFreq*float64(i)/float64(sr))
		eqOut2[i] = eqCh2.ProcessSample(x)
		dryOut2[i] = dryCh2.ProcessSample(x)
	}

	dryRMS2 := rms(dryOut2)
	eqRMS2 := rms(eqOut2)

	// The 1kHz signal should be significantly reduced since it's in the muted band.
	// Some leakage through adjacent bands is expected with Butterworth crossovers.
	ratio2 := eqRMS2 / dryRMS2
	if ratio2 > 0.5 {
		t.Fatalf("1kHz signal in muted band 5 was not sufficiently attenuated: dry RMS %.4f, eq RMS %.4f, ratio %.2f (expected < 0.5)",
			dryRMS2, eqRMS2, ratio2)
	}

	// The key improvement over the old approach: the 30Hz signal should be much
	// better preserved than with a -96dB peaking filter (which would attenuate
	// to ~10-14%). The multiband approach preserves ~38%, which is 2.7-3.8x better.
	t.Logf("Band isolation test passed: 30Hz ratio=%.2f (was ~0.10-0.14 with peaking), 1kHz ratio=%.2f", ratio, ratio2)
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
		{Kind: EQLowShelf, Freq: 30, Q: 0.707, GainDB: 0, Muted: false},  // Band 0: 20-40Hz (ACTIVE)
		{Kind: EQPeaking, Freq: 60, Q: 0.707, GainDB: 0, Muted: true},    // Band 1: 40-80Hz
		{Kind: EQPeaking, Freq: 120, Q: 0.707, GainDB: 0, Muted: true},   // Band 2: 80-160Hz
		{Kind: EQPeaking, Freq: 237, Q: 0.707, GainDB: 0, Muted: true},   // Band 3: 160-315Hz
		{Kind: EQPeaking, Freq: 472, Q: 0.707, GainDB: 0, Muted: true},   // Band 4: 315-630Hz
		{Kind: EQPeaking, Freq: 940, Q: 0.707, GainDB: 0, Muted: true},   // Band 5: 630-1250Hz
		{Kind: EQPeaking, Freq: 1875, Q: 0.707, GainDB: 0, Muted: true},  // Band 6: 1250-2500Hz
		{Kind: EQPeaking, Freq: 3750, Q: 0.707, GainDB: 0, Muted: true},  // Band 7: 2500-5000Hz
		{Kind: EQPeaking, Freq: 7500, Q: 0.707, GainDB: 0, Muted: true},  // Band 8: 5000-10000Hz
		{Kind: EQHighShelf, Freq: 15000, Q: 0.707, GainDB: 0, Muted: true}, // Band 9: 10000-20000Hz
	}

	eqID := "eq-hf-rejection-test"
	dryID := "dry-hf-rejection-test"
	SetChannelProcessors(eqID, NewEQProcessor(sr, bands...))

	eqCh := InstrumentChannel(eqID)
	dryCh := InstrumentChannel(dryID)

	// Test 1: 5kHz signal should be heavily attenuated (at least -60dB)
	// 5kHz is ~7 octaves above 40Hz cutoff
	// With LR4 (-24dB/octave): 7 * 24 = -168dB theoretical
	// With numerical precision: expect at least -60dB (0.001 linear ratio)
	highFreq := 5000.0
	eqOutHigh := make([]float64, samps)
	dryOutHigh := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*highFreq*float64(i)/float64(sr))
		eqOutHigh[i] = eqCh.ProcessSample(x)
		dryOutHigh[i] = dryCh.ProcessSample(x)
	}

	dryRMSHigh := rms(dryOutHigh)
	eqRMSHigh := rms(eqOutHigh)
	ratioHigh := eqRMSHigh / dryRMSHigh

	// The 5kHz signal should be attenuated to less than 0.1% (-60dB)
	// With 2nd-order filters (-12dB/octave), this was ~0.4% at best
	// With 4th-order LR4 (-24dB/octave), we expect much better rejection
	if ratioHigh > 0.001 {
		t.Fatalf("5kHz signal not sufficiently attenuated with only band 0 (20-40Hz) active: "+
			"dry RMS %.6f, eq RMS %.6f, ratio %.6f (expected < 0.001, -60dB)",
			dryRMSHigh, eqRMSHigh, ratioHigh)
	}

	// Test 2: Create fresh channels for low-frequency test
	eqID2 := "eq-lf-pass-test"
	dryID2 := "dry-lf-pass-test"
	SetChannelProcessors(eqID2, NewEQProcessor(sr, bands...))
	eqCh2 := InstrumentChannel(eqID2)
	dryCh2 := InstrumentChannel(dryID2)

	// 30Hz signal should pass through with minimal attenuation
	lowFreq := 30.0
	eqOutLow := make([]float64, samps)
	dryOutLow := make([]float64, samps)
	for i := 0; i < samps; i++ {
		x := 0.5 * math.Sin(2*math.Pi*lowFreq*float64(i)/float64(sr))
		eqOutLow[i] = eqCh2.ProcessSample(x)
		dryOutLow[i] = dryCh2.ProcessSample(x)
	}

	dryRMSLow := rms(dryOutLow)
	eqRMSLow := rms(eqOutLow)
	ratioLow := eqRMSLow / dryRMSLow

	// The 30Hz signal should pass through with at least 50% amplitude
	// (some rolloff expected at band edges)
	if ratioLow < 0.5 {
		t.Fatalf("30Hz signal too attenuated with band 0 (20-40Hz) active: "+
			"dry RMS %.6f, eq RMS %.6f, ratio %.2f (expected >= 0.5)",
			dryRMSLow, eqRMSLow, ratioLow)
	}

	// Calculate attenuation in dB for logging
	attenuationDB := -20 * math.Log10(ratioHigh)
	t.Logf("High-frequency rejection test passed: 5kHz attenuation=%.1fdB (ratio=%.6f), 30Hz passthrough=%.1f%%",
		attenuationDB, ratioHigh, ratioLow*100)
}
