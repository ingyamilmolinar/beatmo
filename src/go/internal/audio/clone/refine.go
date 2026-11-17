package clone

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// refine.go — seeded parallel CMA-ES over SMOOTH corrections of the inverted
// document (spec §4c): per octave of modes an amplitude gain and a decay
// scale; the strike's contact time and hardness; per octave of noise rows a
// level and a decay scale. Frequencies are LOCKED where the analysis put them
// — frequency is the non-convex dimension (Hayes 2023). The spec's ±10-cent
// group tune was tried and dropped: ±3 cents of search noise on a sharp
// partial swamps the 4096-point term, and a struck-bar fixture refined 4×
// slower with it (ESPRIT already places partials within 0.5 cent).
// Corrections are BAKED into the rows and the strike values; the result
// carries identity macros only.

// RenderFunc renders a document at the fit pitch and the reference's rate —
// audio.RenderAsShipped in production, a pure-Go model in -tags test.
type RenderFunc func(d instrument.Doc) []float64

// RefineOptions configures Refine. Zero values pick the defaults.
type RefineOptions struct {
	Render    RenderFunc
	Pitch     float64 // the fit pitch the render uses (for the prior term)
	Gens      int     // default 150
	Lambda    int     // default 4 + ⌊3 ln D⌋ (≥ 8)
	Workers   int     // default NumCPU − 1
	Seed      uint64
	StallGens int // stop after this many generations without a 1e-4 relative gain; default 30
	Weights   LossWeights
}

// GenTrace is one generation of the search.
type GenTrace struct {
	Gen  int     `json:"gen"`
	Best float64 `json:"best"`
	Mean float64 `json:"mean"`
	Ms   float64 `json:"ms"`
}

// RefineResult is the refined document and how it got there.
type RefineResult struct {
	Doc      instrument.Doc `json:"-"`
	Before   LossReport     `json:"before"`
	After    LossReport     `json:"after"`
	Kept     bool           `json:"kept_analysis_only"`
	Dims     []string       `json:"dims"`
	Gens     []GenTrace     `json:"gens"`
	Evals    int            `json:"evals"`
	WarmupMs float64        `json:"warmup_ms"`
	TotalMs  float64        `json:"total_ms"`
}

// priorScaleDB is what one unit of the prior costs: a 6 dB mean departure
// from the measured amplitudes / decays weighs as much as the whole spectral
// loss of the analysis-only candidate.
const priorScaleDB = 6

// refineSigma0 is the initial step in the sin-transformed space (≈ ±0.6 dB on
// a ±12 dB dimension). Refinement POLISHES an analysis that is already close:
// at 0.3 (±3.6 dB) the average marimba candidate scored 6× the analysis-only
// document and 30 generations found nothing better; CMA-ES widens the step
// itself when the start is far off (a synthetic 6 dB / ×1.5 error still
// converges).
const refineSigma0 = 0.05

type dim struct {
	name   string
	lo, hi float64
	apply  func(d *instrument.Doc, v float64)
}

// Refine searches corrections of doc0 against ref. The descriptor term is
// too slow for the search (≈ 0.4 s per score), so the search uses the other
// three; Before / After report all four.
func Refine(doc0 instrument.Doc, ref *LossRef, a Analysis, opt RefineOptions) (RefineResult, error) {
	t0 := time.Now()
	if opt.Render == nil {
		return RefineResult{}, fmt.Errorf("clone: Refine needs a RenderFunc")
	}
	if opt.Gens == 0 {
		// 150 × 16 renders: the clone objective (L1 envelope, equal-peak
		// coupling) converges ~4× slower than a sphere — this CMA-ES matches
		// Hansen's sphere counts (10-D: 1,660 evals to 1e-10), and a struck-bar
		// fixture needed ~120 generations to reach 15 % of its Doc₀ loss.
		opt.Gens = 150
	}
	if opt.Workers <= 0 {
		opt.Workers = max(1, runtime.NumCPU()-1)
	}
	if opt.StallGens <= 0 {
		opt.StallGens = 30
	}
	if opt.Weights == (LossWeights{}) {
		opt.Weights = DefaultLossWeights
	}
	search := opt.Weights
	search.Descriptors = 0

	dims := searchSpace(doc0, opt.Pitch, ref.Rate)
	res := RefineResult{Doc: doc0}
	for _, d := range dims {
		res.Dims = append(res.Dims, d.name)
	}

	tw := time.Now()
	r0 := opt.Render(doc0)
	res.WarmupMs = msSince(tw)
	full := func(d instrument.Doc, r []float64, w LossWeights, scale LossReport) LossReport {
		return ref.Score(r, PriorTerm(d, a, opt.Pitch, ref.Rate), w, scale)
	}
	raw0 := full(doc0, r0, opt.Weights, LossReport{})
	// Spectral and envelope terms are relative to the analysis-only document;
	// the descriptor term is already in threshold units (a registry Norm of 1
	// = at mismatch) — normalised by its own Doc₀ value, a kalimba's 0.115
	// made a move to 0.38 read as a 3.3× regression.
	scale := LossReport{MRSTFT: raw0.MRSTFT, Envelope: raw0.Envelope, Descriptors: 1, Prior: priorScaleDB}
	res.Before = full(doc0, r0, opt.Weights, scale)
	best0 := ref.Score(r0, raw0.Prior, search, scale).Total
	res.After = res.Before
	if len(dims) == 0 || opt.Gens < 0 {
		res.Kept = true
		res.TotalMs = msSince(t0)
		return res, nil
	}

	lambda := opt.Lambda
	if lambda <= 0 {
		// Hansen's default 4 + ⌊3 ln D⌋ (14 for 36 dimensions — two full
		// rounds on 7 workers; 16 left a third round mostly idle). Never a
		// function of the worker count: the result must not be either.
		lambda = max(8, 4+int(3*math.Log(float64(len(dims)))))
	}
	es := NewCMAES(len(dims), refineSigma0, lambda, opt.Seed)
	pool := async.NewPool(context.Background(), async.Options{Name: "clone.refine", MaxConcurrent: opt.Workers, QueueSize: es.Lambda()})
	defer pool.Close()

	toDoc := func(y []float64) instrument.Doc {
		d := doc0.Clone()
		for i, dm := range dims {
			dm.apply(&d, dm.lo+(dm.hi-dm.lo)*(1+math.Sin(y[i]))/2)
		}
		return d
	}
	bestF, bestY := best0, []float64(nil)
	stall, lastBest := 0, best0
	for g := 0; g < opt.Gens; g++ {
		tg := time.Now()
		ys := es.Ask()
		f := make([]float64, len(ys))
		var wg sync.WaitGroup
		for k := range ys {
			wg.Add(1)
			k := k
			if err := pool.SubmitBlocking(context.Background(), func(context.Context) {
				defer wg.Done()
				d := toDoc(ys[k])
				f[k] = ref.Score(opt.Render(d), PriorTerm(d, a, opt.Pitch, ref.Rate), search, scale).Total
			}); err != nil {
				wg.Done()
				return res, err
			}
		}
		wg.Wait()
		res.Evals += len(ys)
		mean := 0.0
		for k, v := range f {
			mean += v / float64(len(f))
			if v < bestF {
				bestF, bestY = v, append([]float64(nil), ys[k]...)
			}
		}
		es.Tell(ys, f)
		res.Gens = append(res.Gens, GenTrace{Gen: g, Best: bestF, Mean: mean, Ms: msSince(tg)})
		if bestF < lastBest*(1-1e-4) {
			lastBest, stall = bestF, 0
		} else if stall++; stall >= opt.StallGens {
			break
		}
	}
	if bestY == nil {
		res.Kept = true
		res.TotalMs = msSince(t0)
		return res, nil
	}
	d := sortRowsByEnergy(toDoc(bestY), opt.Pitch, ref.Rate)
	res.Doc = d
	res.After = full(d, opt.Render(d), opt.Weights, scale)
	res.TotalMs = msSince(t0)
	return res, nil
}

func msSince(t time.Time) float64 { return float64(time.Since(t).Microseconds()) / 1000 }

// searchSpace derives the correction dimensions from the document: only
// populated octaves get dimensions, so the space follows the instrument.
func searchSpace(d instrument.Doc, pitch float64, rate int) []dim {
	var dims []dim
	octave := func(f float64) int { return int(math.Floor(math.Log2(math.Max(f, 1) / 31.25))) }

	if st, ok := d.Stages["modal"]; ok && st.Table != nil {
		col := tableCols(st.Table)
		groups := map[int][]int{}
		for _, m := range DocModes(d, pitch, rate) {
			groups[octave(m.Freq)] = append(groups[octave(m.Freq)], m.Band)
		}
		for _, g := range sortedKeys(groups) {
			rows := groups[g]
			lbl := fmt.Sprintf("modes %.0f–%.0f Hz", 31.25*math.Pow(2, float64(g)), 31.25*math.Pow(2, float64(g+1)))
			dims = append(dims,
				dim{lbl + " gain dB", -12, 12, rowsApply("modal", rows, func(r []float64, v float64) {
					r[col["amp"]] = clampAbs(r[col["amp"]]*math.Pow(10, v/20), rowAmpBound)
				})},
				dim{lbl + " decay log2", -1, 1, rowsApply("modal", rows, func(r []float64, v float64) {
					r[col["decay"]] = math.Min(r[col["decay"]]*math.Pow(2, v), rowDecayBound)
				})},
			)
		}
	}
	if stageOn(d, "strike") {
		if d.Stages["strike"].Values["strike_tc"] > 0 {
			dims = append(dims, dim{"strike contact log2", -1, 1, func(d *instrument.Doc, v float64) {
				vals := d.Stages["strike"].Values
				vals["strike_tc"] = clamp(vals["strike_tc"]*math.Pow(2, v), tcBounds)
			}})
		}
		dims = append(dims, dim{"strike hardness dB/oct", -6, 6, func(d *instrument.Doc, v float64) {
			vals := d.Stages["strike"].Values
			vals["strike_hardness"] = clamp(vals["strike_hardness"]+v, hardBounds)
		}})
	}
	if st, ok := d.Stages["noiseband"]; ok && st.On && st.Table != nil {
		col := tableCols(st.Table)
		_, k := docVoice(d, pitch)
		groups := map[int][]int{}
		for i, r := range st.Table.Rows {
			groups[octave(r[col["center"]]*k)] = append(groups[octave(r[col["center"]]*k)], i)
		}
		for _, g := range sortedKeys(groups) {
			rows := groups[g]
			lbl := fmt.Sprintf("noise %.0f–%.0f Hz", 31.25*math.Pow(2, float64(g)), 31.25*math.Pow(2, float64(g+1)))
			dims = append(dims,
				dim{lbl + " level dB", -12, 12, rowsApply("noiseband", rows, func(r []float64, v float64) {
					r[col["level"]] = math.Min(r[col["level"]]+v, noiseLevelBoundHi)
				})},
				dim{lbl + " decay log2", -1, 1, rowsApply("noiseband", rows, func(r []float64, v float64) {
					r[col["fast_rate"]] = math.Min(r[col["fast_rate"]]*math.Pow(2, v), noiseRateBound)
					r[col["tail_rate"]] = math.Min(r[col["tail_rate"]]*math.Pow(2, v), noiseRateBound)
				})},
			)
		}
	}
	return dims
}

// rowsApply edits the given rows of a stage's table in place (the caller
// works on a Clone).
func rowsApply(stage string, rows []int, f func(r []float64, v float64)) func(*instrument.Doc, float64) {
	return func(d *instrument.Doc, v float64) {
		t := d.Stages[stage].Table
		for _, i := range rows {
			f(t.Rows[i], v)
		}
	}
}

func clampAbs(v, b float64) float64 { return math.Copysign(math.Min(math.Abs(v), b), v) }

func sortedKeys(m map[int][]int) []int {
	ks := make([]int, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

// sortRowsByEnergy restores the "rows in priority order" convention (Keep N
// keeps the first N) after corrections moved the amplitudes and decays.
func sortRowsByEnergy(d instrument.Doc, pitch float64, rate int) instrument.Doc {
	st, ok := d.Stages["modal"]
	if !ok || st.Table == nil {
		return d
	}
	energy := make([]float64, len(st.Table.Rows))
	for _, m := range DocModes(d, pitch, rate) {
		energy[m.Band] = m.Energy()
	}
	idx := make([]int, len(st.Table.Rows))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return energy[idx[a]] > energy[idx[b]] })
	rows := make([][]float64, len(idx))
	for i, j := range idx {
		rows[i] = st.Table.Rows[j]
	}
	st.Table.Rows = rows
	d.Stages["modal"] = st
	return d
}
