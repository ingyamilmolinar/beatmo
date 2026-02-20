package audio

// biquad implements Direct Form I processing.
type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (b *biquad) ProcessSample(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	// Flush denormals to zero to prevent noise buildup during silence.
	const denormalThreshold = 1e-20
	if y > -denormalThreshold && y < denormalThreshold {
		y = 0
	}
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

// ProcessBlockBuf implements BlockProcessor for biquad. Coefficients and state
// are hoisted to locals for a tight inner loop. Internal precision is float64
// (matching ProcessSample) with float32 I/O (matching BlockProcessor interface).
func (b *biquad) ProcessBlockBuf(in []float32, out []float32, samples int) {
	b0, b1, b2 := b.b0, b.b1, b.b2
	a1, a2 := b.a1, b.a2
	x1, x2 := b.x1, b.x2
	y1, y2 := b.y1, b.y2
	const denorm = 1e-20
	for i := 0; i < samples; i++ {
		x := float64(in[i])
		y := b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2
		if y > -denorm && y < denorm {
			y = 0
		}
		x2, x1 = x1, x
		y2, y1 = y1, y
		out[i] = float32(y)
	}
	b.x1, b.x2 = x1, x2
	b.y1, b.y2 = y1, y2
}

func makeBiquad(kind EQKind, sr int, freq, q, gainDB float64) *biquad {
	c := ComputeBiquadCoeffs(kind, sr, freq, q, gainDB)
	if c == (BiquadCoeffs{}) {
		return nil
	}
	return &biquad{
		b0: c.B0, b1: c.B1, b2: c.B2,
		a1: c.A1, a2: c.A2,
	}
}
