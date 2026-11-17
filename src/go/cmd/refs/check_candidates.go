//go:build !js

package main

import (
	"fmt"
	"io"
	"math"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The candidates check — the pre-audition gate (spec §5).
//
// The gate is a FILTER, not the verdict: the listener is the verdict. What it
// exists to stop is a candidate that reads better on the counting criteria
// while being a worse render — which, in practice, is easy to do by accident.
// Every criterion below was added because a real candidate authored in Task 15
// tripped it.

// gateExemption records a bound axis a family's gate must NOT count, and WHY.
// An exemption is a deliberate, reviewed statement that the axis does not
// measure what its question asks on this family — never a convenience for a
// candidate that could not clear it. Both current entries are measurement
// artifacts identified from the tool's own output, not tuning failures.
type gateExemption struct {
	Axis   string
	Reason string
}

// gateExemptions is the ONE table of exempt (family, axis) pairs. An exempt
// axis is dropped from every criterion below — the MISMATCH count, the
// direction check, and the liveness count — and is logged as exempt so the
// exemption stays visible in the gate's own output rather than being silently
// absorbed.
var gateExemptions = map[instrument.ID][]gateExemption{
	roster.TubularBells: {{
		Axis: fingerprint.IDBrightnessRatio,
		Reason: "recorded DetectF0 artifact, not a brightness defect: the axis is " +
			"SpectralCentroid/F0Hz and the synth's centroid is 1314 Hz against the " +
			"reference's 1855 Hz — already DARKER — while candidates that change the " +
			"ratio move the centroid only 1314->1334 Hz. The whole 4.58x delta is the " +
			"DENOMINATOR: on this inharmonic bell DetectF0 latches 144 Hz, a quarter of " +
			"the 577 Hz mode anchor. Chasing the ratio means feeding the detector a " +
			"harmonic skeleton the reference does not have, which regresses mode anchor.",
	}},
	roster.Oboe: {{
		Axis: fingerprint.IDEnvAttack,
		Reason: "the reference's 1.14 s attack is the PERFORMANCE, not the instrument: " +
			"oboe-A4 is a swelled note. Matching it needs amp_attack ~1.0 s, which makes " +
			"the voice unable to speak under a sequencer trigger — a real regression " +
			"traded for a number. A tongued oboe onset is tens of milliseconds.",
	}},
}

// directionAxes are the listener-named axes per family: the ones whose
// question the listener's own words this round point at. The direction
// criterion below is THRESHOLD-INDEPENDENT — it looks at |synth - ref|
// directly rather than at a verdict — because the corpus calibration (Task 8)
// loosened several verdict bands until axes the listener complained about read
// "ok". A candidate that only shuffles verdicts within a loosened band would
// pass the counting criteria while moving nothing the listener can hear.
var directionAxes = map[instrument.ID][]string{
	roster.TubularBells: {fingerprint.IDModeBeatingDepth, fingerprint.IDModeCountPerOctave},
	roster.Snare:        {fingerprint.IDNoiseFlatness, fingerprint.IDDecayT20},
	roster.Oboe:         {fingerprint.IDNoisePersistence, fingerprint.IDBrightnessRatio},
}

const (
	// directionImprove is the share of the current |synth - ref| gap a
	// candidate must close on at least ONE listener-named axis to count as
	// aimed at the complaint rather than incidental.
	directionImprove = 0.25
	// directionWorsen is how far the gap may GROW on any listener-named axis
	// before the candidate is trading the listener's complaint for something
	// else. Same magnitude as directionImprove so the criterion is symmetric.
	directionWorsen = 0.25
)

// predictiveAxes is spec §5's "no PREDICTIVE axis made worse" clause. The
// PREDICTIVE set is what `refs validate` classifies as AUC >= 0.70 against the
// ear verdicts. Rather than recomputing that whole correlation here (it needs
// the full ear_verdicts corpus and takes minutes), the gate hard-codes the
// axis with the strongest live result:
//
//	$ refs validate
//	inharmonicity  AUC 1.00  95% CI 1.00-1.00  PREDICTIVE  ... corpus  partial structure
//
// AUC 1.00 with a CI that clears chance entirely — when this axis says the
// synth got further from the reference, the listener agreed every time in the
// corpus. Re-read the live table before adding to this list; an axis that has
// drifted out of PREDICTIVE must come out of it.
var predictiveAxes = []string{fingerprint.IDInharmonicity}

// noRegressAxes are per-family axes a candidate may not push further from the
// reference, beyond the general PREDICTIVE rule. The bells entries exist
// because the first round of bells candidates bought their numbers by
// shortening the ring (ring T60 ok -> MISMATCH, "decays far too fast", against
// a listener who asked for a ring that PROLONGS) and by retuning the hum
// partial (mode anchor ok +1% -> drift -19%) purely to make DetectF0 lock.
var noRegressAxes = map[instrument.ID][]string{
	roster.TubularBells: {fingerprint.IDEnvRingT60, fingerprint.IDModeAnchorHz},
}

// noRegressTol is the relative slack the no-regress and PREDICTIVE checks
// allow before calling a gap "further from the reference". Rendering is
// deterministic; the slack exists because an override aimed at one axis still
// perturbs the MEASUREMENT of others by a hair. The bells' mode anchor is
// 2.987 Hz from the reference both before and after a candidate that never
// touches the hum partial, but the estimate lands a few ULPs apart because the
// spectrum it is estimated from changed elsewhere — which a bare
// `after > before` reported as a regression. 1% is far below any audible or
// verdict-changing move; the real regression this check was written for took
// the same axis from 2.987 to 108.4 Hz, a 36x move.
const noRegressTol = 0.01

// worsened reports whether after is meaningfully further from the reference
// than before, tolerating noRegressTol of numerical jitter. The absolute floor
// keeps a gap that is already ~0 from failing on a rounding step.
func worsened(before, after float64) bool {
	return after > before*(1+noRegressTol)+1e-9
}

// minLiveBoundAxes is how many of a family's bound axes must actually MEASURE
// on both sides for the gate's verdict to mean anything. A family whose
// questions are mostly bound to axes that read n/a is not being gated at all —
// the snare, for instance, has two bound axes that never produce a row
// (snare.low_body_frac's reference fails RequiresPercussive; noise.fraction's
// f0 confidence is below 0.65 on both sides).
const minLiveBoundAxes = 2

// axisByID indexes a diagnosis's rows by descriptor id.
func axisByID(d instrumentDiagnosis) map[string]axisResult {
	out := make(map[string]axisResult, len(d.Axes))
	for _, a := range d.Axes {
		if a.ID != "" {
			out[a.ID] = a
		}
	}
	return out
}

// live reports whether an axis produced a comparable measurement: present,
// with a verdict, and with neither side flagged n/a.
func live(a axisResult, ok bool) bool {
	return ok && a.Verdict != "" && a.RefNA == "" && a.SynthNA == ""
}

// gap is |synth - ref| for a live axis, and false when the axis is not
// comparable. It is deliberately in the descriptor's own units and NOT
// normalised by any threshold: the point of the direction criterion is to be
// independent of the calibrated bands.
func gap(a axisResult, ok bool) (float64, bool) {
	if !live(a, ok) {
		return 0, false
	}
	return math.Abs(a.Synth - a.Ref), true
}

// gapOf is gap() over an id-indexed diagnosis, since a map index only yields
// its comma-ok second value in an assignment, not as a call argument.
func gapOf(m map[string]axisResult, id string) (float64, bool) {
	a, ok := m[id]
	return gap(a, ok)
}

// checkCandidateGate runs every shipped audition candidate through the gate.
// It renders through CGo, so run it from the untagged build.
func checkCandidateGate(r *checkReport) error {
	p, err := defaultCandidatesPath()
	if err != nil {
		return err
	}
	fams, err := loadCandidates(p)
	if err != nil {
		return err
	}
	names := make([]instrument.ID, 0, len(fams))
	for name := range fams {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		f := fams[name]
		exempt := map[string]string{}
		for _, e := range gateExemptions[name] {
			exempt[e.Axis] = e.Reason
		}
		bound := map[string]bool{}
		for _, q := range deepQuestions(name) {
			if _, skip := exempt[q.Axis]; skip {
				r.Logf("%s: %s is EXEMPT from the gate — %s", name, q.Axis, exempt[q.Axis])
				continue
			}
			bound[q.Axis] = true
		}
		if len(bound) == 0 {
			r.Errorf("%s: no non-exempt bound axes — the gate has nothing to judge", name)
			continue
		}

		cur, err := diagnoseInstrumentOpts(io.Discard, f.Base, f.Reference, 0, nil)
		if err != nil {
			return fmt.Errorf("%s current: %w", name, err)
		}
		curAx := axisByID(cur)

		// ---- liveness -------------------------------------------------
		// Report every dead bound axis by name, then fail if too few are
		// left, so "the gate passed" can never mean "the gate measured one
		// axis".
		nLive := 0
		for id := range bound {
			a, ok := curAx[id]
			if live(a, ok) {
				nLive++
				continue
			}
			switch {
			case !ok:
				r.Logf("%s: bound axis %s produces NO ROW on this family (the descriptor does not apply to the reference)", name, id)
			case a.RefNA != "":
				r.Logf("%s: bound axis %s is N/A on the REFERENCE: %s", name, id, a.RefNA)
			case a.SynthNA != "":
				r.Logf("%s: bound axis %s is N/A on the SYNTH: %s", name, id, a.SynthNA)
			default:
				r.Logf("%s: bound axis %s carries no verdict", name, id)
			}
		}
		if nLive < minLiveBoundAxes {
			r.Errorf("%s: only %d live bound axis/axes (want >= %d) — this family's questions are bound to axes that do not measure, so the gate cannot judge it",
				name, nLive, minLiveBoundAxes)
		}

		curV := map[string]fingerprint.Verdict{}
		curMis, curOff := 0, 0
		for id := range bound {
			a, ok := curAx[id]
			if !ok {
				continue
			}
			curV[id] = a.Verdict
			if a.Verdict == verdictMismatch {
				curMis++
			}
			if a.Verdict == verdictMismatch || a.Verdict == verdictDrift {
				curOff++
			}
		}

		for _, c := range f.Candidates {
			d, err := diagnoseInstrumentOpts(io.Discard, f.Base, f.Reference, 0, c.Overrides)
			if err != nil {
				return fmt.Errorf("%s/%s: %w", name, c.ID, err)
			}
			ax := axisByID(d)

			// ---- counting: MISMATCH, and drift as the fallback ---------
			mis, off := 0, 0
			for id := range bound {
				a, ok := ax[id]
				if !ok {
					continue
				}
				// An axis the current version MEASURES but the candidate does
				// not is never an improvement: an unmeasurable axis carries no
				// verdict, so it would silently leave the MISMATCH column and
				// be counted as progress. The bells' brightness ratio goes
				// unmeasurable ("no fundamental to normalise the centroid by")
				// as soon as the hum partial stops anchoring f0 detection.
				if a.Verdict == "" && curV[id] != "" {
					r.Errorf("%s/%s: %s was %s and is now UNMEASURABLE — %s", name, c.ID, a.Name, curV[id], a.SynthNA)
				}
				if a.Verdict == verdictMismatch {
					mis++
					if curV[id] != verdictMismatch {
						r.Errorf("%s/%s: %s regressed to MISMATCH (was %s)", name, c.ID, a.Name, curV[id])
					}
				}
				if a.Verdict == verdictMismatch || a.Verdict == verdictDrift {
					off++
				}
			}
			switch {
			case curMis > 0 && mis >= curMis:
				r.Errorf("%s/%s: %d MISMATCH bound axes, current has %d — candidate does not improve the targeted axes", name, c.ID, mis, curMis)
			case curMis == 0 && off >= curOff:
				// The spec's clause is vacuous when there is no bound
				// MISMATCH to beat, so require a drift reduction instead.
				r.Errorf("%s/%s: %d off-target (drift+MISMATCH) bound axes, current has %d — current has no bound MISMATCH to beat, so the candidate must reduce drift instead",
					name, c.ID, off, curOff)
			}

			// ---- direction: threshold-independent -----------------------
			improved := false
			for _, id := range directionAxes[name] {
				if _, skip := exempt[id]; skip {
					continue
				}
				before, okB := gapOf(curAx, id)
				after, okA := gapOf(ax, id)
				if !okB || !okA {
					continue
				}
				if before == 0 {
					continue // already exact; cannot shrink a zero gap
				}
				change := (after - before) / before
				dsc, _ := fingerprint.DefaultRegistry().ByID(id)
				r.Logf("%s/%s: direction %s |synth-ref| %.4g -> %.4g (%+.0f%%)", name, c.ID, dsc.Name, before, after, change*100)
				if change <= -directionImprove {
					improved = true
				}
				if change > directionWorsen {
					r.Errorf("%s/%s: %s moved %.0f%% FURTHER from the reference (%.4g -> %.4g) — the candidate trades away a listener-named axis",
						name, c.ID, dsc.Name, change*100, before, after)
				}
			}
			if !improved {
				r.Errorf("%s/%s: no listener-named axis %v closed >= %.0f%% of its gap to the reference — the candidate may improve verdicts without moving what the listener complained about",
					name, c.ID, directionAxes[name], directionImprove*100)
			}

			// ---- no PREDICTIVE axis made worse (spec §5) ----------------
			for _, id := range predictiveAxes {
				before, okB := gapOf(curAx, id)
				after, okA := gapOf(ax, id)
				if okB && !okA {
					// Same trap as the bound-axis UNMEASURABLE check: an axis
					// that stops measuring is not an axis that improved.
					dsc, _ := fingerprint.DefaultRegistry().ByID(id)
					r.Errorf("%s/%s: PREDICTIVE axis %s stopped measuring (was %.4g from the reference) — %s",
						name, c.ID, dsc.Name, before, ax[id].SynthNA)
					continue
				}
				if !okB || !okA {
					continue
				}
				if worsened(before, after) {
					dsc, _ := fingerprint.DefaultRegistry().ByID(id)
					r.Errorf("%s/%s: PREDICTIVE axis %s moved further from the reference (%.4g -> %.4g) — `refs validate` scores this axis AUC 1.00 against the ear verdicts, so this is a change the listener is expected to hear as worse",
						name, c.ID, dsc.Name, before, after)
				}
			}

			// ---- per-family no-regress axes -----------------------------
			for _, id := range noRegressAxes[name] {
				a, ok := ax[id]
				before, okB := gapOf(curAx, id)
				after, okA := gap(a, ok)
				dsc, _ := fingerprint.DefaultRegistry().ByID(id)
				if okB && !okA {
					r.Errorf("%s/%s: no-regress axis %s stopped measuring (was %.4g from the reference)", name, c.ID, dsc.Name, before)
					continue
				}
				if !okB || !okA {
					continue
				}
				curA, curOK := curAx[id]
				if worsened(before, after) {
					r.Errorf("%s/%s: no-regress axis %s moved further from the reference (%.4g -> %.4g, verdict %s -> %s)",
						name, c.ID, dsc.Name, before, after, verdictOf(curA, curOK), a.Verdict)
				}
				if a.Verdict == verdictMismatch && verdictOf(curA, curOK) != verdictMismatch {
					r.Errorf("%s/%s: no-regress axis %s became MISMATCH (was %s)", name, c.ID, dsc.Name, verdictOf(curA, curOK))
				}
			}

			r.Logf("%s/%s: bound MISMATCH %d → %d, bound drift+MISMATCH %d → %d (%d live bound axes)", name, c.ID, curMis, mis, curOff, off, nLive)
		}
	}
	return nil
}

func verdictOf(a axisResult, ok bool) fingerprint.Verdict {
	if !ok {
		return "absent"
	}
	if a.Verdict == "" {
		return "n/a"
	}
	return a.Verdict
}
