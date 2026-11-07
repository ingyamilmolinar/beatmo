package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// So the builder runs under -tags test with fakes instead of the live bridge.
type channelSnapshotFunc func(id channel.ID) audio.AnalyzerSnapshot

type channelMetricsFunc func(id channel.ID) (peak, rms float64, clips int, active bool)

// Pure, for AllocsPerRun budgets. Only Master and Detail carry slices; rows read through the scalar-only
// metrics call, since a full fetch per row now carries the analyser's whole bin count only to be discarded. A nil metrics falls back to fetch.
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

// Only the scalars the Levels renderer reads: the per-frame slice churn drove a production OOM, and on WASM this
// makes ≤ 4 js.Value temporaries per channel instead of ~1024.
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
