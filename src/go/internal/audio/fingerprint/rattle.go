package fingerprint

import "math"

// TailRattle measures RATTLE: sparse, individually audible ticks in the late
// tail of a percussive sound, in ticks per second summed over three high bands
// (2-4, 4-8, 8-16 kHz).
//
// Why it exists. The organic snares model their wires as a collision train —
// a random sequence of impulses re-striking a damped plate. When the train's
// RATE decays together with its amplitude, the collisions thin out late in the
// tail; below roughly 100 per second the ear stops fusing them into a buzz and
// hears each one: rattle. The listener heard it on every take that carried a
// wire layer, including ones whose averaged spectrum and envelope looked right —
// averaged descriptors cannot see it, because the energy is the same and only
// its distribution in time differs.
//
// How it is measured. Each band's 1 ms RMS envelope (dB) over the late tail —
// from 120 ms after the onset while the band is within 45 dB of the whole
// sound's loudest 1 ms frame (so a band too faint to hear is never judged) —
// is compared with a 21 ms moving average of itself; a frame that pokes more
// than 6 dB above that local average is a tick. A dense buzz, a decaying noise
// band or a ringing mode has a smooth envelope and scores ~0; a sparse click
// train scores tens to hundreds.
//
// Calibration (2026-09-29, 44.1 kHz): real snares score 2-3, the hi-hats and
// every wire-less drum ~0; the shipped wire-layered snare-deep / tom-high-acoustic
// scored ~160-260 before the collision rate was decoupled from the amplitude
// (cym_rattle_hold), and ~0-20 after.
func TailRattle(samples []float64, rate int) float64 {
	if rate <= 0 || len(samples) < rate/5 {
		return 0
	}
	x := trimToOnset(samples)
	if len(x) < rate/5 {
		return 0
	}
	// The tail is measured while each band is within tailRattleFloorDB of the
	// WHOLE sound's loudest 1 ms frame — not its own peak — so a band that is
	// faint relative to the drum (inaudible under it) is never analysed.
	global := peakFrameRMS(x, rate)
	total := 0.0
	for _, b := range [][2]float64{{2000, 4000}, {4000, 8000}, {8000, 16000}} {
		if b[1] >= 0.5*float64(rate) {
			b[1] = 0.45 * float64(rate)
		}
		if b[0] >= b[1] {
			continue
		}
		total += bandTickRate(bandLimit(x, rate, b[0], b[1]), rate, global)
	}
	return total
}

// trimToOnset drops everything before the first sample above 5% of the peak.
func trimToOnset(s []float64) []float64 {
	peak := 0.0
	for _, v := range s {
		if a := math.Abs(v); a > peak {
			peak = a
		}
	}
	if peak == 0 {
		return nil
	}
	for i, v := range s {
		if math.Abs(v) > 0.05*peak {
			return s[i:]
		}
	}
	return nil
}

// bandLimit applies a 4th-order band-pass (two RBJ high-passes at lo, two RBJ
// low-passes at hi).
func bandLimit(s []float64, rate int, lo, hi float64) []float64 {
	out := make([]float64, len(s))
	copy(out, s)
	for _, f := range []struct {
		hz float64
		hp bool
	}{{lo, true}, {lo, true}, {hi, false}, {hi, false}} {
		rbjFilter(out, rate, f.hz, f.hp)
	}
	return out
}

// rbjFilter runs one RBJ biquad (Q = 1/sqrt2) in place.
func rbjFilter(s []float64, rate int, hz float64, highpass bool) {
	w0 := 2 * math.Pi * hz / float64(rate)
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2 * 0.7071067811865476) // Q = 1/sqrt2 (Butterworth)
	var b0, b1, b2 float64
	if highpass {
		b0, b1, b2 = (1+cw)/2, -(1 + cw), (1+cw)/2
	} else {
		b0, b1, b2 = (1-cw)/2, 1-cw, (1-cw)/2
	}
	a0, a1, a2 := 1+alpha, -2*cw, 1-alpha
	b0, b1, b2, a1, a2 = b0/a0, b1/a0, b2/a0, a1/a0, a2/a0
	var x1, x2, y1, y2 float64
	for i, x := range s {
		y := b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2
		x2, x1, y2, y1 = x1, x, y1, y
		s[i] = y
	}
}

// tailRattleFloorDB ends the measured tail where a band falls this far below the
// whole sound's loudest 1 ms frame: roughly where it stops being audible under
// the drum.
const tailRattleFloorDB = -45.0

// bandTickRate is the late-tail tick rate (ticks/s) of one band-limited signal.
func bandTickRate(b []float64, rate int, globalPeak float64) float64 {
	w := rate / 1000
	if w < 1 {
		w = 1
	}
	n := len(b) / w
	if n < 200 {
		return 0
	}
	env := make([]float64, n)
	for i := 0; i < n; i++ {
		var e float64
		for _, v := range b[i*w : (i+1)*w] {
			e += v * v
		}
		env[i] = math.Sqrt(e/float64(w)) + 1e-12
	}
	peak := globalPeak
	db := make([]float64, n)
	last := 0
	for i := range env {
		db[i] = 20 * math.Log10(env[i]/peak)
		if db[i] > tailRattleFloorDB {
			last = i
		}
	}
	const start = 120 // ms
	if last-start < 30 {
		return 0
	}
	seg := db[start:last]
	const k = 21
	ticks := 0
	for i := range seg {
		var sum float64
		for j := i - k/2; j <= i+k/2; j++ {
			jj := j
			if jj < 0 {
				jj = 0
			}
			if jj >= len(seg) {
				jj = len(seg) - 1
			}
			sum += seg[jj]
		}
		if seg[i]-sum/k > 6 {
			ticks++
		}
	}
	return float64(ticks) / (float64(len(seg)) / 1000)
}

// peakFrameRMS is the loudest 1 ms RMS frame of the whole (unfiltered) sound.
func peakFrameRMS(s []float64, rate int) float64 {
	w := rate / 1000
	if w < 1 {
		w = 1
	}
	peak := 1e-12
	for i := 0; i+w <= len(s); i += w {
		var e float64
		for _, v := range s[i : i+w] {
			e += v * v
		}
		if r := math.Sqrt(e / float64(w)); r > peak {
			peak = r
		}
	}
	return peak
}
