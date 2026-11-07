package clone

import (
	"math"
	"testing"
)

// TestCMAESMinimisesAQuadratic: a rotated, ill-conditioned (1e3) ellipsoid in
// 10 dimensions, optimum away from the start.
func TestCMAESMinimisesAQuadratic(t *testing.T) {
	const n = 10
	opt := make([]float64, n)
	for i := range opt {
		opt[i] = 0.5 - 0.1*float64(i)
	}
	f := func(x []float64) float64 {
		var s float64
		for i := 0; i < n; i++ {
			// rotate by mixing neighbours, scale by 10^(1.5·i/(n−1))
			u := (x[i] - opt[i]) + 0.5*(x[(i+1)%n]-opt[(i+1)%n])
			s += math.Pow(10, 3*float64(i)/(n-1)) * u * u
		}
		return s
	}
	es := NewCMAES(n, 0.5, 0, 7)
	var best = math.Inf(1)
	for g := 0; g < 600 && best > 1e-10; g++ {
		xs := es.Ask()
		fs := make([]float64, len(xs))
		for k, x := range xs {
			fs[k] = f(x)
			best = math.Min(best, fs[k])
		}
		es.Tell(xs, fs)
	}
	if best > 1e-10 {
		t.Fatalf("best %g after the budget", best)
	}
}

func TestCMAESIsAFunctionOfTheSeed(t *testing.T) {
	run := func(seed uint64) []float64 {
		es := NewCMAES(4, 0.3, 8, seed)
		for g := 0; g < 5; g++ {
			xs := es.Ask()
			fs := make([]float64, len(xs))
			for k, x := range xs {
				fs[k] = x[0]*x[0] + 3*x[1]*x[1] + x[2] + x[3]*x[3]
			}
			es.Tell(xs, fs)
		}
		return es.Mean
	}
	a, b, c := run(1), run(1), run(2)
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("same seed, different run")
		}
	}
	if a[0] == c[0] {
		t.Fatal("different seeds, same run")
	}
}
