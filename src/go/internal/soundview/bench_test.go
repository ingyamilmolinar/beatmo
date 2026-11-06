package soundview

import (
	"math"
	"math/rand"
	"testing"
)

// BenchmarkWarmOneSecondNote is the cost of every view of a 1 s note — the
// work one graph render adds (spec 2026-10-24 §4.4 budget: ≤ 15 ms native).
func BenchmarkWarmOneSecondNote(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	pcm := gen(1, func(t float64) float64 {
		return math.Exp(-t/0.3)*math.Sin(2*math.Pi*(60+200*math.Exp(-t/0.03))*t) + 0.05*rng.NormFloat64()
	})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		New(pcm, sr, 1).Warm(NeedAll, nil)
	}
}
