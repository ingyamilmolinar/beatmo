package clone

import (
	"math"
	"math/cmplx"
	"math/rand"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/clone/linalg"
)

// BandDiag reports what one analysis band did (for the report).
type BandDiag struct {
	Index   int     `json:"index"`
	LoHz    float64 `json:"lo_hz"`
	HiHz    float64 `json:"hi_hz"`
	Decim   int     `json:"decim"`
	Samples int     `json:"samples"`
	Order   int     `json:"order"`
	Signal  int     `json:"signal_eigs"` // order before the ESTER trim
	NoiseDB float64 `json:"noise_db"`
	Skipped string  `json:"skipped,omitempty"`
	// Poles ESPRIT returned but hygiene rejected, by reason.
	RejGrowing int `json:"rej_growing,omitempty"`
	RejOutside int `json:"rej_outside_core,omitempty"`
	RejLowSNR  int `json:"rej_low_snr,omitempty"`
}

// ModesResult is EstimateModes' output.
type ModesResult struct {
	Modes     []Mode     `json:"modes"`
	Bands     []BandDiag `json:"bands"`
	Transient int        `json:"transient"` // fast / over-peak poles dropped as onset transient
}

const (
	maxOrderPerBand = 24
	minBandSamples  = 12
	modeRelFloorDB  = -60 // drop modes this far below the strongest
	// 40 dB = ln(100) Np of decay between onset and window start.
	maxExtrapolationNp = 4.605170185988092
	// maxModeAlpha: T60 ≥ 20 ms (3·ln 10 / 0.02 Np/s) — faster is a transient.
	maxModeAlpha = 345.38776394910684
)

// Amplitudes back-extrapolated to the onset, sorted by energy. A pole is kept only with α > 0, its frequency in its
// band's CORE (overlapping bands never double-count), SNR ≥ 0 dB at the window start, and within 60 dB of the
// strongest.
func EstimateModes(p Prepared) (ModesResult, error) {
	var res ModesResult
	for _, b := range bandLayout(p.Rate) {
		d := BandDiag{Index: b.Index, LoHz: b.Lo, HiHz: b.Hi, Decim: b.Decim, NoiseDB: -200}
		y, first := baseband(p.Samples, p.Rate, p.Onset, p.WindowStart, p.WindowEnd, b)
		y = trimToBandSignal(y, p.FloorRMS, b, p.Rate)
		d.Samples = len(y)
		if len(y) < minBandSamples {
			d.Skipped = "no signal above the band's noise in the window"
			res.Bands = append(res.Bands, d)
			continue
		}
		er, err := esprit(y, maxOrderPerBand)
		if err != nil {
			d.Skipped = err.Error()
			res.Bands = append(res.Bands, d)
			continue
		}
		d.Order, d.Signal, d.NoiseDB = er.Order, er.Signal, 10*math.Log10(math.Max(er.NoiseVar, 1e-20))
		fsd := float64(p.Rate) / float64(b.Decim)
		t0 := float64(first-p.Onset) / float64(p.Rate)
		for k, z := range er.Poles {
			if z == 0 {
				continue
			}
			s := cmplx.Log(z) * complex(fsd, 0) // continuous-time baseband pole
			alpha := -real(s)
			f := b.Center + imag(s)/(2*math.Pi)
			if f < b.Lo || f >= b.Hi {
				d.RejOutside++
				continue
			}
			if alpha <= 0 {
				d.RejGrowing++
				continue
			}
			snr := 10 * math.Log10(cmplxAbs2(er.Amps[k])/math.Max(er.NoiseVar, 1e-30))
			if snr < 0 {
				d.RejLowSNR++
				continue
			}
			a := 2 * er.Amps[k] / (b.gain(s, p.Rate) * cmplx.Exp(s*complex(t0, 0)))
			res.Modes = append(res.Modes, Mode{
				Freq: f, Alpha: alpha, Amp: cmplx.Abs(a), Phase: cmplx.Phase(a),
				Band: b.Index, SNRdB: snr,
			})
		}
		res.Bands = append(res.Bands, d)
	}
	// A mode that fell more than 40 dB before the window opened belongs to the strike transient, and its onset amplitude
	// would be an extrapolation of noise.
	tws := float64(p.WindowStart-p.Onset) / float64(p.Rate)
	observable := res.Modes[:0]
	for _, m := range res.Modes {
		if m.Alpha*tws <= maxExtrapolationNp {
			observable = append(observable, m)
		}
	}
	res.Modes = refitAmplitudes(p, observable)
	// A "mode" ringing out in under 20 ms or louder than the recording's peak is not a resonance: ESPRIT fits an onset
	// burst with clusters of fast cancelling poles. They belong to the residual; drop and refit until none is left.
	for iter := 0; iter < 4; iter++ {
		clean := make([]Mode, 0, len(res.Modes))
		for _, m := range res.Modes {
			if m.Alpha <= maxModeAlpha && m.Amp <= p.Peak {
				clean = append(clean, m)
			}
		}
		if len(clean) == len(res.Modes) {
			break
		}
		res.Transient += len(res.Modes) - len(clean)
		res.Modes = refitAmplitudes(p, clean)
	}
	var strongest float64
	for _, m := range res.Modes {
		strongest = math.Max(strongest, m.Amp)
	}
	floor := strongest * math.Pow(10, modeRelFloorDB/20.0)
	kept := res.Modes[:0]
	for _, m := range res.Modes {
		if m.Amp >= floor {
			kept = append(kept, m)
		}
	}
	res.Modes = kept
	sort.SliceStable(res.Modes, func(i, j int) bool { return res.Modes[i].Energy() > res.Modes[j].Energy() })
	return res, nil
}

// Cut after the last point whose smoothed power is 6 dB above the band's share of the noise floor (white noise of
// RMS σ, heterodyned to two-sided width 2w, carries σ²·2w/rate). nil for a band that never rises above it.
func trimToBandSignal(y []complex128, floorRMS float64, b band, rate int) []complex128 {
	if len(y) == 0 {
		return nil
	}
	const win = 8
	smooth := make([]float64, len(y))
	var acc float64
	quietest := math.Inf(1)
	for m := range y {
		acc += cmplxAbs2(y[m])
		if m >= win {
			acc -= cmplxAbs2(y[m-win])
		}
		smooth[m] = acc / float64(min(m+1, win))
		if m >= win-1 && smooth[m] < quietest {
			quietest = smooth[m]
		}
	}
	// The lower of the band's floor share and its own quietest moment: either can read too high, and too high a noise
	// skips weak bands.
	noise := floorRMS * floorRMS * 2 * (b.Hi - b.Lo) / float64(rate)
	if quietest < noise {
		noise = quietest
	}
	thr := 4 * noise
	last := -1
	for m, v := range smooth {
		if v > thr {
			last = m
		}
	}
	if last < 0 {
		return nil
	}
	return y[:last+1]
}

func cmplxAbs2(z complex128) float64 { return real(z)*real(z) + imag(z)*imag(z) }

const (
	refitRows     = 4096
	refitMaxModes = 128 // the modal layer's table capacity (spec §3b)
	refitDenseSec = 0.2
	// Singular values below 1e-4 of the largest.
	refitRelTol = 1e-8
)

// ONE joint least-squares fit from the onset: per-band amplitudes blow up extrapolated back to it. TRUNCATED
// minimum-norm, since nearly collinear fast modes otherwise take huge cancelling amplitudes; rows are a seeded jittered
// sample so no two modes alias onto one column.
func refitAmplitudes(p Prepared, modes []Mode) []Mode {
	if len(modes) == 0 {
		return modes
	}
	sort.SliceStable(modes, func(i, j int) bool { return modes[i].Energy() > modes[j].Energy() })
	if len(modes) > refitMaxModes {
		modes = modes[:refitMaxModes]
	}
	// Every sample of the first refitDenseSec (where nearly all energy is; a sparse stride cannot pin high modes
	// between rows), then a seeded jittered sample of the rest. Amplitudes must hold FROM THE ONSET, where
	// resynthesis starts.
	k2 := 2 * len(modes)
	gr := make([]float64, k2*k2) // the real normal matrix (upper triangle)
	h := make([]float64, k2)
	row := make([]float64, k2)
	addRow := func(si int) {
		t := float64(si-p.Onset) / float64(p.Rate)
		for k, m := range modes {
			env := math.Exp(-m.Alpha * t)
			sn, cs := math.Sincos(2 * math.Pi * m.Freq * t)
			row[2*k], row[2*k+1] = env*cs, env*sn
		}
		b := p.Samples[si]
		for a := 0; a < k2; a++ {
			ra := row[a]
			if ra == 0 {
				continue
			}
			h[a] += ra * b
			grow := gr[a*k2 : a*k2+k2]
			for c := a; c < k2; c++ {
				grow[c] += ra * row[c]
			}
		}
	}
	dense := min(p.WindowEnd, p.Onset+int(refitDenseSec*float64(p.Rate)))
	for si := p.Onset; si < dense; si++ {
		addRow(si)
	}
	if n := p.WindowEnd - dense; n > 0 {
		rows := min(refitRows, n)
		r := rand.New(rand.NewSource(1))
		for i := 0; i < rows; i++ {
			lo := dense + i*n/rows
			hi := dense + (i+1)*n/rows
			addRow(lo + r.Intn(max(1, hi-lo)))
		}
	}
	g := linalg.New(k2, k2)
	for a := 0; a < k2; a++ {
		for c := a; c < k2; c++ {
			v := complex(gr[a*k2+c], 0)
			g.Data[a*k2+c] = v
			g.Data[c*k2+a] = v
		}
	}
	vals, vecs := linalg.EigHermitian(g)
	x := make([]float64, k2)
	if len(vals) > 0 && vals[0] > 0 {
		tol := vals[0] * refitRelTol
		for e, lam := range vals {
			if lam <= tol {
				break
			}
			var dot complex128
			for a := 0; a < k2; a++ {
				dot += cmplx.Conj(vecs.At(a, e)) * complex(h[a], 0)
			}
			coef := dot / complex(lam, 0)
			for a := 0; a < k2; a++ {
				x[a] += real(vecs.At(a, e) * coef)
			}
		}
	}
	out := modes[:0]
	for k, m := range modes {
		A, B := x[2*k], x[2*k+1]
		m.Amp = math.Hypot(A, B)
		m.Phase = math.Atan2(-B, A)
		out = append(out, m)
	}
	return out
}
