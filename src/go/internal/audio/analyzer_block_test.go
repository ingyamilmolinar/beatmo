package audio

import (
	"math"
	"testing"
)

// TestAnalyzerBlockSampleParity verifies that ProcessBlock produces the same
// RMS/peak as equivalent ProcessSample calls.
func TestAnalyzerBlockSampleParity(t *testing.T) {
	const (
		sr = 44100
		n  = 1024 // must be power-of-two to fill analyzer window exactly
	)
	sig := genSine(440, sr, n)

	// Per-sample.
	a1 := NewAnalyzer(n)
	for i := 0; i < n; i++ {
		a1.ProcessSample(sig[i])
	}
	snap1 := a1.Snapshot()

	// Block.
	a2 := NewAnalyzer(n)
	in32 := make([]float32, n)
	for i := 0; i < n; i++ {
		in32[i] = float32(sig[i])
	}
	a2.ProcessBlock(in32, n)
	snap2 := a2.Snapshot()

	// Compare RMS — the float64→float32→float64 round-trip may introduce small
	// differences since ProcessSample feeds float64 directly while ProcessBlock
	// casts from float32.
	rmsDiff := math.Abs(snap1.RMS - snap2.RMS)
	peakDiff := math.Abs(snap1.Peak - snap2.Peak)
	t.Logf("RMS: sample=%.8f block=%.8f diff=%.2e", snap1.RMS, snap2.RMS, rmsDiff)
	t.Logf("Peak: sample=%.8f block=%.8f diff=%.2e", snap1.Peak, snap2.Peak, peakDiff)

	if rmsDiff > 1e-4 {
		t.Fatalf("RMS mismatch too large: %.6e", rmsDiff)
	}
	if peakDiff > 1e-4 {
		t.Fatalf("Peak mismatch too large: %.6e", peakDiff)
	}
}

// TestPreEQTapInBlockPath verifies that the pre-EQ analyzer tap feeds the
// analyzer when the block processing path is active.
func TestPreEQTapInBlockPath(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 1024
	)
	sig := genSine(440, sr, n)

	id := "analyzer-block-tap"
	// Set up channel with a biquad (BlockProcessor) to enable the block path.
	bq := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	if bq == nil {
		t.Fatal("failed to create biquad")
	}
	SetChannelProcessors(id, bq)
	an := EnablePreEQAnalyzer(id, n)
	ch := InstrumentChannel(id)

	input := make([]float64, n)
	copy(input, sig)
	output := make([]float64, n)
	ch.ProcessBlockLocal(input, output)

	snap := an.Snapshot()
	t.Logf("pre-EQ analyzer RMS=%.6f Peak=%.6f", snap.RMS, snap.Peak)
	if snap.RMS < 1e-6 {
		t.Fatalf("expected pre-EQ analyzer to have non-zero RMS, got %.8e", snap.RMS)
	}
	if snap.Peak < 1e-6 {
		t.Fatalf("expected pre-EQ analyzer to have non-zero Peak, got %.8e", snap.Peak)
	}
}
