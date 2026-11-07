package soundview

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// Known answers: every analysis is checked against a synthetic signal whose
// true value is known in closed form. These are the contracts the Synth-tab
// graphs rely on (spec 2026-10-24 §5 T2).

const sr = 48000

func gen(sec float64, f func(t float64) float64) []float32 {
	n := int(sec * sr)
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(f(float64(i) / sr))
	}
	return out
}

func cents(a, b float64) float64 { return 1200 * math.Log2(a/b) }

func TestEnvelopeAudibleTimeOfAnExponentialDecay(t *testing.T) {
	const tau = 0.1
	pcm := gen(1.5, func(t float64) float64 { return 0.8 * math.Exp(-t/tau) * math.Sin(2*math.Pi*440*t) })
	a := New(pcm, sr, 1.5)
	want := tau * math.Log(100) // -40 dB
	if got := a.AudibleSec(); math.Abs(got-want) > 0.05*want {
		t.Fatalf("AudibleSec = %.4f, want %.4f ±5%%", got, want)
	}
	env := a.Envelope()
	if env.HopSec != 0.005 || len(env.RMSdB) == 0 || len(env.RMSdB) != len(env.PeakdB) {
		t.Fatalf("envelope shape: hop %v, %d rms, %d peak", env.HopSec, len(env.RMSdB), len(env.PeakdB))
	}
	if pk := a.PeakDB(); math.Abs(pk-20*math.Log10(0.8)) > 0.5 {
		t.Fatalf("PeakDB = %.2f, want %.2f", pk, 20*math.Log10(0.8))
	}
}

func TestAttackOfALinearRamp(t *testing.T) {
	for _, lead := range []float64{0, 0.05} {
		pcm := gen(0.6, func(t float64) float64 {
			t -= lead
			if t < 0 {
				return 0
			}
			g := 1.0
			if t < 0.02 {
				g = t / 0.02
			}
			return 0.9 * g * math.Sin(2*math.Pi*300*t)
		})
		a := New(pcm, sr, 0.6)
		if got := a.AttackSec(); math.Abs(got-0.02) > 0.004 {
			t.Fatalf("lead %.2f: AttackSec = %.4f, want 0.020 ±4 ms", lead, got)
		}
		on := a.Onsets()
		if len(on) == 0 || math.Abs(on[0]-lead) > 0.005 {
			t.Fatalf("lead %.2f: Onsets = %v, want the first at %.3f", lead, on, lead)
		}
	}
}

func TestOnsetsFindEachHit(t *testing.T) {
	hit := func(t float64) float64 {
		if t < 0 {
			return 0
		}
		return math.Exp(-t/0.01) * math.Sin(2*math.Pi*1000*t)
	}
	pcm := gen(0.4, func(t float64) float64 { return hit(t) + 0.8*hit(t-0.1) + 0.6*hit(t-0.25) })
	on := New(pcm, sr, 0.4).Onsets()
	want := []float64{0, 0.1, 0.25}
	if len(on) != len(want) {
		t.Fatalf("Onsets = %v, want %v", on, want)
	}
	for i := range want {
		if math.Abs(on[i]-want[i]) > 0.005 {
			t.Fatalf("Onsets = %v, want %v ±5 ms", on, want)
		}
	}
}

// A kick-style exponential glide 150 -> 44 Hz.
func TestPitchTracksAFallingGlide(t *testing.T) {
	freq := func(t float64) float64 { return 44 + 106*math.Exp(-t/0.02) }
	phase := 0.0
	n := int(0.6 * sr)
	pcm := make([]float32, n)
	for i := range pcm {
		pcm[i] = float32(0.9 * math.Sin(phase))
		phase += 2 * math.Pi * freq(float64(i)/sr) / sr
	}
	tr := New(pcm, sr, 0.6).Pitch()
	if tr.HopSec != 0.01 || len(tr.Hz) == 0 {
		t.Fatalf("track shape: hop %v, %d frames", tr.HopSec, len(tr.Hz))
	}
	first := -1
	for i, hz := range tr.Hz {
		if hz > 0 {
			first = i
			break
		}
	}
	if first < 0 || tr.Hz[first] < 60 {
		t.Fatalf("the glide's first voiced frame reads %v Hz, want well above the 44 Hz floor", tr.Hz)
	}
	checked := 0
	for i, hz := range tr.Hz {
		at := tr.T0 + float64(i)*tr.HopSec
		if at < 0.15 || at > 0.5 || hz == 0 {
			continue
		}
		if c := cents(hz, freq(at)); math.Abs(c) > 10 {
			t.Fatalf("at %.3f s the track reads %.2f Hz, true %.2f Hz (%.1f cents)", at, hz, freq(at), c)
		}
		checked++
	}
	if checked < 20 {
		t.Fatalf("only %d voiced frames between 0.15 and 0.5 s", checked)
	}
}

func TestPitchOfASteadyNoteAndVibrato(t *testing.T) {
	saw := func(f, t float64) float64 {
		s := 0.0
		for k := 1; float64(k)*f < 18000; k++ {
			s += math.Sin(2*math.Pi*float64(k)*f*t) / float64(k)
		}
		return 0.4 * s
	}
	tr := New(gen(0.8, func(t float64) float64 { return saw(220, t) }), sr, 0.8).Pitch()
	if m := tr.MedianHz(); math.Abs(cents(m, 220)) > 5 {
		t.Fatalf("saw 220 Hz median pitch %.2f Hz", m)
	}
	// ±30 cents at 5 Hz.
	phase := 0.0
	n := int(1.2 * sr)
	pcm := make([]float32, n)
	for i := range pcm {
		tt := float64(i) / sr
		f := 220 * math.Pow(2, 30*math.Sin(2*math.Pi*5*tt)/1200)
		pcm[i] = float32(0.8 * math.Sin(phase))
		phase += 2 * math.Pi * f / sr
	}
	vt := New(pcm, sr, 1.2).Pitch()
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, hz := range vt.Hz {
		if hz > 0 {
			c := cents(hz, 220)
			lo, hi = math.Min(lo, c), math.Max(hi, c)
		}
	}
	if span := hi - lo; span < 40 || span > 70 {
		t.Fatalf("vibrato ±30 cents tracked as a %.1f-cent span, want ~60", span)
	}
}

func TestSpectrumPeakAndCentroid(t *testing.T) {
	a := New(gen(0.5, func(t float64) float64 { return 0.7 * math.Sin(2*math.Pi*1000*t) }), sr, 0.5)
	sp := a.Spectrum()
	best := 0
	for i := range sp.DB {
		if sp.DB[i] > sp.DB[best] {
			best = i
		}
	}
	if c := cents(sp.Hz[best], 1000); math.Abs(c) > 100 {
		t.Fatalf("spectrum peak at %.0f Hz, want 1000 Hz", sp.Hz[best])
	}
	if sp.DB[best] != 0 {
		t.Fatalf("spectrum is relative to its maximum: peak %.2f dB", sp.DB[best])
	}
	if c := a.CentroidHz(); math.Abs(c-1000) > 30 {
		t.Fatalf("CentroidHz = %.1f, want 1000 ±3%%", c)
	}
	two := New(gen(0.5, func(t float64) float64 {
		return 0.4*math.Sin(2*math.Pi*500*t) + 0.4*math.Sin(2*math.Pi*2000*t)
	}), sr, 0.5)
	if c := two.CentroidHz(); math.Abs(c-1250) > 0.05*1250 {
		t.Fatalf("CentroidHz(500+2000) = %.1f, want 1250 ±5%%", c)
	}
}

func TestHarmonicsOfASaw(t *testing.T) {
	saw := func(t float64) float64 {
		s := 0.0
		for k := 1; float64(k)*110 < 18000; k++ {
			s += math.Sin(2*math.Pi*float64(k)*110*t) / float64(k)
		}
		return 0.4 * s
	}
	h, ok := New(gen(0.8, saw), sr, 0.8).Harmonics()
	if !ok {
		t.Fatal("a saw is pitched: Harmonics must report ok")
	}
	if math.Abs(cents(h.F0, 110)) > 10 {
		t.Fatalf("F0 = %.2f, want 110", h.F0)
	}
	for n := 2; n <= 6; n++ {
		want := -20 * math.Log10(float64(n))
		if got := h.Peaks[n-1] - h.Peaks[0]; math.Abs(got-want) > 1.5 {
			t.Fatalf("harmonic %d at %.2f dB re h1, want %.2f", n, got, want)
		}
	}
	if len(h.Hz) == 0 || len(h.Hz) != len(h.DB) || h.Hz[len(h.Hz)-1] > 12.5*h.F0 {
		t.Fatalf("harmonics curve spans %d points up to %v Hz", len(h.Hz), h.Hz[len(h.Hz)-1])
	}
	rng := rand.New(rand.NewSource(1))
	noise := gen(0.5, func(float64) float64 { return rng.Float64()*2 - 1 })
	if _, ok := New(noise, sr, 0.5).Harmonics(); ok {
		t.Fatal("white noise has no harmonics")
	}
}

func TestNoiseFraction(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	white := func(float64) float64 { return rng.NormFloat64() * 0.2 }
	sine := func(t float64) float64 { return 0.2 * math.Sqrt2 * math.Sin(2*math.Pi*660*t) } // same power as white
	if f := New(gen(0.5, sine), sr, 0.5).Noise().Fraction; f > 0.1 {
		t.Fatalf("pure sine noise fraction %.2f, want < 0.1", f)
	}
	if f := New(gen(0.5, white), sr, 0.5).Noise().Fraction; f < 0.85 {
		t.Fatalf("white noise fraction %.2f, want > 0.85", f)
	}
	mix := New(gen(0.5, func(t float64) float64 { return sine(t) + white(t) }), sr, 0.5).Noise()
	if mix.Fraction < 0.35 || mix.Fraction > 0.65 {
		t.Fatalf("equal-power sine+noise fraction %.2f, want 0.5 ±0.15", mix.Fraction)
	}
	if len(mix.Tonal) == 0 || len(mix.Tonal) != len(mix.Noise) {
		t.Fatalf("noise split shape: %d tonal, %d noise", len(mix.Tonal), len(mix.Noise))
	}
}

func TestSpectrogramTracksAChirp(t *testing.T) {
	// Linear chirp 200 -> 4000 Hz over 1 s.
	pcm := gen(1, func(t float64) float64 { return 0.6 * math.Sin(2*math.Pi*(200*t+0.5*3800*t*t)) })
	sg := New(pcm, sr, 1).Spectrogram()
	if sg.Rows != 64 || sg.Cols == 0 || len(sg.DB) != sg.Rows*sg.Cols {
		t.Fatalf("spectrogram shape %dx%d (%d values)", sg.Cols, sg.Rows, len(sg.DB))
	}
	for c := sg.Cols / 10; c < sg.Cols*9/10; c++ {
		at := sg.T0 + float64(c)*sg.HopSec
		best := 0
		for r := 0; r < sg.Rows; r++ {
			if sg.At(c, r) > sg.At(c, best) {
				best = r
			}
		}
		want := 200 + 3800*at
		if got := sg.RowHz(best); math.Abs(cents(got, want)) > 250 {
			t.Fatalf("column at %.3f s peaks at %.0f Hz, want %.0f Hz", at, got, want)
		}
	}
}

func TestWaveWindowIsPeriodAligned(t *testing.T) {
	w := New(gen(0.5, func(t float64) float64 { return 0.5 * math.Sin(2*math.Pi*220*t+1) }), sr, 0.5).Wave(3)
	if math.Abs(w.PeriodSec-1.0/220) > 0.01/220 {
		t.Fatalf("PeriodSec = %v, want %v", w.PeriodSec, 1.0/220)
	}
	if want := 3 * sr / 220; math.Abs(float64(len(w.Samples)-want)) > 3 {
		t.Fatalf("window holds %d samples, want ~%d", len(w.Samples), want)
	}
	if math.Abs(float64(w.Samples[0])) > 0.03 || w.Samples[1] <= w.Samples[0] {
		t.Fatalf("window must start at a rising zero crossing: %v %v", w.Samples[0], w.Samples[1])
	}
}

func TestSilenceAndEmptyInputAreHarmless(t *testing.T) {
	for _, pcm := range [][]float32{nil, make([]float32, 4800)} {
		a := New(pcm, sr, 0.1)
		if a.AudibleSec() != 0 || a.AttackSec() != 0 || len(a.Onsets()) != 0 || a.CentroidHz() != 0 {
			t.Fatalf("silence: audible %v attack %v onsets %v centroid %v", a.AudibleSec(), a.AttackSec(), a.Onsets(), a.CentroidHz())
		}
		if _, ok := a.Harmonics(); ok {
			t.Fatal("silence has no harmonics")
		}
		_ = a.Envelope()
		_ = a.Pitch()
		_ = a.Spectrum()
		_ = a.Spectrogram()
		_ = a.Noise()
		_ = a.Wave(3)
	}
}

// A bowed or blown note can START with its loudest sample (a bow/reed
// transient, and the render is peak-normalised) and still swell for 300 ms:
// the attack is how long it takes to become LOUD, not to reach its peak sample.
func TestAttackOfASwellWithATransientIsTheSwell(t *testing.T) {
	pcm := gen(1.0, func(t float64) float64 {
		click := 0.0
		if t < 0.001 {
			click = 1.0 // a 1 ms full-scale transient
		}
		g := math.Min(1, t/0.3) // a 300 ms swell
		return click + 0.5*g*math.Sin(2*math.Pi*220*t)
	})
	// Within 1 dB of full on a linear swell: 0.891 × 300 ms.
	want := 0.3 * math.Pow(10, -1.0/20)
	if got := New(pcm, sr, 1).AttackSec(); math.Abs(got-want) > 0.015 {
		t.Fatalf("AttackSec = %.3f, want the swell's %.3f s (a transient sample is not the note being loud)", got, want)
	}
}

// A slow swell from silence never JUMPS 12 dB from one moment to the next,
// but it does start: its first onset is where it first becomes audible.
func TestASwellFromSilenceHasAnOnset(t *testing.T) {
	pcm := gen(1.0, func(t float64) float64 { return 0.5 * math.Min(1, t/0.4) * math.Sin(2*math.Pi*220*t) })
	a := New(pcm, sr, 1)
	if on := a.Onsets(); len(on) != 1 || on[0] > 0.02 {
		t.Fatalf("Onsets = %v, want one onset where the swell becomes audible (≤ 20 ms)", on)
	}
	want := 0.4 * math.Pow(10, -1.0/20)
	if got := a.AttackSec(); math.Abs(got-want) > 0.02 {
		t.Fatalf("AttackSec = %.3f, want %.3f", got, want)
	}
}

// Warming hands the thread back often (the browser runs it on the page's one
// thread): no stretch between two yields is long, and only what was asked
// for is computed.
func TestWarmYieldsOftenAndComputesOnlyWhatIsNeeded(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	pcm := gen(1.5, func(t float64) float64 {
		return math.Exp(-t/0.4)*math.Sin(2*math.Pi*110*t) + 0.05*rng.NormFloat64()
	})
	a := New(pcm, sr, 1.5)
	a.Warm(NeedEnvelope, nil)
	if !a.Has(NeedEnvelope) || a.Has(NeedSTFT) || a.Has(NeedPitch) {
		t.Fatalf("Warm(NeedEnvelope) computed more or less than asked: env %v stft %v pitch %v", a.Has(NeedEnvelope), a.Has(NeedSTFT), a.Has(NeedPitch))
	}
	// Best of five, on the thread's CPU clock: never the time the OS ran
	// something else, yet a long loop with no yield fails every run.
	clock, release := stretchClock()
	defer release()
	best := time.Duration(math.MaxInt64)
	yields := 0
	for run := 0; run < 5; run++ {
		b := New(pcm, sr, 1.5)
		worst := time.Duration(0)
		n := 0
		last := clock()
		b.Warm(NeedAll, func() {
			now := clock()
			if g := now - last; g > worst {
				worst = g
			}
			last = now
			n++
		})
		if !b.Has(NeedAll) {
			t.Fatal("Warm(NeedAll) left something uncomputed")
		}
		if worst < best {
			best = worst
		}
		yields = n
	}
	if yields < 10 {
		t.Fatalf("a 1.5 s note warmed with only %d yields — the long loops do not yield", yields)
	}
	t.Logf("%d yields, longest stretch (best of 5) %v", yields, best)
	if raceEnabled {
		return // the race detector slows these loops several-fold; the budget is for real code
	}
	if best > 10*time.Millisecond {
		t.Fatalf("the longest uninterrupted stretch is %v (budget 10 ms here, 3 ms per frame in the browser)", best)
	}
}

// The short-time transform is the same length at 44.1 and 48 kHz (2048), so
// the browser (48 kHz) and the desktop read the same tone/noise share and
// colour over time for the same sound.
func TestSTFTSizeIsTheSameAtBothShippedRates(t *testing.T) {
	if a, b := stftSize(44100), stftSize(48000); a != 2048 || b != 2048 {
		t.Fatalf("stftSize(44100)=%d stftSize(48000)=%d, want 2048 at both", a, b)
	}
}
