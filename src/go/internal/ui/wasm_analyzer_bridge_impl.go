package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// channelSnapshotFunc abstracts audio.ChannelAnalyzerSnapshot so the
// pure-loop builder below can be exercised from -tags test without a
// live WASM bridge. The WASM build wires the real bridge; tests inject
// fakes that return precomputed snapshots.
type channelSnapshotFunc func(id string) audio.AnalyzerSnapshot

// channelMetricsFunc abstracts audio.ChannelAnalyzerMetrics (the
// scalar-only fast path). Same injection pattern as channelSnapshotFunc.
type channelMetricsFunc func(id string) (peak, rms float64, clips int, active bool)

// buildAnalyzerStateFromSnapshotsImpl walks the rows, fetches per-channel
// snapshots via fetch, and assembles an analyzer.State. Pure function —
// no build tags, no platform calls, suitable for AllocsPerRun budgeting.
//
// Mirrors the precedence the WASM BuildAnalyzerStateFromSnapshots used
// to inline: master + every populated row + optional active detail.
func buildAnalyzerStateFromSnapshotsImpl(activeID string, rows []*DrumRow, sampleRate int, fetch channelSnapshotFunc) *analyzer.State {
	if fetch == nil {
		return nil
	}
	masterSnap := fetch("main")

	rowSnaps := make([]RowSnapshot, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		rowSnaps = append(rowSnaps, RowSnapshot{
			ID:   r.Instrument,
			Name: r.Name,
			Snap: fetch(r.Instrument),
		})
	}

	var detail *audio.AnalyzerSnapshot
	var detailID, detailName string
	if activeID != "" && activeID != "main" {
		d := fetch(activeID)
		detail = &d
		detailID = activeID
		for _, r := range rows {
			if r != nil && r.Instrument == activeID {
				detailName = r.Name
				break
			}
		}
	}

	return SynthesizeAnalyzerState("main", "Master", masterSnap, rowSnaps, detail, detailID, detailName, sampleRate)
}

// buildAnalyzerMetricsOnlyImpl assembles a scalar-only analyzer.State
// using ONLY the lightweight ChannelAnalyzerMetrics path. No Spectrum,
// Waveform, FFTBins, FreqBins, or Detail — just the fields the Meters
// renderer (render_meters.go:38-108) actually reads. Replaces the
// per-frame slice churn that drove the production OOM.
//
// On WASM this also bypasses the per-element js.Value loop in the
// snapshot bridge — only scalar fields are read, so the whole call
// produces ≤ 4 js.Value temporaries per channel instead of ~1024.
func buildAnalyzerMetricsOnlyImpl(rows []*DrumRow, fetch channelMetricsFunc) *analyzer.State {
	if fetch == nil {
		return nil
	}
	masterPeak, masterRMS, masterClips, masterActive := fetch("main")

	instruments := make([]analyzer.InstrumentMetrics, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		peak, rms, clips, active := fetch(r.Instrument)
		instruments = append(instruments, analyzer.InstrumentMetrics{
			ID:        r.Instrument,
			Name:      r.Name,
			PeakDB:    LinearToDBSafe(peak),
			RMSDB:     LinearToDBSafe(rms),
			ClipCount: clips,
			Active:    active,
		})
	}

	return &analyzer.State{
		Instruments: instruments,
		Master: analyzer.ChannelMetrics{
			ID:        "main",
			Name:      "Master",
			PeakDB:    LinearToDBSafe(masterPeak),
			RMSDB:     LinearToDBSafe(masterRMS),
			ClipCount: masterClips,
			Active:    masterActive,
		},
	}
}
