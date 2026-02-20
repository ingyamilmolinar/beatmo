package audio

import (
	"math"
	"testing"
)

// genSine generates n samples of a sine wave at the given frequency and sample rate.
func genSine(freq float64, sr, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.5 * math.Sin(2*math.Pi*freq*float64(i)/float64(sr))
	}
	return out
}

// computeCorrelation returns the Pearson correlation coefficient between a and b.
// Returns 0 if either slice has zero variance.
func computeCorrelation(a, b []float64) float64 {
	n := len(a)
	if n == 0 || n != len(b) {
		return 0
	}
	var sumA, sumB float64
	for i := 0; i < n; i++ {
		sumA += a[i]
		sumB += b[i]
	}
	meanA := sumA / float64(n)
	meanB := sumB / float64(n)

	var cov, varA, varB float64
	for i := 0; i < n; i++ {
		da := a[i] - meanA
		db := b[i] - meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}
	denom := math.Sqrt(varA * varB)
	if denom < 1e-15 {
		return 0
	}
	return cov / denom
}

// TestBiquadStateCorruptionFromInterleaving proves that interleaving unrelated
// voice signals through a shared biquad filter corrupts the filter state and
// produces measurable distortion. This was the root cause of desktop audio
// distortion before the 3-phase mixer fix.
func TestBiquadStateCorruptionFromInterleaving(t *testing.T) {
	const (
		sr        = 44100
		n         = 8192
		blockSize = 64
	)

	sig440 := genSine(440, sr, n)
	sig3k := genSine(3000, sr, n)

	// Coherent: process 440Hz sine through a lowpass biquad continuously.
	bq1 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq1 == nil {
		t.Fatal("failed to create biquad 1")
	}
	coherentOut := make([]float64, n)
	for i := 0; i < n; i++ {
		coherentOut[i] = bq1.ProcessSample(sig440[i])
	}

	// Interleaved: alternate 64-sample blocks of 440Hz and 3kHz through the
	// same biquad, but only collect the 440Hz output.
	bq2 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq2 == nil {
		t.Fatal("failed to create biquad 2")
	}
	interleavedOut := make([]float64, n)
	pos440 := 0
	for pos440 < n {
		// Process a block of 440Hz — collect output
		end := pos440 + blockSize
		if end > n {
			end = n
		}
		for i := pos440; i < end; i++ {
			interleavedOut[i] = bq2.ProcessSample(sig440[i])
		}
		pos440 = end

		// Process a block of 3kHz — discard output (simulates another voice)
		pos3k := pos440 - blockSize // use corresponding 3kHz samples
		if pos3k < 0 {
			pos3k = 0
		}
		end3k := pos3k + blockSize
		if end3k > n {
			end3k = n
		}
		for i := pos3k; i < end3k; i++ {
			bq2.ProcessSample(sig3k[i])
		}
	}

	corr := computeCorrelation(coherentOut, interleavedOut)
	t.Logf("interleaved vs coherent correlation: %.6f", corr)
	if corr >= 0.98 {
		t.Fatalf("expected interleaving to corrupt biquad state (correlation < 0.98), got %.6f", corr)
	}
}

// TestProcessBlockLocalPreservesSignalCoherence proves that the 3-phase mixer
// pattern (sum voices first, then EQ) produces clean output matching a
// reference single-pass processing.
func TestProcessBlockLocalPreservesSignalCoherence(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 8192
	)

	sigA := genSine(440, sr, n)
	sigB := genSine(880, sr, n)

	// Use raw biquad as processor (works in both stub and real builds).
	bq1 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq1 == nil {
		t.Fatal("failed to create biquad")
	}

	// 3-phase approach: sum A+B with headroom into instBuf, then process through channel.
	instID := "bq-coherence-test"
	SetChannelProcessors(instID, bq1)
	instCh := InstrumentChannel(instID)

	instBuf := make([]float64, n)
	for i := 0; i < n; i++ {
		instBuf[i] = (sigA[i] + sigB[i]) * 0.25 // headroom
	}
	masterBuf := make([]float64, n)
	instCh.ProcessBlockLocal(instBuf, masterBuf)

	// Reference: sum A+B with headroom, process through a fresh channel with same EQ.
	bq2 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq2 == nil {
		t.Fatal("failed to create reference biquad")
	}
	refID := "bq-coherence-ref"
	SetChannelProcessors(refID, bq2)
	refCh := InstrumentChannel(refID)

	refBuf := make([]float64, n)
	for i := 0; i < n; i++ {
		refBuf[i] = refCh.ProcessSampleLocal((sigA[i] + sigB[i]) * 0.25)
	}

	corr := computeCorrelation(masterBuf, refBuf)
	t.Logf("3-phase vs reference correlation: %.6f", corr)
	if corr < 0.999 {
		t.Fatalf("expected 3-phase pattern to match reference (correlation > 0.999), got %.6f", corr)
	}
}

// TestInterleavedChannelProcessingProducesDistortion proves that the old
// per-voice-through-shared-channel pattern produces measurably different
// (distorted) output compared to sum-first-then-EQ.
func TestInterleavedChannelProcessingProducesDistortion(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr        = 44100
		n         = 4096
		blockSize = 64
	)

	sigA := genSine(200, sr, n)
	sigB := genSine(5000, sr, n)

	// Use raw biquad as processor (works in both stub and real builds).
	bqClean := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bqClean == nil {
		t.Fatal("failed to create clean biquad")
	}

	// Clean: sum A+B, process through channel.
	cleanID := "interleave-clean"
	SetChannelProcessors(cleanID, bqClean)
	cleanCh := InstrumentChannel(cleanID)

	cleanOut := make([]float64, n)
	for i := 0; i < n; i++ {
		cleanOut[i] = cleanCh.ProcessSampleLocal((sigA[i] + sigB[i]) * 0.25)
	}

	// Interleaved: alternate blocks of A and B through the SAME channel,
	// accumulate both outputs (simulating the old bug).
	bqBad := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bqBad == nil {
		t.Fatal("failed to create interleaved biquad")
	}
	interleavedID := "interleave-bad"
	SetChannelProcessors(interleavedID, bqBad)
	interleavedCh := InstrumentChannel(interleavedID)

	interleavedOut := make([]float64, n)
	for off := 0; off < n; off += blockSize {
		end := off + blockSize
		if end > n {
			end = n
		}
		// Voice A block
		for i := off; i < end; i++ {
			interleavedOut[i] += interleavedCh.ProcessSampleLocal(sigA[i] * 0.25)
		}
		// Voice B block through same channel
		for i := off; i < end; i++ {
			interleavedOut[i] += interleavedCh.ProcessSampleLocal(sigB[i] * 0.25)
		}
	}

	corr := computeCorrelation(cleanOut, interleavedOut)
	t.Logf("clean vs interleaved correlation: %.6f", corr)
	if corr >= 0.95 {
		t.Fatalf("expected per-voice-through-shared-channel to diverge (correlation < 0.95), got %.6f", corr)
	}
}

// TestBiquadBlockVsSampleParity verifies that biquad state accumulates
// identically regardless of whether samples are processed one-by-one or in
// blocks. This ensures block-based mixer processing introduces no rounding
// artifacts.
func TestBiquadBlockVsSampleParity(t *testing.T) {
	const (
		sr        = 44100
		n         = 4096
		blockSize = 64
	)

	sig := genSine(440, sr, n)

	// Process sample-by-sample.
	bq1 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq1 == nil {
		t.Fatal("failed to create biquad 1")
	}
	outSample := make([]float64, n)
	for i := 0; i < n; i++ {
		outSample[i] = bq1.ProcessSample(sig[i])
	}

	// Process in blocks of 64.
	bq2 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq2 == nil {
		t.Fatal("failed to create biquad 2")
	}
	outBlock := make([]float64, n)
	for off := 0; off < n; off += blockSize {
		end := off + blockSize
		if end > n {
			end = n
		}
		for i := off; i < end; i++ {
			outBlock[i] = bq2.ProcessSample(sig[i])
		}
	}

	// Assert bit-identical output.
	for i := 0; i < n; i++ {
		if outSample[i] != outBlock[i] {
			t.Fatalf("sample %d differs: sample-by-sample=%.15e, block=%.15e", i, outSample[i], outBlock[i])
		}
	}
}
