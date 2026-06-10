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
//
// SAFETY: the returned Waveform slice shares its backing array with
// s.Waveform. The audio.AnalyzerSnapshot owner is the per-channel cache
// in internal/audio/analyzer_snapshot_cache.go, which mutates the slice
// only on TTL-driven refresh; the upstream BuildAnalyzerStateFromSnapshots
// State cache pins the snapshot for the same TTL window, so within one
// State lifetime no mutation can occur. Consumers must not retain or
// modify the returned slice past the State's TTL. Removing the previous
// `append([]float64(nil), s.Waveform...)` copy eliminated ~4 KB per
// channel-per-state-rebuild on the WASM hot path.
func SnapshotToChannelMetrics(id, name string, s audio.AnalyzerSnapshot, sr int) analyzer.ChannelMetrics {
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

// ScopeStageSnapshots groups the four per-stage WASM analyzer snapshots used
// by scopeTapFromSnapshot. Two helpers (synth + master) plus the existing
// preEQ/postEQ pair give us coverage for all six scope.Stage values.
//
//   - Synth   → channel ingress, before any inserts. Also reused for AntiPop:
//     the per-source anti-pop GainNode envelope is applied at the source
//     before the bus, so by the time the signal arrives at ingress it has
//     already passed through anti-pop. Tapping ingress is therefore the
//     correct WASM representation for both StageSynth and StageAntiPop.
//   - PreEQ   → channel preEQAnalyser (post-inserts, pre-EQ). Feeds
//     StageInsertFX.
//   - PostEQ  → channel main analyser (post-EQ). Feeds StageEQ.
//   - Sends   → shared send-bus analyser (delay + reverb returns summed).
//     Feeds StageSends.
//   - Master  → main channel analyser. Feeds StageMaster.
type ScopeStageSnapshots struct {
	Synth  audio.AnalyzerSnapshot
	PreEQ  audio.AnalyzerSnapshot
	PostEQ audio.AnalyzerSnapshot
	Sends  audio.AnalyzerSnapshot
	Master audio.AnalyzerSnapshot
}

// scopeTapFromSnapshot builds a TapData for a given stage using the per-stage
// snapshots. Returns an inactive tap (Active=false, no Samples) when the
// chosen stage's snapshot has no signal — that's how the renderer detects
// "JS bridge hasn't enabled this analyser yet" and skips drawing a trace.
func scopeTapFromSnapshot(stage scope.Stage, instID string, snaps ScopeStageSnapshots) scope.TapData {
	var snap audio.AnalyzerSnapshot
	switch stage {
	case scope.StageSynth, scope.StageAntiPop:
		snap = snaps.Synth
	case scope.StageInsertFX:
		snap = snaps.PreEQ
	case scope.StageEQ:
		snap = snaps.PostEQ
	case scope.StageSends:
		snap = snaps.Sends
	case scope.StageMaster:
		snap = snaps.Master
	default:
		return scope.TapData{Stage: stage, InstID: instID, Active: false}
	}
	// Same shared-slice contract as SnapshotToChannelMetrics: snap.Waveform
	// is owned by the per-channel snapshot cache and stable for the State
	// cache's TTL window. Removing the previous fresh-slice copy eliminated
	// ~4 KB per tap-per-state-rebuild.
	return scope.TapData{
		Stage:   stage,
		InstID:  instID,
		Samples: snap.Waveform,
		PeakDB:  LinearToDBSafe(snap.Peak),
		RMSDB:   LinearToDBSafe(snap.RMS),
		Active:  snapshotIsActive(snap),
	}
}

// SynthesizeScopeState builds a scope.State with two taps fed from WASM
// per-stage snapshots. The caller supplies the zone-local tapA/tapB stage
// selection and the four+1 per-stage snapshots (see ScopeStageSnapshots).
func SynthesizeScopeState(
	instID string,
	tapA, tapB scope.Stage,
	snaps ScopeStageSnapshots,
) *scope.State {
	return &scope.State{
		TapA: scopeTapFromSnapshot(tapA, instID, snaps),
		TapB: scopeTapFromSnapshot(tapB, instID, snaps),
	}
}
