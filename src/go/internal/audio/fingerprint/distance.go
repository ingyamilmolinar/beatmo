package fingerprint

import "math"

// Score holds the per-term and total weighted distance between two Fingerprints.
// Lower values indicate more similar timbres. All terms are normalized to ~[0,1]
// for a typical mismatch before weighting.
type Score struct {
	Spectral    float64 // spectral-shape term (centroid, rolloff, LSD, convergence)
	MFCC        float64 // perceptual timbre term (coefficients 1..12)
	Partials    float64 // harmonic partial amplitudes term
	Temporal    float64 // STFT-evolution term (flux, centroid range, vibrato)
	Envelope    float64 // amplitude-envelope term (attack, decay, sustain, release, release-time)
	Noise       float64 // noise/flatness term
	Timbre      float64 // secondary timbre term (Tristimulus, SpectralCrest/Skewness/Kurtosis, HNR)
	Vibrato     float64 // vibrato term (rate, extent, AM depth)
	Coupling    float64 // dynamic coupling term (centroid-RMS correlation, brightness mod depth)
	Formant     float64 // spectral envelope term (bridge hill frequency and gain)
	HTrajectory float64 // harmonic trajectory term (per-harmonic decay rate)
	Total       float64 // weighted sum of all terms
}

// distanceWeights controls the relative contribution of each term.
// All weights are ~1.0 so the total is roughly the sum of equally important
// sub-distances. Spectral and MFCC are slightly higher because they capture
// the most perceptually salient timbre differences; Temporal is lower because
// short synthetic tones have little temporal evolution; Timbre is modest
// (0.5) because the metrics it covers (crest, HNR, Tristimulus) are partially
// redundant with Spectral and Partials — they add resolution without
// dominating the loss landscape, which keeps the optimizer convergence stable.
// Vibrato (0.8), Coupling (1.0), Formant (0.7), and HTrajectory (0.6) are
// the four new extended terms covering fine-grained timbral dimensions that
// supplement the existing spectral/temporal terms.
var distanceWeights = struct {
	Spectral, MFCC, Partials, Temporal, Envelope, Noise, Timbre float64
	Vibrato, Coupling, Formant, HTrajectory                     float64
}{
	Spectral:    1.2, // spectral centroid/rolloff/LSD/convergence — perceptually central
	MFCC:        1.2, // Mel-cepstrum — proven timbre proxy
	Partials:    1.0, // harmonic balance
	Temporal:    0.6, // STFT dynamics — reduced; synthetics have little temporal content
	Envelope:    1.0, // ADSR shape + release time
	Noise:       1.0, // tonality vs noise
	Timbre:      0.5, // secondary timbre: Tristimulus, SpectralCrest/Skewness/Kurtosis, HNR
	Vibrato:     0.8, // vibrato: rate, extent, AM depth
	Coupling:    1.0, // dynamic coupling: centroid-RMS correlation, brightness mod depth
	Formant:     0.7, // spectral envelope: bridge hill frequency and gain
	HTrajectory: 0.6, // harmonic trajectory: per-harmonic decay rates
}

// clamp1 clamps v to [0, 1].
func clamp1(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// absDelta returns |a - b|.
func absDelta(a, b float64) float64 {
	d := a - b
	if d < 0 {
		return -d
	}
	return d
}

// Distance returns the weighted, normalized distance between two fingerprints.
// Symmetric: Distance(a, b).Total == Distance(b, a).Total.
// Identity:  Distance(fp, fp).Total == 0.
// Lower values indicate more similar instruments.
func Distance(ref, cand Fingerprint) Score {
	var s Score

	// -------------------------------------------------------------------------
	// Spectral term: mean of sub-terms for centroid, rolloff, LSD, convergence.
	// Scale denominators:
	//   centroid  / 4000 Hz — typical centroid spans 0–4 kHz for most instruments
	//   rolloff   / 8000 Hz — rolloff spans 0–8 kHz for broadband content
	//   LSD       / 40 dB  — LSD rarely exceeds 40 dB for related timbres
	//   symConv   / 1      — SpectralConvergence is already dimensionless (~0..1+)
	// -------------------------------------------------------------------------
	{
		var terms []float64

		// Centroid sub-term.
		terms = append(terms, clamp1(absDelta(ref.SpectralCentroid, cand.SpectralCentroid)/4000.0))

		// Rolloff sub-term.
		terms = append(terms, clamp1(absDelta(ref.SpectralRolloff, cand.SpectralRolloff)/8000.0))

		// LSD and symmetrized convergence only when both SustainMag are available.
		if len(ref.SustainMag) > 0 && len(cand.SustainMag) > 0 {
			lsd := LogSpectralDistance(ref.SustainMag, cand.SustainMag)
			terms = append(terms, clamp1(lsd/40.0))

			// Symmetrized convergence: average both directions to ensure symmetry
			// because SpectralConvergence(a,b) != SpectralConvergence(b,a) in general.
			symConv := 0.5 * (SpectralConvergence(ref.SustainMag, cand.SustainMag) +
				SpectralConvergence(cand.SustainMag, ref.SustainMag))
			terms = append(terms, clamp1(symConv))
		}

		sum := 0.0
		for _, t := range terms {
			sum += t
		}
		if len(terms) > 0 {
			s.Spectral = sum / float64(len(terms))
		}
	}

	// -------------------------------------------------------------------------
	// MFCC term: L2 distance over coefficients 1..12 (skip c0 — energy offset).
	// Scale: ~50 — empirically derived from typical MFCC ranges for instrument
	// pairs; rare for L2 over 12 coeffs to exceed 50 for related timbres.
	// -------------------------------------------------------------------------
	{
		sumSq := 0.0
		for i := 1; i <= 12; i++ {
			d := ref.MFCC[i] - cand.MFCC[i]
			sumSq += d * d
		}
		s.MFCC = clamp1(math.Sqrt(sumSq) / 50.0)
	}

	// -------------------------------------------------------------------------
	// Partials term: mean absolute difference over all 16 partials.
	// Partials are normalized so Partials[0]==1 (or 0); differences are ~0..1.
	// -------------------------------------------------------------------------
	{
		sum := 0.0
		for k := 0; k < 16; k++ {
			sum += absDelta(ref.Partials[k], cand.Partials[k])
		}
		s.Partials = clamp1(sum / 16.0)
	}

	// -------------------------------------------------------------------------
	// Temporal term: mean of sub-terms from TemporalFingerprint.
	// If either fingerprint has no Temporal, the term is 0.
	// Scales:
	//   HarmonicFluxMean  / 0.1   — flux normalized by frame energy; >0.1 = high change
	//   CentroidRange     / 2000  Hz — centroid can wander up to 2 kHz between frames
	//   VibratoDepthCents / 50    — 50 cents is a very wide vibrato
	// -------------------------------------------------------------------------
	{
		if ref.Temporal != nil && cand.Temporal != nil {
			rt, ct := ref.Temporal, cand.Temporal
			terms := [3]float64{
				clamp1(absDelta(rt.HarmonicFluxMean, ct.HarmonicFluxMean) / 0.1),
				clamp1(absDelta(rt.CentroidRange, ct.CentroidRange) / 2000.0),
				clamp1(absDelta(rt.VibratoDepthCents, ct.VibratoDepthCents) / 50.0),
			}
			s.Temporal = (terms[0] + terms[1] + terms[2]) / 3.0
		}
		// else s.Temporal stays 0
	}

	// -------------------------------------------------------------------------
	// Envelope term: mean of sub-terms for ADSR shape + release time.
	// Scales:
	//   AttackTimeSec  / 0.2  s   — attacks rarely exceed 200 ms
	//   LogAttackTime  / 2        — log(attack) typically in [-5, 0]; span ~2
	//   DecaySlope     / 100  dB/s — steep decay ~100 dB/s
	//   SustainLevel   / 1        — already 0..1 relative to peak
	//   ReleaseTimeSec / 0.5  s   — release rarely exceeds 500 ms for synthetics
	// -------------------------------------------------------------------------
	{
		terms := [5]float64{
			clamp1(absDelta(ref.AttackTimeSec, cand.AttackTimeSec) / 0.2),
			clamp1(absDelta(ref.LogAttackTime, cand.LogAttackTime) / 2.0),
			clamp1(absDelta(ref.DecaySlope, cand.DecaySlope) / 100.0),
			clamp1(absDelta(ref.SustainLevel, cand.SustainLevel) / 1.0),
			clamp1(absDelta(ref.ReleaseTimeSec, cand.ReleaseTimeSec) / 0.5),
		}
		s.Envelope = (terms[0] + terms[1] + terms[2] + terms[3] + terms[4]) / 5.0
	}

	// -------------------------------------------------------------------------
	// Noise term: mean of NoiseRatio and SpectralFlatness deltas.
	// Both are already in ~[0,1] so no additional scaling needed.
	// -------------------------------------------------------------------------
	{
		n := clamp1(absDelta(ref.NoiseRatio, cand.NoiseRatio))
		f := clamp1(absDelta(ref.SpectralFlatness, cand.SpectralFlatness))
		s.Noise = (n + f) / 2.0
	}

	// -------------------------------------------------------------------------
	// Timbre term: secondary timbre descriptors that were previously computed
	// but not consumed by Distance. Each sub-term is an absolute delta so the
	// identity (Distance(fp,fp)==0) and symmetry invariants are preserved.
	// Scales:
	//   Tristimulus[i]   — already ~[0,1] (each is a fraction of partial energy)
	//   SpectralCrest    / 50    — crest can be large for tonal signals
	//   HNR              / 20    — HNR is in dB-ish; 20 dB covers typical range
	//   SpectralSkewness / 5     — skewness typically in [-3, 3] for audio
	//   SpectralKurtosis / 10    — kurtosis typically < 10 for typical spectra
	// -------------------------------------------------------------------------
	{
		// Tristimulus: mean absolute difference over 3 components.
		trisSum := 0.0
		for i := 0; i < 3; i++ {
			trisSum += clamp1(absDelta(ref.Tristimulus[i], cand.Tristimulus[i]))
		}
		trisTerm := trisSum / 3.0

		crestTerm := clamp1(absDelta(ref.SpectralCrest, cand.SpectralCrest) / 50.0)
		hnrTerm := clamp1(absDelta(ref.HNR, cand.HNR) / 20.0)
		skewTerm := clamp1(absDelta(ref.SpectralSkewness, cand.SpectralSkewness) / 5.0)
		kurtTerm := clamp1(absDelta(ref.SpectralKurtosis, cand.SpectralKurtosis) / 10.0)

		s.Timbre = (trisTerm + crestTerm + hnrTerm + skewTerm + kurtTerm) / 5.0
	}

	// -------------------------------------------------------------------------
	// Vibrato term: mean of normalized absolute deltas for RateHz, ExtentCents,
	// and AMDepth. Term is 0 when either fingerprint has nil Vibrato.
	// Scales:
	//   RateHz      / 8    — vibrato rate spans ~3–9 Hz; max delta ≈ 8 Hz
	//   ExtentCents / 50   — vibrato extent rarely exceeds ±50 cents
	//   AMDepth     / 0.3  — AM depth is a relative ratio; 0.3 covers typical range
	// -------------------------------------------------------------------------
	{
		if ref.Vibrato != nil && cand.Vibrato != nil {
			rv, cv := ref.Vibrato, cand.Vibrato
			t0 := clamp1(absDelta(rv.RateHz, cv.RateHz) / 8.0)
			t1 := clamp1(absDelta(rv.ExtentCents, cv.ExtentCents) / 50.0)
			t2 := clamp1(absDelta(rv.AMDepth, cv.AMDepth) / 0.3)
			s.Vibrato = (t0 + t1 + t2) / 3.0
		}
		// else s.Vibrato stays 0
	}

	// -------------------------------------------------------------------------
	// Coupling term: mean of normalized absolute deltas for CentroidRMSCorr,
	// BrightnessModDepth, and MacroCentroidRMSCorr. Term is 0 when either
	// fingerprint has nil Coupling.
	// Scales:
	//   CentroidRMSCorr      / 1.0 — Pearson correlation is in [-1, 1]; max delta = 2; clamp handles it
	//   BrightnessModDepth   / 0.3 — relative centroid std/mean; 0.3 covers typical range
	//   MacroCentroidRMSCorr / 1.0 — Pearson correlation is in [-1, 1]; max delta = 2; clamp handles it
	// -------------------------------------------------------------------------
	{
		if ref.Coupling != nil && cand.Coupling != nil {
			rc, cc := ref.Coupling, cand.Coupling
			t0 := clamp1(absDelta(rc.CentroidRMSCorr, cc.CentroidRMSCorr) / 1.0)
			t1 := clamp1(absDelta(rc.BrightnessModDepth, cc.BrightnessModDepth) / 0.3)
			t2 := clamp1(absDelta(rc.MacroCentroidRMSCorr, cc.MacroCentroidRMSCorr) / 1.0)
			s.Coupling = (t0 + t1 + t2) / 3.0
		}
		// else s.Coupling stays 0
	}

	// -------------------------------------------------------------------------
	// Formant term: mean of normalized absolute deltas for BridgeHillHz and
	// BridgeHillGainDB. Term is 0 when either fingerprint has nil SpectralEnv.
	// Scales:
	//   BridgeHillHz     / 2000  — the bridge hill spans 1.5–4 kHz; max delta ≈ 2 kHz
	//   BridgeHillGainDB / 20    — relative gain in dB; 20 dB covers typical range
	// -------------------------------------------------------------------------
	{
		if ref.SpectralEnv != nil && cand.SpectralEnv != nil {
			re, ce := ref.SpectralEnv, cand.SpectralEnv
			t0 := clamp1(absDelta(re.BridgeHillHz, ce.BridgeHillHz) / 2000.0)
			t1 := clamp1(absDelta(re.BridgeHillGainDB, ce.BridgeHillGainDB) / 20.0)
			s.Formant = (t0 + t1) / 2.0
		}
		// else s.Formant stays 0
	}

	// -------------------------------------------------------------------------
	// HTrajectory term: mean absolute delta of DecayRate[k] over k=0..min(N)-1.
	// Term is 0 when either fingerprint has nil Harmonics or N==0.
	// Scale:
	//   DecayRate[k] / 100  — dB/sec; 100 dB/sec covers typical range
	// -------------------------------------------------------------------------
	{
		if ref.Harmonics != nil && cand.Harmonics != nil {
			rh, ch := ref.Harmonics, cand.Harmonics
			nk := rh.N
			if ch.N < nk {
				nk = ch.N
			}
			if nk > 0 {
				sum := 0.0
				for k := 0; k < nk; k++ {
					sum += clamp1(absDelta(rh.DecayRate[k], ch.DecayRate[k]) / 100.0)
				}
				s.HTrajectory = sum / float64(nk)
			}
		}
		// else s.HTrajectory stays 0
	}

	// -------------------------------------------------------------------------
	// Total: weighted sum of all terms.
	// -------------------------------------------------------------------------
	w := distanceWeights
	s.Total = w.Spectral*s.Spectral +
		w.MFCC*s.MFCC +
		w.Partials*s.Partials +
		w.Temporal*s.Temporal +
		w.Envelope*s.Envelope +
		w.Noise*s.Noise +
		w.Timbre*s.Timbre +
		w.Vibrato*s.Vibrato +
		w.Coupling*s.Coupling +
		w.Formant*s.Formant +
		w.HTrajectory*s.HTrajectory

	return s
}
