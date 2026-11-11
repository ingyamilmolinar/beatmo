package clone

import (
	"math"
	"sort"
	"testing"
)

func nearest(modes []Mode, f float64) (Mode, bool) {
	best, ok := Mode{}, false
	for _, m := range modes {
		if !ok || math.Abs(m.Freq-f) < math.Abs(best.Freq-f) {
			best, ok = m, true
		}
	}
	return best, ok
}

func cents(a, b float64) float64 { return 1200 * math.Log2(a/b) }

func checkRecovered(t *testing.T, got []Mode, want []Mode) {
	t.Helper()
	for _, w := range want {
		m, ok := nearest(got, w.Freq)
		if !ok {
			t.Fatalf("no modes at all")
		}
		if c := math.Abs(cents(m.Freq, w.Freq)); c > 0.5 {
			t.Errorf("%.1f Hz: got %.4f Hz (%.2f cents)", w.Freq, m.Freq, c)
		}
		if r := math.Abs(m.Alpha/w.Alpha - 1); r > 0.05 {
			t.Errorf("%.1f Hz: alpha %.3f, want %.3f (%.1f%%)", w.Freq, m.Alpha, w.Alpha, 100*r)
		}
		if d := math.Abs(dB(m.Amp) - dB(w.Amp)); d > 0.5 {
			t.Errorf("%.1f Hz: amp %.2f dB, want %.2f dB", w.Freq, dB(m.Amp), dB(w.Amp))
		}
	}
}

// 40 dB SNR: noise RMS 1% of the strongest mode's amplitude.
func TestEstimateModesKnownAnswer(t *testing.T) {
	want := barModes()
	x := modalSignal(want, 48000, 0.05, 2.5, 0.004, 11)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range res.Modes {
		t.Logf("mode %9.4f Hz  α %7.4f  amp %.5f  φ %+.3f  band %2d  SNR %.0f dB", m.Freq, m.Alpha, m.Amp, m.Phase, m.Band, m.SNRdB)
	}
	checkRecovered(t, res.Modes, want)
	if len(res.Modes) > len(want)+2 {
		t.Errorf("%d modes for %d true ones: %+v", len(res.Modes), len(want), res.Modes)
	}
	if !sort.SliceIsSorted(res.Modes, func(i, j int) bool { return res.Modes[i].Energy() > res.Modes[j].Energy() }) {
		t.Errorf("modes not sorted by energy")
	}
}

func TestEstimateModesResolvesADoublet(t *testing.T) {
	want := []Mode{{Freq: 440, Alpha: 1.0, Amp: 0.3}, {Freq: 441, Alpha: 1.3, Amp: 0.25, Phase: 1}}
	x := modalSignal(want, 48000, 0.05, 2, 0.003, 12)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	var near []Mode
	for _, m := range res.Modes {
		if m.Freq > 438 && m.Freq < 443 {
			near = append(near, m)
		}
	}
	if len(near) != 2 {
		t.Fatalf("doublet 440/441 Hz resolved into %d modes: %+v", len(near), near)
	}
	sort.Slice(near, func(i, j int) bool { return near[i].Freq < near[j].Freq })
	for i, w := range want {
		if math.Abs(near[i].Freq-w.Freq) > 0.1 {
			t.Errorf("doublet member %d at %.3f Hz, want %.1f", i, near[i].Freq, w.Freq)
		}
	}
}

func TestEstimateModesAt44100(t *testing.T) {
	want := barModes()
	x := modalSignal(want, 44100, 0.05, 2.5, 0.004, 13)
	p, err := Prepare(x, 44100, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	checkRecovered(t, res.Modes, want)
}

func TestEstimateModesBandEdgeCountedOnce(t *testing.T) {
	edge := bandLayout(48000)[12].Lo
	want := []Mode{{Freq: edge * 1.0005, Alpha: 3, Amp: 0.3}, {Freq: 300, Alpha: 2, Amp: 0.3}}
	x := modalSignal(want, 48000, 0.05, 2, 0.003, 14)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := EstimateModes(p)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range res.Modes {
		if math.Abs(cents(m.Freq, want[0].Freq)) < 20 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("edge mode at %.2f Hz counted %d times: %+v", want[0].Freq, n, res.Modes)
	}
	checkRecovered(t, res.Modes, want)
}
