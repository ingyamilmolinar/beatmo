package clone

import (
	"math"
	"sort"
)

// Level, attack, and a two-rate decay A·(mix·e^{−fast·t} + (1−mix)·e^{−tail·t}) after the peak: a MEASUREMENT of the
// residual, never the residual itself.
type NoiseBand struct {
	CenterHz  float64 `json:"center_hz"`
	Q         float64 `json:"q"`
	LevelDB   float64 `json:"level_db"`
	AttackSec float64 `json:"attack_s"`
	FastRate  float64 `json:"fast_rate"`
	FastMix   float64 `json:"fast_mix"`
	TailRate  float64 `json:"tail_rate"`
	Active    bool    `json:"active"`
}

// Residual summarises reference − modal resynthesis.
type Residual struct {
	EnergyFraction float64     `json:"energy_fraction"` // residual / reference energy over the window
	ClickLevelDB   float64     `json:"click_level_db"`  // the same ratio over the contact, dB
	Bands          []NoiseBand `json:"bands"`
}

const (
	thirdOctaveQ   = 4.318 // 1 / (2^(1/6) − 2^(−1/6))
	residualHopSec = 0.004
)

// Third-octave centres 31.5·2^(k/3) Hz below 0.45·rate; clickSec is the contact length the click level is read over.
func MeasureResidual(p Prepared, modes []Mode, clickSec float64) Residual {
	ref := p.Samples[p.Onset:p.WindowEnd]
	syn := Synthesize(modes, p.Rate, len(ref))
	res := make([]float64, len(ref))
	var eRef, eRes float64
	for i, v := range ref {
		res[i] = v - syn[i]
		eRef += v * v
		eRes += res[i] * res[i]
	}
	out := Residual{EnergyFraction: eRes / math.Max(eRef, 1e-30)}
	nc := min(len(ref), max(1, int(clickSec*float64(p.Rate))))
	out.ClickLevelDB = dB(rms(res[:nc])) - dB(rms(ref[:nc]))
	for k := 0; ; k++ {
		fc := 31.5 * math.Pow(2, float64(k)/3)
		if fc*math.Pow(2, 1.0/6) >= 0.45*float64(p.Rate) {
			break
		}
		out.Bands = append(out.Bands, measureBand(bandpass(res, p.Rate, fc, thirdOctaveQ), p.Rate, fc))
	}
	return out
}

func measureBand(x []float64, rate int, fc float64) NoiseBand {
	nb := NoiseBand{CenterHz: fc, Q: thirdOctaveQ, LevelDB: -200}
	hop := max(1, int(residualHopSec*float64(rate)))
	var env []float64
	for a := 0; a+hop <= len(x); a += hop {
		env = append(env, rms(x[a:a+hop]))
	}
	if len(env) < 4 {
		return nb
	}
	tail := append([]float64(nil), env[len(env)*4/5:]...)
	sort.Float64s(tail)
	floor := tail[len(tail)/2]
	// Band-limited noise fluctuates several dB per frame, so the raw maximum lands on a fluctuation.
	ip := 0
	sm := func(i int) float64 {
		lo, hi := max(0, i-1), min(len(env)-1, i+1)
		var s float64
		for k := lo; k <= hi; k++ {
			s += env[k]
		}
		return s / float64(hi-lo+1)
	}
	for i := range env {
		if sm(i) > sm(ip) {
			ip = i
		}
	}
	peak := sm(ip)
	if peak <= 2*floor || peak == 0 {
		return nb
	}
	// The first frame within 3 dB of the peak: a later jitter maximum must not swallow the fast decay.
	for i := 0; i < ip; i++ {
		if sm(i) >= peak/math.Sqrt2 {
			ip = i
			break
		}
	}
	last := ip
	for i := ip; i < len(env); i++ {
		if env[i] > 2*floor {
			last = i
		}
	}
	dt := residualHopSec
	nb.Active = true
	nb.AttackSec = float64(ip) * dt
	if last-ip < 6 {
		// Too short for a two-rate fit, but it DOES end: a rate of 0 would never decay.
		nb.LevelDB = dB(peak)
		nb.TailRate = math.Log(peak/math.Max(floor, peak*1e-3)) / (float64(last-ip+1) * residualHopSec)
		nb.FastRate = nb.TailRate
		return nb
	}
	// Tail rate: log-linear fit over the second half of [peak, last].
	mid := ip + (last-ip)/2
	s2, _ := logLine(env, ip, mid, last, dt)
	nb.TailRate = math.Max(-s2, 0)
	a1, a2, r1 := fitTwoRate(env[ip:last+1], dt, nb.TailRate)
	nb.FastRate = r1
	if a1+a2 > 0 {
		nb.FastMix = a1 / (a1 + a2)
	}
	if nb.FastMix == 0 {
		nb.FastRate = nb.TailRate
	}
	nb.LevelDB = dB(a1 + a2)
	return nb
}

// RELATIVE error, since band-noise jitter is multiplicative; IRLS weights come from the MODEL, because 1/env over-trusts
// frames that dipped low and biases the fast rate up. A pure tail (a1 = 0) competes too.
func fitTwoRate(env []float64, dt, r2 float64) (a1, a2, r1 float64) {
	n := len(env)
	e2s := make([]float64, n)
	for k := range e2s {
		e2s[k] = math.Exp(-r2 * float64(k) * dt)
	}
	e1s := make([]float64, n)
	w := make([]float64, n)
	solve := func(oneTerm bool) (float64, float64) {
		var s11, s12, s22, s1, s2 float64
		for k, v := range env {
			ww := w[k] * w[k]
			s11 += ww * e1s[k] * e1s[k]
			s12 += ww * e1s[k] * e2s[k]
			s22 += ww * e2s[k] * e2s[k]
			s1 += ww * e1s[k] * v
			s2 += ww * e2s[k] * v
		}
		if oneTerm || s11*s22-s12*s12 <= 1e-300*s11*s22 {
			if s22 == 0 {
				return 0, 0
			}
			return 0, math.Max(s2/s22, 0)
		}
		det := s11*s22 - s12*s12
		x1 := (s1*s22 - s2*s12) / det
		x2 := (s2*s11 - s1*s12) / det
		if x1 < 0 {
			return 0, math.Max(s2/s22, 0)
		}
		if x2 < 0 {
			return math.Max(s1/s11, 0), 0
		}
		return x1, x2
	}
	fit := func(r float64, oneTerm bool) (c, x1, x2 float64) {
		for k := range e1s {
			e1s[k] = math.Exp(-r * float64(k) * dt)
		}
		for k, v := range env {
			w[k] = 0
			if v > 0 {
				w[k] = 1 / v
			}
		}
		for it := 0; it < 4; it++ {
			x1, x2 = solve(oneTerm)
			if it == 3 {
				break
			}
			for k := range w {
				if m := x1*e1s[k] + x2*e2s[k]; m > 0 {
					w[k] = 1 / m
				}
			}
		}
		for k, v := range env {
			m := x1*e1s[k] + x2*e2s[k]
			if m <= 0 {
				continue
			}
			d := (m - v) / m
			c += d * d
		}
		return c, x1, x2
	}
	// The value at t = 0 must stay near the measured onset: one frame's relative cost is bounded by 1 however far the model
	// overshoots, so an instant spike far above the peak would otherwise win.
	peak0 := 0.0
	for _, v := range env[:min(3, n)] {
		peak0 = math.Max(peak0, v)
	}
	ok := func(x1, x2 float64) bool { return x1+x2 <= 2*peak0 }
	best, a1, a2 := fit(r2, true)
	if !ok(a1, a2) {
		best, a1, a2 = math.Inf(1), 0, peak0
	}
	r1 = r2
	lo := math.Max(r2*1.05, 1)
	for i := 0; i <= 200; i++ {
		r := lo * math.Pow(2000/lo, float64(i)/200)
		if c, x1, x2 := fit(r, false); c < best-1e-12 && ok(x1, x2) {
			best, a1, a2, r1 = c, x1, x2, r
		}
	}
	return a1, a2, r1
}

// logLine fits ln(v[i]) ≈ c + s·(i−origin)·dt over i ∈ [lo, hi] where v > 0.
func logLine(v []float64, origin, lo, hi int, dt float64) (s, c float64) {
	var u, r, w []float64
	for i := lo; i <= hi; i++ {
		if v[i] > 0 {
			u = append(u, float64(i-origin)*dt)
			r = append(r, math.Log(v[i]))
			w = append(w, 1)
		}
	}
	c, s, _ = weightedLine(u, r, w)
	return s, c
}

// bandpass is an RBJ constant-0-dB-peak band-pass, run forward.
func bandpass(x []float64, rate int, fc, q float64) []float64 {
	w0 := 2 * math.Pi * fc / float64(rate)
	alpha := math.Sin(w0) / (2 * q)
	a0 := 1 + alpha
	b0, b2 := alpha/a0, -alpha/a0
	a1, a2 := -2*math.Cos(w0)/a0, (1-alpha)/a0
	out := make([]float64, len(x))
	var x1, x2, y1, y2 float64
	for i, v := range x {
		y := b0*v + b2*x2 - a1*y1 - a2*y2
		x2, x1 = x1, v
		y2, y1 = y1, y
		out[i] = y
	}
	return out
}
