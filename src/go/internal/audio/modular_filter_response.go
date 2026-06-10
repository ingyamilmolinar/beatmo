package audio

import "math"

// ModularFilterResponse computes the magnitude response |H(e^jw)| of the
// modular voice's filter stage (a single RBJ biquad) across log-spaced
// frequencies. The coefficients mirror src/c/modular.c's mod_biquad_set EXACTLY
// (clamps included) so the Synth-tab filter preview reflects the tone the voice
// actually renders, rather than the channel EQ stack (which is a separate
// processor). filterType: 0=low-pass, 1=high-pass, 2=band-pass.
func ModularFilterResponse(filterType int, cutoffHz, q float64, sampleRate, numPoints int, startHz, endHz float64) []FreqResponsePoint {
	if numPoints <= 0 || sampleRate <= 0 || startHz <= 0 || endHz <= startHz {
		return nil
	}
	c := modularBiquadCoeffs(filterType, cutoffHz, q, float64(sampleRate))

	sr := float64(sampleRate)
	logStart := math.Log10(startHz)
	logEnd := math.Log10(endHz)
	points := make([]FreqResponsePoint, numPoints)
	for i := 0; i < numPoints; i++ {
		t := float64(i) / float64(numPoints-1)
		freq := math.Pow(10, logStart+t*(logEnd-logStart))
		w := 2 * math.Pi * freq / sr
		cosW, sinW := math.Cos(w), math.Sin(w)
		cos2W, sin2W := math.Cos(2*w), math.Sin(2*w)

		numReal := c.B0 + c.B1*cosW + c.B2*cos2W
		numImag := -(c.B1*sinW + c.B2*sin2W)
		denReal := 1 + c.A1*cosW + c.A2*cos2W
		denImag := -(c.A1*sinW + c.A2*sin2W)

		denMag2 := denReal*denReal + denImag*denImag
		if denMag2 < 1e-30 {
			denMag2 = 1e-30
		}
		mag := math.Sqrt((numReal*numReal + numImag*numImag) / denMag2)
		if mag < 1e-30 {
			mag = 1e-30
		}
		points[i] = FreqResponsePoint{FreqHz: freq, GainDB: 20 * math.Log10(mag)}
	}
	return points
}

// modularBiquadCoeffs replicates src/c/modular.c mod_biquad_set: RBJ LP/HP/BP
// coefficients with the same cutoff/Q clamps. Normalised by a0.
func modularBiquadCoeffs(filterType int, cutoff, q, sr float64) BiquadCoeffs {
	if cutoff < 20.0 {
		cutoff = 20.0
	}
	nyq := 0.5 * sr
	if cutoff > nyq*0.99 {
		cutoff = nyq * 0.99
	}
	if q < 0.25 {
		q = 0.25
	}
	w0 := 2.0 * math.Pi * cutoff / sr
	cw, sw := math.Cos(w0), math.Sin(w0)
	alpha := sw / (2.0 * q)

	var b0, b1, b2, a0, a1, a2 float64
	switch filterType {
	case 1: // high-pass
		b0 = (1.0 + cw) / 2.0
		b1 = -(1.0 + cw)
		b2 = (1.0 + cw) / 2.0
		a0 = 1.0 + alpha
		a1 = -2.0 * cw
		a2 = 1.0 - alpha
	case 2: // band-pass (constant 0 dB peak)
		b0 = alpha
		b1 = 0.0
		b2 = -alpha
		a0 = 1.0 + alpha
		a1 = -2.0 * cw
		a2 = 1.0 - alpha
	default: // low-pass
		b0 = (1.0 - cw) / 2.0
		b1 = 1.0 - cw
		b2 = (1.0 - cw) / 2.0
		a0 = 1.0 + alpha
		a1 = -2.0 * cw
		a2 = 1.0 - alpha
	}
	return BiquadCoeffs{B0: b0 / a0, B1: b1 / a0, B2: b2 / a0, A1: a1 / a0, A2: a2 / a0}
}
