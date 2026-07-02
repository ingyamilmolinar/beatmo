package synthmatch

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Result holds the outcome of a Match call.
type Result struct {
	InstID    string
	Params    map[string]float64 // best tuned params (the tuned keys only)
	StartLoss float64
	BestLoss  float64
	Evals     int
	RefFP     fingerprint.Fingerprint
	BestFP    fingerprint.Fingerprint
}

// Match tunes instID toward ref by minimizing fingerprint.Distance. It renders
// at the reference's detected pitch, runs Hooke–Jeeves coordinate descent over
// SpecForInstrument(instID) with `restarts` seeded random restarts, bounded by
// `budget` total evaluations. Deterministic for a given (instID, ref, budget,
// restarts). Returns an error if the instrument has no spec or fails to render.
func Match(instID string, ref wave.Wave, budget, restarts int) (Result, error) {
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
	f0 := fingerprint.DetectF0(ref)
	pitch := 0.0
	if f0 > 0 {
		pitch = 12 * math.Log2(f0/220)
	}
	sr := ref.SampleRate
	if sr == 0 {
		sr = 44100
	}
	dur := ref.Duration()
	// dur<=0 means use instrument default (resolveRender handles <=0 → instrument default)

	// Build merged defaults for the start point.
	recipeID := audio.RecipeForInstrument(instID)
	mergedDefaults := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))

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

	// Divide the remaining budget evenly among initial run + restarts.
	// Each of the (restarts+1) runs gets a fair share so restarts are not
	// starved when the initial run converges quickly to a local minimum.
	totalRuns := restarts + 1
	remainingAfterStart := budget - totalEvals
	perRunBudget := remainingAfterStart / totalRuns
	if perRunBudget < 1 {
		perRunBudget = 1
	}

	// Step 5: Hooke-Jeeves from the default start point.
	runCap := totalEvals + perRunBudget
	hookeJeeves(startPt, params, evalFn, runCap, &totalEvals)

	// Step 6: Restarts with seeded RNG. Use ONE rand.New(rand.NewSource(1))
	// for all restarts — deterministic regardless of call order.
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

// hookeJeeves runs a single Hooke-Jeeves coordinate descent from startPt.
// It uses evalFn and stops when *evals >= evalCap.
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

// exploratoryMove performs a coordinate search around base, trying ±step for
// each param in sequence. Returns (improved bool, bestPoint []float64, bestLoss float64).
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

// clampParam clamps v to p's [Min, Max] range, snaps to Discrete if non-empty,
// and rounds to integer if p.Int is set.
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
