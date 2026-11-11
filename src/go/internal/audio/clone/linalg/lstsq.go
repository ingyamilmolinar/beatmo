package linalg

import (
	"errors"
	"math"
	"math/cmplx"
)

// ErrRankDeficient is returned when a column is numerically dependent on the
// ones before it.
var ErrRankDeficient = errors.New("linalg: matrix is rank deficient")

// QR is a thin QR factorisation (Q m×n with orthonormal columns, R n×n upper
// triangular).
type QR struct{ Q, R *Matrix }

// FactorQR factors a (m ≥ n) by modified Gram–Schmidt with one full
// re-orthogonalisation pass ("twice is enough"), which keeps Q orthonormal to
// working precision for the near-dependent columns a Vandermonde matrix of a
// doublet produces.
func FactorQR(a *Matrix) (*QR, error) {
	m, n := a.Rows, a.Cols
	if m < n {
		return nil, errors.New("linalg: FactorQR needs rows >= cols")
	}
	q := New(m, n)
	r := New(n, n)
	v := make([]complex128, m)
	var scale float64
	for j := 0; j < n; j++ {
		for i := 0; i < m; i++ {
			v[i] = a.At(i, j)
		}
		if cn := norm2(v); cn > scale {
			scale = cn
		}
		for pass := 0; pass < 2; pass++ {
			for k := 0; k < j; k++ {
				var dot complex128
				for i := 0; i < m; i++ {
					dot += cmplx.Conj(q.At(i, k)) * v[i]
				}
				r.Set(k, j, r.At(k, j)+dot)
				for i := 0; i < m; i++ {
					v[i] -= dot * q.At(i, k)
				}
			}
		}
		nv := norm2(v)
		if nv == 0 || nv <= 1e-13*scale {
			return nil, ErrRankDeficient
		}
		r.Set(j, j, complex(nv, 0))
		for i := 0; i < m; i++ {
			q.Set(i, j, v[i]/complex(nv, 0))
		}
	}
	return &QR{Q: q, R: r}, nil
}

// Solve returns x minimising ‖a·x − b‖₂ for the factored a.
func (f *QR) Solve(b []complex128) []complex128 {
	m, n := f.Q.Rows, f.Q.Cols
	y := make([]complex128, n)
	for k := 0; k < n; k++ {
		var s complex128
		for i := 0; i < m; i++ {
			s += cmplx.Conj(f.Q.At(i, k)) * b[i]
		}
		y[k] = s
	}
	x := make([]complex128, n)
	for k := n - 1; k >= 0; k-- {
		s := y[k]
		for j := k + 1; j < n; j++ {
			s -= f.R.At(k, j) * x[j]
		}
		x[k] = s / f.R.At(k, k)
	}
	return x
}

func norm2(v []complex128) float64 {
	var s float64
	for _, x := range v {
		s += real(x)*real(x) + imag(x)*imag(x)
	}
	return math.Sqrt(s)
}
