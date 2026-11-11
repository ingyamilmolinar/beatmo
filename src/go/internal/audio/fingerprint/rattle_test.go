package fingerprint

import (
	"math"
	"math/rand"
	"testing"
)

// rattleFixture renders a decaying excitation through a bank of damped modes
// between 2 and 12 kHz: the same topology as the cymbal plate's wire layer.
// dense=true drives it with continuous noise (a fused buzz); dense=false with a
// Poisson click train whose RATE decays with its amplitude — the legacy wire
// behaviour that the listener heard as rattle.
func rattleFixture(dense bool) []float64 {
	const sr = 44100
	n := sr
	rng := rand.New(rand.NewSource(7))
	exc := make([]float64, n)
	if dense {
		for i := range exc {
			exc[i] = rng.NormFloat64() * math.Exp(-float64(i)/float64(sr)/0.12)
		}
	} else {
		t := 0.0
		for {
			lam := 1500 * math.Exp(-t/0.12)
			if lam < 5 {
				break
			}
			t += -math.Log(1-rng.Float64()) / lam
			at := int(t * sr)
			if at >= n {
				break
			}
			exc[at] += math.Exp(-t/0.12) * (0.2 + 0.8*rng.Float64())
		}
	}
	out := make([]float64, n)
	for m := 0; m < 60; m++ {
		f := 2000 + 10000*rng.Float64()
		r := math.Exp(-120.0 / sr)
		w := 2 * math.Pi * f / sr
		c1, c2 := 2*r*math.Cos(w), -r*r
		var y1, y2 float64
		for i := range out {
			y := exc[i] + c1*y1 + c2*y2
			y2, y1 = y1, y
			out[i] += y
		}
	}
	return out
}

func TestTailRattleSeparatesClicksFromBuzz(t *testing.T) {
	buzz := TailRattle(rattleFixture(true), 44100)
	clicks := TailRattle(rattleFixture(false), 44100)
	// A synthetic noise buzz still has chance 1 ms spikes near the -55 dB floor,
	// so it is not exactly 0; what matters is an order-of-magnitude gap.
	if buzz > 25 {
		t.Errorf("dense buzz scored %.1f ticks/s, want < 25 (a fused buzz is not rattle)", buzz)
	}
	if clicks < 100 {
		t.Errorf("sparse decaying click train scored %.1f ticks/s, want > 100", clicks)
	}
	if clicks < 10*buzz {
		t.Errorf("clicks %.1f vs buzz %.1f: want at least a 10x separation", clicks, buzz)
	}
	t.Logf("buzz %.1f, clicks %.1f", buzz, clicks)
}

func TestTailRattleIsZeroForARingingTone(t *testing.T) {
	const sr = 44100
	s := make([]float64, sr)
	for i := range s {
		tt := float64(i) / sr
		s[i] = math.Sin(2*math.Pi*3000*tt) * math.Exp(-6*tt)
	}
	if got := TailRattle(s, sr); got > 1 {
		t.Errorf("a clean decaying 3 kHz tone scored %.1f ticks/s, want ~0", got)
	}
	if got := TailRattle(nil, sr); got != 0 {
		t.Errorf("empty input scored %.1f", got)
	}
}
