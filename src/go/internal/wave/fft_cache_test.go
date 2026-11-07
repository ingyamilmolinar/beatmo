package wave

import (
	"math"
	"math/cmplx"
	"math/rand"
	"testing"
)

// The cached FFT must reproduce this BIT FOR BIT: the audio-tab digests compare raw IEEE-754 bits downstream of it.
func fftUncached(buf []complex128) {
	n := len(buf)
	bits := 0
	for m := n; m > 1; m >>= 1 {
		bits++
	}
	for i := 0; i < n; i++ {
		if j := bitReverse(i, bits); j > i {
			buf[i], buf[j] = buf[j], buf[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half := size / 2
		wn := -2.0 * math.Pi / float64(size)
		for start := 0; start < n; start += size {
			for k := 0; k < half; k++ {
				twiddle := cmplx.Rect(1, wn*float64(k))
				even := buf[start+k]
				odd := buf[start+k+half] * twiddle
				buf[start+k] = even + odd
				buf[start+k+half] = even - odd
			}
		}
	}
}

func windowUncached(seg []float64, wt WindowType) {
	fn := float64(len(seg))
	for i := range seg {
		var c float64
		switch wt {
		case WindowHann:
			c = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/fn))
		case WindowHamming:
			c = 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/fn)
		case WindowBlackman:
			c = 0.42 - 0.5*math.Cos(2*math.Pi*float64(i)/fn) + 0.08*math.Cos(4*math.Pi*float64(i)/fn)
		}
		seg[i] *= c
	}
}

func TestCachedFFTIsBitIdentical(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for _, n := range []int{1, 2, 4, 64, 1024, 2048, 4096, 16384} {
		a := make([]complex128, n)
		for i := range a {
			a[i] = complex(r.NormFloat64(), r.NormFloat64())
		}
		b := append([]complex128(nil), a...)
		fft(a)
		if n > 1 {
			fftUncached(b)
		}
		for i := range a {
			if math.Float64bits(real(a[i])) != math.Float64bits(real(b[i])) || math.Float64bits(imag(a[i])) != math.Float64bits(imag(b[i])) {
				t.Fatalf("n=%d bin %d: cached %v != uncached %v", n, i, a[i], b[i])
			}
		}
	}
}

func TestCachedWindowIsBitIdentical(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	for _, wt := range []WindowType{WindowHann, WindowHamming, WindowBlackman} {
		for _, n := range []int{1, 3, 2048, 4097} {
			a := make([]float64, n)
			for i := range a {
				a[i] = r.NormFloat64()
			}
			b := append([]float64(nil), a...)
			applyWindowInPlace(a, wt)
			windowUncached(b, wt)
			for i := range a {
				if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
					t.Fatalf("window %d n=%d i=%d: %v != %v", wt, n, i, a[i], b[i])
				}
			}
		}
	}
}

// Exactly the old copy → window → FFT result, zero-padded, truncated and exact, under every window.
func TestMagnitudeSpectrumIsBitIdentical(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for _, wt := range []WindowType{WindowHann, WindowHamming, WindowBlackman} {
		for _, tc := range []struct{ samples, size int }{{100, 2048}, {2048, 2048}, {5000, 4096}, {3, 1}, {0, 64}} {
			s := make([]float64, tc.samples)
			for i := range s {
				s[i] = r.NormFloat64()
			}
			got, gotHz := MagnitudeSpectrum(Wave{Samples: s, SampleRate: 48000}, tc.size, wt)
			n := nextPow2(tc.size)
			seg := make([]float64, n)
			copy(seg, s)
			windowUncached(seg, wt)
			buf := make([]complex128, n)
			for i, v := range seg {
				buf[i] = complex(v, 0)
			}
			if n > 1 {
				fftUncached(buf)
			}
			if gotHz != 48000/float64(n) || len(got) != n/2+1 {
				t.Fatalf("window %d %+v: %d bins at %v Hz", wt, tc, len(got), gotHz)
			}
			for i := range got {
				if want := cmplx.Abs(buf[i]); math.Float64bits(got[i]) != math.Float64bits(want) {
					t.Fatalf("window %d %+v bin %d: %v != %v", wt, tc, i, got[i], want)
				}
			}
		}
	}
}
