package ui

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Must match the spectrum renderer's floor (spectrumMinDB).
const analyzerDBFloor = -80.0

// The floor instead of -Inf, so renderers draw predictable bars.
func LinearToDBSafe(v float64) float64 {
	if v <= 0 {
		return analyzerDBFloor
	}
	db := 20 * math.Log10(v)
	if db < analyzerDBFloor {
		return analyzerDBFloor
	}
	return db
}

// A pure function of (bins, rate), memoized: at the full bin count the bridge must carry, rebuilding it was
// 8 KB per channel ~30 times a second. The slice is SHARED and read-only.
var freqBinsCache sync.Map // freqBinsKey -> []float64

type freqBinsKey struct {
	n          int
	sampleRate int
}

func freqBinsFor(n, sampleRate int) []float64 {
	key := freqBinsKey{n: n, sampleRate: sampleRate}
	if v, ok := freqBinsCache.Load(key); ok {
		return v.([]float64)
	}
	freq := make([]float64, n)
	nyquist := float64(sampleRate) / 2.0
	for i := 0; i < n; i++ {
		freq[i] = float64(i) * nyquist / float64(n)
	}
	freqBinsCache.Store(key, freq)
	return freq
}

// The FreqBins half is memoized and SHARED. nil,nil for an empty input.
func SpectrumLinearToDBBins(spec []float64, sampleRate int) ([]float64, []float64) {
	n := len(spec)
	if n == 0 {
		return nil, nil
	}
	fft := make([]float64, n)
	for i := 0; i < n; i++ {
		fft[i] = LinearToDBSafe(spec[i])
	}
	return fft, freqBinsFor(n, sampleRate)
}

func snapshotIsActive(s audio.AnalyzerSnapshot) bool {
	if s.Peak > 0 || s.RMS > 0 {
		return true
	}
	for _, v := range s.Waveform {
		if v != 0 {
			return true
		}
	}
	for _, v := range s.Spectrum {
		if v != 0 {
			return true
		}
	}
	return false
}

// WASM levels arrive linear and are converted to dB to match desktop. Waveform SHARES s.Waveform's backing
// array: it is stable for the State cache's TTL, and consumers must not keep or modify it past that.
func SnapshotToChannelMetrics(id channel.ID, name string, s audio.AnalyzerSnapshot, sr int) analyzer.ChannelMetrics {
	fft, freq := SpectrumLinearToDBBins(s.Spectrum, sr)
	return analyzer.ChannelMetrics{
		ID:       id,
		Name:     name,
		PeakDB:   LinearToDBSafe(s.Peak),
		RMSDB:    LinearToDBSafe(s.RMS),
		Waveform: s.Waveform,
		FFTBins:  fft,
		FreqBins: freq,
		Active:   snapshotIsActive(s),
	}
}

func SnapshotToInstrumentMetrics(id channel.ID, name string, s audio.AnalyzerSnapshot) analyzer.InstrumentMetrics {
	return analyzer.InstrumentMetrics{
		ID:     id,
		Name:   name,
		PeakDB: LinearToDBSafe(s.Peak),
		RMSDB:  LinearToDBSafe(s.RMS),
		Active: snapshotIsActive(s),
	}
}

type RowSnapshot struct {
	ID   channel.ID
	Name string
	Snap audio.AnalyzerSnapshot
}

// Pure (no build tags, no platform calls), so it is directly testable.
func SynthesizeAnalyzerState(
	masterID channel.ID, masterName string,
	masterSnap audio.AnalyzerSnapshot,
	rows []RowSnapshot,
	detail *audio.AnalyzerSnapshot,
	detailID channel.ID, detailName string,
	sampleRate int,
) *analyzer.State {
	instruments := make([]analyzer.InstrumentMetrics, 0, len(rows))
	for _, r := range rows {
		instruments = append(instruments, SnapshotToInstrumentMetrics(r.ID, r.Name, r.Snap))
	}
	return SynthesizeAnalyzerStateWithInstruments(masterID, masterName, masterSnap, instruments,
		detail, detailID, detailName, sampleRate)
}

// For callers that already hold the per-row metrics, like the WASM builder reading rows through the
// scalar-only bridge call.
func SynthesizeAnalyzerStateWithInstruments(
	masterID channel.ID, masterName string,
	masterSnap audio.AnalyzerSnapshot,
	instruments []analyzer.InstrumentMetrics,
	detail *audio.AnalyzerSnapshot,
	detailID channel.ID, detailName string,
	sampleRate int,
) *analyzer.State {
	var detailPtr *analyzer.ChannelMetrics
	if detail != nil {
		m := SnapshotToChannelMetrics(detailID, detailName, *detail, sampleRate)
		detailPtr = &m
	}
	return &analyzer.State{
		Instruments: instruments,
		Master:      SnapshotToChannelMetrics(masterID, masterName, masterSnap, sampleRate),
		Detail:      detailPtr,
	}
}

// Ingress serves both Synth and AntiPop: the anti-pop envelope is applied at the source before the bus.
// PreEQ feeds InsertFX, PostEQ feeds EQ, Master feeds Master.
type ScopeStageSnapshots struct {
	Synth  audio.AnalyzerSnapshot
	PreEQ  audio.AnalyzerSnapshot
	PostEQ audio.AnalyzerSnapshot
	Master audio.AnalyzerSnapshot
}

// An inactive tap is how the renderer detects an analyser the JS bridge has not enabled yet.
func scopeTapFromSnapshot(stage scope.Stage, instID channel.ID, snaps ScopeStageSnapshots) scope.TapData {
	var snap audio.AnalyzerSnapshot
	switch stage {
	case scope.StageSynth, scope.StageAntiPop:
		snap = snaps.Synth
	case scope.StageInsertFX:
		snap = snaps.PreEQ
	case scope.StageEQ:
		snap = snaps.PostEQ
	case scope.StageMaster:
		snap = snaps.Master
	default:
		return scope.TapData{Stage: stage, InstID: instID, Active: false}
	}
	// Same shared-slice contract as SnapshotToChannelMetrics.
	return scope.TapData{
		Stage:   stage,
		InstID:  instID,
		Samples: snap.Waveform,
		PeakDB:  LinearToDBSafe(snap.Peak),
		RMSDB:   LinearToDBSafe(snap.RMS),
		Active:  snapshotIsActive(snap),
	}
}

func SynthesizeScopeState(
	instID channel.ID,
	tapA, tapB scope.Stage,
	snaps ScopeStageSnapshots,
) *scope.State {
	return &scope.State{
		TapA: scopeTapFromSnapshot(tapA, instID, snaps),
		TapB: scopeTapFromSnapshot(tapB, instID, snaps),
	}
}
