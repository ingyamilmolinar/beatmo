package clone

import (
	"math"
	"sync"
)

// stft.go — a lean Hann-windowed magnitude STFT for the loss. Refine scores
// ~1,000 renders per instrument; wave.MagnitudeSpectrum allocates three
// buffers per FRAME (≈14 MB per 4096-point spectrogram of a 3 s clip), so the
// loss carries its own radix-2 plan with precomputed twiddles and reuses one
// scratch per call.

type fftPlan struct {
	n        int
	rev      []int
	cos, sin []float64
	win      []float64
}

var fftPlans sync.Map // n → *fftPlan, read-only once built

func planFor(n int) *fftPlan {
	if p, ok := fftPlans.Load(n); ok {
		return p.(*fftPlan)
	}
	p := &fftPlan{n: n, rev: make([]int, n), cos: make([]float64, n/2), sin: make([]float64, n/2), win: make([]float64, n)}
	bits := 0
	for 1<<bits < n {
		bits++
	}
	for i := range p.rev {
		r := 0
		for b := 0; b < bits; b++ {
			if i&(1<<b) != 0 {
				r |= 1 << (bits - 1 - b)
			}
		}
		p.rev[i] = r
	}
	for i := range p.cos {
		p.cos[i] = math.Cos(2 * math.Pi * float64(i) / float64(n))
		p.sin[i] = -math.Sin(2 * math.Pi * float64(i) / float64(n))
	}
	for i := range p.win {
		p.win[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n)))
	}
	got, _ := fftPlans.LoadOrStore(n, p)
	return got.(*fftPlan)
}

// fft2 transforms two windowed real frames at once (a + i·b in one complex
// FFT — the textbook two-for-one) and writes both magnitude spectra.
func (p *fftPlan) fft2(fa, fb, re, im, ma, mb []float64) {
	n := p.n
	for i := 0; i < n; i++ {
		j := p.rev[i]
		re[j] = fa[i] * p.win[i]
		im[j] = fb[i] * p.win[i]
	}
	for size := 2; size <= n; size <<= 1 {
		half, step := size/2, n/size
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				c, s := p.cos[k*step], p.sin[k*step]
				a, b := start+k, start+k+half
				tr := re[b]*c - im[b]*s
				ti := re[b]*s + im[b]*c
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a], im[a] = re[a]+tr, im[a]+ti
			}
		}
	}
	// A[k] = (Z[k] + conj Z[N−k]) / 2, B[k] = (Z[k] − conj Z[N−k]) / 2i.
	for k := 0; k <= n/2; k++ {
		nk := (n - k) & (n - 1)
		zr, zi, wr, wi := re[k], im[k], re[nk], -im[nk]
		ar, ai := (zr+wr)/2, (zi+wi)/2
		br, bi := (zi-wi)/2, -(zr-wr)/2
		ma[k] = math.Sqrt(ar*ar + ai*ai)
		mb[k] = math.Sqrt(br*br + bi*bi)
	}
}

// eachFrame calls fn with the magnitude spectrum of every frame of x at size
// n, hop n/4 (frames start at 0, hop, …, the last that fits; a signal shorter
// than n is one zero-padded frame), two frames per FFT. m is reused between
// calls.
func eachFrame(x []float64, n int, fn func(i int, m []float64)) {
	p := planFor(n)
	hop := n / 4
	count := 1
	if len(x) > n {
		count = (len(x)-n)/hop + 1
	}
	re, im := make([]float64, n), make([]float64, n)
	fa, fb := make([]float64, n), make([]float64, n)
	ma, mb := make([]float64, n/2+1), make([]float64, n/2+1)
	frame := func(dst []float64, i int) {
		clear(dst)
		if i < count {
			start := i * hop
			copy(dst, x[start:min(len(x), start+n)])
		}
	}
	for i := 0; i < count; i += 2 {
		frame(fa, i)
		frame(fb, i+1)
		p.fft2(fa, fb, re, im, ma, mb)
		fn(i, ma)
		if i+1 < count {
			fn(i+1, mb)
		}
	}
}

// spectrogram stores every frame of eachFrame.
func spectrogram(x []float64, n int) [][]float64 {
	var out [][]float64
	eachFrame(x, n, func(_ int, m []float64) { out = append(out, append([]float64(nil), m...)) })
	return out
}
