package fingerprint

import "math"

// Sparse, individually audible ticks in a tail (ticks/s over 2–16 kHz), which averaged descriptors cannot see: a 1 ms
// envelope frame 6 dB over its 21 ms moving average is a tick, so a buzz scores ~0 and a thinning wire train tens to
// hundreds.
func TailRattle(samples []float64, rate int) float64 {
	if rate <= 0 || len(samples) < rate/5 {
		return 0
	}
	x := trimToOnset(samples)
	if len(x) < rate/5 {
		return 0
	}
	// Against the WHOLE sound's loudest frame, not the band's own, so a band inaudible under the drum is never judged.
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

// Two RBJ high-passes at lo and two low-passes at hi.
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

// Roughly where a band stops being audible under the drum.
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
