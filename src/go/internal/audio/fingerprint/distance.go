package fingerprint

import "math"

// Lower is more similar; each term is ~[0,1] for a typical mismatch before weighting.
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

// About 1 each. Spectral and MFCC slightly higher (the most salient differences), Temporal lower (short synthetic
// tones barely evolve), Timbre modest (partly redundant with Spectral and Partials, keeping the optimiser stable).
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

// Symmetric, zero at identity.
func Distance(ref, cand Fingerprint) Score {
	var s Score

	// Scales: centroid / 4000 Hz, rolloff / 8000 Hz, LSD / 40 dB, convergence dimensionless: typical spans for related
	// timbres.
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

			// SpectralConvergence is not symmetric, so both directions are averaged.
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

	// c0 is skipped (an energy offset); 50 is an empirical span for 12 coefficients.
	{
		sumSq := 0.0
		for i := 1; i <= 12; i++ {
			d := ref.MFCC[i] - cand.MFCC[i]
			sumSq += d * d
		}
		s.MFCC = clamp1(math.Sqrt(sumSq) / 50.0)
	}

	// Partials are normalised so Partials[0] is 1 (or 0).
	{
		sum := 0.0
		for k := 0; k < 16; k++ {
			sum += absDelta(ref.Partials[k], cand.Partials[k])
		}
		s.Partials = clamp1(sum / 16.0)
	}

	// 0 when either has no Temporal. Scales: flux / 0.1, centroid range / 2000 Hz, vibrato depth / 50 cents.
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

	// Scales: attack / 0.2 s, log attack / 2, decay / 100 dB/s, sustain already 0..1, release / 0.5 s.
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

	// Both already in ~[0,1].
	{
		n := clamp1(absDelta(ref.NoiseRatio, cand.NoiseRatio))
		f := clamp1(absDelta(ref.SpectralFlatness, cand.SpectralFlatness))
		s.Noise = (n + f) / 2.0
	}

	// Absolute deltas, so identity and symmetry hold. Scales: tristimulus already ~[0,1], crest / 50, HNR / 20, skewness /
	// 5, kurtosis / 10.
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

	// 0 when either has nil Vibrato. Scales: rate / 8 Hz, extent / 50 cents, AM depth / 0.3.
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

	// 0 when either has nil Coupling. Correlations span [-1, 1]; the clamp handles a delta of 2.
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

	// 0 when either has nil SpectralEnv. Scales: bridge hill / 2000 Hz, gain / 20 dB.
	{
		if ref.SpectralEnv != nil && cand.SpectralEnv != nil {
			re, ce := ref.SpectralEnv, cand.SpectralEnv
			t0 := clamp1(absDelta(re.BridgeHillHz, ce.BridgeHillHz) / 2000.0)
			t1 := clamp1(absDelta(re.BridgeHillGainDB, ce.BridgeHillGainDB) / 20.0)
			s.Formant = (t0 + t1) / 2.0
		}
		// else s.Formant stays 0
	}

	// 0 when either has no harmonics. Decay rates / 100 dB/s.
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
