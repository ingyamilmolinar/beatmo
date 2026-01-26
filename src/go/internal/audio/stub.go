//go:build test

package audio

import "math"

type Voice interface{}

type Instrument interface{ NewVoice(int, int) Voice }

var insts = []string{"snare", "kick", "hihat", "tom", "clap"}

func registerStub(id string) {
	for _, existing := range insts {
		if existing == id {
			return
		}
	}
	insts = append(insts, id)
	bumpInstrumentsVersion()
	InstrumentChannel(id)
}

func Register(id string, inst Instrument) {
	registerStub(id)
}

func RegisterAudio(id, path string) error {
	registerStub(id)
	return nil
}

func RegisterWAV(id, path string) error {
	registerStub(id)
	return nil
}

func SelectWAV() (string, error) { return "dummy.wav", nil }

// Play is a stub used during tests to avoid initializing audio devices.
func Play(id string, when ...float64) {}

// PlayVol is a stub used during tests for volume-controlled playback.
func PlayVol(id string, vol float64, when ...float64) {}

// PlayParams is a stub used during tests to accept extended playback params.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {}

// PlayParamsAt is a stub used during tests to avoid varargs allocation.
func PlayParamsAt(id string, vol, pitch, dur, when float64) {}

// SampleSeconds is a stub for tests.
func SampleSeconds(id string) float64 { return 0 }

var stopHook func(string)

func Stop(id string) {
	if stopHook != nil {
		stopHook(id)
	}
}

func SetStopHook(fn func(string)) { stopHook = fn }

// Now returns 0 during tests.
func Now() float64 { return 0 }

// Resume is a no-op in tests.
func Resume() {}

// Reset is a stub used during tests.
func Reset() { resetChannels() }

var SetBPMFunc = func(int) {}

func SetBPM(bpm int) { SetBPMFunc(bpm) }

// Instruments returns placeholder instrument IDs during tests.
func Instruments() []string { return insts }

func ResetInstruments() {
	insts = []string{
		"snare", "kick", "hihat", "tom", "clap", "cowbell",
		"snare-1", "kick-1", "hihat-1", "tom-1", "clap-1", "cowbell-1",
		"snare-2", "kick-2", "hihat-2", "tom-2", "clap-2", "cowbell-2",
	}
	bumpInstrumentsVersion()
	ResetCatalogForTest(nil)
	resetInstrumentChannels(insts)
}

func RenameInstrument(oldID, newID string) {
	for i, id := range insts {
		if id == oldID {
			insts[i] = newID
			bumpInstrumentsVersion()
			break
		}
	}
	renameInstrumentChannel(oldID, newID)
}

// EQ / analyzer stubs for tests.
type Analyzer struct {
	window []float64
	write  int
	filled bool
	rms    float64
	peak   float64
}
type AnalyzerSnapshot struct {
	RMS      float64
	Peak     float64
	Spectrum []float64
	Waveform []float64
}

// NewAnalyzer creates an analyzer with the requested window size. The size is
// rounded to the nearest power-of-two between 64 and 8192.
func NewAnalyzer(window int) *Analyzer {
	if window < 64 {
		window = 64
	}
	if window > 8192 {
		window = 8192
	}
	window = nearestPow2(window)
	return &Analyzer{window: make([]float64, window)}
}

func (a *Analyzer) ProcessSample(x float64) float64 {
	if len(a.window) == 0 {
		return x
	}
	a.window[a.write] = x
	a.write++
	if a.write >= len(a.window) {
		a.write = 0
		a.filled = true
		a.compute()
	}
	return x
}

func (a *Analyzer) Snapshot() AnalyzerSnapshot { return a.snapshot() }

func (a *Analyzer) compute() {
	n := len(a.window)
	var sum float64
	var peak float64
	for _, v := range a.window {
		if v < 0 {
			if -v > peak {
				peak = -v
			}
		} else if v > peak {
			peak = v
		}
		sum += v * v
	}
	a.rms = 0
	if n > 0 {
		a.rms = sum / float64(n)
		a.rms = math.Sqrt(a.rms)
	}
	a.peak = peak
}

func (a *Analyzer) snapshot() AnalyzerSnapshot {
	if a == nil {
		return AnalyzerSnapshot{}
	}
	specBins := len(a.window) / 2
	if specBins < 1 {
		specBins = 1
	}
	spec := make([]float64, specBins)
	// Encode RMS into the first bin so tests see non-zero energy.
	spec[0] = a.rms
	var wave []float64
	if a.filled {
		wave = make([]float64, len(a.window))
		copy(wave, a.window[a.write:])
		copy(wave[len(a.window)-a.write:], a.window[:a.write])
	} else {
		wave = make([]float64, a.write)
		copy(wave, a.window[:a.write])
	}
	return AnalyzerSnapshot{
		RMS:      a.rms,
		Peak:     a.peak,
		Spectrum: spec,
		Waveform: wave,
	}
}

type gainProcessor struct{ gain float64 }

func (g *gainProcessor) ProcessSample(x float64) float64 { return x * g.gain }

// silenceProcessor outputs zero for all samples. Used when all bands are muted.
type silenceProcessor struct{}

func (s *silenceProcessor) ProcessSample(x float64) float64 { return 0 }

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

// biquad implements Direct Form I processing for crossover filters.
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

	var b0, b1, b2, a0, a1, a2 float64
	switch kind {
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
// processes each band independently, then recombines them.
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
func newMultibandProcessor(sampleRate int, bands []EQBand) *multibandProcessor {
	numBands := len(bands)
	if numBands == 0 {
		return &multibandProcessor{}
	}

	// Safety check: if all bands are muted, mark them all as muted
	allMuted := true
	for _, b := range bands {
		if !b.Muted {
			allMuted = false
			break
		}
	}
	if allMuted {
		procs := make([]bandProcessor, numBands)
		for i := range procs {
			procs[i].muted = true
		}
		return &multibandProcessor{bands: procs}
	}

	defs := defaultBandDefs
	if numBands != len(defs) {
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
			gain:  math.Pow(10, b.GainDB/20),
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

	// No muting - use simple gain processor
	gain := 1.0
	for _, b := range bands {
		gain *= math.Pow(10, b.GainDB/20)
	}
	if gain < 0.0001 {
		gain = 0.0001
	}
	return &gainProcessor{gain: gain}
}

func NewPeakingEQ(sampleRate int, freq, q, gainDB float64) Processor {
	return NewEQProcessor(sampleRate, EQBand{Kind: EQPeaking, Freq: freq, Q: q, GainDB: gainDB})
}

func NewShelfEQ(sampleRate int, low bool, freq, q, gainDB float64) Processor {
	kind := EQHighShelf
	if low {
		kind = EQLowShelf
	}
	return NewEQProcessor(sampleRate, EQBand{Kind: kind, Freq: freq, Q: q, GainDB: gainDB})
}

func SetChannelEQ(id string, sampleRate int, bands ...EQBand) {
	SetChannelProcessors(id, NewEQProcessor(sampleRate, bands...))
	lastSetEQ = eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
}

func ClearChannelProcessors(id string) { SetChannelProcessors(id) }

var analyzerRegistryStub = map[string]*Analyzer{}

type eqRecord struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

var lastSetEQ eqRecord

func LastSetEQ() eqRecord { return lastSetEQ }
func EnableChannelAnalyzer(id string, window int) *Analyzer {
	an := NewAnalyzer(window)
	AddChannelProcessor(id, an)
	analyzerRegistryStub[id] = an
	return an
}

func ChannelAnalyzerSnapshot(id string) AnalyzerSnapshot {
	if an, ok := analyzerRegistryStub[id]; ok {
		return an.snapshot()
	}
	return AnalyzerSnapshot{}
}

func resetAnalyzers() { analyzerRegistryStub = map[string]*Analyzer{} }

// nearestPow2 rounds v to the nearest power-of-two (preferring the larger on ties).
func nearestPow2(v int) int {
	p := 1
	for p < v {
		p <<= 1
	}
	if p>>1 != 0 && (p-v) > (v-(p>>1)) {
		return p >> 1
	}
	return p
}
