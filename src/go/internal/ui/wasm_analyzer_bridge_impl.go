package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// channelSnapshotFunc abstracts audio.ChannelAnalyzerSnapshot so the
// pure-loop builder below can be exercised from -tags test without a
// live WASM bridge. The WASM build wires the real bridge; tests inject
// fakes that return precomputed snapshots.
type channelSnapshotFunc func(id channel.ID) audio.AnalyzerSnapshot

// channelMetricsFunc abstracts audio.ChannelAnalyzerMetrics (the
// scalar-only fast path). Same injection pattern as channelSnapshotFunc.
type channelMetricsFunc func(id channel.ID) (peak, rms float64, clips int, active bool)

// buildAnalyzerStateFromSnapshotsImpl walks the rows, fetches per-channel
// snapshots via fetch, and assembles an analyzer.State. Pure function —
// no build tags, no platform calls, suitable for AllocsPerRun budgeting.
//
// Mirrors the precedence the WASM BuildAnalyzerStateFromSnapshots used
// to inline: master + every populated row + optional active detail.
// Only TWO channels in the returned State carry slice data: Master and
// Detail. Every row becomes an analyzer.InstrumentMetrics, which holds
// PeakDB / RMSDB / Active and nothing else — so a row fetched through
// `fetch` hauls a full Spectrum + Waveform across the JS bridge for the
// builder to throw away. `metrics` is the scalar-only bridge call
// (audio.ChannelAnalyzerMetrics) and rows go through it instead.
//
// That waste was affordable while the bridge downsampled the spectrum to
// 64 bins. It is not now: the spectrum carries the analyser's full bin
// count so the low display bands actually contain bins (see
// analyzer.SpectrumFFTSize), which is 16x the payload per channel.
//
// Measured per state rebuild, 6 rows, the two changes roughly cancel:
// bridge reads fall from ~32 KB (7 distinct channels x 64-bin spectrum +
// 512-sample wave) to ~24 KB (2 channels x 1024-bin spectrum + 512-sample
// wave), while the builder's own dB conversion rises from 2.1 KB to
// 17.5 KB (TestBuildAnalyzerStateBytesPerCall; FreqBins is memoized, or it
// would be double that). Net ~+20% for 16x the frequency resolution —
// without routing rows to the scalar path it would have been ~+120%.
//
// `metrics` may be nil, in which case rows fall back to `fetch` and only
// the scalars are used; the -tags test harness relies on that.
func buildAnalyzerStateFromSnapshotsImpl(activeID channel.ID, rows []*DrumRow, sampleRate int, fetch channelSnapshotFunc, metrics channelMetricsFunc) *analyzer.State {
	if fetch == nil {
		return nil
	}
	masterSnap := fetch(channel.Master)

	instruments := make([]analyzer.InstrumentMetrics, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		if metrics != nil {
			peak, rms, clips, active := metrics(channel.Of(r.Instrument))
			instruments = append(instruments, analyzer.InstrumentMetrics{
				ID:        channel.Of(r.Instrument),
				Name:      r.Name,
				PeakDB:    LinearToDBSafe(peak),
				RMSDB:     LinearToDBSafe(rms),
				ClipCount: clips,
				Active:    active,
			})
			continue
		}
		instruments = append(instruments, SnapshotToInstrumentMetrics(channel.Of(r.Instrument), r.Name, fetch(channel.Of(r.Instrument))))
	}

	var detail *audio.AnalyzerSnapshot
	var detailID channel.ID
	var detailName string
	if _, isInstrument := activeID.Instrument(); isInstrument {
		d := fetch(activeID)
		detail = &d
		detailID = activeID
		for _, r := range rows {
			if r != nil && channel.Of(r.Instrument) == activeID {
				detailName = r.Name
				break
			}
		}
	}

	return SynthesizeAnalyzerStateWithInstruments(channel.Master, "Master", masterSnap, instruments, detail, detailID, detailName, sampleRate)
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
	masterPeak, masterRMS, masterClips, masterActive := fetch(channel.Master)

	instruments := make([]analyzer.InstrumentMetrics, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		peak, rms, clips, active := fetch(channel.Of(r.Instrument))
		instruments = append(instruments, analyzer.InstrumentMetrics{
			ID:        channel.Of(r.Instrument),
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
			ID:        channel.Master,
			Name:      "Master",
			PeakDB:    LinearToDBSafe(masterPeak),
			RMSDB:     LinearToDBSafe(masterRMS),
			ClipCount: masterClips,
			Active:    masterActive,
		},
	}
}
