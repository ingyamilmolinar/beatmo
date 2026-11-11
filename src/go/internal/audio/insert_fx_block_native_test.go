//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// ProcessBlockBuf coverage for every registered C insert effect. The mixer's
// block path (channels.go) is the production caller, but the fast native
// test run only exercised ProcessSample — leaving every ProcessBlockBuf in
// insert_fx_c.go at 0%. This table drives each effect's block API directly
// and cross-checks it against the per-sample API on a stateless effect.

func noiseBurst(n int) []float32 {
	// Deterministic pseudo-noise — no math/rand so runs are reproducible.
	buf := make([]float32, n)
	state := uint32(0x2545F491)
	for i := range buf {
		state = state*1664525 + 1013904223
		buf[i] = (float32(state>>8)/float32(1<<24))*2 - 1
	}
	return buf
}

func TestInsertFXProcessBlockBufAllEffects(t *testing.T) {
	in := noiseBurst(512)
	for _, typ := range EffectTypeOrder() {
		typ := typ
		t.Run(string(typ), func(t *testing.T) {
			reg := EffectRegistrations()[typ]
			if reg == nil {
				t.Fatalf("no registration for %s", typ)
			}
			fx := reg.New(44100, DefaultParams(typ))

			bp, ok := fx.(BlockProcessor)
			if !ok {
				t.Fatalf("%s does not implement BlockProcessor", typ)
			}

			// Run several blocks: time-based effects (pitchshift's 20-100ms
			// grain window, delay lines) have startup latency longer than
			// one block, so total energy is measured across the whole run.
			var energy float64
			out := make([]float32, len(in))
			for block := 0; block < 16; block++ {
				bp.ProcessBlockBuf(in, out, len(in))
				for i, v := range out {
					f := float64(v)
					if math.IsNaN(f) || math.IsInf(f, 0) {
						t.Fatalf("block %d out[%d] not finite: %v", block, i, v)
					}
					if math.Abs(f) > 16 {
						t.Fatalf("block %d out[%d] unbounded: %v", block, i, v)
					}
					energy += f * f
				}
			}
			// A sustained noise feed through any default-params effect must
			// not be silenced to digital zero (gate included: default
			// threshold passes a full-scale noise burst).
			if energy == 0 {
				t.Fatalf("%s produced all-zero output from a sustained noise feed", typ)
			}

			// samples <= 0 must be a no-op, not a crash.
			bp.ProcessBlockBuf(in, out, 0)

			// In-place processing (in == out) must stay finite.
			inPlace := make([]float32, len(in))
			copy(inPlace, in)
			fx.Reset()
			bp.ProcessBlockBuf(inPlace, inPlace, len(inPlace))
			for i, v := range inPlace {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Fatalf("in-place out[%d] not finite: %v", i, v)
				}
			}
		})
	}
}

// Block and per-sample APIs must agree on a stateless effect (distortion has
// no time-dependent state, so sample-by-sample == block).
func TestInsertFXBlockMatchesPerSampleStateless(t *testing.T) {
	in := noiseBurst(256)

	reg := EffectRegistrations()[EffectDistortion]
	blockFX := reg.New(44100, DefaultParams(EffectDistortion))
	sampleFX := reg.New(44100, DefaultParams(EffectDistortion))

	out := make([]float32, len(in))
	blockFX.(BlockProcessor).ProcessBlockBuf(in, out, len(in))

	for i, x := range in {
		want := sampleFX.ProcessSample(float64(x))
		if math.Abs(float64(out[i])-want) > 1e-5 {
			t.Fatalf("block[%d] = %v, per-sample = %v", i, out[i], want)
		}
	}
}
