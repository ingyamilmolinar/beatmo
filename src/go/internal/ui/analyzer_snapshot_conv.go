package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// analyzerDBFloor is the minimum dB value we report for silent/zero signals.
// Matches the rendering floor used by drawAnalyzerSpectrum (spectrumMinDB = -80).
const analyzerDBFloor = -80.0

// LinearToDBSafe converts a linear amplitude to dB, clamping to analyzerDBFloor.
// A non-positive input returns the floor instead of -Inf so downstream renderers
// can draw predictable bars.
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

// SpectrumLinearToDBBins converts a linear-magnitude spectrum into the paired
// (FFTBins in dB, FreqBins in Hz) arrays the spectrum renderer expects.
// FreqBins[i] = i * (sampleRate/2) / n, matching a standard FFT bin-to-Hz mapping.
// Returns nil,nil when the input is empty.
func SpectrumLinearToDBBins(spec []float64, sampleRate int) ([]float64, []float64) {
	n := len(spec)
	if n == 0 {
		return nil, nil
	}
	fft := make([]float64, n)
	freq := make([]float64, n)
	nyquist := float64(sampleRate) / 2.0
	for i := 0; i < n; i++ {
		fft[i] = LinearToDBSafe(spec[i])
		freq[i] = float64(i) * nyquist / float64(n)
	}
	return fft, freq
}

// snapshotIsActive returns true when the snapshot carries any signal —
// used to drive the `Active` flag on both channel and instrument metrics.
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

// SnapshotToChannelMetrics converts a raw WASM AnalyzerSnapshot into the
// analyzer.ChannelMetrics struct the UI renderers consume. RMS/Peak come in
// as linear amplitudes on WASM and must be converted to dB to match desktop.
func SnapshotToChannelMetrics(id, name string, s audio.AnalyzerSnapshot, sr int) analyzer.ChannelMetrics {
	fft, freq := SpectrumLinearToDBBins(s.Spectrum, sr)
	var wave []float64
	if len(s.Waveform) > 0 {
		wave = append([]float64(nil), s.Waveform...)
	}
	return analyzer.ChannelMetrics{
		ID:       id,
		Name:     name,
		PeakDB:   LinearToDBSafe(s.Peak),
		RMSDB:    LinearToDBSafe(s.RMS),
		Waveform: wave,
		FFTBins:  fft,
		FreqBins: freq,
		Active:   snapshotIsActive(s),
	}
}

// SnapshotToInstrumentMetrics returns the lightweight per-instrument record
// the meter bridge draws.
func SnapshotToInstrumentMetrics(id, name string, s audio.AnalyzerSnapshot) analyzer.InstrumentMetrics {
	return analyzer.InstrumentMetrics{
		ID:     id,
		Name:   name,
		PeakDB: LinearToDBSafe(s.Peak),
		RMSDB:  LinearToDBSafe(s.RMS),
		Active: snapshotIsActive(s),
	}
}

// RowSnapshot pairs a row's identity with its analyzer snapshot so
// SynthesizeAnalyzerState can build per-instrument metrics in one pass.
type RowSnapshot struct {
	ID   string
	Name string
	Snap audio.AnalyzerSnapshot
}

// SynthesizeAnalyzerState assembles a full analyzer.State from the master
// snapshot, a list of row snapshots, and an optional detail snapshot (used when
// the EQ panel has an instrument selected for detailed waveform/spectrum).
//
// Pure function — no build tags, no platform calls — so it is directly testable.
func SynthesizeAnalyzerState(
	masterID, masterName string,
	masterSnap audio.AnalyzerSnapshot,
	rows []RowSnapshot,
	detail *audio.AnalyzerSnapshot,
	detailID, detailName string,
	sampleRate int,
) *analyzer.State {
	instruments := make([]analyzer.InstrumentMetrics, 0, len(rows))
	for _, r := range rows {
		instruments = append(instruments, SnapshotToInstrumentMetrics(r.ID, r.Name, r.Snap))
	}
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

// scopeTapFromSnapshot builds a TapData for a given stage. Only StageSynth and
// StageEQ are bridged on WASM (pre-EQ and post-EQ taps); other stages return an
// inactive tap so the renderer shows a blank trace.
func scopeTapFromSnapshot(stage scope.Stage, instID string, preSnap, postSnap audio.AnalyzerSnapshot) scope.TapData {
	var snap audio.AnalyzerSnapshot
	switch stage {
	case scope.StageSynth:
		snap = preSnap
	case scope.StageEQ:
		snap = postSnap
	default:
		return scope.TapData{Stage: stage, InstID: instID, Active: false}
	}
	var samples []float64
	if len(snap.Waveform) > 0 {
		samples = append([]float64(nil), snap.Waveform...)
	}
	return scope.TapData{
		Stage:   stage,
		InstID:  instID,
		Samples: samples,
		PeakDB:  LinearToDBSafe(snap.Peak),
		RMSDB:   LinearToDBSafe(snap.RMS),
		Active:  snapshotIsActive(snap),
	}
}

// SynthesizeScopeState builds a scope.State with two taps fed from WASM
// pre/post-EQ snapshots. The caller supplies the zone-local tapA/tapB stage
// selection, so stage buttons in the UI still drive which data source lands
// in which tap (Synth→pre-EQ, EQ→post-EQ; other stages render inactive).
func SynthesizeScopeState(
	instID string,
	tapA, tapB scope.Stage,
	preSnap, postSnap audio.AnalyzerSnapshot,
) *scope.State {
	return &scope.State{
		TapA: scopeTapFromSnapshot(tapA, instID, preSnap, postSnap),
		TapB: scopeTapFromSnapshot(tapB, instID, preSnap, postSnap),
	}
}
