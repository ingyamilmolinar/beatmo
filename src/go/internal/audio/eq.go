//go:build !js && !test

package audio

import "math"

// eqProcessor chains a list of biquads.
type eqProcessor struct {
	filters []*biquad
}

func (e *eqProcessor) ProcessSample(x float64) float64 {
	y := x
	for _, f := range e.filters {
		y = f.ProcessSample(y)
	}
	return y
}

// silenceProcessor outputs zero for all samples. Used when all bands are muted.
type silenceProcessor struct{}

func (s *silenceProcessor) ProcessSample(x float64) float64 {
	return 0
}

// multibandBandDef defines frequency boundaries for a single band in the multiband processor.
type multibandBandDef struct {
	loHz float64
	hiHz float64
}

// defaultBandDefs matches the 10-band EQ definitions used in the UI.
var defaultBandDefs = []multibandBandDef{
	{loHz: 20, hiHz: 40},
	{loHz: 40, hiHz: 80},
	{loHz: 80, hiHz: 160},
	{loHz: 160, hiHz: 315},
	{loHz: 315, hiHz: 630},
	{loHz: 630, hiHz: 1250},
	{loHz: 1250, hiHz: 2500},
	{loHz: 2500, hiHz: 5000},
	{loHz: 5000, hiHz: 10000},
	{loHz: 10000, hiHz: 20000},
}

// bandProcessor handles one frequency band in the parallel multiband processor.
// Uses 4th-order Linkwitz-Riley crossovers (two cascaded Butterworth stages)
// for -24dB/octave rolloff and better band isolation.
type bandProcessor struct {
	lowCut1  *biquad // First highpass stage at loHz (nil for band 0)
	lowCut2  *biquad // Second highpass stage for LR4 cascade
	highCut1 *biquad // First lowpass stage at hiHz (nil for last band)
	highCut2 *biquad // Second lowpass stage for LR4 cascade
	gain     float64 // Linear gain from dB
	muted    bool    // If true, output 0
}

// multibandProcessor splits input into separate frequency bands using crossover filters,
// processes each band independently, then recombines them. This ensures muting one band
// has zero effect on adjacent bands.
type multibandProcessor struct {
	bands []bandProcessor
}

func (m *multibandProcessor) ProcessSample(x float64) float64 {
	var sum float64
	for i := range m.bands {
		b := &m.bands[i]
		if b.muted {
			continue
		}
		y := x
		// Apply cascaded highpass filters (LR4: -24dB/octave)
		if b.lowCut1 != nil {
			y = b.lowCut1.ProcessSample(y)
		}
		if b.lowCut2 != nil {
			y = b.lowCut2.ProcessSample(y)
		}
		// Apply cascaded lowpass filters (LR4: -24dB/octave)
		if b.highCut1 != nil {
			y = b.highCut1.ProcessSample(y)
		}
		if b.highCut2 != nil {
			y = b.highCut2.ProcessSample(y)
		}
		sum += y * b.gain
	}
	return sum
}

// newMultibandProcessor creates a parallel multiband processor for the given bands.
// Uses crossover filters to split the signal into non-overlapping frequency bands.
func newMultibandProcessor(sampleRate int, bands []EQBand) *multibandProcessor {
	numBands := len(bands)
	if numBands == 0 {
		return &multibandProcessor{}
	}

	// Safety check: if all bands are muted, mark them all as muted
	// (The caller should have caught this, but just in case)
	allMuted := true
	for _, b := range bands {
		if !b.Muted {
			allMuted = false
			break
		}
	}
	if allMuted {
		// Return a processor with all bands marked muted - will output silence
		procs := make([]bandProcessor, numBands)
		for i := range procs {
			procs[i].muted = true
		}
		return &multibandProcessor{bands: procs}
	}

	// Use default band definitions if count matches
	defs := defaultBandDefs
	if numBands != len(defs) {
		// Fallback: create uniform bands if count doesn't match
		defs = make([]multibandBandDef, numBands)
		freqRange := 20000.0 - 20.0
		bandWidth := freqRange / float64(numBands)
		for i := range defs {
			defs[i].loHz = 20.0 + float64(i)*bandWidth
			defs[i].hiHz = 20.0 + float64(i+1)*bandWidth
		}
	}

	procs := make([]bandProcessor, numBands)
	butterworthQ := 0.707 // Butterworth Q for flat passband

	for i, b := range bands {
		def := defs[i]
		bp := bandProcessor{
			muted: b.Muted,
			gain:  math.Pow(10, b.GainDB/20), // dB to linear amplitude
		}

		// Highpass at loHz - two cascaded stages for LR4 (-24dB/octave)
		if i > 0 {
			bp.lowCut1 = makeBiquad(EQHighpass, sampleRate, def.loHz, butterworthQ, 0)
			bp.lowCut2 = makeBiquad(EQHighpass, sampleRate, def.loHz, butterworthQ, 0)
		}

		// Lowpass at hiHz - two cascaded stages for LR4 (-24dB/octave)
		if i < numBands-1 {
			bp.highCut1 = makeBiquad(EQLowpass, sampleRate, def.hiHz, butterworthQ, 0)
			bp.highCut2 = makeBiquad(EQLowpass, sampleRate, def.hiHz, butterworthQ, 0)
		}

		procs[i] = bp
	}

	return &multibandProcessor{bands: procs}
}

// NewEQProcessor constructs a multi-band EQ for the given sample rate.
// When all bands are muted, returns a processor that outputs complete silence.
// When any band is muted, uses a parallel multiband processor for proper isolation.
// Otherwise uses a simple chain of biquad filters for efficiency.
func NewEQProcessor(sampleRate int, bands ...EQBand) Processor {
	// Check if all bands are muted - if so, output complete silence
	allMuted := len(bands) > 0
	anyMuted := false
	for _, b := range bands {
		if !b.Muted {
			allMuted = false
		} else {
			anyMuted = true
		}
	}
	if allMuted {
		return &silenceProcessor{}
	}

	// If any band is muted, use multiband processor for proper isolation
	if anyMuted {
		return newMultibandProcessor(sampleRate, bands)
	}

	// No muting - use simple biquad chain for efficiency
	fs := make([]*biquad, 0, len(bands))
	for _, b := range bands {
		if f := makeBiquad(b.Kind, sampleRate, b.Freq, b.Q, b.GainDB); f != nil {
			fs = append(fs, f)
		}
	}
	return &eqProcessor{filters: fs}
}

// NewPeakingEQ convenience helper for a single peaking band.
func NewPeakingEQ(sampleRate int, freq, q, gainDB float64) Processor {
	return NewEQProcessor(sampleRate, EQBand{Kind: EQPeaking, Freq: freq, Q: q, GainDB: gainDB})
}

// NewShelfEQ convenience helper for low/high shelf bands.
func NewShelfEQ(sampleRate int, low bool, freq, q, gainDB float64) Processor {
	kind := EQHighShelf
	if low {
		kind = EQLowShelf
	}
	return NewEQProcessor(sampleRate, EQBand{Kind: kind, Freq: freq, Q: q, GainDB: gainDB})
}

// SetChannelEQ replaces the processor chain on a channel with the given EQ bands.
func SetChannelEQ(id string, sampleRate int, bands ...EQBand) {
	SetChannelProcessors(id, NewEQProcessor(sampleRate, bands...))
	lastSetEQ = eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
}

// ClearChannelProcessors removes all processors from the channel.
func ClearChannelProcessors(id string) {
	SetChannelProcessors(id)
}

// biquad implements Direct Form I processing.
type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

func (b *biquad) ProcessSample(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

func makeBiquad(kind EQKind, sr int, freq, q, gainDB float64) *biquad {
	if sr <= 0 || freq <= 0 || q <= 0 {
		return nil
	}
	if freq > float64(sr)/2 {
		freq = float64(sr) / 2
	}
	w0 := 2 * math.Pi * freq / float64(sr)
	cosw := math.Cos(w0)
	sinw := math.Sin(w0)
	alpha := sinw / (2 * q)
	A := math.Pow(10, gainDB/40) // amplitude for shelves/peaks

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
		// Butterworth lowpass (Q=0.707 for flat passband)
		b0 = (1 - cosw) / 2
		b1 = 1 - cosw
		b2 = (1 - cosw) / 2
		a0 = 1 + alpha
		a1 = -2 * cosw
		a2 = 1 - alpha
	case EQHighpass:
		// Butterworth highpass (Q=0.707 for flat passband)
		b0 = (1 + cosw) / 2
		b1 = -(1 + cosw)
		b2 = (1 + cosw) / 2
		a0 = 1 + alpha
		a1 = -2 * cosw
		a2 = 1 - alpha
	default:
		return nil
	}

	return &biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}
