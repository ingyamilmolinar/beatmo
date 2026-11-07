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

// The pre-audition gate is a FILTER, the listener the verdict: it stops a candidate that reads better on the counting
// criteria while being a worse render, which is easy to do by accident.

// A reviewed statement that the axis does not measure its question on this family (a measurement artifact), never a
// convenience for a candidate that could not clear it.
type gateExemption struct {
	Axis   string
	Reason string
}

// Dropped from every criterion (MISMATCH count, direction, liveness) and logged as exempt, so it stays visible.
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

// The axes the listener's own words point at. THRESHOLD-INDEPENDENT (|synth - ref|, not a verdict): the corpus
// calibration loosened bands until complained-about axes read "ok", so reshuffling verdicts must not count.
var directionAxes = map[instrument.ID][]string{
	roster.TubularBells: {fingerprint.IDModeBeatingDepth, fingerprint.IDModeCountPerOctave},
	roster.Snare:        {fingerprint.IDNoiseFlatness, fingerprint.IDDecayT20},
	roster.Oboe:         {fingerprint.IDNoisePersistence, fingerprint.IDBrightnessRatio},
}

const (
	// The share of the gap a candidate must close on at least ONE listener-named axis to count as aimed at the complaint.
	directionImprove = 0.25
	// How far the gap may grow on a listener-named axis; symmetric with directionImprove.
	directionWorsen = 0.25
)

// The "no PREDICTIVE axis made worse" clause: refs validate's strongest live axis (inharmonicity, AUC 1.00, CI clear of
// chance) instead of recomputing the whole correlation here. Re-read the live table first; an axis no longer PREDICTIVE
// comes out.
var predictiveAxes = []string{fingerprint.IDInharmonicity}

// Per-family axes a candidate may not push further from the reference: bells candidates bought their numbers by
// shortening the ring (asked to PROLONG) and retuning the hum partial just to make DetectF0 lock.
var noRegressAxes = map[instrument.ID][]string{
	roster.TubularBells: {fingerprint.IDEnvRingT60, fingerprint.IDModeAnchorHz},
}

// Slack for measurement jitter: an edit aimed at one axis nudges others' estimates by ULPs (the bells' mode anchor),
// which a bare after > before reports as a regression. Far below any audible move; the real regression was 36x.
const noRegressTol = 0.01

// worsened reports whether after is meaningfully further from the reference
// than before, tolerating noRegressTol of numerical jitter. The absolute floor
// keeps a gap that is already ~0 from failing on a rounding step.
func worsened(before, after float64) bool {
	return after > before*(1+noRegressTol)+1e-9
}

// A family whose questions mostly bind to axes reading n/a is not being gated at all (the snare has two that never
// produce a row).
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

// In the descriptor's own units, never normalised by a threshold: the direction criterion is independent of the bands.
func gap(a axisResult, ok bool) (float64, bool) {
	if !live(a, ok) {
		return 0, false
	}
	return math.Abs(a.Synth - a.Ref), true
}

// A map index yields its comma-ok value only in an assignment, not as a call argument.
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

		// Every dead bound axis is named, so "the gate passed" never means "the gate measured one axis".
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
				// An axis that stops measuring is never an improvement: with no verdict it would leave the MISMATCH column and read as
				// progress (the bells' brightness once the hum partial stops anchoring f0).
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
