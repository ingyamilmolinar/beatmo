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
	// maxExtrapolationNp: 40 dB = ln(100) Np of decay between the onset and
	// the window start.
	maxExtrapolationNp = 4.605170185988092
	// maxModeAlpha: T60 ≥ 20 ms (3·ln 10 / 0.02 Np/s) — faster is a transient.
	maxModeAlpha = 345.38776394910684
)

// EstimateModes runs sub-band ESPRIT over p's decay window (spec §2b) and
// returns the modes with their amplitudes back-extrapolated to the onset,
// sorted by descending energy. Pole hygiene: α > 0, frequency inside the
// band's CORE (so overlapping bands never double-count), SNR at the window
// start ≥ 0 dB, amplitude within 60 dB of the strongest mode.
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
	// A mode that had to fall more than 40 dB before the window opened was
	// essentially over before analysis began: it belongs to the strike
	// transient (the residual), and its onset amplitude would be a 40 dB+
	// extrapolation of noise.
	tws := float64(p.WindowStart-p.Onset) / float64(p.Rate)
	observable := res.Modes[:0]
	for _, m := range res.Modes {
		if m.Alpha*tws <= maxExtrapolationNp {
			observable = append(observable, m)
		}
	}
	res.Modes = refitAmplitudes(p, observable)
	// Transient hygiene: a "mode" that rings out in under 20 ms, or whose
	// amplitude exceeds the recording's own peak, is not a resonance — ESPRIT
	// fits an onset noise burst with clusters of fast poles whose amplitudes
	// cancel (a WAV-clone round trip measured eight of them at up to 8× full
	// scale around a −30 dB noise band). They belong to the residual (noise
	// rows); drop them and refit until none is left.
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

// trimToBandSignal cuts y after the last point where its smoothed power is
// still 6 dB above the band's share of the recording's noise floor (white
// noise of RMS σ heterodyned and low-passed to two-sided width 2w carries
// σ²·2w/rate). A band that never rises above it returns nil.
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
	// The band's noise is the lower of its share of the recording floor and
	// its own quietest moment: either can be read too high (a floor taken
	// from a ring; a band that rings throughout), and too high a noise skips
	// weak bands outright.
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
	// refitRelTol drops normal-matrix eigen-directions below this fraction of
	// the largest (singular values below 1e-4 of the largest).
	refitRelTol = 1e-8
)

// refitAmplitudes replaces every mode's amplitude and phase with ONE joint
// least-squares fit of the full-band recording from the onset, keeping
// ESPRIT's frequencies and decay rates:
//
//	x(t) ≈ Σ_k e^{−α_k·t}·(A_k·cos ω_k t + B_k·sin ω_k t),  t from the onset.
//
// The per-band Vandermonde amplitudes are only valid INSIDE each band's
// window, which starts a long filter half-length after the onset in the low
// bands; extrapolated back to the onset they blew up — a spurious 32 Hz
// "mode" in VCSL's marimba came back +48 dB, and clustered poles (cowbell's
// 464–491 Hz group, a tom's pitch glide) cancel each other inside the window
// and stop cancelling outside it. A joint fit sees the early signal.
//
// The solve is the TRUNCATED minimum-norm one: fast modes at nearby
// frequencies are nearly collinear over the window, and a plain least-squares
// gives them huge cancelling amplitudes (measured: residuals of 10⁶ % on the
// corpus). The normal matrix is eigendecomposed and directions below
// refitRelTol of the largest — exactly those cancelling combinations — are
// dropped. Rows are a seeded jittered sample of times (deterministic; no two
// modes alias onto one column the way a uniform stride could).
func refitAmplitudes(p Prepared, modes []Mode) []Mode {
	if len(modes) == 0 {
		return modes
	}
	sort.SliceStable(modes, func(i, j int) bool { return modes[i].Energy() > modes[j].Energy() })
	if len(modes) > refitMaxModes {
		modes = modes[:refitMaxModes]
	}
	// Rows span [onset, window end): frequencies and decays come from the
	// settled window, but the amplitudes must hold FROM THE ONSET, where the
	// resynthesis starts — fitted on the window alone, fast modes were free
	// before it and blew up there.
	// Rows: EVERY sample of the first refitDenseSec after the onset (where
	// nearly all the energy is — a sparse sample there leaves the high modes
	// unconstrained between rows: one row per 16 samples cannot pin a mode
	// above ~1.4 kHz, and the fit blew up between them), then a seeded
	// jittered sample of the rest of the window. The amplitudes must hold FROM
	// THE ONSET, where the resynthesis starts; frequencies and decays still
	// come from the settled window.
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
