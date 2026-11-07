package fingerprint

import "math"

// Moved verbatim so the registry owns classification; only thresholds became parameters. Keep the logic as is
// unless the diagnose acceptance gate is re-run.

// Thresholds on the axis's natural scale (Kind.Thresholds).
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

// ref==0 or synth==0 (no event on one side) is an automatic MISMATCH, not a divide by zero.
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

// A signed percentage of the reference.
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

// Below this a slope is "no measurable trend": the OLS fit over envelope hops carries noise at this scale.
const decayRateFlatEps = 1e-6

// For two extrapolated ring T60s, compares the decay RATES through the T60s they imply: T60 = 60/|r|, so the
// seconds thresholds apply to |ref|/|synth| directly (a larger rate means a SHORTER T60). Exported for the ring-T60
// descriptor's Compare override.
func ClassifyDecayRate(refRate, synthRate, driftRatio, mismatchRatio float64) (Verdict, string) {
	refAbs, synthAbs := math.Abs(refRate), math.Abs(synthRate)
	switch {
	case refAbs < decayRateFlatEps && synthAbs < decayRateFlatEps:
		return VerdictOK, ""
	case synthAbs < decayRateFlatEps:
		// Synth never fades while ref does: far too long.
		return VerdictMismatch, "high"
	case refAbs < decayRateFlatEps:
		// Ref never fades while synth does: synth is the short one.
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
