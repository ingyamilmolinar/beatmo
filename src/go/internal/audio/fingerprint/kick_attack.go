package fingerprint

import "math"

// kick_attack.go — MPEG-7 LOG-ATTACK-TIME + TEMPORAL CENTROID.
//
// Motivation: AttackRiseSec (a 10%→90% rise time) is a linear-domain landmark and
// saturates for very sharp onsets (both a 1 ms click and a 5 ms thump read "fast").
// The MPEG-7 descriptors capture the transient character on a perceptual (log) time
// axis and describe where the signal's energy sits in time — two axes the existing
// envelope landmarks compress away.
//
//   - LogAttackTime = log10(max(t_peak − t_start, 1e-4)), where t_start is the
//     first time the RMS envelope exceeds a small fraction (MPEG-7: 2%) of its peak
//     and t_peak is the envelope-peak time (seconds, log10 → NEGATIVE). A sharp
//     click → very negative (~−4); a slow swell → higher (less negative).
//
//   - TemporalCentroidSec = Σ(t·env(t)) / Σ(env(t)) over the whole signal — the
//     energy-weighted mean time. A front-loaded transient → small; a long
//     sustain/tail → larger.
//
// Reference: MPEG-7 Audio (ISO/IEC 15938-4) LogAttackTime + TemporalCentroid
// low-level descriptors.

const (
	// MPEG-7 start threshold: the attack begins where the envelope first reaches
	// this fraction of its peak.
	kickAttackStartFrac = 0.02

	// Floor for the attack duration before taking log10 (avoids log10(0) for a
	// single-hop attack). 1e-4 s → LogAttackTime floor of −4.
	kickAttackMinDurSec = 1e-4

	// Epsilon (seconds) guarding the TemporalCentroid ratio in the distance term.
	kickAttackEps = 1e-3
)

// computeAttack fills fp.LogAttackTime + fp.TemporalCentroidSec from the RMS
// envelope. Mirrors the computeX pattern in kick_metrics.go and reuses the same
// envelope framing as computeEnvelope.
func (fp *KickFingerprint) computeAttack(samples []float64, sr int) {
	win := secToSamples(kickEnvWindowSec, sr)
	hop := secToSamples(kickEnvHopSec, sr)
	env := kickRMSEnvelope(samples, win, hop)
	if len(env) == 0 {
		return
	}
	hopSec := float64(hop) / float64(sr)

	peakIdx, peakVal := argmax(env)
	if peakVal <= 0 {
		return
	}

	// t_start: first envelope sample reaching 2% of peak.
	startIdx := 0
	thr := kickAttackStartFrac * peakVal
	for i := 0; i < len(env); i++ {
		if env[i] >= thr {
			startIdx = i
			break
		}
	}
	tStart := float64(startIdx) * hopSec
	tPeak := float64(peakIdx) * hopSec
	fp.LogAttackTime = math.Log10(math.Max(tPeak-tStart, kickAttackMinDurSec))

	// TemporalCentroid over the whole envelope.
	var num, den float64
	for i, e := range env {
		num += float64(i) * hopSec * e
		den += e
	}
	if den > 0 {
		fp.TemporalCentroidSec = num / den
	}
}
