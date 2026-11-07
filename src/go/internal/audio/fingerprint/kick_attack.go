package fingerprint

import "math"

// A 10–90 % rise saturates for sharp onsets (a 1 ms click and a 5 ms thump both read fast), so MPEG-7's log attack
// time (log10 of start → peak, start at 2 % of peak) and temporal centroid (energy-weighted mean time) describe the
// transient on a perceptual axis.

const (
	kickAttackStartFrac = 0.02

	// 1e-4 s floors LogAttackTime at −4.
	kickAttackMinDurSec = 1e-4

	// Epsilon (seconds) guarding the TemporalCentroid ratio in the distance term.
	kickAttackEps = 1e-3
)

// The same envelope framing as computeEnvelope.
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
