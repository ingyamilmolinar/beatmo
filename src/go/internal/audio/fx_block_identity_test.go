package audio

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// applyDefaultFXPerSample is applyDefaultFX as it was before the block path:
// every sample through every processor, one call each.
func applyDefaultFXPerSample(buf []float32, sampleRate int, slots []EffectSlot) {
	var procs []InsertEffect
	for _, s := range slots {
		if s.Enabled {
			procs = append(procs, NewEffectProcessor(s, sampleRate))
		}
	}
	for i, x := range buf {
		sample := float64(x)
		for _, p := range procs {
			sample = p.ProcessSample(sample)
		}
		buf[i] = float32(sample)
	}
}

// fxIdentitySignal has what makes an effect's state matter: a transient, a
// held tone, a decay, silence and a second hit.
func fxIdentitySignal(sr int) []float32 {
	r := rand.New(rand.NewSource(3))
	out := make([]float32, sr)
	for i := range out {
		t := float64(i) / float64(sr)
		v := 0.0
		switch {
		case t < 0.3:
			v = 0.8*math.Exp(-6*t)*math.Sin(2*math.Pi*196*t) + 0.3*math.Exp(-40*t)*(2*r.Float64()-1)
		case t < 0.5:
			v = 0
		default:
			v = 0.9*math.Sin(2*math.Pi*523*t) + 0.05*(2*r.Float64()-1)
		}
		out[i] = float32(v)
	}
	return out
}

// A document's FX chain renders through one block call per effect; the voice
// must be the per-sample voice bit for bit, for every effect at its defaults,
// at either end of every parameter, at random settings, and chained.
func TestDocumentFXBlockMatchesPerSample(t *testing.T) {
	const sr = 44100
	regs := EffectRegistrations()
	var types []EffectType
	for ty := range regs {
		types = append(types, ty)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	r := rand.New(rand.NewSource(5))
	var chains [][]EffectSlot
	for _, ty := range types {
		chains = append(chains, []EffectSlot{{Type: ty, Enabled: true}})
		for _, end := range []func(EffectParamDef) float64{
			func(d EffectParamDef) float64 { return d.Min },
			func(d EffectParamDef) float64 { return d.Max },
			func(d EffectParamDef) float64 { return d.Min + r.Float64()*(d.Max-d.Min) },
		} {
			p := map[string]float64{}
			for _, d := range regs[ty].Params {
				p[d.Name] = end(d)
			}
			chains = append(chains, []EffectSlot{{Type: ty, Enabled: true, Params: p}})
		}
	}
	for k := 0; k < 8; k++ { // random chains of three, one disabled
		var c []EffectSlot
		for j := 0; j < 3; j++ {
			c = append(c, EffectSlot{Type: types[r.Intn(len(types))], Enabled: j != 1 || k%2 == 0})
		}
		chains = append(chains, c)
	}
	in := fxIdentitySignal(sr)
	for _, c := range chains {
		got := append([]float32(nil), in...)
		want := append([]float32(nil), in...)
		applyDefaultFX(got, sr, c)
		applyDefaultFXPerSample(want, sr, c)
		for i := range got {
			if math.Float32bits(got[i]) != math.Float32bits(want[i]) {
				t.Fatalf("%+v: sample %d block %v, per-sample %v", c, i, got[i], want[i])
			}
		}
	}
}
