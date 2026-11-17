package clone

import (
	"math"
	"testing"
)

func TestStrikeSpectrumShape(t *testing.T) {
	tc := 0.001
	if StrikeSpectrum(0, tc) != 1 {
		t.Fatal("DC gain must be 1")
	}
	prev := 1.0
	for f := 10.0; f < 40000; f *= 1.05 {
		v := StrikeSpectrum(f, tc)
		if !(v > 0) || v > prev {
			t.Fatalf("S(%.0f Hz) = %g: must be positive and non-increasing (no nulls)", f, v)
		}
		prev = v
	}
	// The exact raised-cosine asymptote 1/(π x³) far above the corner.
	x := 20.0
	if v := StrikeSpectrum(x/tc, tc) * math.Pi * x * x * x; math.Abs(v-1) > 1e-6 {
		t.Fatalf("asymptote ratio %v, want 1", v)
	}
}

func TestEstimateStrikeRecoversContactTime(t *testing.T) {
	for _, c := range []struct{ tc, h float64 }{{0.001, 0}, {0.0004, -3}, {0.003, 2}} {
		var modes []Mode
		for i := 0; i < 40; i++ {
			f := 100 * math.Pow(150, float64(i)/39) // 100 Hz .. 15 kHz
			amp := 0.2 * StrikeSpectrum(f, c.tc) * HardnessGain(f, c.h)
			if amp < 1e-7 {
				continue
			}
			modes = append(modes, Mode{Freq: f, Alpha: 5, Amp: amp})
		}
		got := EstimateStrike(modes)
		if math.Abs(got.TcSec/c.tc-1) > 0.10 {
			t.Errorf("tc %.4f ms: got %.4f ms", 1000*c.tc, 1000*got.TcSec)
		}
		if math.Abs(got.HardnessDBPerOct-c.h) > 0.5 {
			t.Errorf("tc %.4f ms: hardness %.2f, want %.2f", 1000*c.tc, got.HardnessDBPerOct, c.h)
		}
	}
}
