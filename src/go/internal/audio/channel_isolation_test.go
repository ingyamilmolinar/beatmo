package audio

import (
	"math"
	"testing"
)

// TestProcessBlockLocalDoesNotChainToParent verifies that ProcessBlockLocal
// applies this channel's volume and EQ but does NOT recurse to the parent
// channel. This is the key distinction that makes the 3-phase mixer work.
func TestProcessBlockLocalDoesNotChainToParent(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 4096
	)

	// Master channel with aggressive lowpass biquad that attenuates high frequencies.
	bqMain := makeBiquad(EQLowpass, sr, 500, 0.707, 0)
	if bqMain == nil {
		t.Fatal("failed to create master biquad")
	}
	SetChannelProcessors("main", bqMain)

	// Instrument channel with no EQ (just passes signal through).
	instID := "chain-test"
	_ = InstrumentChannel(instID)

	// 8kHz sine — well above the master's 500Hz lowpass cutoff.
	sig := genSine(8000, sr, n)
	inputRMS := rms(sig)

	// ProcessBlockLocal: should NOT apply master EQ → signal should pass.
	localOut := make([]float64, n)
	instCh := InstrumentChannel(instID)
	instCh.ProcessBlockLocal(sig, localOut)
	localRMS := rms(localOut)

	// ProcessBlock: SHOULD apply master EQ → signal should be attenuated.
	instID2 := "chain-test-2"
	_ = InstrumentChannel(instID2)
	chainedOut := make([]float64, n)
	instCh2 := InstrumentChannel(instID2)
	instCh2.ProcessBlock(sig, chainedOut)
	chainedRMS := rms(chainedOut)

	localRatio := localRMS / inputRMS
	chainedRatio := chainedRMS / inputRMS

	t.Logf("local ratio: %.4f, chained ratio: %.4f", localRatio, chainedRatio)

	// Local should preserve most of the signal (volume=1, no EQ).
	if localRatio < 0.9 {
		t.Fatalf("ProcessBlockLocal attenuated signal too much: ratio=%.4f, expected > 0.9", localRatio)
	}

	// Chained should attenuate significantly via master lowpass (8kHz through 500Hz LP).
	if chainedRatio > 0.3 {
		t.Fatalf("ProcessBlock did not attenuate through master: ratio=%.4f, expected < 0.3", chainedRatio)
	}
}

// TestMultiInstrumentMasterBiquadCoherence verifies that the 3-phase mixer
// pattern produces correct results when multiple instruments are summed and
// then processed through master EQ. The master lowpass should attenuate
// high-frequency instruments while passing low-frequency ones.
func TestMultiInstrumentMasterBiquadCoherence(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 8192
	)

	// Create 3 instrument channels with no per-instrument EQ.
	kickID := "coherence-kick"
	snareID := "coherence-snare"
	hihatID := "coherence-hihat"
	_ = InstrumentChannel(kickID)
	_ = InstrumentChannel(snareID)
	_ = InstrumentChannel(hihatID)

	// Master channel with lowpass biquad at 2kHz.
	bqMain := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	if bqMain == nil {
		t.Fatal("failed to create master biquad")
	}
	SetChannelProcessors("main", bqMain)

	sigKick := genSine(200, sr, n)   // well below cutoff
	sigSnare := genSine(800, sr, n)  // below cutoff
	sigHihat := genSine(5000, sr, n) // above cutoff

	// 3-phase simulation:
	// Phase 1: accumulate per-instrument (with headroom)
	kickBuf := make([]float64, n)
	snareBuf := make([]float64, n)
	hihatBuf := make([]float64, n)
	for i := 0; i < n; i++ {
		kickBuf[i] = sigKick[i] * 0.25
		snareBuf[i] = sigSnare[i] * 0.25
		hihatBuf[i] = sigHihat[i] * 0.25
	}

	// Phase 2: ProcessBlockLocal per instrument → masterBuf
	masterBuf := make([]float64, n)
	InstrumentChannel(kickID).ProcessBlockLocal(kickBuf, masterBuf)
	InstrumentChannel(snareID).ProcessBlockLocal(snareBuf, masterBuf)
	InstrumentChannel(hihatID).ProcessBlockLocal(hihatBuf, masterBuf)

	// Phase 3: master ProcessBlockLocal → workBuf
	workBuf := make([]float64, n)
	mainCh := channelForInstrument("")
	mainCh.ProcessBlockLocal(masterBuf, workBuf)

	// Reference: sum all 3 signals, process through a fresh lowpass biquad.
	bqRef := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	if bqRef == nil {
		t.Fatal("failed to create reference biquad")
	}
	refID := "coherence-ref"
	SetChannelProcessors(refID, bqRef)
	refCh := InstrumentChannel(refID)
	refBuf := make([]float64, n)
	for i := 0; i < n; i++ {
		mixed := (sigKick[i] + sigSnare[i] + sigHihat[i]) * 0.25
		refBuf[i] = refCh.ProcessSampleLocal(mixed)
	}

	corr := computeCorrelation(workBuf, refBuf)
	t.Logf("3-phase multi-instrument vs reference correlation: %.6f", corr)
	if corr < 0.999 {
		t.Fatalf("3-phase multi-instrument output diverges from reference (correlation < 0.999): %.6f", corr)
	}

	// Also verify that 5kHz hihat is attenuated by the master lowpass.
	// Process hihat alone through the 3-phase path with a fresh master biquad.
	bqMain2 := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	if bqMain2 == nil {
		t.Fatal("failed to create second master biquad")
	}
	SetChannelProcessors("main", bqMain2)

	hihatOnlyMaster := make([]float64, n)
	hihatOnlyID := "coherence-hihat-only"
	_ = InstrumentChannel(hihatOnlyID)
	hihatOnlyInst := make([]float64, n)
	for i := 0; i < n; i++ {
		hihatOnlyInst[i] = sigHihat[i] * 0.25
	}
	InstrumentChannel(hihatOnlyID).ProcessBlockLocal(hihatOnlyInst, hihatOnlyMaster)

	hihatWork := make([]float64, n)
	mainCh2 := channelForInstrument("")
	mainCh2.ProcessBlockLocal(hihatOnlyMaster, hihatWork)

	hihatDryRMS := rms(hihatOnlyInst)
	hihatWetRMS := rms(hihatWork)
	attenuation := hihatWetRMS / hihatDryRMS
	t.Logf("hihat attenuation through 2kHz LP: %.4f (dry RMS=%.6f, wet RMS=%.6f)",
		attenuation, hihatDryRMS, hihatWetRMS)
	if attenuation > 0.5 {
		t.Fatalf("expected 5kHz hihat to be attenuated by 2kHz lowpass, got ratio=%.4f", attenuation)
	}
}

// TestProcessBlockLocalAccumulates verifies that ProcessBlockLocal accumulates
// into the output buffer rather than overwriting it. This is critical for the
// mixer's per-instrument → master accumulation pattern.
func TestProcessBlockLocalAccumulates(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 1024
	)

	instID := "accum-test"
	_ = InstrumentChannel(instID)
	// Volume = 1, no EQ — pass-through.

	sig := genSine(1000, sr, n)

	// Pre-fill output with 0.5.
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.5
	}

	instCh := InstrumentChannel(instID)
	instCh.ProcessBlockLocal(sig, out)

	// Each output sample should be 0.5 + sig[i].
	for i := 0; i < n; i++ {
		expected := 0.5 + sig[i]
		if math.Abs(out[i]-expected) > 1e-10 {
			t.Fatalf("sample %d: expected %.15f, got %.15f (diff=%.2e)",
				i, expected, out[i], out[i]-expected)
		}
	}
}
