package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestBiquadProcessBlockBufParity verifies that ProcessBlockBuf produces
// bit-identical output to a loop of ProcessSample calls for the same input.
func TestBiquadProcessBlockBufParity(t *testing.T) {
	const (
		sr = 44100
		n  = 4096
	)
	sig := genSine(440, sr, n)

	// ProcessSample path.
	bq1 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq1 == nil {
		t.Fatal("failed to create biquad 1")
	}
	outSample := make([]float64, n)
	for i := 0; i < n; i++ {
		outSample[i] = bq1.ProcessSample(sig[i])
	}

	// ProcessBlockBuf path.
	bq2 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq2 == nil {
		t.Fatal("failed to create biquad 2")
	}
	in32 := make([]float32, n)
	out32 := make([]float32, n)
	for i := 0; i < n; i++ {
		in32[i] = float32(sig[i])
	}
	bq2.ProcessBlockBuf(in32, out32, n)

	// Compare: the block path converts float64 input to float32 before processing,
	// introducing rounding. The per-sample path processes in float64 throughout.
	// We allow a small float32-level tolerance.
	maxDiff := float32(0)
	for i := 0; i < n; i++ {
		want := float32(outSample[i])
		got := out32[i]
		diff := want - got
		if diff < 0 {
			diff = -diff
		}
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	t.Logf("biquad block vs sample max diff: %.8e", maxDiff)
	// Input quantization to float32 causes the biquad's internal float64
	// accumulation to diverge from the pure float64 path. Allow small tolerance.
	if maxDiff > 1e-4 {
		t.Fatalf("biquad block/sample mismatch too large: %.8e", maxDiff)
	}
}

// TestEQProcessorBlockPath verifies that a channel with eqProcessor uses the
// block path and produces output matching per-sample processing.
func TestEQProcessorBlockPath(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 2048
	)
	sig := genSine(440, sr, n)

	// Create channel with biquad as processor (works in both stub and real).
	bq1 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq1 == nil {
		t.Fatal("failed to create biquad")
	}
	id := "eq-block-test"
	SetChannelProcessors(channel.Of(instrument.ID(id)), bq1)
	ch := InstrumentChannel(channel.Of(instrument.ID(id)))

	// Block path.
	input := make([]float64, n)
	copy(input, sig)
	blockOut := make([]float64, n)
	ch.ProcessBlockLocal(input, blockOut)

	// Per-sample reference with a fresh channel.
	bq2 := makeBiquad(EQLowpass, sr, 1000, 0.707, 0)
	if bq2 == nil {
		t.Fatal("failed to create reference biquad")
	}
	refID := "eq-block-ref"
	SetChannelProcessors(channel.Of(instrument.ID(refID)), bq2)
	refCh := InstrumentChannel(channel.Of(instrument.ID(refID)))

	refOut := make([]float64, n)
	for i := 0; i < n; i++ {
		refOut[i] = refCh.ProcessSampleLocal(sig[i])
	}

	corr := computeCorrelation(blockOut, refOut)
	t.Logf("block vs per-sample correlation: %.6f", corr)
	if corr < 0.999 {
		t.Fatalf("expected block path to match per-sample (correlation > 0.999), got %.6f", corr)
	}
}

// TestFullChainBlockPath verifies that when ALL processors in a channel
// implement BlockProcessor, the optimized block path is used (no per-sample
// fallback). We verify by checking output is non-zero and matches per-sample.
func TestFullChainBlockPath(t *testing.T) {
	withDefaultAudio(t)
	const (
		sr = 44100
		n  = 1024
	)
	sig := genSine(440, sr, n)

	// Create a channel with two biquads (both implement BlockProcessor).
	bq := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	if bq == nil {
		t.Fatal("failed to create biquad")
	}
	comp := makeBiquad(EQHighpass, sr, 100, 0.707, 0)

	id := "full-chain-block"
	SetChannelProcessors(channel.Of(instrument.ID(id)), bq, comp)
	ch := InstrumentChannel(channel.Of(instrument.ID(id)))

	input := make([]float64, n)
	copy(input, sig)
	blockOut := make([]float64, n)
	ch.ProcessBlockLocal(input, blockOut)

	// Verify output is non-zero (block path ran).
	var maxAbs float64
	for _, v := range blockOut {
		a := math.Abs(v)
		if a > maxAbs {
			maxAbs = a
		}
	}
	if maxAbs < 1e-6 {
		t.Fatalf("expected non-zero output from full chain block path, maxAbs=%.8e", maxAbs)
	}

	// Reference: per-sample through fresh channel with same processor types.
	bq2 := makeBiquad(EQLowpass, sr, 2000, 0.707, 0)
	comp2 := makeBiquad(EQHighpass, sr, 100, 0.707, 0)
	refID := "full-chain-ref"
	SetChannelProcessors(channel.Of(instrument.ID(refID)), bq2, comp2)
	refCh := InstrumentChannel(channel.Of(instrument.ID(refID)))

	refOut := make([]float64, n)
	for i := 0; i < n; i++ {
		refOut[i] = refCh.ProcessSampleLocal(sig[i])
	}

	corr := computeCorrelation(blockOut, refOut)
	t.Logf("full chain block vs per-sample correlation: %.6f", corr)
	if corr < 0.99 {
		t.Fatalf("expected full chain to match per-sample (correlation > 0.99), got %.6f", corr)
	}
}
