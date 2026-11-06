package clone

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// loss.go — how far a candidate render is from the reference (spec §4). Four
// terms, each reported on its own: a loss RANKS candidates; the ear accepts
// them. Weights start equal; `refs validate -loss` reports which term agrees
// with the listener within a family.

// LossWeights weigh the terms in Total.
type LossWeights struct {
	MRSTFT      float64 `json:"mrstft"`
	Envelope    float64 `json:"envelope"`
	Descriptors float64 `json:"descriptors"`
	Prior       float64 `json:"prior"`
}

// DefaultLossWeights are equal (spec §4b): nothing has the data to rank them yet.
var DefaultLossWeights = LossWeights{MRSTFT: 1, Envelope: 1, Descriptors: 1, Prior: 1}

// LossReport is one candidate's score. MRSTFT = mean over the three
// resolutions of LogMagDB/10 + SC (two numbers of order one); Envelope is a
// mean |dB|; Descriptors the mean registry Norm (≥ 1 = past mismatch); Prior a
// mean |dB| of amplitude and decay against the analysis. Total = Σ wₖ·termₖ/scaleₖ.
type LossReport struct {
	MRSTFT      float64            `json:"mrstft"`
	LogMagDB    float64            `json:"log_mag_db"`
	SC          float64            `json:"spectral_convergence"`
	Envelope    float64            `json:"envelope_db"`
	Descriptors float64            `json:"descriptors"`
	Prior       float64            `json:"prior_db"`
	Total       float64            `json:"total"`
	Desc        map[string]float64 `json:"desc,omitempty"`
}

// TrustedDescriptors are the registry descriptors the docs trust for struck
// sounds (spec §4b).
var TrustedDescriptors = []string{fingerprint.IDInharmonicity, fingerprint.IDTristimulus, fingerprint.IDTrackCrestMedian}

var stftSizes = []int{256, 1024, 4096}

const (
	lossFloorDB    = -80
	envFloorDB     = -60
	envFrameSec    = 0.002
	earlyWeightSec = 0.100
	alignRelLevel  = 0.01 // −40 dB re peak: the onset rule applied to BOTH sides
)

type refSpec struct {
	n       int
	floor   float64 // linear magnitude floor (−80 dB re the reference max)
	floorDB float64
	frames  [][]float64
	db      [][]float64
	weights []float64
	energy  float64 // Σ w·|S|²
}

// LossRef is a reference prepared for scoring; read-only, safe for
// concurrent Score calls.
type LossRef struct {
	Rate  int
	x     []float64 // aligned at the onset, peak-normalised to 1
	specs []refSpec
	env   []float64 // dB, clamped at −60 dB re its maximum
	desc  map[string]fingerprint.Measurement
	// descZero is each descriptor's Norm of the reference against ITSELF: a
	// ratio-kind comparator does not read 0 at identity (track.crest.median
	// reports Norm 0.68 at 1.00×), so the term is the excess over it.
	descZero map[string]float64
}

// alignStart is the first sample at or above −40 dB re the peak, searched
// from `from`.
func alignStart(x []float64, from int) int {
	peak := 0.0
	for _, v := range x[from:] {
		peak = math.Max(peak, math.Abs(v))
	}
	for i := from; i < len(x); i++ {
		if math.Abs(x[i]) >= alignRelLevel*peak {
			return i
		}
	}
	return from
}

func peakAbs(x []float64) float64 {
	p := 0.0
	for _, v := range x {
		p = math.Max(p, math.Abs(v))
	}
	return p
}

// NewLossRef aligns the reference at its onset (the −40 dB rule, searched
// from 20 ms before Prepare's onset) and keeps at most maxSec of it.
func NewLossRef(p Prepared, maxSec float64) *LossRef {
	from := max(0, p.Onset-int(0.02*float64(p.Rate)))
	s := alignStart(p.Samples, from)
	end := len(p.Samples)
	if maxSec > 0 {
		end = min(end, s+int(maxSec*float64(p.Rate)))
	}
	x := append([]float64(nil), p.Samples[s:end]...)
	if pk := peakAbs(x); pk > 0 {
		for i := range x {
			x[i] /= pk
		}
	}
	r := &LossRef{Rate: p.Rate, x: x}
	for _, n := range stftSizes {
		frames := spectrogram(x, n)
		mx := 0.0
		for _, f := range frames {
			mx = math.Max(mx, peakAbs(f))
		}
		sp := refSpec{n: n, frames: frames, floor: mx * math.Pow(10, lossFloorDB/20.0)}
		sp.floorDB = 20 * math.Log10(math.Max(sp.floor, 1e-300))
		hopSec := float64(n/4) / float64(p.Rate)
		for i, f := range frames {
			w := 1.0
			if float64(i)*hopSec < earlyWeightSec {
				w = 2
			}
			sp.weights = append(sp.weights, w)
			row := make([]float64, len(f))
			for k, v := range f {
				row[k] = 20 * math.Log10(math.Max(v, sp.floor))
				sp.energy += w * v * v
			}
			sp.db = append(sp.db, row)
		}
		r.specs = append(r.specs, sp)
	}
	r.env = envelopeDB(x, p.Rate)
	floor := maxOf(r.env) + envFloorDB
	for i := range r.env {
		r.env[i] = math.Max(r.env[i], floor)
	}
	r.desc = measureTrusted(x, p.Rate)
	r.descZero = map[string]float64{}
	for id, m := range r.desc {
		if d, ok := fingerprint.DefaultRegistry().ByID(id); ok {
			r.descZero[id] = d.CompareWith(m, m).Norm
		}
	}
	return r
}

func maxOf(x []float64) float64 {
	m := math.Inf(-1)
	for _, v := range x {
		m = math.Max(m, v)
	}
	return m
}

// envelopeDB is the 2 ms RMS envelope in dB.
func envelopeDB(x []float64, rate int) []float64 {
	n := max(1, int(envFrameSec*float64(rate)))
	var out []float64
	for a := 0; a < len(x); a += n {
		out = append(out, dB(rms(x[a:min(len(x), a+n)])))
	}
	return out
}

func measureTrusted(x []float64, rate int) map[string]fingerprint.Measurement {
	b := fingerprint.NewBundle(wave.Wave{Samples: x, SampleRate: rate}, "clone")
	reg := fingerprint.DefaultRegistry()
	out := map[string]fingerprint.Measurement{}
	for _, id := range TrustedDescriptors {
		d, ok := reg.ByID(id)
		if !ok || (d.Applies != nil && !d.Applies(b)) {
			continue
		}
		out[id] = d.Extract(b)
	}
	return out
}

// alignCandidate aligns cand by the same onset rule, trims / zero-pads it to
// the reference length and matches its PEAK to the reference's (1).
func (r *LossRef) alignCandidate(cand []float64) []float64 {
	out := make([]float64, len(r.x))
	if len(cand) == 0 {
		return out
	}
	s := alignStart(cand, 0)
	copy(out, cand[s:])
	if pk := peakAbs(out); pk > 0 {
		for i := range out {
			out[i] /= pk
		}
	}
	return out
}

// Score computes every term for one candidate render (at the reference's
// rate). prior is PriorTerm's value (computed by the caller, which holds the
// document); scale normalises each term (0 ⇒ 1).
func (r *LossRef) Score(cand []float64, prior float64, w LossWeights, scale LossReport) LossReport {
	y := r.alignCandidate(cand)
	var rep LossReport
	for _, sp := range r.specs {
		var l1, wsum, diff2 float64
		eachFrame(y, sp.n, func(i int, m []float64) {
			if i >= len(sp.frames) {
				return
			}
			wt := sp.weights[i]
			ref, rdb := sp.frames[i], sp.db[i]
			for k, v := range m {
				cdb := sp.floorDB
				if v > sp.floor {
					cdb = 20 * math.Log10(v)
				}
				l1 += wt * math.Abs(cdb-rdb[k])
				d := v - ref[k]
				diff2 += wt * d * d
			}
			wsum += wt * float64(len(m))
		})
		lm := l1 / math.Max(wsum, 1)
		sc := math.Sqrt(diff2 / math.Max(sp.energy, 1e-30))
		rep.LogMagDB += lm / float64(len(r.specs))
		rep.SC += sc / float64(len(r.specs))
	}
	rep.MRSTFT = rep.LogMagDB/10 + rep.SC

	env := envelopeDB(y, r.Rate)
	floor := maxOf(r.env) + envFloorDB
	var e float64
	for i, v := range r.env {
		c := floor
		if i < len(env) {
			c = math.Max(env[i], floor)
		}
		e += math.Abs(c - v)
	}
	rep.Envelope = e / float64(max(1, len(r.env)))

	if w.Descriptors != 0 && len(r.desc) > 0 {
		rep.Desc = map[string]float64{}
		cm := measureTrusted(y, r.Rate)
		reg := fingerprint.DefaultRegistry()
		var sum float64
		var n int
		for _, id := range TrustedDescriptors {
			rm, ok1 := r.desc[id]
			c, ok2 := cm[id]
			d, ok3 := reg.ByID(id)
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			delta := d.CompareWith(rm, c)
			if delta.NA != "" || math.IsNaN(delta.Norm) || math.IsInf(delta.Norm, 0) {
				continue
			}
			v := math.Max(0, delta.Norm-r.descZero[id])
			rep.Desc[id] = v
			sum += v
			n++
		}
		if n > 0 {
			rep.Descriptors = sum / float64(n)
		}
	}
	rep.Prior = prior

	div := func(v, s float64) float64 {
		if s <= 0 {
			return v
		}
		return v / s
	}
	rep.Total = w.MRSTFT*div(rep.MRSTFT, scale.MRSTFT) + w.Envelope*div(rep.Envelope, scale.Envelope) +
		w.Descriptors*div(rep.Descriptors, scale.Descriptors) + w.Prior*div(rep.Prior, scale.Prior)
	return rep
}

// PriorTerm is the energy-weighted mean |dB| of amplitude and of decay rate
// between the document's modes (at pitch, rate) and the analysis's modes —
// each analysis mode matched to the nearest document mode within 1 %. It keeps
// the optimizer from trading modes for noise. Analysis modes the document
// dropped (the budget) are not counted.
func PriorTerm(d instrument.Doc, a Analysis, pitch float64, rate int) float64 {
	dm := DocModes(d, pitch, rate)
	var sum, wsum float64
	for _, m := range a.Modes {
		best, bi := math.Inf(1), -1
		for i, c := range dm {
			if r := math.Abs(c.Freq/m.Freq - 1); r < best {
				best, bi = r, i
			}
		}
		if bi < 0 || best > 0.01 || m.Alpha <= 0 || dm[bi].Alpha <= 0 || m.Amp == 0 || dm[bi].Amp == 0 {
			continue
		}
		w := m.Energy()
		sum += w * (math.Abs(20*math.Log10(math.Abs(dm[bi].Amp/m.Amp))) + math.Abs(20*math.Log10(dm[bi].Alpha/m.Alpha)))
		wsum += w
	}
	if wsum == 0 {
		return 0
	}
	return sum / wsum
}
