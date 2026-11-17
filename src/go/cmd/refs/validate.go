//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// validate.go — `refs validate`: does each descriptor actually predict what a
// listener hears?
//
// WHY THIS EXISTS
// ---------------
// The battery grew to ~50 descriptors with no evidence that any of them tracks
// perceived quality. Running `refs diagnose -all` over the corpus shows the
// problem plainly: `MFCC profile` reports MISMATCH on 56 of 57 instruments,
// `centroid spread` on 48, `partial stretch` on 34 of 37. An axis that fires on
// nearly everything cannot rank anything -- it is a constant, not a
// measurement, and it crowds out the axes that do carry signal.
//
// ear_verdicts.jsonl is the missing ground truth: a listener's judgement of
// every family, collected for exactly this purpose. This command joins the two.
// For each descriptor it asks a single falsifiable question:
//
//	do the instruments the listener liked score LOWER on this axis than the
//	ones they disliked?
//
// The answer is an AUC (area under the ROC curve) over the good-vs-bad split:
// 1.0 means the axis orders them perfectly, 0.5 means it is no better than a
// coin flip, and below 0.5 means it is ANTI-correlated -- actively pointing the
// wrong way, which is worse than useless for tuning.
//
// This does not replace listening. It says which measurements have earned the
// right to be believed when nobody is listening.

// verdictClass buckets an ear verdict into the three groups the AUC is computed
// over. The numbered picks ("1", "2", ...) mean "this option is closest", which
// is a comparative statement and not an absolute quality judgement, so they sit
// with "partly" in the middle rather than counting as good.
type verdictClass int

const (
	classBad verdictClass = iota
	classMid
	classGood
)

func (c verdictClass) String() string {
	switch c {
	case classGood:
		return "good"
	case classBad:
		return "bad"
	default:
		return "mid"
	}
}

func classify(verdict string) verdictClass {
	switch verdict {
	case "y":
		return classGood
	case "n":
		return classBad
	default: // "p", "1", "2", ...
		return classMid
	}
}

// axisScore is one descriptor's validation result.
type axisScore struct {
	// ID is the registry descriptor ID. Axis is the human-facing name; the ID
	// is what the calibration file and the Demoted flag are keyed on, so it
	// has to travel with the row rather than being re-derived from the name.
	ID    string
	Axis  string
	Group string

	// FlagDemoted mirrors Descriptor.Demoted -- a demotion recorded in the
	// registry by hand, from evidence gathered outside this run.
	FlagDemoted bool

	// GoodFiresRate is the share of GOOD-graded families on which this axis
	// reads at or past its mismatch threshold (Norm >= 1), over
	// NGoodMeasured families where it was measurable at all.
	//
	// This is the self-demotion signal, and it is a different question from
	// MismatchRate below. Firing on most of the CORPUS can be honest -- most
	// instruments really are wrong on most axes. Firing on most of the
	// families the listener ACCEPTED cannot be: the axis is calling a defect
	// where a human heard none, so whatever it measures is not what makes an
	// instrument sound right.
	GoodFiresRate float64
	NGoodMeasured int

	NGood, NMid, NBad int
	MeanGood          float64
	MeanMid           float64
	MeanBad           float64

	// PrefAgree/PrefTotal come from scorePreferences: over every preference
	// pair (chosen vs rejected) where this descriptor was measurable on both
	// clips, PrefAgree counts the pairs where the CHOSEN clip's Norm read
	// strictly lower (closer to the reference) than the rejected clip's.
	// This is a second, independent evidence source from the AUC's
	// good/bad-family split above: it uses within-family relative
	// preferences (including session data) rather than absolute per-family
	// verdicts.
	PrefAgree, PrefTotal int

	// AUC over the good-vs-bad split, computed on Norm. Undefined (and
	// Comparable=false) when either class is empty for this axis.
	AUC        float64
	Comparable bool
	// SE is the Hanley-McNeil standard error of AUC. With a corpus of ~7 good
	// and ~6 bad families this is LARGE -- around 0.16 -- and reporting a bare
	// AUC without it would invite treating a 0.70 as established when its
	// confidence interval comfortably spans chance. Every label below is gated
	// on AUC being separated from 0.5 by more than 1.96 SE.
	SE float64

	// MismatchRate is the share of ALL measured instruments the axis flags.
	// An axis near 1.0 is a constant; an axis near 0.0 never speaks.
	MismatchRate float64
	NMeasured    int
	NUnavailable int
}

// auc computes the probability that a randomly chosen BAD instrument scores
// higher on this axis than a randomly chosen GOOD one, counting ties as half.
// This is the Mann-Whitney U statistic normalized to 0..1 -- rank-based, so it
// is insensitive to the axis's units and to outliers, which matters when the
// axes span seconds, hertz, decibels and unit fractions.
func auc(good, bad []float64) (float64, bool) {
	if len(good) == 0 || len(bad) == 0 {
		return 0, false
	}
	var wins float64
	for _, b := range bad {
		for _, g := range good {
			switch {
			case b > g:
				wins++
			case b == g:
				wins += 0.5
			}
		}
	}
	return wins / float64(len(good)*len(bad)), true
}

// aucStandardError is the Hanley-McNeil (1982) estimator for the standard
// error of an AUC, which accounts for both class sizes.
//
// It exists so this command cannot overstate what 13 labelled families can
// support. The exponential approximations for Q1/Q2 are the standard ones and
// are adequate here: the point is the ORDER OF MAGNITUDE of the uncertainty,
// not a precise interval.
func aucStandardError(a float64, nGood, nBad int) float64 {
	if nGood == 0 || nBad == 0 {
		return math.NaN()
	}
	ng, nb := float64(nGood), float64(nBad)
	q1 := a / (2 - a)
	q2 := 2 * a * a / (1 + a)
	v := (a*(1-a) + (nb-1)*(q1-a*a) + (ng-1)*(q2-a*a)) / (ng * nb)
	if v < 0 {
		return 0
	}
	return math.Sqrt(v)
}

// significantlyBetterThanChance reports whether the AUC's 95% interval clears
// 0.5. Below this bar an axis has not earned a directional claim.
func significantlyBetterThanChance(a, se float64) bool {
	return !math.IsNaN(se) && a-1.96*se > 0.5
}

// significantlyWorseThanChance reports whether the axis is confidently
// ANTI-correlated -- the case that matters most, since acting on such an axis
// actively degrades the instrument.
func significantlyWorseThanChance(a, se float64) bool {
	return !math.IsNaN(se) && a+1.96*se < 0.5
}

func mean(vs []float64) float64 {
	if len(vs) == 0 {
		return math.NaN()
	}
	var s float64
	for _, v := range vs {
		s += v
	}
	return s / float64(len(vs))
}

// meanCell renders a mean(good)/mean(bad) column. An EMPTY group has no mean,
// and mean() says so with NaN -- but "%.2f" of a NaN prints the literal text
// "NaN" in a numeric column, which reads as a computed value that went wrong
// rather than as "this group was empty". A dash says the latter.
func meanCell(v float64) string {
	if math.IsNaN(v) {
		return "   - "
	}
	return fmt.Sprintf("%.2f", v)
}

// prefBackwardsMaxRate is the agreement rate below which a preference result
// is pointing the WRONG way rather than merely being weak: below one half, the
// axis agrees with the listener less often than a coin would.
const prefBackwardsMaxRate = 0.5

// prefBackwardsMaxP is the significance the backwards marker requires. Without
// it, 1/3 (p=1.00) would be marked backwards on three pairs of noise.
const prefBackwardsMaxP = 0.05

// prefCell renders the PREF column.
//
// A SIGNIFICANTLY backwards axis is marked. "2/11 p=0.02" is a strong result
// -- the axis tracks the listener's preference reliably, in the opposite
// direction -- and scanning a column of AGREE/TOTAL fractions for the small
// ones is exactly the reading mistake that let bright.centroid look
// reasonable for as long as it did. The arrow says which way it points; the
// legend under the table says what to do about it.
func prefCell(s axisScore) string {
	if s.PrefTotal == 0 {
		return "-"
	}
	p := signTestP(s.PrefAgree, s.PrefTotal)
	cell := fmt.Sprintf("%d/%d p=%.2f", s.PrefAgree, s.PrefTotal, p)
	if float64(s.PrefAgree)/float64(s.PrefTotal) < prefBackwardsMaxRate && p < prefBackwardsMaxP {
		cell += " ↓backwards"
	}
	return cell
}

// validationSample is one (family, verdict) row resolved to the instrument and
// reference the listener actually judged.
type validationSample struct {
	Family    instrument.ID
	Inst      instrument.ID
	RefID     string
	Class     verdictClass
	Diagnosis instrumentDiagnosis
}

// chosenMember returns the instrument whose sound the verdict describes.
//
// For a numbered pick that is the chosen option. For "neither" there is no
// single culprit, so the FIRST option stands in -- the listener rejected the
// whole family, which means every member failed, and option 1 is as
// representative as any. For a single-option family it is simply that member.
func chosenMember(v earVerdict) instrument.ID {
	if v.Chosen != "" {
		return v.Chosen
	}
	if len(v.Options) > 0 {
		return v.Options[0]
	}
	return v.Family
}

// staleness reports whether a recorded verdict's RecipeSHA still matches
// what the chosen instrument renders with today: "" (current), "unhashed"
// (recorded before RecipeSHA existed -- provenance unknown, not a lie), or
// "STALE" (the recipe has changed since the listener judged it, so the
// verdict no longer describes what's playing).
func staleness(v earVerdict) string {
	if v.RecipeSHA == "" {
		return "unhashed"
	}
	if audio.InstrumentSoundHash(chosenMember(v), nil) != v.RecipeSHA {
		return "STALE"
	}
	return ""
}

// collectValidationSamples diagnoses every recorded family against the SAME
// reference the listener judged it against.
//
// Using the verdict's own recorded reference rather than `refs compare`'s
// default candidate is essential: for a variant family the audition renders
// every member against one shared family reference, which need not be the
// instrument's own first candidate. The shipped cello is the worked case --
// the listener judged it at A3 against cello-A3, while `refs diagnose
// -instrument cello` defaults to cello-C2 two octaves down. Correlating a
// verdict about one render with a diagnosis of a different render would
// measure nothing.
func collectValidationSamples(diag io.Writer, verdicts []earVerdict) []validationSample {
	out := make([]validationSample, 0, len(verdicts))
	for _, v := range verdicts {
		inst := chosenMember(v)
		d, err := diagnoseInstrumentDur(io.Discard, inst, v.Reference, 0)
		if err != nil {
			fmt.Fprintf(diag, "skipping %s (%s vs %s): %v\n", v.Family, inst, v.Reference, err)
			continue
		}
		out = append(out, validationSample{
			Family: v.Family, Inst: inst, RefID: v.Reference,
			Class: classify(v.Verdict), Diagnosis: d,
		})
	}
	return out
}

// scoreAxes turns the per-instrument diagnoses into one row per descriptor.
func scoreAxes(samples []validationSample) []axisScore {
	type bucket struct {
		id                    string
		group                 string
		demoted               bool
		good, mid, bad        []float64
		mismatches, total     int
		goodFires, goodMeasur int
		unavailable           int
	}
	byAxis := map[string]*bucket{}
	order := []string{}

	for _, s := range samples {
		for _, a := range s.Diagnosis.Axes {
			b, ok := byAxis[a.Name]
			if !ok {
				b = &bucket{id: a.ID, group: a.Group}
				if d, found := fingerprint.DefaultRegistry().ByID(a.ID); found {
					b.demoted = d.Demoted
				}
				byAxis[a.Name] = b
				order = append(order, a.Name)
			}
			if a.RefNA != "" || a.SynthNA != "" {
				b.unavailable++
				continue
			}
			b.total++
			if a.Verdict == verdictMismatch {
				b.mismatches++
			}
			switch s.Class {
			case classGood:
				b.good = append(b.good, a.Norm)
				b.goodMeasur++
				if a.Norm >= 1 {
					b.goodFires++
				}
			case classBad:
				b.bad = append(b.bad, a.Norm)
			default:
				b.mid = append(b.mid, a.Norm)
			}
		}
	}

	out := make([]axisScore, 0, len(order))
	for _, name := range order {
		b := byAxis[name]
		a, ok := auc(b.good, b.bad)
		sc := axisScore{
			ID: b.id, Axis: name, Group: b.group, FlagDemoted: b.demoted,
			NGoodMeasured: b.goodMeasur,
			NGood:         len(b.good), NMid: len(b.mid), NBad: len(b.bad),
			MeanGood: mean(b.good), MeanMid: mean(b.mid), MeanBad: mean(b.bad),
			AUC: a, Comparable: ok, SE: aucStandardError(a, len(b.good), len(b.bad)),
			NMeasured: b.total, NUnavailable: b.unavailable,
		}
		if b.total > 0 {
			sc.MismatchRate = float64(b.mismatches) / float64(b.total)
		}
		if b.goodMeasur > 0 {
			sc.GoodFiresRate = float64(b.goodFires) / float64(b.goodMeasur)
		}
		out = append(out, sc)
	}
	// Most discriminating first; demoted axes sink below the trusted set and
	// incomparable ones to the bottom. The top of the table is meant to be
	// readable as "the axes to believe", so an axis that has been excluded
	// from that set must not sit in it however well it happens to order this
	// particular corpus.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Comparable != out[j].Comparable {
			return out[i].Comparable
		}
		if out[i].Demoted() != out[j].Demoted() {
			return !out[i].Demoted()
		}
		return out[i].AUC > out[j].AUC
	})
	return out
}

// scorePreferencesFromNorms is scorePreferences's pure counting core: given,
// for each pair (by index), the chosen and rejected clip's per-descriptor
// Norm map (nil when that clip could not be diagnosed), it counts, per
// descriptor ID, the pairs where the chosen clip's Norm read strictly lower
// (closer to the reference) than the rejected clip's. Split out from
// scorePreferences so the counting rule is testable against precomputed norm
// maps, without rendering or diagnosing anything.
//
// This is a preference test, distinct from scoreAxes's AUC: scoreAxes asks
// "does this axis order GOOD families ahead of BAD ones", using one absolute
// verdict per family. This asks "within a single head-to-head comparison,
// does this axis agree with which clip the listener preferred", using every
// relative preference the corpus records (numbered picks from
// ear_verdicts.jsonl, plus the richer better/worse/rating comparisons from
// ear_sessions.jsonl). An axis can pass one test and fail the other; both are
// reported.
func scorePreferencesFromNorms(pairs []prefPair, chosenNorms, rejectedNorms []map[string]float64) map[string][2]int {
	out := map[string][2]int{}
	for i := range pairs {
		ch, rj := chosenNorms[i], rejectedNorms[i]
		if ch == nil || rj == nil {
			continue
		}
		for id, cn := range ch {
			rn, ok := rj[id]
			if !ok {
				continue
			}
			e := out[id]
			e[1]++
			if cn < rn {
				e[0]++
			}
			out[id] = e
		}
	}
	return out
}

// scorePreferences diagnoses both clips of every pair against the pair's
// reference, then delegates the actual agreement counting to
// scorePreferencesFromNorms.
func scorePreferences(diag io.Writer, pairs []prefPair) map[string][2]int {
	normsOf := func(p prefPair, c sessionClip) map[string]float64 {
		inst := c.renderInst()
		d, err := diagnoseInstrumentOpts(io.Discard, inst, p.RefID, 0, c.Overrides)
		if err != nil {
			fmt.Fprintf(diag, "pref: skipping %s/%s: %v\n", p.Family, c.ID, err)
			return nil
		}
		m := map[string]float64{}
		for _, a := range d.Axes {
			if a.ID != "" && a.RefNA == "" && a.SynthNA == "" {
				m[a.ID] = a.Norm
			}
		}
		return m
	}
	chosenNorms := make([]map[string]float64, len(pairs))
	rejectedNorms := make([]map[string]float64, len(pairs))
	for i, p := range pairs {
		chosenNorms[i] = normsOf(p, p.Chosen)
		rejectedNorms[i] = normsOf(p, p.Rejected)
	}
	return scorePreferencesFromNorms(pairs, chosenNorms, rejectedNorms)
}

// mergePreferenceScores copies each axis's preference-pair tally (from
// scorePreferences/scorePreferencesFromNorms, keyed by descriptor ID) onto
// the matching axisScore's PrefAgree/PrefTotal fields. An axis absent from
// prefByID (no preference pair ever measured it) is left at its zero value,
// which the PREF column renders as "-".
func mergePreferenceScores(scores []axisScore, prefByID map[string][2]int) []axisScore {
	for i := range scores {
		if pt, ok := prefByID[scores[i].ID]; ok {
			scores[i].PrefAgree, scores[i].PrefTotal = pt[0], pt[1]
		}
	}
	return scores
}

// binomialPMF is P(X=k) for X ~ Binomial(n, 0.5), computed from the log
// binomial coefficient (math.Lgamma) rather than raw factorials so it stays
// accurate at realistic corpus sizes without overflowing.
func binomialPMF(k, n int) float64 {
	if k < 0 || k > n {
		return 0
	}
	logNFact, _ := math.Lgamma(float64(n + 1))
	logKFact, _ := math.Lgamma(float64(k + 1))
	logNKFact, _ := math.Lgamma(float64(n - k + 1))
	return math.Exp(logNFact-logKFact-logNKFact) * math.Pow(0.5, float64(n))
}

// signTestP is the two-sided exact binomial sign-test p-value for observing
// `agree` agreements out of `total` preference pairs under the null
// hypothesis that the axis is no better than a coin flip (p=0.5): twice the
// smaller of the two tail probabilities P(X<=agree) and P(X>=agree), capped
// at 1. An exact test rather than a normal approximation, because `total` is
// routinely single digits here, where the approximation is unreliable.
func signTestP(agree, total int) float64 {
	if total <= 0 {
		return math.NaN()
	}
	var pLE, pGE float64
	for k := 0; k <= agree; k++ {
		pLE += binomialPMF(k, total)
	}
	for k := agree; k <= total; k++ {
		pGE += binomialPMF(k, total)
	}
	p := 2 * math.Min(pLE, pGE)
	if p > 1 {
		p = 1
	}
	return p
}

// Verdict bands for an axis's AUC.
const (
	aucPredictive = 0.70 // orders good-before-bad well enough to trust
	aucWeak       = 0.55 // better than chance, but not by much
	aucAnti       = 0.45 // below this it is actively pointing the wrong way
)

// aucLabel bands an axis, but only ever asserts a DIRECTION the sample size can
// support. A high AUC whose interval still covers 0.5 is reported as
// "unproven", not as predictive -- the whole point of this command is to say
// what may be believed, so it must not be the first thing to overclaim.
func aucLabel(s axisScore) string {
	if !s.Comparable {
		return "n/a"
	}
	switch {
	case s.AUC >= aucPredictive && significantlyBetterThanChance(s.AUC, s.SE):
		return "PREDICTIVE"
	case s.AUC >= aucWeak && significantlyBetterThanChance(s.AUC, s.SE):
		return "weak"
	case s.AUC < aucAnti && significantlyWorseThanChance(s.AUC, s.SE):
		return "ANTI"
	case s.AUC >= aucPredictive:
		return "unproven+"
	case s.AUC < aucAnti:
		return "unproven-"
	default:
		return "noise"
	}
}

// saturatedRate is the mismatch rate above which an axis is flagged as firing
// on nearly everything: it can no longer rank instruments against each other,
// whatever its AUC says.
const saturatedRate = 0.85

// prefPredictiveMinTotal/prefPredictiveMinRate gate the PREDICTIVE (pref) flag:
// an axis needs at least this many measured preference pairs, and to agree
// with the listener's choice on at least this share of them, before the
// preference test will vouch for it.
//
// 6 is the smallest sample at which the exact two-sided sign-test p-value
// (signTestP) can clear the conventional p<0.05 significance bar at all: with
// n=6, 6/6 agreements gives p=2*0.5^6=0.03125, just under 0.05, while n=5
// tops out at p=2*0.5^5=0.0625 -- 5/5 can never be significant, whatever the
// rate. 0.75 (three-in-four) is a deliberately blunt "clearly more than a
// coin flip" bar, the same kind of round-number choice goodFiresDemoteRate
// makes for the good-group self-demotion rule below, rather than tuning a
// second rate to the letter on a corpus this small. Both this rate and the
// minimum sample size will tighten as the preference corpus (numbered-pick
// verdicts plus recorded sessions) grows.
const (
	prefPredictiveMinTotal = 6
	prefPredictiveMinRate  = 0.75
)

// goodFiresDemoteRate is the good-group fires-rate above which an axis
// demotes itself.
//
// Half. An axis that flags more than half the families a listener called
// right is not measuring rightness; whether its AUC happens to order the
// corpus is beside the point, because acting on it would mean "fixing"
// instruments that are already correct. The threshold is deliberately blunt:
// the good group is 8 families, so anything finer than a half would be
// reading noise.
const goodFiresDemoteRate = 0.5

// Demoted reports whether the axis is excluded from the trusted set, for
// either reason.
func (s axisScore) Demoted() bool {
	return s.FlagDemoted || (s.NGoodMeasured > 0 && s.GoodFiresRate > goodFiresDemoteRate)
}

// DemotionReason names which rule fired, for the report footer.
func (s axisScore) DemotionReason() string {
	switch {
	case s.FlagDemoted:
		return "registry flag"
	case s.NGoodMeasured > 0 && s.GoodFiresRate > goodFiresDemoteRate:
		return "fires on the good group"
	default:
		return ""
	}
}

// axisFlag renders the VERDICT column.
//
// A demoted axis prints DEMOTED and NOTHING ELSE from the AUC vocabulary. It
// used to be downgraded to "unproven+", which was worse than useless: every
// word in that vocabulary makes a claim about how far to trust the axis, and
// demotion is the statement that none of them applies. The AUC and its
// interval are still in their own columns for anyone who wants to look.
//
// SATURATED still appends, because it names a different failure (cannot rank)
// than demotion (must not be trusted) and an axis can have both.
//
// PREDICTIVE (pref) is a THIRD, independent signal from the preference test
// (scorePreferences): it appends when there are at least prefPredictiveMinTotal
// pairs measured on this axis and the chosen clip read lower at least
// prefPredictiveMinRate of the time. It is not folded into aucLabel's own
// PREDICTIVE because the two tests use different corpora (absolute
// good/bad-family verdicts vs. head-to-head preference pairs) and an axis can
// earn one without the other.
func axisFlag(s axisScore) string {
	flag := aucLabel(s)
	if s.Demoted() {
		flag = "DEMOTED"
	}
	if s.MismatchRate >= saturatedRate {
		flag += " SATURATED"
	}
	if s.PrefTotal >= prefPredictiveMinTotal && float64(s.PrefAgree)/float64(s.PrefTotal) >= prefPredictiveMinRate {
		flag += " PREDICTIVE (pref)"
	}
	return flag
}

// calLabel renders the CAL column: where this descriptor's drift/mismatch
// thresholds came from. "hand" is a real state, not a blank -- a reader has
// to be able to tell a threshold nobody has validated from one the corpus or
// a listener produced.
func calLabel(id string) string {
	if src := fingerprint.DefaultRegistry().CalibratedFrom(id); src != "" {
		return src
	}
	return "hand"
}

func runValidate(w, diag io.Writer, outPath, sessionsPath string, minSamples int) error {
	verdicts, err := readVerdicts(outPath)
	if err != nil {
		return err
	}
	if len(verdicts) == 0 {
		return fmt.Errorf("refs validate: no verdicts in %s -- run `make ab-all` first", outPath)
	}
	samples := collectValidationSamples(diag, verdicts)
	if len(samples) == 0 {
		return fmt.Errorf("refs validate: no family could be diagnosed")
	}

	sessions, _ := readSessions(sessionsPath)
	pairs := append(prefPairsFromVerdicts(verdicts), prefPairsFromSessions(sessions)...)
	prefByID := scorePreferences(diag, pairs)

	var nGood, nMid, nBad int
	for _, s := range samples {
		switch s.Class {
		case classGood:
			nGood++
		case classBad:
			nBad++
		default:
			nMid++
		}
	}
	fmt.Fprintf(w, "%d families diagnosed against the reference the listener judged them on: "+
		"%d good (y), %d mid (p / numbered pick), %d bad (n)\n\n", len(samples), nGood, nMid, nBad)

	var nStale, nUnhashed int
	var staleFamilies []string
	for _, v := range verdicts {
		switch staleness(v) {
		case "STALE":
			nStale++
			staleFamilies = append(staleFamilies, string(v.Family))
		case "unhashed":
			nUnhashed++
		}
	}
	fmt.Fprintf(w, "%d families STALE (recipe changed since the verdict), %d unhashed (pre-provenance)\n", nStale, nUnhashed)
	if nStale > 0 {
		fmt.Fprintf(w, "STALE: %s\n", strings.Join(staleFamilies, ", "))
	}
	fmt.Fprintln(w)

	if nGood < minSamples || nBad < minSamples {
		fmt.Fprintf(w, "WARNING: only %d good and %d bad families -- AUC over samples this small is\n", nGood, nBad)
		fmt.Fprintf(w, "indicative, not conclusive. Treat the ordering as a hypothesis to listen against.\n\n")
	}

	scores := mergePreferenceScores(scoreAxes(samples), prefByID)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tAUC\t95% CI\tVERDICT\tFIRES\tGOOD-FIRES\tPREF\tmean(good)\tmean(bad)\tn/a\tCAL\tGROUP")
	for _, s := range scores {
		aucStr, ciStr := "  -  ", "     -     "
		if s.Comparable {
			aucStr = fmt.Sprintf("%.2f", s.AUC)
			lo := math.Max(0, s.AUC-1.96*s.SE)
			hi := math.Min(1, s.AUC+1.96*s.SE)
			ciStr = fmt.Sprintf("%.2f-%.2f", lo, hi)
		}
		goodFires := "  - "
		if s.NGoodMeasured > 0 {
			goodFires = fmt.Sprintf("%.0f%%", s.GoodFiresRate*100)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.0f%%\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			s.Axis, aucStr, ciStr, axisFlag(s), s.MismatchRate*100, goodFires, prefCell(s),
			meanCell(s.MeanGood), meanCell(s.MeanBad), s.NUnavailable, calLabel(s.ID), s.Group)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(w, "\nAUC = P(a bad-sounding instrument scores worse on this axis than a good-sounding one).")
	fmt.Fprintf(w, "  >= %.2f PREDICTIVE -- trust this axis when tuning (and the CI clears chance)\n", aucPredictive)
	fmt.Fprintf(w, "  >= %.2f weak       -- better than chance; corroborate before acting\n", aucWeak)
	fmt.Fprintf(w, "  >= %.2f noise      -- carries no usable signal on this corpus\n", aucAnti)
	fmt.Fprintf(w, "  <  %.2f ANTI       -- ordered BACKWARDS; acting on it makes instruments worse\n", aucAnti)
	fmt.Fprintln(w, "  unproven+/-       -- points that way, but the 95% CI still covers 0.50: NOT established")
	fmt.Fprintf(w, "SATURATED = fires on >=%.0f%% of instruments, so it cannot rank them regardless of AUC.\n", saturatedRate*100)
	fmt.Fprintf(w, "DEMOTED   = excluded from the trusted set, and its AUC band withdrawn. Either the registry\n")
	fmt.Fprintf(w, "            flags it, or it fires on >%.0f%% of the families the LISTENER accepted -- an axis that\n", goodFiresDemoteRate*100)
	fmt.Fprintf(w, "            calls a defect where a human heard none is not measuring what makes a sound right.\n")
	fmt.Fprintln(w, "GOOD-FIRES = share of GOOD-graded families where this axis reads at/past mismatch.")
	fmt.Fprintln(w, "\nCAL = where this axis's drift/mismatch thresholds came from:")
	fmt.Fprintln(w, "  corpus -- p50/p90 of the good group (refs calibrate-thresholds)")
	fmt.Fprintln(w, "  probe  -- a listener-measured audibility step (refs probe)")
	fmt.Fprintln(w, "  hand   -- a hand-chosen guess, or a deliberate exclusion from calibration")
	fmt.Fprintln(w, "\n'FIRES' is the MISMATCH rate over every measured instrument, not just good/bad ones.")
	fmt.Fprintln(w, "PREF = AGREE/TOTAL over head-to-head preference pairs (numbered picks from verdicts, plus")
	fmt.Fprintln(w, "       better/worse/rating comparisons from sessions): how often the clip the listener")
	fmt.Fprintln(w, "       preferred read LOWER (closer to the reference) on this axis than the one they rejected.")
	fmt.Fprintln(w, "       p=... is the two-sided exact binomial sign-test p-value for that AGREE/TOTAL under")
	fmt.Fprintln(w, "       the null of no preference (p=0.5) -- small p means the agreement rate is unlikely by")
	fmt.Fprintf(w, "       chance alone. PREDICTIVE (pref) requires TOTAL>=%d and AGREE/TOTAL>=%.2f.\n",
		prefPredictiveMinTotal, prefPredictiveMinRate)
	fmt.Fprintf(w, "       \u2193backwards marks AGREE/TOTAL < %.2f at p < %.2f: the axis tracks the listener's\n",
		prefBackwardsMaxRate, prefBackwardsMaxP)
	fmt.Fprintln(w, "       preference RELIABLY, in the opposite direction. Do not read a small fraction with a")
	fmt.Fprintln(w, "       small p as a weak result -- it is a strong one pointing the wrong way.")
	fmt.Fprintln(w, "mean(good)/mean(bad) print '-' when that group is empty; no mean exists to report.")
	return nil
}

// sessionScaleHit is one (session candidate, scale answer) evaluation against
// the axis the scale names -- the -sessions-table analogue of validate_terms's
// termHit.
type sessionScaleHit struct {
	Family, Candidate, Q string
	Answer               int
	WantDir, GotDir      string
	Agree, Measured      bool
}

// sessionScaleScore aggregates every scale answer recorded against one axis.
type sessionScaleScore struct {
	Axis         string
	Agree, Total int
	Hits         []sessionScaleHit
}

// scaleDirection is one axis's diagnosed Direction plus whether it was
// measurable at all -- the per-clip input scoreCandidateScales needs to score
// one candidate's scale answers without re-deriving it from a full
// instrumentDiagnosis.
type scaleDirection struct {
	Direction  string
	Measurable bool
}

// scoreCandidateScales is scoreSessionScales's pure per-candidate counting
// core: given one candidate's answered scales and its diagnosed axis
// directions (keyed by descriptor ID), it appends one sessionScaleHit per
// non-zero answer to byAxis/order and updates that axis's running
// agree/total tally. Split out from scoreSessionScales so the sign-agreement
// rule -- Answer > 0 ("synth should read HIGH on this axis") agrees with a
// measured Direction of "high", Answer < 0 agrees with "low", and Answer == 0
// ("no opinion") is skipped entirely, neither hit nor tally -- is testable
// against a precomputed direction map, without diagnosing anything.
func scoreCandidateScales(family instrument.ID, candID string, scales []sessionScale, directions map[string]scaleDirection, byAxis map[string]*sessionScaleScore, order *[]string) {
	for _, sc := range scales {
		if sc.Answer == 0 {
			continue
		}
		agg, ok := byAxis[sc.Axis]
		if !ok {
			agg = &sessionScaleScore{Axis: sc.Axis}
			byAxis[sc.Axis] = agg
			*order = append(*order, sc.Axis)
		}
		wantDir := "low"
		if sc.Answer > 0 {
			wantDir = "high"
		}
		h := sessionScaleHit{Family: string(family), Candidate: candID, Q: sc.Q, Answer: sc.Answer, WantDir: wantDir}

		d, found := directions[sc.Axis]
		switch {
		case !found || !d.Measurable:
			h.GotDir = "n/a"
		default:
			agg.Total++
			h.Measured = true
			h.GotDir = d.Direction
			if d.Direction == wantDir {
				h.Agree = true
				agg.Agree++
			}
		}
		agg.Hits = append(agg.Hits, h)
	}
}

// scoreSessionScales diagnoses every session candidate that carries at least
// one answered sessionScale, then delegates the counting to
// scoreCandidateScales. A candidate is diagnosed once and its answers checked
// against that single diagnosis, rather than once per scale, since multiple
// scales commonly name axes on the same clip.
func scoreSessionScales(diag io.Writer, sessions []earSession) []sessionScaleScore {
	byAxis := map[string]*sessionScaleScore{}
	var order []string

	for _, s := range sessions {
		for _, c := range s.Candidates {
			hasScale := false
			for _, sc := range c.Scales {
				if sc.Answer != 0 {
					hasScale = true
					break
				}
			}
			if !hasScale {
				continue
			}
			d, err := diagnoseInstrumentOpts(io.Discard, c.renderInst(), s.Reference, 0, c.Overrides)
			if err != nil {
				fmt.Fprintf(diag, "sessions: skipping %s/%s: %v\n", s.Family, c.ID, err)
				continue
			}
			directions := make(map[string]scaleDirection, len(d.Axes))
			for _, a := range d.Axes {
				directions[a.ID] = scaleDirection{Direction: a.Direction, Measurable: a.RefNA == "" && a.SynthNA == ""}
			}
			scoreCandidateScales(s.Family, c.ID, c.Scales, directions, byAxis, &order)
		}
	}

	sort.Strings(order)
	out := make([]sessionScaleScore, 0, len(order))
	for _, id := range order {
		out = append(out, *byAxis[id])
	}
	return out
}

// runValidateSessions is `refs validate -sessions-table`: does the
// perceptual-scale answer a listener gave in a session agree with the axis it
// names?
func runValidateSessions(w, diag io.Writer, sessionsPath string) error {
	sessions, err := readSessions(sessionsPath)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return fmt.Errorf("refs validate -sessions-table: no sessions in %s -- record a session first", sessionsPath)
	}

	fmt.Fprintln(w, "Does each perceptual-scale answer agree with the axis it names?")
	fmt.Fprintln(w, "AGREE/TOTAL counts answers where the diagnosed Direction on that candidate matched")
	fmt.Fprintln(w, "what the answer implied (answer>0 => synth should read HIGH; answer<0 => LOW).")
	fmt.Fprintln(w)

	scores := scoreSessionScales(diag, sessions)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AXIS\tAGREE\tTOTAL")
	for _, sc := range scores {
		fmt.Fprintf(tw, "%s\t%d\t%d\n", sc.Axis, sc.Agree, sc.Total)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	fmt.Fprintln(w, "\nPer-row detail:")
	for _, sc := range scores {
		fmt.Fprintf(w, "\n  %s\n", sc.Axis)
		for _, h := range sc.Hits {
			mark := "MISS"
			switch {
			case h.GotDir == "n/a":
				mark = "n/a "
			case h.Agree:
				mark = "OK  "
			case h.Measured:
				mark = "BACKWARDS"
			}
			fmt.Fprintf(w, "    %-9s %-16s %-8s q=%-24q answer=%+d want=%-4s got=%s\n",
				mark, h.Family, h.Candidate, h.Q, h.Answer, h.WantDir, h.GotDir)
		}
	}
	return nil
}

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	out := fs.String("out", "", "verdicts jsonl path (default: internal/audio/testdata/ear_verdicts.jsonl)")
	sessionsFlag := fs.String("sessions", "", "sessions jsonl path (default: internal/audio/testdata/ear_sessions.jsonl)")
	terms := fs.Bool("terms", false, "instead of the AUC table, check whether each perceptual axis agrees with the WORDS the listener used")
	sessionsTable := fs.Bool("sessions-table", false, "instead of the AUC table, check whether each session's perceptual-scale answers agree with the axis they name")
	minSamples := fs.Int("min-samples", 5, "warn when either the good or bad class has fewer than this many families")
	loss := fs.Bool("loss", false, "instead of the AUC table, score each WAV-clone loss term (and fingerprint.Distance) against the within-family preference pairs")
	if err := fs.Parse(args); err != nil {
		return err
	}
	outPath := *out
	if outPath == "" {
		p, err := defaultVerdictsPath()
		if err != nil {
			return err
		}
		outPath = p
	}
	sessionsPath := *sessionsFlag
	if sessionsPath == "" {
		p, err := defaultSessionsPath()
		if err != nil {
			return err
		}
		sessionsPath = p
	}
	if *terms {
		return runValidateTerms(os.Stdout, os.Stderr, outPath)
	}
	if *loss {
		return runValidateLoss(os.Stdout, os.Stderr, outPath, sessionsPath, filepath.Join(wavref.Root(), "clones"))
	}
	if *sessionsTable {
		return runValidateSessions(os.Stdout, os.Stderr, sessionsPath)
	}
	return runValidate(os.Stdout, os.Stderr, outPath, sessionsPath, *minSamples)
}
