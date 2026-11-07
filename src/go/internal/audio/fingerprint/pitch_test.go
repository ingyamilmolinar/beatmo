package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestDetectF0_PureTone(t *testing.T) {
	got := DetectF0(wave.Sine(220, 0.9, 0.5, 44100))
	if math.Abs(got-220) > 220*0.02 {
		t.Errorf("F0=%.1f want ~220", got)
	}
}

func TestDetectF0_MissingFundamental(t *testing.T) {
	// 440+660+880 (harmonics 2,3,4 of 220) — naive lowest-peak picks 440.
	sr := 44100
	mk := func(f float64) []float64 { return wave.Sine(f, 0.3, 0.5, sr).Samples }
	a, b, c := mk(440), mk(660), mk(880)
	sum := make([]float64, len(a))
	for i := range sum {
		sum[i] = a[i] + b[i] + c[i]
	}
	got := DetectF0(wave.Wave{Samples: sum, SampleRate: sr})
	if math.Abs(got-220) > 220*0.05 {
		t.Errorf("F0=%.1f want ~220 (missing fundamental)", got)
	}
}

func TestDetectF0_DominantSecondHarmonic(t *testing.T) {
	// A spectral-only octave guard is tempted by 440.
	sr := 44100
	n := int(0.6 * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		s[i] = 0.25*math.Sin(2*math.Pi*220*tsec) +
			0.9*math.Sin(2*math.Pi*440*tsec) +
			0.3*math.Sin(2*math.Pi*660*tsec)
	}
	got := DetectF0(wave.Wave{Samples: s, SampleRate: sr})
	if math.Abs(got-220) > 220*0.05 {
		t.Errorf("F0=%.1f want ~220 (dominant 2nd harmonic must not cause octave-up)", got)
	}
}

func TestDetectF0_RichWeakFundamental(t *testing.T) {
	// A full 12-harmonic series with H1 ~7 dB under H2: coverage ties f0 and 2f0 and the k=1 tiebreak picks the octave up;
	// the strong odd harmonics cannot come from 2f0, and the detector must use that.
	sr := 48000
	n := int(1.5 * float64(sr))
	f0 := 122.8
	amps := []float64{0.45, 1.0, 0.92, 0.55, 0.40, 0.64, 0.33, 0.19, 0.23, 0.32, 0.16, 0.14}
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		for k, a := range amps {
			s[i] += 0.2 * a * math.Sin(2*math.Pi*f0*float64(k+1)*tsec)
		}
	}
	got := DetectF0(wave.Wave{Samples: s, SampleRate: sr})
	if math.Abs(got-f0) > f0*0.05 {
		t.Errorf("F0=%.1f want ~%.1f (rich weak-fundamental reed spectrum must not octave-up)", got, f0)
	}
}

func TestDetectF0_RichWeakFundamental_H3Lock(t *testing.T) {
	// H2 ≈ H3: YIN's median can lock onto H3's period; 1×, 2× and 4× of f0 cannot come from 3f0.
	sr := 48000
	n := int(1.5 * float64(sr))
	f0 := 122.8
	amps := []float64{0.55, 1.0, 0.99, 0.68, 0.40, 0.69, 0.34, 0.24, 0.17, 0.17, 0.20, 0.24, 0.27, 0.31, 0.32, 0.31}
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		for k, a := range amps {
			s[i] += 0.15 * a * math.Sin(2*math.Pi*f0*float64(k+1)*tsec)
		}
	}
	got := DetectF0(wave.Wave{Samples: s, SampleRate: sr})
	if math.Abs(got-f0) > f0*0.05 {
		t.Errorf("F0=%.1f want ~%.1f (equal-H2/H3 reed spectrum must not lock onto H3)", got, f0)
	}
}

func TestDetectF0_SubOctaveLayerDoesNotHalve(t *testing.T) {
	// A real half-grid series and period-doubling components, yet the note IS D2: the sub-grid's odd teeth carry clearly
	// less than a true weak fundamental's.
	sr := 48000
	n := int(1.5 * float64(sr))
	f0 := 146.8
	s := make([]float64, n)
	type comp struct{ hz, a float64 }
	comps := []comp{
		{f0 * 0.5, 0.36}, {f0, 1.0}, {f0 * 1.5, 0.70}, {f0 * 2, 0.77},
		{f0 * 2.5, 0.83}, {f0 * 3, 1.52}, {f0 * 3.5, 0.43}, {f0 * 4, 0.60},
	}
	for i := range s {
		tsec := float64(i) / float64(sr)
		for _, c := range comps {
			s[i] += 0.1 * c.a * math.Sin(2*math.Pi*c.hz*tsec)
		}
	}
	got := DetectF0(wave.Wave{Samples: s, SampleRate: sr})
	if math.Abs(got-f0) > f0*0.09 && math.Abs(got-2*f0) > 2*f0*0.09 {
		t.Errorf("F0=%.1f want ~%.1f (sub-octave layer must not force a halving)", got, f0)
	}
}

func TestDetectF0_Vibrato(t *testing.T) {
	sr := 44100
	// Phase accumulation, as the synth's oscillator does; a closed-form sin(f·t) with varying f is not a true vibrato.
	mk := func(f0, vibHz, depth, dur float64) wave.Wave {
		n := int(dur * float64(sr))
		s := make([]float64, n)
		var phase1, phase2, phase3 float64
		for i := range s {
			ts := float64(i) / float64(sr)
			f := f0 * (1 + depth*math.Sin(2*math.Pi*vibHz*ts))
			inc := 2 * math.Pi * f / float64(sr)
			phase1 += inc
			phase2 += 2 * inc
			phase3 += 3 * inc
			s[i] = 0.6*math.Sin(phase1) + 0.3*math.Sin(phase2) + 0.15*math.Sin(phase3)
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	cases := []struct{ vibHz, depth float64 }{{5.2, 0.07}, {5.2, 0.02}, {6.0, 0.05}}
	for _, c := range cases {
		got := DetectF0(mk(220, c.vibHz, c.depth, 1.2))
		if math.Abs(got-220) > 220*0.03 {
			t.Errorf("vib %.1fHz depth %.2f: F0=%.1f want ~220 (±3%%)", c.vibHz, c.depth, got)
		}
	}
}

// 591 Hz with a weaker 197 Hz component gives the whole signal a 197 Hz period; f0 and f0/3 tie on coverage, and only a
// guard that tests 3f can reach 591, chosen by its k=1 energy.
func TestDetectF0_ThirdSubharmonicGuard(t *testing.T) {
	sr := 44100
	trueF0 := 591.0     // D5
	subHz := trueF0 / 3 // ~197 Hz
	n := int(0.5 * float64(sr))
	buf := make([]float64, n)
	for i := range buf {
		tsec := float64(i) / float64(sr)
		// Main signal: 591 Hz with harmonics 1182 and 1773.
		buf[i] = 0.8*math.Sin(2*math.Pi*trueF0*tsec) +
			0.5*math.Sin(2*math.Pi*2*trueF0*tsec) +
			0.25*math.Sin(2*math.Pi*3*trueF0*tsec) +
			// Weaker 197 Hz component causes YIN to lock onto the 197 Hz period.
			0.35*math.Sin(2*math.Pi*subHz*tsec)
	}
	w := wave.Wave{Samples: buf, SampleRate: sr}
	got := DetectF0(w)
	if got < 575 || got > 620 {
		t.Fatalf("DetectF0 = %.1f Hz, want ~591 (D5); old guard {f/2,f,2f} would miss the 3f=591 candidate and stay at ~197", got)
	}
}
