package synthmatch

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Result holds the outcome of a Match call.
type Result struct {
	InstID    instrument.ID
	Params    map[string]float64 // best tuned params (the tuned keys only)
	StartLoss float64
	BestLoss  float64
	Evals     int
	RefFP     fingerprint.Fingerprint
	BestFP    fingerprint.Fingerprint
}

// Hooke–Jeeves over SpecForInstrument with seeded restarts, bounded by budget evaluations: deterministic.
func Match(instID instrument.ID, ref wave.Wave, budget, restarts int) (Result, error) {
	// Step 1: get spec and validate keys.
	spec, ok := SpecForInstrument(instID)
	if !ok {
		return Result{}, fmt.Errorf("synthmatch: no TunableSpec for instrument %q", instID)
	}
	if err := spec.ValidateKeys(instID); err != nil {
		return Result{}, err
	}

	// Step 2: build reference fingerprint, detect pitch, determine sr/dur.
	refFP := fingerprint.FromWave(ref, "ref")
	sr := ref.SampleRate
	if sr == 0 {
		sr = 44100
	}
	dur := ref.Duration()
	// dur<=0 means use instrument default (resolveRender handles <=0 → instrument default)

	// Calibrated against the synth's OWN pitch-0 output: the voices carry a small fixed detune, and the ideal-220 formula
	// would apply it twice, a systematic pitch error that dominates the distance and biases every other parameter.
	f0 := fingerprint.DetectF0(ref)
	pitch := 0.0
	if f0 > 0 {
		baseF0 := 220.0
		if probe, perr := RenderWithParams(instID, 0, sr, dur, nil); perr == nil {
			if pf0 := fingerprint.DetectF0(probe); pf0 > 0 {
				baseF0 = pf0
			}
		}
		pitch = 12 * math.Log2(f0/baseF0)
	}

	// The instrument document's values are the start point.
	mergedDefaults := audio.MergedInstrumentParamsRO(instID)

	params := spec.Params
	n := len(params)

	// Step 3: eval function with budget guard.
	totalEvals := 0

	// bestLoss and bestParams track the global optimum across all runs.
	var bestLoss float64
	var bestParams map[string]float64
	var bestFP fingerprint.Fingerprint

	evalFn := func(pt []float64) float64 {
		if totalEvals >= budget {
			return math.Inf(1)
		}
		overrides := make(map[string]float64, n)
		for i, p := range params {
			overrides[p.Key] = pt[i]
		}
		rendered, err := RenderWithParams(instID, pitch, sr, dur, overrides)
		totalEvals++
		if err != nil {
			return math.Inf(1)
		}
		fp := fingerprint.FromWave(rendered, "cand")
		loss := fingerprint.Distance(refFP, fp).Total
		if loss < bestLoss || bestParams == nil {
			bestLoss = loss
			bestFP = fp
			bestParams = make(map[string]float64, n)
			for k, v := range overrides {
				bestParams[k] = v
			}
		}
		return loss
	}

	// Step 4: Build the default start point from merged recipe defaults.
	startPt := make([]float64, n)
	for i, p := range params {
		if v, ok := mergedDefaults[p.Key]; ok {
			startPt[i] = clampParam(p, v)
		} else {
			startPt[i] = clampParam(p, (p.Min+p.Max)/2)
		}
	}

	// Evaluate start point once to record StartLoss (counts toward budget).
	startLoss := evalFn(startPt)
	if bestParams == nil {
		bestParams = make(map[string]float64, n)
		for i, p := range params {
			bestParams[p.Key] = startPt[i]
		}
		bestLoss = startLoss
	}

	// Split evenly so restarts are not starved when the first run converges early.
	totalRuns := restarts + 1
	remainingAfterStart := budget - totalEvals
	perRunBudget := remainingAfterStart / totalRuns
	if perRunBudget < 1 {
		perRunBudget = 1
	}

	// Step 5: Hooke-Jeeves from the default start point.
	runCap := totalEvals + perRunBudget
	hookeJeeves(startPt, params, evalFn, runCap, &totalEvals)

	// ONE seeded source for all restarts: deterministic regardless of call order.
	rng := rand.New(rand.NewSource(1))
	for r := 0; r < restarts; r++ {
		if totalEvals >= budget {
			break
		}
		// Random start point within [Min, Max] per param, snapped/rounded.
		rndPt := make([]float64, n)
		for i, p := range params {
			v := p.Min + rng.Float64()*(p.Max-p.Min)
			rndPt[i] = clampParam(p, v)
		}
		runCap = totalEvals + perRunBudget
		if runCap > budget {
			runCap = budget
		}
		hookeJeeves(rndPt, params, evalFn, runCap, &totalEvals)
	}

	res := Result{
		InstID:    instID,
		Params:    bestParams,
		StartLoss: startLoss,
		BestLoss:  bestLoss,
		Evals:     totalEvals,
		RefFP:     refFP,
		BestFP:    bestFP,
	}
	return res, nil
}

func hookeJeeves(startPt []float64, params []Param, evalFn func([]float64) float64, evalCap int, evals *int) {
	n := len(params)
	if n == 0 {
		return
	}

	// Per-param step sizes: Param.Step or (Max-Min)/10 when Step==0.
	steps := make([]float64, n)
	for i, p := range params {
		if p.Step > 0 {
			steps[i] = p.Step
		} else {
			steps[i] = (p.Max - p.Min) / 10.0
		}
	}

	// Minimum step threshold: 1e-3 of param range.
	minStep := make([]float64, n)
	for i, p := range params {
		minStep[i] = (p.Max - p.Min) * 1e-3
		if minStep[i] <= 0 {
			minStep[i] = 1e-9
		}
	}

	// Start from the given point.
	base := make([]float64, n)
	copy(base, startPt)
	baseLoss := evalFn(base)
	if *evals >= evalCap {
		return
	}

	// Scratch buffer for trial points (re-used inside exploratoryMove).
	trial := make([]float64, n)

	for *evals < evalCap {
		// Stop when all steps have collapsed to below the convergence threshold.
		allTiny := true
		for i := range steps {
			if steps[i] >= minStep[i] {
				allTiny = false
				break
			}
		}
		if allTiny {
			break
		}

		// Exploratory move: coordinate search from current base.
		improved, newPt, newLoss := exploratoryMove(base, baseLoss, steps, params, evalFn, evals, evalCap, trial)

		if improved {
			// Pattern move: extrapolate in the improvement direction.
			pattern := make([]float64, n)
			for i := range pattern {
				delta := newPt[i] - base[i]
				pattern[i] = clampParam(params[i], newPt[i]+delta)
			}

			var patternLoss float64
			if *evals < evalCap {
				patternLoss = evalFn(pattern)
			} else {
				patternLoss = math.Inf(1)
			}

			if patternLoss < newLoss && *evals < evalCap {
				// Pattern improved: re-explore from pattern point.
				imp2, newPt2, newLoss2 := exploratoryMove(pattern, patternLoss, steps, params, evalFn, evals, evalCap, trial)
				if imp2 {
					copy(base, newPt2)
					baseLoss = newLoss2
				} else {
					// Re-explore from pattern didn't help; fall back to exploratory result.
					copy(base, newPt)
					baseLoss = newLoss
				}
			} else {
				// Pattern didn't help; accept exploratory result.
				copy(base, newPt)
				baseLoss = newLoss
			}
		} else {
			// No exploratory improvement: halve all step sizes.
			for i := range steps {
				steps[i] /= 2.0
			}
		}
	}
}

// ±step per param in sequence.
func exploratoryMove(
	base []float64,
	baseLoss float64,
	steps []float64,
	params []Param,
	evalFn func([]float64) float64,
	evals *int,
	evalCap int,
	trial []float64,
) (improved bool, newPt []float64, newLoss float64) {
	n := len(params)
	newPt = make([]float64, n)
	copy(newPt, base)
	newLoss = baseLoss

	for i, p := range params {
		if *evals >= evalCap {
			break
		}
		bestVal := newPt[i]
		bestL := newLoss

		// Try positive step.
		vUp := clampParam(p, newPt[i]+steps[i])
		if vUp != newPt[i] {
			copy(trial, newPt)
			trial[i] = vUp
			l := evalFn(trial)
			if l < bestL {
				bestL = l
				bestVal = vUp
			}
		}

		// Try negative step (if budget allows and not redundant).
		if *evals < evalCap {
			vDown := clampParam(p, newPt[i]-steps[i])
			if vDown != newPt[i] && vDown != vUp {
				copy(trial, newPt)
				trial[i] = vDown
				l := evalFn(trial)
				if l < bestL {
					bestL = l
					bestVal = vDown
				}
			}
		}

		newPt[i] = bestVal
		newLoss = bestL
	}

	improved = newLoss < baseLoss
	return
}

// Discrete snaps first; Int rounds.
func clampParam(p Param, v float64) float64 {
	if len(p.Discrete) > 0 {
		best := p.Discrete[0]
		bestDist := math.Abs(v - best)
		for _, d := range p.Discrete[1:] {
			if dist := math.Abs(v - d); dist < bestDist {
				bestDist = dist
				best = d
			}
		}
		return best
	}
	if v < p.Min {
		v = p.Min
	}
	if v > p.Max {
		v = p.Max
	}
	if p.Int {
		v = math.Round(v)
	}
	return v
}
