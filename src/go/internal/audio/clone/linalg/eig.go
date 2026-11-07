package linalg

import (
	"errors"
	"math"
	"math/cmplx"
)

// ErrNoConvergence is returned when the QR iteration exceeds its budget.
var ErrNoConvergence = errors.New("linalg: QR iteration did not converge")

// Hessenberg reduction, then single-shift QR with Wilkinson shifts (an exceptional shift every 10th iteration);
// only eigenvalues are needed, so each step touches only the active block. a is not modified.
func Eigenvalues(a *Matrix) ([]complex128, error) {
	n := a.Rows
	if n == 0 {
		return nil, nil
	}
	h := hessenberg(a)
	var hnorm float64
	for _, v := range h.Data {
		if m := cmplx.Abs(v); m > hnorm {
			hnorm = m
		}
	}
	const eps = 2.220446049250313e-16
	eig := make([]complex128, n)
	hi := n - 1
	iter, total := 0, 0
	for hi >= 0 {
		if hi == 0 {
			eig[0] = h.At(0, 0)
			break
		}
		l := hi
		for ; l > 0; l-- {
			s := cmplx.Abs(h.At(l-1, l-1)) + cmplx.Abs(h.At(l, l))
			if s == 0 {
				s = hnorm
			}
			if cmplx.Abs(h.At(l, l-1)) <= eps*s {
				h.Set(l, l-1, 0)
				break
			}
		}
		if l == hi {
			eig[hi] = h.At(hi, hi)
			hi--
			iter = 0
			continue
		}
		if total > 100*n {
			return nil, ErrNoConvergence
		}
		iter++
		total++
		mu := wilkinson(h.At(hi-1, hi-1), h.At(hi-1, hi), h.At(hi, hi-1), h.At(hi, hi))
		if iter%10 == 0 {
			mu = h.At(hi, hi) + complex(cmplx.Abs(h.At(hi, hi-1)), 0)
		}
		qrStep(h, l, hi, mu)
	}
	return eig, nil
}

// wilkinson returns the eigenvalue of [[a b] [c d]] closer to d.
func wilkinson(a, b, c, d complex128) complex128 {
	tr := (a + d) / 2
	disc := cmplx.Sqrt((a-d)*(a-d)/4 + b*c)
	m1, m2 := tr+disc, tr-disc
	if cmplx.Abs(m1-d) < cmplx.Abs(m2-d) {
		return m1
	}
	return m2
}

// givens returns (c, s), c real, with [c s; −s̄ c]·[a; b] = [r; 0].
func givens(a, b complex128) (float64, complex128) {
	if b == 0 {
		return 1, 0
	}
	if a == 0 {
		return 0, cmplx.Conj(b) / complex(cmplx.Abs(b), 0)
	}
	aa, bb := cmplx.Abs(a), cmplx.Abs(b)
	r := math.Hypot(aa, bb)
	return aa / r, (a / complex(aa, 0)) * cmplx.Conj(b) / complex(r, 0)
}

// H ← RQ + μI on the active block [l, hi].
func qrStep(h *Matrix, l, hi int, mu complex128) {
	for k := l; k <= hi; k++ {
		h.Set(k, k, h.At(k, k)-mu)
	}
	cs := make([]float64, hi-l)
	sn := make([]complex128, hi-l)
	for k := l; k < hi; k++ {
		c, s := givens(h.At(k, k), h.At(k+1, k))
		cs[k-l], sn[k-l] = c, s
		cc := complex(c, 0)
		for j := k; j <= hi; j++ {
			x, y := h.At(k, j), h.At(k+1, j)
			h.Set(k, j, cc*x+s*y)
			h.Set(k+1, j, -cmplx.Conj(s)*x+cc*y)
		}
	}
	for k := l; k < hi; k++ {
		cc, s := complex(cs[k-l], 0), sn[k-l]
		for i := l; i <= k+1; i++ {
			x, y := h.At(i, k), h.At(i, k+1)
			h.Set(i, k, x*cc+y*cmplx.Conj(s))
			h.Set(i, k+1, -x*s+y*cc)
		}
	}
	for k := l; k <= hi; k++ {
		h.Set(k, k, h.At(k, k)+mu)
	}
}

func hessenberg(a *Matrix) *Matrix {
	n := a.Rows
	h := a.Clone()
	v := make([]complex128, n)
	for k := 0; k < n-2; k++ {
		var norm float64
		for i := k + 1; i < n; i++ {
			x := h.At(i, k)
			norm += real(x)*real(x) + imag(x)*imag(x)
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		x0 := h.At(k+1, k)
		ph := complex(1, 0)
		if x0 != 0 {
			ph = x0 / complex(cmplx.Abs(x0), 0)
		}
		alpha := -ph * complex(norm, 0)
		var vn float64
		for i := k + 1; i < n; i++ {
			v[i] = h.At(i, k)
		}
		v[k+1] -= alpha
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
		for j := 0; j < n; j++ { // H ← P·H
			var dot complex128
			for i := k + 1; i < n; i++ {
				dot += cmplx.Conj(v[i]) * h.At(i, j)
			}
			for i := k + 1; i < n; i++ {
				h.Set(i, j, h.At(i, j)-2*v[i]*dot)
			}
		}
		for i := 0; i < n; i++ { // H ← H·P
			var dot complex128
			for j := k + 1; j < n; j++ {
				dot += h.At(i, j) * v[j]
			}
			for j := k + 1; j < n; j++ {
				h.Set(i, j, h.At(i, j)-2*dot*cmplx.Conj(v[j]))
			}
		}
		for i := k + 2; i < n; i++ {
			h.Set(i, k, 0)
		}
	}
	return h
}
