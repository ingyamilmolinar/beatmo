package clone

import (
	"errors"
	"math"
	"math/cmplx"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone/linalg"
)

var errTooShort = errors.New("clone: too few samples for ESPRIT")

// espritResult is one band's pole estimate. Poles are per decimated sample;
// Amps are the complex amplitudes at y[0].
type espritResult struct {
	Poles    []complex128
	Amps     []complex128
	NoiseVar float64 // noise eigenvalue median (per-sample power of y)
	Order    int
	Signal   int // eigenvalues ≥ 6 dB above the noise median, before ESTER
}

const (
	espritMaxRows      = 128 // Hankel rows L
	espritMaxSnapshots = 384 // covariance snapshots (strided)
	espritMaxLSRows    = 4096
	ompMinGain         = 20 // a kept pole must explain ≥ 20σ² (13 dB over chance)
)

// esprit estimates the damped exponentials in y (spec §2b): L × L sample
// covariance → Hermitian eigendecomposition → candidate order = eigenvalues
// ≥ 6 dB above the noise median (≤ maxOrder) → candidate poles = eig(Φ) →
// orthogonal matching pursuit keeps the ones that explain y → least squares
// for their amplitudes.
func esprit(y []complex128, maxOrder int) (espritResult, error) {
	n := len(y)
	L := min(n/3, espritMaxRows)
	if L < 4 {
		return espritResult{}, errTooShort
	}
	K := n - L + 1
	stride := (K + espritMaxSnapshots - 1) / espritMaxSnapshots
	R := linalg.New(L, L)
	count := 0
	for k := 0; k < K; k += stride {
		count++
		for i := 0; i < L; i++ {
			yi := y[k+i]
			for j := i; j < L; j++ {
				R.Data[i*L+j] += yi * cmplx.Conj(y[k+j])
			}
		}
	}
	inv := complex(1/float64(count), 0)
	for i := 0; i < L; i++ {
		for j := i; j < L; j++ {
			v := R.Data[i*L+j] * inv
			R.Data[i*L+j] = v
			R.Data[j*L+i] = cmplx.Conj(v)
		}
	}
	vals, vecs := linalg.EigHermitian(R)
	lower := append([]float64(nil), vals[L/2:]...)
	sort.Float64s(lower)
	noise := math.Max(lower[len(lower)/2], 0)
	res := espritResult{NoiseVar: noise}
	pmax := min(maxOrder, L/2)
	p := 0
	for p < pmax && vals[p] > 4*noise {
		p++
	}
	res.Signal = p
	if p == 0 {
		return res, nil
	}
	phi, err := rotation(vecs, L, p)
	if err != nil {
		return res, err
	}
	poles, err := linalg.Eigenvalues(phi)
	if err != nil {
		return res, err
	}
	poles, amps, err := selectPoles(y, dedupePoles(poles), noise)
	if err != nil {
		return res, err
	}
	res.Poles, res.Amps, res.Order = poles, amps, len(poles)
	return res, nil
}

// selectPoles keeps the candidate poles that actually explain y: orthogonal
// matching pursuit over strided rows — repeatedly add the candidate whose
// component orthogonal to those already chosen removes the most residual
// energy, while that removal is ≥ ompMinGain·σ² (a column fits ~σ² of white
// noise by chance). Then one least-squares fit over the chosen poles.
//
// Order selection on recorded audio, measured on the VCSL corpus: the
// noise-threshold count saturates (a real recording has no clean eigenvalue
// floor), ESTER at a fixed bar (‖W↑ − W↓Φ‖_F ≤ 0.3) trimmed real tubular-bell
// partials away (E(1) is already 0.3–0.6 in long high bands full of strike
// noise), and the untrimmed order handed the joint refit clusters of
// cancelling poles that blew up outside the window. A pursuit never keeps a
// cancelling phantom: on its own it explains nothing.
func selectPoles(y, cand []complex128, noiseVar float64) ([]complex128, []complex128, error) {
	n := len(y)
	stride := (n + espritMaxLSRows - 1) / espritMaxLSRows
	rows := (n + stride - 1) / stride
	b := make([]complex128, rows)
	for r := range b {
		b[r] = y[r*stride]
	}
	cols := make([][]complex128, len(cand))
	for k, z := range cand {
		col := make([]complex128, rows)
		zs := cmplx.Pow(z, complex(float64(stride), 0))
		acc := complex(1, 0)
		for r := range col {
			col[r] = acc
			acc *= zs
		}
		cols[k] = col
	}
	res := append([]complex128(nil), b...)
	// Each candidate's component orthogonal to the chosen basis, updated
	// incrementally as the basis grows (O(candidates·rows) per step).
	perp := make([][]complex128, len(cols))
	norm0 := make([]float64, len(cols))
	for k, col := range cols {
		perp[k] = append([]complex128(nil), col...)
		for _, v := range col {
			norm0[k] += cmplxAbs2(v)
		}
	}
	used := make([]bool, len(cand))
	var chosen []complex128
	thr := ompMinGain * math.Max(noiseVar, 1e-300)
	for len(chosen) < len(cand) {
		best, bestGain, bestNorm := -1, 0.0, 0.0
		for k, v := range perp {
			if used[k] {
				continue
			}
			var nv float64
			var dot complex128
			for r := range v {
				nv += cmplxAbs2(v[r])
				dot += cmplx.Conj(v[r]) * res[r]
			}
			if nv <= 1e-10*norm0[k] {
				continue
			}
			if g := cmplxAbs2(dot) / nv; g > bestGain {
				best, bestGain, bestNorm = k, g, nv
			}
		}
		if best < 0 || bestGain < thr {
			break
		}
		inv := complex(1/math.Sqrt(bestNorm), 0)
		q := make([]complex128, rows)
		for r := range q {
			q[r] = perp[best][r] * inv
		}
		project := func(v []complex128) {
			var dot complex128
			for r := range q {
				dot += cmplx.Conj(q[r]) * v[r]
			}
			for r := range q {
				v[r] -= dot * q[r]
			}
		}
		project(res)
		used[best] = true
		chosen = append(chosen, cand[best])
		for k := range perp {
			if !used[k] {
				project(perp[k])
			}
		}
	}
	if len(chosen) == 0 {
		return nil, nil, nil
	}
	amps, err := vandermonde(y, chosen)
	if err != nil {
		return nil, nil, err
	}
	return chosen, amps, nil
}

// rotation solves W↓·Φ = W↑ in the least-squares sense for the first p
// eigenvectors (W↓ = rows 0..L−2, W↑ = rows 1..L−1).
func rotation(vecs *linalg.Matrix, l, p int) (*linalg.Matrix, error) {
	down := linalg.New(l-1, p)
	for i := 0; i < l-1; i++ {
		for j := 0; j < p; j++ {
			down.Set(i, j, vecs.At(i, j))
		}
	}
	f, err := linalg.FactorQR(down)
	if err != nil {
		return nil, err
	}
	phi := linalg.New(p, p)
	col := make([]complex128, l-1)
	for j := 0; j < p; j++ {
		for i := 0; i < l-1; i++ {
			col[i] = vecs.At(i+1, j)
		}
		x := f.Solve(col)
		for i := 0; i < p; i++ {
			phi.Set(i, j, x[i])
		}
	}
	return phi, nil
}

// dedupePoles drops a pole numerically identical to an earlier one (it would
// make the Vandermonde matrix singular).
func dedupePoles(p []complex128) []complex128 {
	var out []complex128
	for _, z := range p {
		dup := false
		for _, o := range out {
			if cmplx.Abs(z-o) < 1e-12 {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, z)
		}
	}
	return out
}

// vandermonde fits y[m] ≈ Σ_k c_k z_k^m by least squares over at most
// espritMaxLSRows uniformly strided samples.
func vandermonde(y []complex128, poles []complex128) ([]complex128, error) {
	n := len(y)
	stride := (n + espritMaxLSRows - 1) / espritMaxLSRows
	rows := (n + stride - 1) / stride
	v := linalg.New(rows, len(poles))
	b := make([]complex128, rows)
	for k, z := range poles {
		zs := cmplx.Pow(z, complex(float64(stride), 0))
		acc := complex(1, 0)
		for r := 0; r < rows; r++ {
			v.Set(r, k, acc)
			acc *= zs
		}
	}
	for r := 0; r < rows; r++ {
		b[r] = y[r*stride]
	}
	f, err := linalg.FactorQR(v)
	if err != nil {
		return nil, err
	}
	return f.Solve(b), nil
}
