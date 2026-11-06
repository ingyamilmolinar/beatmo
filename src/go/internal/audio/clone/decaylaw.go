package clone

import "math"

// DecayLaw is α(f) = A·(f/1 kHz)^P + B·(f/10 kHz)^6 (spec §2e). Both
// reference frequencies are fixed: a free HF corner is not identifiable (B and
// the corner only ever appear as B/f_c⁶).
type DecayLaw struct {
	A      float64 `json:"a"`
	P      float64 `json:"p"`
	B      float64 `json:"b"`
	RelRMS float64 `json:"rel_rms"` // energy-weighted RMS of (fit/measured − 1)
}

// Eval returns α at frequency f (Np/s).
func (d DecayLaw) Eval(f float64) float64 {
	return d.A*math.Pow(f/1000, d.P) + d.B*math.Pow(f/10000, 6)
}

// FitDecayLaw fits the law across the modes, weighted by energy, minimising
// the RELATIVE error (fast high modes and slow low ones count alike): a grid
// over P ∈ [−1, 3] (step 0.01) with a non-negative linear solve for (A, B) at
// each P. Fewer than three modes cannot pin a shape: a flat law at the
// energy-weighted mean α.
func FitDecayLaw(modes []Mode) DecayLaw {
	if len(modes) == 0 {
		return DecayLaw{}
	}
	w := make([]float64, len(modes))
	var wsum float64
	for i, m := range modes {
		w[i] = m.Energy()
		wsum += w[i]
	}
	if len(modes) < 3 {
		var a float64
		for i, m := range modes {
			a += w[i] * m.Alpha
		}
		return DecayLaw{A: a / wsum}
	}
	best := DecayLaw{RelRMS: math.MaxFloat64}
	for step := 0; step <= 400; step++ {
		p := -1 + 0.01*float64(step)
		var suu, suv, svv, su, sv float64
		for i, m := range modes {
			u := math.Pow(m.Freq/1000, p) / m.Alpha
			v := math.Pow(m.Freq/10000, 6) / m.Alpha
			suu += w[i] * u * u
			suv += w[i] * u * v
			svv += w[i] * v * v
			su += w[i] * u
			sv += w[i] * v
		}
		var cands [][2]float64
		if det := suu*svv - suv*suv; det > 1e-300 {
			a := (su*svv - sv*suv) / det
			b := (sv*suu - su*suv) / det
			if a >= 0 && b >= 0 {
				cands = append(cands, [2]float64{a, b})
			}
		}
		if suu > 0 {
			cands = append(cands, [2]float64{su / suu, 0})
		}
		if svv > 0 {
			cands = append(cands, [2]float64{0, sv / svv})
		}
		for _, c := range cands {
			law := DecayLaw{A: c[0], P: p, B: c[1]}
			var e float64
			for i, m := range modes {
				r := law.Eval(m.Freq)/m.Alpha - 1
				e += w[i] * r * r
			}
			law.RelRMS = math.Sqrt(e / wsum)
			if law.RelRMS < best.RelRMS-1e-12 {
				best = law
			}
		}
	}
	return best
}
