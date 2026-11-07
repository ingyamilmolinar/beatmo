//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The anti-watering-down net for the modal snare, measured from its reference. The plateau matters most: the
// reference's fundamental loses 0.4 dB from 200 to 400 ms; a single-exponential mode sounds right for 150 ms and
// hollow after, which a "simplify the envelope" change would bring back.
func TestSnareAcousticIsRingingModalSnare(t *testing.T) {
	buf, sr := RenderInstrumentOneShotRaw(roster.SnareAcoustic)
	if len(buf) == 0 {
		t.Fatal("snare-acoustic playback produced no samples")
	}
	x := make([]float64, len(buf))
	peak := 0.0
	for i, v := range buf {
		x[i] = float64(v)
		if a := math.Abs(x[i]); a > peak {
			peak = a
		}
	}
	if peak < 0.1 {
		t.Fatalf("snare-acoustic too quiet: peak=%.4f", peak)
	}

	// Narrowband complex demodulation, as the design's measurements used.
	modeEnv := func(fq float64) []float64 {
		w := int(float64(sr) / 25.0) // ~25 Hz analysis bandwidth
		if w < 8 || w > len(x) {
			t.Fatalf("bad analysis window %d for %d samples", w, len(x))
		}
		win := make([]float64, w)
		sum := 0.0
		for i := range win {
			win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(w-1))
			sum += win[i]
		}
		out := make([]float64, len(x))
		for n := 0; n+w <= len(x); n++ {
			var re, im float64
			for i := 0; i < w; i++ {
				tt := float64(n+i) / float64(sr)
				c := math.Cos(2 * math.Pi * fq * tt)
				s := math.Sin(2 * math.Pi * fq * tt)
				re += x[n+i] * c * win[i]
				im -= x[n+i] * s * win[i]
			}
			out[n+w/2] = 2 * math.Hypot(re, im) / sum
		}
		return out
	}
	at := func(e []float64, ms int) float64 {
		i := ms * sr / 1000
		if i >= len(e) {
			return 0
		}
		return e[i]
	}

	const f0 = 225.0
	e1 := modeEnv(f0)
	p1 := 0.0
	for _, v := range e1 {
		if v > p1 {
			p1 = v
		}
	}
	if p1 <= 0 {
		t.Fatal("no energy at the 225 Hz fundamental")
	}
	db := func(v float64) float64 { return 20 * math.Log10(v/p1+1e-12) }

	// 1. The fundamental is struck, not swelled: it peaks in the first 30 ms.
	iPeak := 0
	for i, v := range e1 {
		if v == p1 {
			iPeak = i
			break
		}
	}
	if ms := iPeak * 1000 / sr; ms > 30 {
		t.Errorf("fundamental peaks at %d ms, want <=30 ms (struck, no swell)", ms)
	}

	// Without the fast first stage the snare reads as a tom.
	if d := db(at(e1, 100)); d > -14 {
		t.Errorf("fundamental at 100ms = %.1f dB, want <=-14 dB (fast struck-head stage)", d)
	}

	// A single exponential drops 15-20 dB here.
	d200, d400 := db(at(e1, 200)), db(at(e1, 400))
	if drop := d200 - d400; drop > 8 {
		t.Errorf("fundamental fell %.1f dB from 200ms (%.1f) to 400ms (%.1f); want <=8 dB "+
			"— the two-stage shell/air term is gone, which is the whole instrument", drop, d200, d400)
	}

	// Or the voice rings past its own buffer.
	if d := db(at(e1, 1200)); d > -40 {
		t.Errorf("fundamental at 1200ms = %.1f dB, want <=-40 dB (hum must collapse)", d)
	}

	// Every upper mode audible at 300 ms, none a harmonic of f0.
	for _, m := range []struct {
		fq    float64
		ratio float64
	}{{355, 1.57}, {520, 2.31}, {673, 2.99}, {838, 3.72}} {
		e := modeEnv(m.fq)
		pk := 0.0
		for _, v := range e {
			if v > pk {
				pk = v
			}
		}
		if pk <= 0 {
			t.Errorf("no energy at mode %.0f Hz", m.fq)
			continue
		}
		// Relative to the fundamental's peak: the reference sits at 0.24-0.29.
		if rel := pk / p1; rel < 0.08 || rel > 0.60 {
			t.Errorf("mode %.0f Hz peak is %.2f of the fundamental, want 0.08-0.60", m.fq, rel)
		}
		if 20*math.Log10(at(e, 300)/pk+1e-12) < -30 {
			t.Errorf("mode %.0f Hz already gone at 300ms; the ring must outlive the thump", m.fq)
		}
	}

	// A snare without wires is a tom; wires that never die are a cymbal.
	band := func(a, b float64, lo, hi float64) float64 {
		i0, i1 := int(a*float64(sr)), int(b*float64(sr))
		if i1 > len(x) {
			i1 = len(x)
		}
		if i1-i0 < 64 {
			return 0
		}
		mag, binHz := wave.MagnitudeSpectrum(
			wave.Wave{Samples: x[i0:i1], SampleRate: sr}, 1<<15, wave.WindowHann)
		var tot, sel float64
		for k, m := range mag {
			p := m * m
			tot += p
			if f := float64(k) * binHz; f >= lo && f < hi {
				sel += p
			}
		}
		if tot == 0 {
			return 0
		}
		return sel / tot
	}
	if hf := band(0, 0.06, 2000, 8000); hf < 0.02 {
		t.Errorf("attack has %.1f%% of energy in 2-8 kHz, want >=2%% (the wires)", hf*100)
	}
	if hf := band(0.4, 0.9, 2000, 8000); hf > 0.10 {
		t.Errorf("400-900ms still has %.1f%% in 2-8 kHz, want <=10%% (wires must die fast)", hf*100)
	}
}
