package audio

import "math"

// BiquadCoeffs holds the normalized biquad filter coefficients (a0 = 1).
type BiquadCoeffs struct {
	B0, B1, B2 float64
	A1, A2     float64
}

// ComputeBiquadCoeffs returns biquad coefficients for the given filter parameters
// using RBJ Audio EQ Cookbook formulas. Returns zero coefficients if parameters
// are invalid.
func ComputeBiquadCoeffs(kind EQKind, sampleRate int, freq, q, gainDB float64) BiquadCoeffs {
	if sampleRate <= 0 || freq <= 0 || q <= 0 {
		return BiquadCoeffs{}
	}
	sr := float64(sampleRate)
	if freq > sr/2 {
		freq = sr / 2
	}
	w0 := 2 * math.Pi * freq / sr
	cosw := math.Cos(w0)
	sinw := math.Sin(w0)
	alpha := sinw / (2 * q)
	A := math.Pow(10, gainDB/40)

	var b0, b1, b2, a0, a1, a2 float64
	switch kind {
	case EQPeaking:
		b0 = 1 + alpha*A
		b1 = -2 * cosw
		b2 = 1 - alpha*A
		a0 = 1 + alpha/A
		a1 = -2 * cosw
		a2 = 1 - alpha/A
	case EQLowShelf:
		sqrtA := math.Sqrt(A)
		b0 = A * ((A + 1) - (A-1)*cosw + 2*sqrtA*alpha)
		b1 = 2 * A * ((A - 1) - (A+1)*cosw)
		b2 = A * ((A + 1) - (A-1)*cosw - 2*sqrtA*alpha)
		a0 = (A + 1) + (A-1)*cosw + 2*sqrtA*alpha
		a1 = -2 * ((A - 1) + (A+1)*cosw)
		a2 = (A + 1) + (A-1)*cosw - 2*sqrtA*alpha
	case EQHighShelf:
		sqrtA := math.Sqrt(A)
		b0 = A * ((A + 1) + (A-1)*cosw + 2*sqrtA*alpha)
		b1 = -2 * A * ((A - 1) + (A+1)*cosw)
		b2 = A * ((A + 1) + (A-1)*cosw - 2*sqrtA*alpha)
		a0 = (A + 1) - (A-1)*cosw + 2*sqrtA*alpha
		a1 = 2 * ((A - 1) - (A+1)*cosw)
		a2 = (A + 1) - (A-1)*cosw - 2*sqrtA*alpha
	case EQLowpass:
		b0 = (1 - cosw) / 2
		b1 = 1 - cosw
		b2 = (1 - cosw) / 2
		a0 = 1 + alpha
		a1 = -2 * cosw
		a2 = 1 - alpha
	case EQHighpass:
		b0 = (1 + cosw) / 2
		b1 = -(1 + cosw)
		b2 = (1 + cosw) / 2
		a0 = 1 + alpha
		a1 = -2 * cosw
		a2 = 1 - alpha
	case EQBandpass:
		b0 = alpha
		b1 = 0
		b2 = -alpha
		a0 = 1 + alpha
		a1 = -2 * cosw
		a2 = 1 - alpha
	default:
		return BiquadCoeffs{}
	}

	if a0 == 0 {
		return BiquadCoeffs{}
	}
	return BiquadCoeffs{
		B0: b0 / a0,
		B1: b1 / a0,
		B2: b2 / a0,
		A1: a1 / a0,
		A2: a2 / a0,
	}
}

// FreqResponsePoint is a single point on the frequency response curve.
type FreqResponsePoint struct {
	FreqHz float64
	GainDB float64
}

// ComputeFreqResponse computes the combined magnitude response |H(e^jw)| of a
// chain of biquad filters at log-spaced frequencies between startHz and endHz.
// Muted bands are skipped. Multiple bands are summed in dB (serial chain =
// product of magnitudes = sum of dB).
func ComputeFreqResponse(sampleRate int, bands []EQBand, numPoints int, startHz, endHz float64) []FreqResponsePoint {
	if numPoints <= 0 || sampleRate <= 0 || startHz <= 0 || endHz <= startHz {
		return nil
	}

	sr := float64(sampleRate)
	logStart := math.Log10(startHz)
	logEnd := math.Log10(endHz)

	// Precompute coefficients for non-muted bands.
	type bandCoeffs struct {
		c BiquadCoeffs
	}
	var active []bandCoeffs
	for _, b := range bands {
		if b.Muted {
			continue
		}
		c := ComputeBiquadCoeffs(b.Kind, sampleRate, b.Freq, b.Q, b.GainDB)
		active = append(active, bandCoeffs{c})
	}

	points := make([]FreqResponsePoint, numPoints)
	for i := 0; i < numPoints; i++ {
		t := float64(i) / float64(numPoints-1)
		logF := logStart + t*(logEnd-logStart)
		freq := math.Pow(10, logF)

		// Normalized angular frequency.
		w := 2 * math.Pi * freq / sr

		// Accumulate dB from each active band.
		totalDB := 0.0
		for _, bc := range active {
			c := bc.c
			// Evaluate H(z) at z = e^(jw):
			// H(z) = (b0 + b1*z^-1 + b2*z^-2) / (1 + a1*z^-1 + a2*z^-2)
			cosW := math.Cos(w)
			sinW := math.Sin(w)
			cos2W := math.Cos(2 * w)
			sin2W := math.Sin(2 * w)

			// Numerator: b0 + b1*e^(-jw) + b2*e^(-2jw)
			numReal := c.B0 + c.B1*cosW + c.B2*cos2W
			numImag := -(c.B1*sinW + c.B2*sin2W)

			// Denominator: 1 + a1*e^(-jw) + a2*e^(-2jw)
			denReal := 1 + c.A1*cosW + c.A2*cos2W
			denImag := -(c.A1*sinW + c.A2*sin2W)

			numMag2 := numReal*numReal + numImag*numImag
			denMag2 := denReal*denReal + denImag*denImag

			if denMag2 < 1e-30 {
				denMag2 = 1e-30
			}
			mag := math.Sqrt(numMag2 / denMag2)
			if mag < 1e-30 {
				mag = 1e-30
			}
			totalDB += 20 * math.Log10(mag)
		}

		points[i] = FreqResponsePoint{FreqHz: freq, GainDB: totalDB}
	}
	return points
}
