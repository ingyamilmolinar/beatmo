// Package linalg is the small dense complex linear algebra the WAV-clone analysis needs; gonum has no complex
// decompositions. Every routine is deterministic. Dev-only.
package linalg

import "math/cmplx"

// Matrix is a dense row-major complex matrix.
type Matrix struct {
	Rows, Cols int
	Data       []complex128
}

// New returns a zero rows×cols matrix.
func New(rows, cols int) *Matrix {
	return &Matrix{Rows: rows, Cols: cols, Data: make([]complex128, rows*cols)}
}

// At returns element (i, j).
func (m *Matrix) At(i, j int) complex128 { return m.Data[i*m.Cols+j] }

// Set writes element (i, j).
func (m *Matrix) Set(i, j int, v complex128) { m.Data[i*m.Cols+j] = v }

// Clone returns a deep copy.
func (m *Matrix) Clone() *Matrix {
	c := &Matrix{Rows: m.Rows, Cols: m.Cols, Data: make([]complex128, len(m.Data))}
	copy(c.Data, m.Data)
	return c
}

// Mul returns a·b.
func Mul(a, b *Matrix) *Matrix {
	out := New(a.Rows, b.Cols)
	for i := 0; i < a.Rows; i++ {
		for k := 0; k < a.Cols; k++ {
			aik := a.At(i, k)
			if aik == 0 {
				continue
			}
			for j := 0; j < b.Cols; j++ {
				out.Data[i*out.Cols+j] += aik * b.At(k, j)
			}
		}
	}
	return out
}

// ConjTranspose returns aᴴ.
func ConjTranspose(a *Matrix) *Matrix {
	out := New(a.Cols, a.Rows)
	for i := 0; i < a.Rows; i++ {
		for j := 0; j < a.Cols; j++ {
			out.Set(j, i, cmplx.Conj(a.At(i, j)))
		}
	}
	return out
}
