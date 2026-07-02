//go:build test

package audio

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/scope"
	"github.com/ingyamilmolinar/beatmo/internal/scopeexport"
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
// Records the trigger timestamp so tests can verify the UI's trigger-
// pulse pipeline (Phase 4 audio-panel redesign) without instantiating
// a real oto context.
func Play(id string, when ...float64) { RecordVoiceTrigger(id) }

// PlayVol is a stub used during tests for volume-controlled playback.
func PlayVol(id string, vol float64, when ...float64) { RecordVoiceTrigger(id) }

// PlayParams is a stub used during tests to accept extended playback params.
func PlayParams(id string, vol, pitch, dur float64, when ...float64) {
	RecordVoiceTrigger(id)
}

// PlayParamsAt is a stub used during tests to avoid varargs allocation.
func PlayParamsAt(id string, vol, pitch, dur, when float64) {
	RecordVoiceTrigger(id)
}

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
	clearInstrumentDisplayNames()
	bindBuiltinInstrumentRecipes()
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

// ConfigureSendDelay is a no-op in test builds.
func ConfigureSendDelay(timeMs, feedback, dampingHz float64) {
	stubSendDelayParams = [3]float64{timeMs, feedback, dampingHz}
}

// ConfigureSendReverb is a no-op in test builds.
func ConfigureSendReverb(room, damping, wet float64) {
	stubSendReverbParams = [3]float64{room, damping, wet}
}

// stubSendDelayParams stores the last configured delay parameters for test queries.
var stubSendDelayParams = [3]float64{300, 0.3, 3000}

// stubSendReverbParams stores the last configured reverb parameters for test queries.
var stubSendReverbParams = [3]float64{0.7, 0.4, 0.3}

// SendDelayParams returns the current delay send configuration (test stub).
func SendDelayParams() (float64, float64, float64) {
	return stubSendDelayParams[0], stubSendDelayParams[1], stubSendDelayParams[2]
}

// SendReverbParams returns the current reverb send configuration (test stub).
func SendReverbParams() (float64, float64, float64) {
	return stubSendReverbParams[0], stubSendReverbParams[1], stubSendReverbParams[2]
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
	recordChannelEQ(id, eqRecord{ID: id, SampleRate: sampleRate, Bands: append([]EQBand(nil), bands...)})
	RebuildChannelWithEQ(id, NewEQProcessor(sampleRate, bands...))
}

func ClearChannelProcessors(id string) {
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

// AnalyzerBridgeStats returns zero counters in test builds; the real
// counters live behind the WASM bridge in analyzer_wasm.go.
func AnalyzerBridgeStats() (calls, elementReads uint64) { return 0, 0 }

// ResetAnalyzerBridgeStats is a no-op in test builds.
func ResetAnalyzerBridgeStats() {}

// ChannelAnalyzerMetrics returns the scalar fields of the latest
// snapshot. In test builds we route through ChannelAnalyzerSnapshot so
// existing fixtures keep working; the WASM build uses a dedicated path
// that skips the per-element JS array readback.
func ChannelAnalyzerMetrics(id string) (peak, rms float64, clips int, active bool) {
	s := ChannelAnalyzerSnapshot(id)
	clips = s.ClipCount
	active = s.Peak > 0 || s.RMS > 0 || clips > 0
	return s.Peak, s.RMS, clips, active
}

var analyzerRegistryStub = map[string]*Analyzer{}

// eqRecord, lastSetEQ, LastSetEQ — moved to eq_state.go so the test build
// and the native build share one implementation. Phase 5 of the synthesis
// remediation plan also added recordChannelEQ + lookupChannelEQ for the
// per-channel EQ storage that effect_chain.go now reads.

// Per-id call counters for the three analyser-enable entry points. Used by
// tests to assert the Chn-tab analyser-enable contract (every per-instrument
// channel needs synth + preEQ + channel analysers, mirroring "main").
var (
	analyserEnableMu         sync.Mutex
	channelAnalyzerEnableCnt = map[string]int{}
	preEQAnalyzerEnableCnt   = map[string]int{}
	synthAnalyzerEnableCnt   = map[string]int{}
	sendBusAnalyzerEnableCnt int
)

// ResetAnalyserEnableCounts zeroes every analyser-enable counter. Tests call
// this at the start of a case so the assertions describe a known window.
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
	sendBusAnalyzerEnableCnt = 0
}

// ChannelAnalyzerEnableCount returns how many times EnableChannelAnalyzer
// has been called for id since the last reset.
func ChannelAnalyzerEnableCount(id string) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return channelAnalyzerEnableCnt[id]
}

// PreEQAnalyzerEnableCount returns how many times EnablePreEQAnalyzer has
// been called for id since the last reset.
func PreEQAnalyzerEnableCount(id string) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return preEQAnalyzerEnableCnt[id]
}

// SynthAnalyzerEnableCount returns how many times EnableSynthAnalyzer has
// been called for id since the last reset.
func SynthAnalyzerEnableCount(id string) int {
	analyserEnableMu.Lock()
	defer analyserEnableMu.Unlock()
	return synthAnalyzerEnableCnt[id]
}

func EnableChannelAnalyzer(id string, window int) *Analyzer {
	analyserEnableMu.Lock()
	channelAnalyzerEnableCnt[id]++
	analyserEnableMu.Unlock()
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
func PreEQAnalyzerSnapshot(id string) AnalyzerSnapshot {
	if an, ok := preEQAnalyzerRegistryStub[id]; ok {
		return an.snapshot()
	}
	return AnalyzerSnapshot{}
}

// EnableSynthAnalyzer is a no-op under -tags test (no WebAudio JS bridge,
// no real per-stage analyser nodes). Returns nil to mirror the WASM signature.
// The call is still counted so the Chn-tab contract test
// (chain_tab_per_instrument_test.go) can assert it was invoked for the right id.
func EnableSynthAnalyzer(id string, window int) *Analyzer {
	analyserEnableMu.Lock()
	synthAnalyzerEnableCnt[id]++
	analyserEnableMu.Unlock()
	return nil
}

// SynthAnalyzerSnapshot is a no-op under -tags test.
func SynthAnalyzerSnapshot(id string) AnalyzerSnapshot { return AnalyzerSnapshot{} }

// EnableSendBusAnalyzer is a no-op under -tags test.
func EnableSendBusAnalyzer(window int) *Analyzer { return nil }

// SendBusAnalyzerSnapshot is a no-op under -tags test.
func SendBusAnalyzerSnapshot() AnalyzerSnapshot { return AnalyzerSnapshot{} }

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
