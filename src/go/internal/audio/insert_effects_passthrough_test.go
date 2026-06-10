package audio

import (
	"testing"
)

// TestPassThroughFallback exercises every method on the unregistered-
// effect fallback returned by NewEffectProcessor.
func TestPassThroughFallback(t *testing.T) {
	eff := NewEffectProcessor(EffectSlot{Type: EffectType("__unregistered__")}, 48000)

	for _, x := range []float64{-1.0, 0, 0.5, 1.0} {
		if got := eff.ProcessSample(x); got != x {
			t.Errorf("ProcessSample(%v) = %v; want %v (identity)", x, got, x)
		}
	}

	eff.Reset()
	eff.SetParam("nope", 123.45)
	if got := eff.ProcessSample(0.3); got != 0.3 {
		t.Errorf("after Reset/SetParam, ProcessSample(0.3) = %v; want 0.3", got)
	}

	bp, ok := eff.(interface {
		ProcessBlockBuf(in, out []float32, samples int)
	})
	if !ok {
		t.Fatalf("passThrough must implement ProcessBlockBuf via BlockProcessor interface")
	}
	in := []float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	out := make([]float32, len(in))
	bp.ProcessBlockBuf(in, out, len(in))
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("ProcessBlockBuf[%d] = %v; want %v", i, out[i], in[i])
		}
	}
}
