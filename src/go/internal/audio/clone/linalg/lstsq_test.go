package linalg

import (
	"errors"
	"math/cmplx"
	"math/rand"
	"testing"
)

func TestLeastSquaresRecoversKnownSolution(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	m, n := 50, 6
	a := New(m, n)
	for i := range a.Data {
		a.Data[i] = complex(r.NormFloat64(), r.NormFloat64())
	}
	x := []complex128{1, -2i, 0.5 + 0.5i, 3, -1, 0.25i}
	b := make([]complex128, m)
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			b[i] += a.At(i, j) * x[j]
		}
	}
	f, err := FactorQR(a)
	if err != nil {
		t.Fatal(err)
	}
	got := f.Solve(b)
	for j := range x {
		if cmplx.Abs(got[j]-x[j]) > 1e-10 {
			t.Fatalf("x[%d] = %v, want %v", j, got[j], x[j])
		}
	}
}

func TestLeastSquaresRejectsDependentColumns(t *testing.T) {
	a := New(4, 2)
	for i := 0; i < 4; i++ {
		a.Set(i, 0, complex(float64(i+1), 0))
		a.Set(i, 1, complex(2*float64(i+1), 0))
	}
	if _, err := FactorQR(a); !errors.Is(err, ErrRankDeficient) {
		t.Fatalf("err = %v, want ErrRankDeficient", err)
	}
}
