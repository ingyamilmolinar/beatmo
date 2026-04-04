//go:build test

package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
)

type Voice interface{}

type Instrument interface{ NewVoice(int, int) Voice }

var insts = []string{"snare", "kick", "hihat", "tom", "clap"}

// SampleRate returns the audio output sample rate (stub returns 44100 for tests).
func SampleRate() int {
	return 44100
}

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

// nowOverride allows tests to inject a deterministic audio clock.
var nowOverride func() float64

// Now returns 0 during tests unless overridden via SetNowForTest.
func Now() float64 {
	if nowOverride != nil {
		return nowOverride()
	}
	return 0
}

// SetNowForTest overrides audio.Now() to return values from fn.
// Returns a restore function.
func SetNowForTest(fn func() float64) func() {
	old := nowOverride
	nowOverride = fn
	return func() { nowOverride = old }
}

// Resume is a no-op in tests.
func Resume() {}

// Close is a no-op in tests (no audio device to release).
func Close() {}

// Reset is a stub used during tests.
func Reset() { resetChannels() }

var SetBPMFunc = func(int) {}

func SetBPM(bpm int) { SetBPMFunc(bpm) }

// Instruments returns placeholder instrument IDs during tests.
func Instruments() []string { return insts }

func ResetInstruments() {
	insts = append([]string(nil), BuiltinInstrumentIDs...)
	bumpInstrumentsVersion()
	ResetCatalogForTest(nil)
	ClearAllInsertEffects()
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

// Send effect stubs for tests.
var stubSendLevels = map[string][2]float64{} // [delay, reverb]

func SetDelaySend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	v := stubSendLevels[id]
	v[0] = amount
	stubSendLevels[id] = v
}

func DelaySend(id string) float64 {
	return stubSendLevels[id][0]
}

func SetReverbSend(id string, amount float64) {
	if amount < 0 {
		amount = 0
	}
	if amount > 1 {
		amount = 1
	}
	v := stubSendLevels[id]
	v[1] = amount
	stubSendLevels[id] = v
}

func ReverbSend(id string) float64 {
	return stubSendLevels[id][1]
}

// EQ / analyzer stubs for tests.
type Analyzer struct {
	window  []float64
	write   int
	filled  bool
	rms     float64
	peak    float64
	enabled bool
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
	return &Analyzer{window: make([]float64, window), enabled: true}
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

// ProcessBlock batches circular buffer writes with minimal overhead.
func (a *Analyzer) ProcessBlock(samples []float32, n int) {
	ws := len(a.window)
	if ws == 0 || n <= 0 {
		return
	}
	idx := a.write
	for i := 0; i < n; i++ {
		if idx >= ws {
			idx = 0
		}
		a.window[idx] = float64(samples[i])
		idx++
		if idx == ws {
			a.filled = true
			a.write = idx
			a.compute()
			idx = 0
		}
	}
	a.write = idx % ws
}

// ProcessBlockBuf implements BlockProcessor for Analyzer (stub).
func (a *Analyzer) ProcessBlockBuf(in, out []float32, samples int) {
	copy(out[:samples], in[:samples])
	a.ProcessBlock(in, samples)
}

func (a *Analyzer) Snapshot() AnalyzerSnapshot { return a.snapshot() }

// SetEnabled controls whether compute() runs (stub).
func (a *Analyzer) SetEnabled(on bool) { a.enabled = on }

// Enabled returns whether the analyzer's compute path is active (stub).
func (a *Analyzer) Enabled() bool { return a.enabled }

func (a *Analyzer) compute() {
	if !a.enabled {
		return
	}
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

// ProcessBlockBuf implements BlockProcessor for gainProcessor.
func (g *gainProcessor) ProcessBlockBuf(in, out []float32, samples int) {
	gain := float32(g.gain)
	for i := 0; i < samples; i++ {
		out[i] = in[i] * gain
	}
}

// silenceProcessor outputs zero for all samples. Used when all bands are muted.
type silenceProcessor struct{}

func (s *silenceProcessor) ProcessSample(x float64) float64 { return 0 }

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
	{loHz: 22, hiHz: 44},      // 31 Hz
	{loHz: 44, hiHz: 88},      // 62 Hz
	{loHz: 88, hiHz: 177},     // 125 Hz
	{loHz: 177, hiHz: 354},    // 250 Hz
	{loHz: 354, hiHz: 707},    // 500 Hz
	{loHz: 707, hiHz: 1414},   // 1 kHz
	{loHz: 1414, hiHz: 2828},  // 2 kHz
	{loHz: 2828, hiHz: 5657},  // 4 kHz
	{loHz: 5657, hiHz: 11314}, // 8 kHz
	{loHz: 11314, hiHz: 20000}, // 16 kHz
}

// biquad and makeBiquad are defined in biquad.go (shared across all build tags).

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

// ProcessBlockBuf implements BlockProcessor for multibandProcessor (stub).
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

// NewEQProcessor creates an EQ processor for the given bands.
//
// WARNING (test stub behavior): Under the `test` build tag, this returns a
// gainProcessor (simple amplitude scaling) or silenceProcessor, NOT real biquad
// filters. Tests that need actual frequency filtering must use makeBiquad()
// directly and pass it to SetChannelProcessors() — the *biquad type satisfies
// the Processor interface. The multibandProcessor and silenceProcessor DO work
// in stub mode (for band muting tests). Desktop-only mixer tests (!test build
// tag) get the real eqProcessor with biquad chains.
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
	lastSetEQ = eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)}
	RebuildChannelWithEQ(id, NewEQProcessor(sampleRate, bands...))
}

func ClearChannelProcessors(id string) {
	lastSetEQ = eqRecord{}
	RebuildChannelWithEQ(id, nil)
}

// AnalyzerService returns nil in test builds (no real audio engine).
func AnalyzerService() *analyzer.Service { return nil }

// ScopeService returns nil in test builds (no real audio engine).
func ScopeService() *scope.Service { return nil }

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

var preEQAnalyzerRegistryStub = map[string]*Analyzer{}

// EnablePreEQAnalyzer creates a pre-EQ analyzer on the channel (stub version).
func EnablePreEQAnalyzer(id string, window int) *Analyzer {
	an := NewAnalyzer(window)
	ch := chanMgr.ensureChannel(id)
	ch.mu.Lock()
	ch.preEQAnalyzer = an
	ch.mu.Unlock()
	preEQAnalyzerRegistryStub[id] = an
	return an
}

// PreEQAnalyzerSnapshot returns the latest pre-EQ snapshot (stub version).
func PreEQAnalyzerSnapshot(id string) AnalyzerSnapshot {
	if an, ok := preEQAnalyzerRegistryStub[id]; ok {
		return an.snapshot()
	}
	return AnalyzerSnapshot{}
}

// SetAnalyzerEnabled enables or disables FFT compute for a channel's analyzers (stub).
func SetAnalyzerEnabled(id string, on bool) {
	if an, ok := analyzerRegistryStub[id]; ok {
		an.SetEnabled(on)
	}
	if pre, ok := preEQAnalyzerRegistryStub[id]; ok {
		pre.SetEnabled(on)
	}
}

func resetAnalyzers() {
	analyzerRegistryStub = map[string]*Analyzer{}
	preEQAnalyzerRegistryStub = map[string]*Analyzer{}
}

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
