package clone

import (
	"math"
	"math/cmplx"
	"math/rand"
	"sort"
	"testing"
)

func TestESPRITRecoversDampedExponentials(t *testing.T) {
	poles := []complex128{cmplx.Rect(0.995, 0.4), cmplx.Rect(0.99, -1.3), cmplx.Rect(0.985, 2.1)}
	amps := []complex128{1, 0.5i, -0.3}
	n := 400
	r := rand.New(rand.NewSource(1))
	y := make([]complex128, n)
	for m := 0; m < n; m++ {
		for k := range poles {
			y[m] += amps[k] * cmplx.Pow(poles[k], complex(float64(m), 0))
		}
		y[m] += complex(1e-3*r.NormFloat64(), 1e-3*r.NormFloat64())
	}
	res, err := esprit(y, 24)
	if err != nil {
		t.Fatal(err)
	}
	if res.Order != 3 {
		t.Fatalf("order %d, want 3", res.Order)
	}
	sort.Slice(res.Poles, func(i, j int) bool { return cmplx.Phase(res.Poles[i]) < cmplx.Phase(res.Poles[j]) })
	want := []complex128{poles[1], poles[0], poles[2]}
	for i := range want {
		if cmplx.Abs(res.Poles[i]-want[i]) > 1e-4 {
			t.Fatalf("pole %d = %v, want %v", i, res.Poles[i], want[i])
		}
	}
}

func TestBandLayoutCoversTheSpectrumOnce(t *testing.T) {
	for _, rate := range []int{44100, 48000} {
		bs := bandLayout(rate)
		if len(bs) != 24 || bs[0].Lo != 30 {
			t.Fatalf("rate %d: %d bands from %.1f", rate, len(bs), bs[0].Lo)
		}
		for i := 1; i < len(bs); i++ {
			if math.Abs(bs[i].Lo-bs[i-1].Hi) > 1e-9 {
				t.Fatalf("rate %d: gap between band %d and %d", rate, i-1, i)
			}
		}
		if top := bs[len(bs)-1].Hi; math.Abs(top-math.Min(20000, 0.45*float64(rate))) > 1e-6 {
			t.Fatalf("rate %d: top edge %.1f", rate, top)
		}
		for _, b := range bs {
			if float64(rate)/float64(b.Decim) < 2*(b.Hi-b.Lo) {
				t.Fatalf("rate %d band %d: decimated rate below twice the band width", rate, b.Index)
			}
		}
	}
}

func TestBasebandPreservesAModeAfterGainCorrection(t *testing.T) {
	rate := 48000
	m := Mode{Freq: 1000, Alpha: 3, Amp: 0.5, Phase: 0.9}
	x := Synthesize([]Mode{m}, rate, rate*2)
	var b band
	for _, c := range bandLayout(rate) {
		if m.Freq >= c.Lo && m.Freq < c.Hi {
			b = c
		}
	}
	from, to := 2400, len(x)
	y, first := baseband(x, rate, 0, from, to, b)
	if len(y) < 50 {
		t.Fatalf("only %d decimated samples", len(y))
	}
	s := complex(-m.Alpha, 2*math.Pi*(m.Freq-b.Center))
	tt := float64(first) / float64(rate)
	want := cmplx.Rect(m.Amp/2, m.Phase) * cmplx.Exp(s*complex(tt, 0)) * b.gain(s, rate)
	if cmplx.Abs(y[0]-want) > 1e-6 {
		t.Fatalf("y[0] = %v, want %v", y[0], want)
	}
}
