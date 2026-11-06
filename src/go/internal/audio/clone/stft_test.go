package clone

import (
	"math"
	"math/rand"
	"testing"
)

// TestEachFrameMatchesADirectDFT: the two-frames-per-FFT spectrogram equals a
// naive DFT of each Hann-windowed frame, odd frame counts included.
func TestEachFrameMatchesADirectDFT(t *testing.T) {
	const n = 64
	r := rand.New(rand.NewSource(4))
	for _, length := range []int{30, n, n + 16, n + 5*16 + 3} { // 1, 1, 2, 6 frames
		x := make([]float64, length)
		for i := range x {
			x[i] = r.NormFloat64()
		}
		p := planFor(n)
		frames := 0
		eachFrame(x, n, func(i int, m []float64) {
			if i != frames {
				t.Fatalf("frame %d delivered as %d", frames, i)
			}
			frames++
			seg := make([]float64, n)
			copy(seg, x[i*n/4:min(len(x), i*n/4+n)])
			for k := 0; k <= n/2; k++ {
				var re, im float64
				for j, v := range seg {
					a := -2 * math.Pi * float64(j*k) / n
					re += v * p.win[j] * math.Cos(a)
					im += v * p.win[j] * math.Sin(a)
				}
				if d := math.Abs(math.Hypot(re, im) - m[k]); d > 1e-9 {
					t.Fatalf("len %d frame %d bin %d: %.12f vs %.12f", length, i, k, m[k], math.Hypot(re, im))
				}
			}
		})
		want := 1
		if length > n {
			want = (length-n)/(n/4) + 1
		}
		if frames != want {
			t.Fatalf("len %d: %d frames, want %d", length, frames, want)
		}
	}
}
