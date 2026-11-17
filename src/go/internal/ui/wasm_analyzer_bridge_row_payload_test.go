//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// Only Master and Detail carry slice data in an analyzer.State. A row
// becomes an InstrumentMetrics, which holds two levels and two flags — so
// pulling a full snapshot per row hauls a Spectrum and a Waveform across
// the JS bridge for the builder to discard.
//
// That was merely wasteful while the bridge downsampled the spectrum to 64
// bins. Once it carries the analyser's full bin count — which it must, or
// the low display bands hold no bins at all (analyzer.SpectrumFFTSize) —
// the same waste is 16x bigger, on the hot path that drove a production
// OOM once already. Rows go through the scalar-only bridge call instead.
func TestAnalyzerStateFetchesSliceDataOnlyForMasterAndDetail(t *testing.T) {
	rows := fakeRows(6)

	var snapshotFetches []string
	snap := func(id channel.ID) audio.AnalyzerSnapshot {
		snapshotFetches = append(snapshotFetches, id.String())
		return audio.AnalyzerSnapshot{
			RMS:      0.2,
			Peak:     0.5,
			Spectrum: make([]float64, audio.BridgeSpectrumBins),
			Waveform: make([]float64, audio.BridgeWaveformSamples),
		}
	}
	var metricFetches []string
	metrics := func(id channel.ID) (float64, float64, int, bool) {
		metricFetches = append(metricFetches, id.String())
		return 0.5, 0.2, 0, true
	}

	st := buildAnalyzerStateFromSnapshotsImpl(channel.Of("inst3"), rows, 48000, snap, metrics)
	if st == nil {
		t.Fatal("builder returned nil")
	}

	// Master + the one detail channel, and nothing else.
	want := []string{"main", "inst3"}
	if len(snapshotFetches) != len(want) {
		t.Fatalf("full snapshots fetched for %v, want exactly %v — every extra one is a "+
			"Spectrum+Waveform across the bridge that the builder throws away",
			snapshotFetches, want)
	}
	for i, id := range want {
		if snapshotFetches[i] != id {
			t.Errorf("full snapshot %d: got %q, want %q", i, snapshotFetches[i], id)
		}
	}

	// Every row still reported, via the scalar path.
	if len(metricFetches) != len(rows) {
		t.Errorf("scalar metrics fetched for %d channels, want %d (one per row)", len(metricFetches), len(rows))
	}
	if len(st.Instruments) != len(rows) {
		t.Fatalf("state carries %d instruments, want %d", len(st.Instruments), len(rows))
	}
	for _, im := range st.Instruments {
		if im.PeakDB <= analyzerDBFloor || !im.Active {
			t.Errorf("instrument %q lost its levels on the scalar path: %+v", im.ID, im)
		}
	}

	// Detail still carries the slice data the Spectrum tab draws from.
	if st.Detail == nil || len(st.Detail.FFTBins) != audio.BridgeSpectrumBins {
		t.Errorf("detail channel lost its spectrum: %+v", st.Detail)
	}
	if len(st.Master.FFTBins) != audio.BridgeSpectrumBins {
		t.Errorf("master lost its spectrum: %d bins", len(st.Master.FFTBins))
	}
}
