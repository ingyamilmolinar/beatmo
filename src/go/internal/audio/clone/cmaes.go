package clone

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone/linalg"
)

// cmaes.go — (μ/μ_w, λ)-CMA-ES after Hansen 2016, "The CMA Evolution
// Strategy: A Tutorial" (the purecmaes reference listing), unconstrained;
// Refine maps it into a box. All randomness is consumed in Ask, so a run is a
// pure function of the seed and of the fitness values — whatever evaluates
// them, in whatever order.

// CMAES is one optimizer state.
type CMAES struct {
	n, lambda, mu int
	weights       []float64
	mueff         float64
	cc, cs, c1    float64
	cmu, damps    float64
	chiN          float64

	Mean  []float64
	Sigma float64
	pc    []float64
	ps    []float64
	c     [][]float64 // covariance
	b     [][]float64 // eigenvectors (columns)
	d     []float64   // sqrt eigenvalues
	inv   [][]float64 // C^-1/2

	evals, eigenEval int
	rng              *rand.Rand
	z                [][]float64
}

// NewCMAES starts at the origin with step sigma; lambda ≤ 0 picks
// 4 + ⌊3 ln n⌋.
func NewCMAES(n int, sigma float64, lambda int, seed uint64) *CMAES {
	if lambda <= 0 {
		lambda = 4 + int(3*math.Log(float64(n)))
	}
	o := &CMAES{n: n, lambda: lambda, mu: lambda / 2, Sigma: sigma, rng: rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))}
	sw, sw2 := 0.0, 0.0
	for i := 0; i < o.mu; i++ {
		w := math.Log(float64(o.mu)+0.5) - math.Log(float64(i+1))
		o.weights = append(o.weights, w)
		sw += w
	}
	for i := range o.weights {
		o.weights[i] /= sw
		sw2 += o.weights[i] * o.weights[i]
	}
	o.mueff = 1 / sw2
	fn := float64(n)
	o.cc = (4 + o.mueff/fn) / (fn + 4 + 2*o.mueff/fn)
	o.cs = (o.mueff + 2) / (fn + o.mueff + 5)
	o.c1 = 2 / ((fn+1.3)*(fn+1.3) + o.mueff)
	o.cmu = math.Min(1-o.c1, 2*(o.mueff-2+1/o.mueff)/((fn+2)*(fn+2)+o.mueff))
	o.damps = 1 + 2*math.Max(0, math.Sqrt((o.mueff-1)/(fn+1))-1) + o.cs
	o.chiN = math.Sqrt(fn) * (1 - 1/(4*fn) + 1/(21*fn*fn))
	o.Mean = make([]float64, n)
	o.pc, o.ps = make([]float64, n), make([]float64, n)
	o.c, o.b, o.inv = eye(n), eye(n), eye(n)
	o.d = make([]float64, n)
	for i := range o.d {
		o.d[i] = 1
	}
	return o
}

func eye(n int) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		m[i][i] = 1
	}
	return m
}

// Lambda is the population size.
func (o *CMAES) Lambda() int { return o.lambda }

// Ask samples one generation: x_k = mean + σ·B·(D ∘ z_k).
func (o *CMAES) Ask() [][]float64 {
	o.z = make([][]float64, o.lambda)
	xs := make([][]float64, o.lambda)
	for k := range xs {
		z := make([]float64, o.n)
		for i := range z {
			z[i] = o.rng.NormFloat64()
		}
		o.z[k] = z
		x := make([]float64, o.n)
		for i := 0; i < o.n; i++ {
			var s float64
			for j := 0; j < o.n; j++ {
				s += o.b[i][j] * o.d[j] * z[j]
			}
			x[i] = o.Mean[i] + o.Sigma*s
		}
		xs[k] = x
	}
	return xs
}

// Tell updates the state from Ask's points and their fitness (lower is better).
func (o *CMAES) Tell(xs [][]float64, f []float64) {
	n := o.n
	idx := make([]int, len(xs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return f[idx[a]] < f[idx[b]] })
	o.evals += len(xs)
	old := append([]float64(nil), o.Mean...)
	for i := range o.Mean {
		var s float64
		for k := 0; k < o.mu; k++ {
			s += o.weights[k] * xs[idx[k]][i]
		}
		o.Mean[i] = s
	}
	step := make([]float64, n)
	for i := range step {
		step[i] = (o.Mean[i] - old[i]) / o.Sigma
	}
	cs := math.Sqrt(o.cs * (2 - o.cs) * o.mueff)
	var psn float64
	for i := 0; i < n; i++ {
		var s float64
		for j := 0; j < n; j++ {
			s += o.inv[i][j] * step[j]
		}
		o.ps[i] = (1-o.cs)*o.ps[i] + cs*s
		psn += o.ps[i] * o.ps[i]
	}
	psn = math.Sqrt(psn)
	hsig := 0.0
	if psn/math.Sqrt(1-math.Pow(1-o.cs, 2*float64(o.evals)/float64(o.lambda)))/o.chiN < 1.4+2/float64(n+1) {
		hsig = 1
	}
	cc := math.Sqrt(o.cc * (2 - o.cc) * o.mueff)
	for i := range o.pc {
		o.pc[i] = (1-o.cc)*o.pc[i] + hsig*cc*step[i]
	}
	art := make([][]float64, o.mu)
	for k := range art {
		art[k] = make([]float64, n)
		for i := range art[k] {
			art[k][i] = (xs[idx[k]][i] - old[i]) / o.Sigma
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			v := (1-o.c1-o.cmu)*o.c[i][j] + o.c1*(o.pc[i]*o.pc[j]+(1-hsig)*o.cc*(2-o.cc)*o.c[i][j])
			for k := 0; k < o.mu; k++ {
				v += o.cmu * o.weights[k] * art[k][i] * art[k][j]
			}
			o.c[i][j], o.c[j][i] = v, v
		}
	}
	o.Sigma *= math.Exp((o.cs / o.damps) * (psn/o.chiN - 1))
	if float64(o.evals-o.eigenEval) > float64(o.lambda)/(o.c1+o.cmu)/float64(n)/10 {
		o.eigenEval = o.evals
		o.decompose()
	}
}

// decompose refreshes B, D and C^-1/2 from C (C = B·D²·Bᵀ).
func (o *CMAES) decompose() {
	n := o.n
	m := linalg.New(n, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			m.Set(i, j, complex(o.c[i][j], 0))
		}
	}
	vals, vecs := linalg.EigHermitian(m)
	for j := 0; j < n; j++ {
		o.d[j] = math.Sqrt(math.Max(vals[j], 1e-20))
		// A real symmetric matrix's eigenvectors are real up to a unit phase:
		// rotate each column so its largest entry is real.
		ph := complex(1, 0)
		best := 0.0
		for i := 0; i < n; i++ {
			if a := cmplxAbs2(vecs.At(i, j)); a > best {
				best, ph = a, vecs.At(i, j)
			}
		}
		ph = complex(real(ph), -imag(ph)) / complex(math.Sqrt(best), 0)
		for i := 0; i < n; i++ {
			o.b[i][j] = real(vecs.At(i, j) * ph)
		}
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			var s float64
			for k := 0; k < n; k++ {
				s += o.b[i][k] / o.d[k] * o.b[j][k]
			}
			o.inv[i][j] = s
		}
	}
}
