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

// ProcessBlockBuf implements BlockProcessor for eqProcessor. Chains biquads
// block-by-block with ping-pong buffers.
func (e *eqProcessor) ProcessBlockBuf(in, out []float32, samples int) {
	if len(e.filters) == 0 {
		copy(out[:samples], in[:samples])
		return
	}
	e.filters[0].ProcessBlockBuf(in, out, samples)
	if len(e.filters) > 1 {
		tmp := blockBufPool.get(samples)
		defer blockBufPool.put(tmp)
		src, dst := out, tmp
		for _, f := range e.filters[1:] {
			f.ProcessBlockBuf(src, dst, samples)
			src, dst = dst, src
		}
		if &src[0] != &out[0] {
			copy(out[:samples], src[:samples])
		}
	}
}

// silenceProcessor outputs zero for all samples. Used when all bands are muted.
type silenceProcessor struct{}

func (s *silenceProcessor) ProcessSample(x float64) float64 {
	return 0
}

// ProcessBlockBuf implements BlockProcessor for silenceProcessor.
func (s *silenceProcessor) ProcessBlockBuf(in, out []float32, samples int) {
	for i := 0; i < samples; i++ {
		out[i] = 0
	}
}

// multibandBandDef defines frequency boundaries for a single band in the multiband processor.
type multibandBandDef struct {
	loHz float64
	hiHz float64
}

// defaultBandDefs matches the 10-band ISO standard EQ definitions used in the UI.
var defaultBandDefs = []multibandBandDef{
	{loHz: 22, hiHz: 44},       // 31 Hz
	{loHz: 44, hiHz: 88},       // 62 Hz
	{loHz: 88, hiHz: 177},      // 125 Hz
	{loHz: 177, hiHz: 354},     // 250 Hz
	{loHz: 354, hiHz: 707},     // 500 Hz
	{loHz: 707, hiHz: 1414},    // 1 kHz
	{loHz: 1414, hiHz: 2828},   // 2 kHz
	{loHz: 2828, hiHz: 5657},   // 4 kHz
	{loHz: 5657, hiHz: 11314},  // 8 kHz
	{loHz: 11314, hiHz: 20000}, // 16 kHz
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

// ProcessBlockBuf implements BlockProcessor for multibandProcessor. Processes
// each band's full block then accumulates, using ping-pong buffers.
func (m *multibandProcessor) ProcessBlockBuf(in, out []float32, samples int) {
	for i := 0; i < samples; i++ {
		out[i] = 0
	}
	bandBuf := blockBufPool.get(samples)
	filterBuf := blockBufPool.get(samples)
	defer blockBufPool.put(bandBuf)
	defer blockBufPool.put(filterBuf)
	for i := range m.bands {
		b := &m.bands[i]
		if b.muted {
			continue
		}
		copy(bandBuf[:samples], in[:samples])
		// Apply cascaded HP/LP biquads with ping-pong.
		for _, filt := range []*biquad{b.lowCut1, b.lowCut2, b.highCut1, b.highCut2} {
			if filt != nil {
				filt.ProcessBlockBuf(bandBuf, filterBuf, samples)
				bandBuf, filterBuf = filterBuf, bandBuf
			}
		}
		gain := float32(b.gain)
		for j := 0; j < samples; j++ {
			out[j] += bandBuf[j] * gain
		}
	}
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

// SetChannelEQ replaces the EQ on a channel, preserving insert effects.
func SetChannelEQ(id string, sampleRate int, bands ...EQBand) {
	lastSetEQ = eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
	RebuildChannelWithEQ(id, NewEQProcessor(sampleRate, bands...))
}

// ClearChannelProcessors removes EQ from the channel, preserving insert effects.
func ClearChannelProcessors(id string) {
	lastSetEQ = eqRecord{}
	RebuildChannelWithEQ(id, nil)
}

// biquad and makeBiquad are defined in biquad.go (shared across all build tags).
