package linalg

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

func randHermitian(n int, seed int64) *Matrix {
	r := rand.New(rand.NewSource(seed))
	a := New(n, n)
	for i := 0; i < n; i++ {
		a.Set(i, i, complex(r.NormFloat64(), 0))
		for j := i + 1; j < n; j++ {
			v := complex(r.NormFloat64(), r.NormFloat64())
			a.Set(i, j, v)
			a.Set(j, i, cmplx.Conj(v))
		}
	}
	return a
}

func TestEigHermitianReconstructs(t *testing.T) {
	for _, n := range []int{1, 2, 5, 40} {
		a := randHermitian(n, int64(n))
		vals, v := EigHermitian(a)
		for i := 1; i < n; i++ {
			if vals[i] > vals[i-1] {
				t.Fatalf("n=%d: eigenvalues not descending: %v", n, vals)
			}
		}
		// A·v_k = λ_k·v_k and vᴴv = I
		for k := 0; k < n; k++ {
			for i := 0; i < n; i++ {
				var av complex128
				for j := 0; j < n; j++ {
					av += a.At(i, j) * v.At(j, k)
				}
				if cmplx.Abs(av-complex(vals[k], 0)*v.At(i, k)) > 1e-9 {
					t.Fatalf("n=%d: A·v != λv at (%d,%d)", n, i, k)
				}
			}
		}
		vh := Mul(ConjTranspose(v), v)
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				want := 0.0
				if i == j {
					want = 1
				}
				if cmplx.Abs(vh.At(i, j)-complex(want, 0)) > 1e-10 {
					t.Fatalf("n=%d: VᴴV not identity at (%d,%d): %v", n, i, j, vh.At(i, j))
				}
			}
		}
	}
}

func TestEigHermitianKnownSpectrum(t *testing.T) {
	// diag(3, 1, -2) rotated by a fixed unitary: eigenvalues must come back.
	q := randHermitian(3, 7) // any Hermitian has a unitary eigenbasis
	_, u := EigHermitian(q)
	d := New(3, 3)
	d.Set(0, 0, 3)
	d.Set(1, 1, 1)
	d.Set(2, 2, -2)
	a := Mul(Mul(u, d), ConjTranspose(u))
	vals, _ := EigHermitian(a)
	for i, want := range []float64{3, 1, -2} {
		if math.Abs(vals[i]-want) > 1e-10 {
			t.Fatalf("eig %d = %v, want %v", i, vals[i], want)
		}
	}
}

// Random and rank-deficient (ESPRIT-like) matrices.
func TestEigHermitianMatchesJacobi(t *testing.T) {
	for _, n := range []int{3, 17, 64} {
		a := randHermitian(n, int64(100+n))
		got, _ := EigHermitian(a)
		want, _ := eigHermitianJacobi(a)
		for i := range want {
			if math.Abs(got[i]-want[i]) > 1e-9*math.Max(1, math.Abs(want[i])) {
				t.Fatalf("n=%d eig %d: %v vs Jacobi %v", n, i, got[i], want[i])
			}
		}
	}
	// rank 2 + tiny noise, like a two-mode covariance
	n := 32
	a := New(n, n)
	r := rand.New(rand.NewSource(9))
	for _, z := range []complex128{cmplx.Rect(0.99, 0.5), cmplx.Rect(0.98, -1)} {
		u := make([]complex128, n)
		acc := complex(1, 0)
		for i := range u {
			u[i] = acc
			acc *= z
		}
		for i := 0; i < n; i++ {
			for j := 0; j < n; j++ {
				a.Set(i, j, a.At(i, j)+u[i]*cmplx.Conj(u[j]))
			}
		}
	}
	for i := 0; i < n; i++ {
		a.Set(i, i, a.At(i, i)+complex(1e-6*(1+r.Float64()), 0))
	}
	got, _ := EigHermitian(a)
	want, _ := eigHermitianJacobi(a)
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9*math.Max(1, math.Abs(want[i])) {
			t.Fatalf("rank-2 eig %d: %v vs Jacobi %v", i, got[i], want[i])
		}
	}
}

func BenchmarkEigHermitian128(b *testing.B) {
	a := randHermitian(128, 5)
	for i := 0; i < b.N; i++ {
		EigHermitian(a)
	}
}
