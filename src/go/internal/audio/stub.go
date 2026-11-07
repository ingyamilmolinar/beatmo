//go:build test

package audio

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
)

type Voice interface{}

type Instrument interface{ NewVoice(int) Voice }

var insts = []instrument.ID{roster.Snare, roster.Kick, roster.Hihat, roster.Tom, roster.Clap}

// Mutated only through SetSampleRateForTest, so every override comes with a restore.
var testSampleRate int32 = 44100

func SampleRate() int {
	return int(atomic.LoadInt32(&testSampleRate))
}

// Without the restore the override leaks into later tests in the process.
func SetSampleRateForTest(sr int) func() {
	prev := atomic.LoadInt32(&testSampleRate)
	atomic.StoreInt32(&testSampleRate, int32(sr))
	return func() { atomic.StoreInt32(&testSampleRate, prev) }
}

// The ring is not implemented under -tags test: no mixer loop produces reports.
type EmitReport struct {
	Row, Abs int
	DevPos   int64
}

// No mixer clock exists under -tags test.
func DeviceClockSnapshot() (int64, float64, bool) { return 0, 0, false }

// DrainEmitReports is a no-op stub under -tags test: always empty.
func DrainEmitReports(dst []EmitReport) int { return 0 }

func registerStub(id instrument.ID) {
	for _, existing := range insts {
		if existing == id {
			return
		}
	}
	insts = append(insts, id)
	bumpInstrumentsVersion()
}

func Register(id instrument.ID, inst Instrument) {
	registerStub(id)
}

// No-op if absent.
func Unregister(id instrument.ID) {
	found := false
	out := insts[:0]
	for _, existing := range insts {
		if existing == id {
			found = true
			continue
		}
		out = append(out, existing)
	}
	insts = out
	if found {
		bumpInstrumentsVersion()
	}
}

func RegisterAudio(id instrument.ID, path string) error {
	registerStub(id)
	return nil
}

func RegisterWAV(id instrument.ID, path string) error {
	registerStub(id)
	return nil
}

func SelectWAV() (string, error) { return "dummy.wav", nil }

// Records the trigger so tests can drive the UI's trigger-pulse pipeline without an audio device.
func Play(id instrument.ID, when ...float64) { RecordVoiceTrigger(id) }

// PlayVol is a stub used during tests for volume-controlled playback.
func PlayVol(id instrument.ID, vol float64, when ...float64) { RecordVoiceTrigger(id) }

// PlayParams is a stub used during tests to accept extended playback params.
func PlayParams(id instrument.ID, vol, pitch, dur float64, when ...float64) {
	RecordVoiceTrigger(id)
}

// PlayParamsAt is a stub used during tests to avoid varargs allocation.
func PlayParamsAt(id instrument.ID, vol, pitch, dur, when float64) {
	RecordVoiceTrigger(id)
}

// SampleSeconds is a stub for tests.
func SampleSeconds(id instrument.ID) float64 { return 0 }

var stopHook func(instrument.ID)

func Stop(id instrument.ID) {
	if stopHook != nil {
		stopHook(id)
	}
}

func SetStopHook(fn func(instrument.ID)) { stopHook = fn }

// nowOverride allows tests to inject a deterministic audio clock.
var nowOverride func() float64

// Now returns 0 during tests unless overridden via SetNowForTest.
func Now() float64 {
	if nowOverride != nil {
		return nowOverride()
	}
	return 0
}

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

// Atomic: SetBPM runs from Game.Update goroutines while tests swap and restore the hook.
var bpmFunc atomic.Pointer[func(int)]

func init() {
	noop := func(int) {}
	bpmFunc.Store(&noop)
}

func SetBPM(bpm int) {
	if fp := bpmFunc.Load(); fp != nil {
		(*fp)(bpm)
	}
}

// To capture the previous hook before overriding it.
func BPMFuncForTest() func(int) {
	if fp := bpmFunc.Load(); fp != nil {
		return *fp
	}
	return nil
}

// Instruments returns placeholder instrument IDs during tests.
func Instruments() []instrument.ID { return insts }

func ResetInstruments() {
	ResetInstrumentDocs()
	insts = append(append([]instrument.ID(nil), roster.All...), userOnlyLibraryIDs()...)
	bumpInstrumentsVersion()
	ResetCatalogForTest(nil)
	ClearAllInsertEffects()
	resetInstrumentChannels()
	clearInstrumentDisplayNames()
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
	RMS       float64
	Peak      float64
	ClipCount int // running count of samples >|1.0|; populated by the JS bridge or analyzer engine
	Spectrum  []float64
	Waveform  []float64
}

// Rounded to the nearest power of two in [64, 8192].
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

// So the next snapshot reads flat.
func (a *Analyzer) Reset() {
	if a == nil {
		return
	}
	for i := range a.window {
		a.window[i] = 0
	}
	a.write = 0
	a.filled = false
	a.rms = 0
	a.peak = 0
}

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

// biquad and makeBiquad are defined in biquad.go (shared across all build tags).

// 4th-order Linkwitz-Riley crossovers (two cascaded Butterworths): -24 dB/oct and better band isolation.
type bandProcessor struct {
	lowCut1  *biquad // First highpass stage at loHz (nil for band 0)
	lowCut2  *biquad // Second highpass stage for LR4 cascade
	highCut1 *biquad // First lowpass stage at hiHz (nil for last band)
	highCut2 *biquad // Second lowpass stage for LR4 cascade
	gain     float64 // Linear gain from dB
	muted    bool    // If true, output 0
}

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

// Under -tags test this is a gainProcessor or silenceProcessor, NOT a biquad: tests needing real filtering pass
// makeBiquad() to SetChannelProcessors. The multiband and silence processors work (band-mute tests).
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

func SetChannelEQ(id channel.ID, sampleRate int, bands ...EQBand) {
	channelEQWrites.Add(1)
	proc := NewEQProcessor(sampleRate, bands...)
	recordChannelEQ(id, eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...), proc: proc})
	RebuildChannelWithEQ(id, proc)
}

func ClearChannelProcessors(id channel.ID) {
	recordChannelEQ(id, eqRecord{})
	RebuildChannelWithEQ(id, nil)
}

// AnalyzerService returns nil in test builds (no real audio engine).
func AnalyzerService() *analyzer.Service { return nil }

// ScopeService returns nil in test builds (no real audio engine).
func ScopeService() *scope.Service { return nil }

// ExportService returns nil in test builds (no export service).
func ExportService() *scopeexport.Service { return nil }

// EnableScopeExport is a no-op in test builds.
func EnableScopeExport() {}

func AnalyzerBridgeStats() (calls, elementReads uint64) { return 0, 0 }

// ResetAnalyzerBridgeStats is a no-op in test builds.
func ResetAnalyzerBridgeStats() {}

// Through ChannelAnalyzerSnapshot so fixtures keep working.
func ChannelAnalyzerMetrics(id channel.ID) (peak, rms float64, clips int, active bool) {
	s := ChannelAnalyzerSnapshot(id)
	clips = s.ClipCount
	active = s.Peak > 0 || s.RMS > 0 || clips > 0
	return s.Peak, s.RMS, clips, active
}

var analyzerRegistryStub = map[channel.ID]*Analyzer{}

// Every per-instrument channel needs the synth, pre-EQ and channel analysers, as "main" does.
var (
	analyserEnableMu         sync.Mutex
	channelAnalyzerEnableCnt = map[channel.ID]int{}
	preEQAnalyzerEnableCnt   = map[channel.ID]int{}
	synthAnalyzerEnableCnt   = map[channel.ID]int{}
)

// So assertions describe a known window.
func ResetAnalyserEnableCounts() {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	for k := range channelAnalyzerEnableCnt {
		delete(channelAnalyzerEnableCnt, k)
	}
	for k := range preEQAnalyzerEnableCnt {
		delete(preEQAnalyzerEnableCnt, k)
	}
	for k := range synthAnalyzerEnableCnt {
		delete(synthAnalyzerEnableCnt, k)
	}
}

func ChannelAnalyzerEnableCount(id channel.ID) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return channelAnalyzerEnableCnt[id]
}

func PreEQAnalyzerEnableCount(id channel.ID) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return preEQAnalyzerEnableCnt[id]
}

func SynthAnalyzerEnableCount(id channel.ID) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return synthAnalyzerEnableCnt[id]
}

// Re-issuing reuses the wired analyzer; the counter still counts every call, since tests assert on how often the UI
// asked.
func EnableChannelAnalyzer(id channel.ID, window int) *Analyzer {
	analyserEnableMu.Lock()
	channelAnalyzerEnableCnt[id]++
	analyserEnableMu.Unlock()
	an := wireChannelAnalyzer(id, window)
	analyzerRegistryStub[id] = an
	return an
}

func ChannelAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	if an, ok := analyzerRegistryStub[id]; ok {
		return an.snapshot()
	}
	return AnalyzerSnapshot{}
}

var preEQAnalyzerRegistryStub = map[channel.ID]*Analyzer{}

// EnablePreEQAnalyzer creates a pre-EQ analyzer on the channel (stub version).
func EnablePreEQAnalyzer(id channel.ID, window int) *Analyzer {
	analyserEnableMu.Lock()
	preEQAnalyzerEnableCnt[id]++
	analyserEnableMu.Unlock()
	an := NewAnalyzer(window)
	ch := chanMgr.ensureChannel(id)
	ch.mu.Lock()
	ch.preEQAnalyzer = an
	ch.mu.Unlock()
	preEQAnalyzerRegistryStub[id] = an
	return an
}

// PreEQAnalyzerSnapshot returns the latest pre-EQ snapshot (stub version).
func PreEQAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot {
	if an, ok := preEQAnalyzerRegistryStub[id]; ok {
		return an.snapshot()
	}
	return AnalyzerSnapshot{}
}

// No JS bridge under -tags test; the call is still counted.
func EnableSynthAnalyzer(id channel.ID, window int) *Analyzer {
	analyserEnableMu.Lock()
	synthAnalyzerEnableCnt[id]++
	analyserEnableMu.Unlock()
	return nil
}

// SynthAnalyzerSnapshot is a no-op under -tags test.
func SynthAnalyzerSnapshot(id channel.ID) AnalyzerSnapshot { return AnalyzerSnapshot{} }

// SetAnalyzerEnabled enables or disables FFT compute for a channel's analyzers (stub).
func SetAnalyzerEnabled(id channel.ID, on bool) {
	if an, ok := analyzerRegistryStub[id]; ok {
		an.SetEnabled(on)
	}
	if pre, ok := preEQAnalyzerRegistryStub[id]; ok {
		pre.SetEnabled(on)
	}
}

func resetAnalyzers() {
	analyzerRegistryStub = map[channel.ID]*Analyzer{}
	preEQAnalyzerRegistryStub = map[channel.ID]*Analyzer{}
}

// Without unregistering them.
func ResetAllChannelAnalyzers() {
	for _, an := range analyzerRegistryStub {
		an.Reset()
	}
	for _, an := range preEQAnalyzerRegistryStub {
		an.Reset()
	}
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
