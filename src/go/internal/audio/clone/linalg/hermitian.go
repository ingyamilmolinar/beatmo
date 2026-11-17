package linalg

import (
	"math"
	"math/cmplx"
	"sort"
)

// eigHermitianJacobi is the cyclic-Jacobi Hermitian eigensolver: accurate to
// working precision and simple, but O(n³) PER SWEEP (0.25 s at 128 × 128), so
// it is kept only as the test oracle for EigHermitian.
//
// Each rotation first turns the pivot a_pq real with a diagonal phase, then
// applies the classic real Jacobi rotation (Numerical Recipes' θ/t/c/s); the
// product J = diag(1, e^{−iφ})·P is unitary and zeroes a_pq.
func eigHermitianJacobi(a *Matrix) ([]float64, *Matrix) {
	n := a.Rows
	A := a.Clone()
	V := New(n, n)
	for i := 0; i < n; i++ {
		V.Set(i, i, 1)
	}
	var total float64
	for _, v := range A.Data {
		total += real(v)*real(v) + imag(v)*imag(v)
	}
	tol := 1e-30 * total
	for sweep := 0; sweep < 100; sweep++ {
		var off float64
		for p := 0; p < n; p++ {
			for q := p + 1; q < n; q++ {
				v := A.At(p, q)
				off += real(v)*real(v) + imag(v)*imag(v)
			}
		}
		if off <= tol {
			break
		}
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				apq := A.At(p, q)
				mag := cmplx.Abs(apq)
				if mag*mag <= tol/float64(n*n) {
					continue
				}
				app, aqq := real(A.At(p, p)), real(A.At(q, q))
				theta := (aqq - app) / (2 * mag)
				t := 1 / (math.Abs(theta) + math.Sqrt(theta*theta+1))
				if theta < 0 {
					t = -t
				}
				c := 1 / math.Sqrt(t*t+1)
				s := t * c
				eNeg := cmplx.Conj(apq / complex(mag, 0)) // e^{−iφ}
				jpp, jpq := complex(c, 0), complex(s, 0)
				jqp, jqq := complex(-s, 0)*eNeg, complex(c, 0)*eNeg
				for k := 0; k < n; k++ { // A ← A·J
					akp, akq := A.At(k, p), A.At(k, q)
					A.Set(k, p, akp*jpp+akq*jqp)
					A.Set(k, q, akp*jpq+akq*jqq)
				}
				for k := 0; k < n; k++ { // A ← Jᴴ·A
					apk, aqk := A.At(p, k), A.At(q, k)
					A.Set(p, k, cmplx.Conj(jpp)*apk+cmplx.Conj(jqp)*aqk)
					A.Set(q, k, cmplx.Conj(jpq)*apk+cmplx.Conj(jqq)*aqk)
				}
				A.Set(p, q, 0)
				A.Set(q, p, 0)
				A.Set(p, p, complex(real(A.At(p, p)), 0))
				A.Set(q, q, complex(real(A.At(q, q)), 0))
				for k := 0; k < n; k++ { // V ← V·J
					vkp, vkq := V.At(k, p), V.At(k, q)
					V.Set(k, p, vkp*jpp+vkq*jqp)
					V.Set(k, q, vkp*jpq+vkq*jqq)
				}
			}
		}
	}
	diag := make([]float64, n)
	for i := range diag {
		diag[i] = real(A.At(i, i))
	}
	return sortDescending(diag, V)
}

// sortDescending orders eigenpairs by descending eigenvalue (stable).
func sortDescending(diag []float64, v *Matrix) ([]float64, *Matrix) {
	n := len(diag)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(x, y int) bool { return diag[idx[x]] > diag[idx[y]] })
	vals := make([]float64, n)
	vecs := New(v.Rows, n)
	for c, i := range idx {
		vals[c] = diag[i]
		for r := 0; r < v.Rows; r++ {
			vecs.Set(r, c, v.At(r, i))
		}
	}
	return vals, vecs
}

// EigHermitian returns the eigenvalues of the Hermitian matrix a in
// DESCENDING order and the matching unit eigenvectors as the columns of vecs.
// a is not modified. Householder reduction to a Hermitian tridiagonal T
// (A = Q·T·Qᴴ), a diagonal phase D that makes T real (T = D·T'·Dᴴ), then the
// implicit QL iteration on T' with eigenvector accumulation: eigenvectors are
// Q·D·Z. O(n³) once — ~100× faster than Jacobi at the 128 × 128 ESPRIT
// covariances, same accuracy (TestEigHermitianMatchesJacobi).
func EigHermitian(a *Matrix) ([]float64, *Matrix) {
	n := a.Rows
	if n == 0 {
		return nil, New(0, 0)
	}
	A := a.Clone()
	Q := New(n, n)
	for i := 0; i < n; i++ {
		Q.Set(i, i, 1)
	}
	v := make([]complex128, n)
	w := make([]complex128, n)
	for k := 0; k < n-2; k++ {
		var norm float64
		for i := k + 1; i < n; i++ {
			x := A.At(i, k)
			norm += real(x)*real(x) + imag(x)*imag(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		x0 := A.At(k+1, k)
		ph := complex(1, 0)
		if x0 != 0 {
			ph = x0 / complex(cmplx.Abs(x0), 0)
		}
		alpha := -ph * complex(norm, 0)
		for i := range v {
			v[i] = 0
		}
		for i := k + 1; i < n; i++ {
			v[i] = A.At(i, k)
		}
		v[k+1] -= alpha
		var vn float64
		for i := k + 1; i < n; i++ {
			vn += real(v[i])*real(v[i]) + imag(v[i])*imag(v[i])
		}
		if vn == 0 {
			continue
		}
		vn = math.Sqrt(vn)
		for i := k + 1; i < n; i++ {
			v[i] /= complex(vn, 0)
		}
		// H = I − 2vvᴴ; HAH = A − v·wᴴ − w·vᴴ with p = A·v, κ = vᴴp, w = 2(p − κv).
		// v is zero above k+1, so only rows/columns ≥ k are touched (column k
		// becomes [.., alpha, 0, …]).
		var kappa complex128
		ad := A.Data
		for i := k; i < n; i++ {
			var s complex128
			row := ad[i*n : i*n+n]
			for j := k + 1; j < n; j++ {
				s += row[j] * v[j]
			}
			w[i] = s
		}
		for i := k + 1; i < n; i++ {
			kappa += cmplx.Conj(v[i]) * w[i]
		}
		for i := k; i < n; i++ {
			w[i] = 2 * (w[i] - kappa*v[i])
		}
		cv := make([]complex128, n)
		cw := make([]complex128, n)
		for j := k; j < n; j++ {
			cv[j], cw[j] = cmplx.Conj(v[j]), cmplx.Conj(w[j])
		}
		for i := k; i < n; i++ {
			row := ad[i*n : i*n+n]
			vi, wi := v[i], w[i]
			for j := k; j < n; j++ {
				row[j] -= vi*cw[j] + wi*cv[j]
			}
		}
		for i := k + 2; i < n; i++ {
			A.Set(i, k, 0)
			A.Set(k, i, 0)
		}
		// Q ← Q·H
		for i := 0; i < n; i++ {
			row := Q.Data[i*n : i*n+n]
			var s complex128
			for j := k + 1; j < n; j++ {
				s += row[j] * v[j]
			}
			s *= 2
			for j := k + 1; j < n; j++ {
				row[j] -= s * cv[j]
			}
		}
	}
	// T = D·T'·Dᴴ with T' real: δ_0 = 1, δ_{i+1} = δ_i·e_i/|e_i|.
	d := make([]float64, n)
	e := make([]float64, n)
	delta := make([]complex128, n)
	delta[0] = 1
	for i := 0; i < n; i++ {
		d[i] = real(A.At(i, i))
		if i+1 < n {
			sub := A.At(i+1, i)
			mag := cmplx.Abs(sub)
			e[i] = mag
			if mag > 0 {
				delta[i+1] = delta[i] * sub / complex(mag, 0)
			} else {
				delta[i+1] = delta[i]
			}
		}
	}
	z := make([]float64, n*n)
	for i := 0; i < n; i++ {
		z[i*n+i] = 1
	}
	tqli(d, e, z, n)
	// eigenvectors = Q·D·Z
	vecs := New(n, n)
	qd := make([]complex128, n)
	for r := 0; r < n; r++ {
		for k := 0; k < n; k++ {
			qd[k] = Q.Data[r*n+k] * delta[k]
		}
		out := vecs.Data[r*n : r*n+n]
		for k := 0; k < n; k++ {
			q := qd[k]
			zr := z[k*n : k*n+n]
			for c := 0; c < n; c++ {
				out[c] += q * complex(zr[c], 0)
			}
		}
	}
	return sortDescending(d, vecs)
}

// tqli is the implicit QL iteration with Wilkinson-type shifts on the real
// symmetric tridiagonal (d diagonal, e[i] = T[i+1][i], e[n−1] ignored),
// accumulating the rotations into z (row-major n×n). Numerical Recipes §11.3.
func tqli(d, e, z []float64, n int) {
	if n > 0 {
		e[n-1] = 0
	}
	for l := 0; l < n; l++ {
		for iter := 0; iter < 60; iter++ {
			m := l
			for ; m < n-1; m++ {
				dd := math.Abs(d[m]) + math.Abs(d[m+1])
				if math.Abs(e[m])+dd == dd {
					break
				}
			}
			if m == l {
				break
			}
			g := (d[l+1] - d[l]) / (2 * e[l])
			r := math.Hypot(g, 1)
			g = d[m] - d[l] + e[l]/(g+math.Copysign(r, g))
			s, c, p := 1.0, 1.0, 0.0
			underflow := false
			i := m - 1
			for ; i >= l; i-- {
				f := s * e[i]
				b := c * e[i]
				r = math.Hypot(f, g)
				e[i+1] = r
				if r == 0 {
					d[i+1] -= p
					e[m] = 0
					underflow = true
					break
				}
				s = f / r
				c = g / r
				g = d[i+1] - p
				r = (d[i]-g)*s + 2*c*b
				p = s * r
				d[i+1] = g + p
				g = c*r - b
				for k := 0; k < n; k++ {
					f = z[k*n+i+1]
					z[k*n+i+1] = s*z[k*n+i] + c*f
					z[k*n+i] = c*z[k*n+i] - s*f
				}
			}
			if underflow {
				continue
			}
			d[l] -= p
			e[l] = g
			e[m] = 0
		}
	}
}
