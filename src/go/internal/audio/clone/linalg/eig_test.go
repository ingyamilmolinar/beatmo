package linalg

import (
	"math/cmplx"
	"math/rand"
	"sort"
	"testing"
)

// matching sorts both lists by (real, imag) and compares pairwise.
func sameSpectrum(t *testing.T, got, want []complex128, tol float64) {
	t.Helper()
	key := func(v []complex128) func(i, j int) bool {
		return func(i, j int) bool {
			if real(v[i]) != real(v[j]) {
				return real(v[i]) < real(v[j])
			}
			return imag(v[i]) < imag(v[j])
		}
	}
	g := append([]complex128(nil), got...)
	w := append([]complex128(nil), want...)
	sort.Slice(g, key(g))
	sort.Slice(w, key(w))
	if len(g) != len(w) {
		t.Fatalf("got %d eigenvalues, want %d", len(g), len(w))
	}
	for i := range g {
		if cmplx.Abs(g[i]-w[i]) > tol {
			t.Fatalf("eigenvalue %d: got %v want %v (all got %v)", i, g[i], w[i], g)
		}
	}
}

func TestEigenvaluesKnownPoles(t *testing.T) {
	// Damped-pole-like spectrum, incl. a close pair (a doublet).
	want := []complex128{
		cmplx.Rect(0.999, 0.3), cmplx.Rect(0.998, 0.3005), cmplx.Rect(0.95, -1.2),
		cmplx.Rect(0.9, 2.5), cmplx.Rect(0.7, 0.01), 0.5,
	}
	n := len(want)
	r := rand.New(rand.NewSource(3))
	s := New(n, n) // a random (invertible with probability 1) similarity
	for i := range s.Data {
		s.Data[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	// A = S·D·S⁻¹, built as solving S·X = D·S⁻¹… simpler: A = S·D·S⁻¹ via
	// the QR solve of S against D columns.
	d := New(n, n)
	for i, v := range want {
		d.Set(i, i, v)
	}
	sd := Mul(s, d)
	f, err := FactorQR(ConjTranspose(s)) // (S⁻¹)ᴴ = (Sᴴ)⁻¹; solve Sᴴ·Y = I → Y = S⁻ᴴ
	if err != nil {
		t.Fatal(err)
	}
	sinvH := New(n, n)
	for j := 0; j < n; j++ {
		e := make([]complex128, n)
		e[j] = 1
		y := f.Solve(e)
		for i := 0; i < n; i++ {
			sinvH.Set(i, j, y[i])
		}
	}
	a := Mul(sd, ConjTranspose(sinvH))
	got, err := Eigenvalues(a)
	if err != nil {
		t.Fatal(err)
	}
	sameSpectrum(t, got, want, 1e-8)
}

func TestEigenvaluesTriangularAndScalar(t *testing.T) {
	a := New(3, 3)
	a.Set(0, 0, 2)
	a.Set(0, 1, 5)
	a.Set(1, 1, complex(0, 1))
	a.Set(1, 2, 4)
	a.Set(2, 2, -1)
	got, err := Eigenvalues(a)
	if err != nil {
		t.Fatal(err)
	}
	sameSpectrum(t, got, []complex128{2, complex(0, 1), -1}, 1e-12)
	one := New(1, 1)
	one.Set(0, 0, 0.3+0.4i)
	got, _ = Eigenvalues(one)
	sameSpectrum(t, got, []complex128{0.3 + 0.4i}, 0)
}
