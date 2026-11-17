package fingerprint

import "math"

// classify.go — the verdict classifiers, moved VERBATIM out of
// cmd/refs/diagnose.go so the registry can own classification.
//
// Bodies are unchanged apart from the axisVerdict -> Verdict rename and
// taking thresholds as parameters instead of reading package-level constants
// in cmd/refs. That verbatim-ness is deliberate and load-bearing: it is what
// guarantees cmd/refs/diagnose_acceptance_test.go's ten-instrument gate keeps
// producing identical verdicts across the refactor. Do not "clean up" the
// logic here without re-running that gate.

// classifyFraction returns the verdict and direction for a fraction-style
// axis, using the (driftTh, mismatchTh) pair appropriate to that axis's
// natural scale — see Kind.Thresholds.
func classifyFraction(ref, synth, driftTh, mismatchTh float64) (Verdict, string) {
	d := synth - ref
	a := math.Abs(d)
	v := VerdictOK
	switch {
	case a >= mismatchTh:
		v = VerdictMismatch
	case a >= driftTh:
		v = VerdictDrift
	}
	dir := ""
	if v != VerdictOK {
		if d > 0 {
			dir = "high"
		} else {
			dir = "low"
		}
	}
	return v, dir
}

// classifySeconds returns the verdict and direction for a duration-style
// axis. Handles the degenerate ref==0 or synth==0 case (silence / no event on
// one side) as an automatic MISMATCH rather than a divide-by-zero.
func classifySeconds(ref, synth, driftRatio, mismatchRatio float64) (Verdict, string) {
	if ref <= 0 && synth <= 0 {
		return VerdictOK, ""
	}
	if ref <= 0 || synth <= 0 {
		dir := "high"
		if synth < ref {
			dir = "low"
		}
		return VerdictMismatch, dir
	}
	ratio := synth / ref
	tooShort := ratio < 1
	if tooShort {
		ratio = 1 / ratio
	}
	v := VerdictOK
	switch {
	case ratio >= mismatchRatio:
		v = VerdictMismatch
	case ratio >= driftRatio:
		v = VerdictDrift
	}
	dir := ""
	if v != VerdictOK {
		if tooShort {
			dir = "low"
		} else {
			dir = "high"
		}
	}
	return v, dir
}

// classifyHz returns the verdict and direction for a frequency-style axis,
// compared as a signed percentage of the reference.
func classifyHz(ref, synth, driftPct, mismatchPct float64) (Verdict, string) {
	if ref <= 0 {
		if synth <= 0 {
			return VerdictOK, ""
		}
		return VerdictMismatch, "high"
	}
	pct := (synth - ref) / ref * 100
	a := math.Abs(pct)
	v := VerdictOK
	switch {
	case a >= mismatchPct:
		v = VerdictMismatch
	case a >= driftPct:
		v = VerdictDrift
	}
	dir := ""
	if v != VerdictOK {
		if pct > 0 {
			dir = "high"
		} else {
			dir = "low"
		}
	}
	return v, dir
}

// decayRateFlatEps: a |dB/s| below this is treated as "no measurable decay
// trend at all" (a perfectly flat/sustained envelope) rather than a tiny but
// real slope, since the OLS fit over RMS-envelope hops carries measurement
// noise at this scale.
const decayRateFlatEps = 1e-6

// ClassifyDecayRate is the escalation classifier for the
// both-sides-RingT60-extrapolated case: it compares ref/synth
// EnvelopeSegments.OverallDecayRateDBPerSec via the RATIO OF IMPLIED T60s
// each rate predicts. A constant rate r implies T60 = 60/|r| seconds, so the
// ratio of implied T60s (synth/ref) equals |ref|/|synth| — the same
// seconds thresholds therefore apply directly to the rate magnitudes, just
// inverted (a bigger decay-rate magnitude implies a SHORTER T60).
//
// Exported because the ring-T60 descriptor uses it as a custom Compare
// override rather than the renderer special-casing it.
func ClassifyDecayRate(refRate, synthRate, driftRatio, mismatchRatio float64) (Verdict, string) {
	refAbs, synthAbs := math.Abs(refRate), math.Abs(synthRate)
	switch {
	case refAbs < decayRateFlatEps && synthAbs < decayRateFlatEps:
		return VerdictOK, ""
	case synthAbs < decayRateFlatEps:
		// synth shows literally no decay trend while ref does — synth's
		// implied ring never fades, i.e. far too long/sustained.
		return VerdictMismatch, "high"
	case refAbs < decayRateFlatEps:
		// ref shows no decay trend but synth does — synth is, relatively,
		// the one that fades (too short).
		return VerdictMismatch, "low"
	}
	t60Ratio := refAbs / synthAbs // == impliedSynthT60 / impliedRefT60
	tooShort := t60Ratio < 1
	cmp := t60Ratio
	if tooShort {
		cmp = 1 / cmp
	}
	v := VerdictOK
	switch {
	case cmp >= mismatchRatio:
		v = VerdictMismatch
	case cmp >= driftRatio:
		v = VerdictDrift
	}
	dir := ""
	if v != VerdictOK {
		if tooShort {
			dir = "low"
		} else {
			dir = "high"
		}
	}
	return v, dir
}
