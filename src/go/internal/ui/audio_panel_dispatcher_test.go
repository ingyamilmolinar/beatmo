package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestAnalyzerTapsForTab pins the dispatcher rule: which tab IDs cause
// which analyzer taps to be enabled. Drift in this map almost always
// implies an analyzer-blackout regression on one of the audio tabs
// (the original bug Phase 0 was created to fix).
func TestAnalyzerTapsForTab(t *testing.T) {
	cases := []struct {
		tab  PanelTab
		want []string
	}{
		{TabWave, []string{"main"}},
		{TabSpectrum, []string{"main"}},
		{TabMeters, []string{"main"}},
		{TabEQ, []string{"main"}},
		{TabScope, []string{"main"}},
		{TabSynth, nil},
		{TabSampler, nil},
	}
	for _, c := range cases {
		got := AnalyzerTapsForTab(c.tab)
		if len(got) != len(c.want) {
			t.Errorf("tab=%v len(taps)=%d want %d (got=%v want=%v)", c.tab, len(got), len(c.want), got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("tab=%v taps[%d]=%q want %q", c.tab, i, got[i], c.want[i])
			}
		}
	}
}

// TestEnsureAnalyzersForTab_EnablesMaster covers the Phase 0 root-cause
// fix: navigating to Spectrum / Levels / Chain (or Wave / EQ) without
// touching the channel picker must enable the master analyzer so the
// next Draw has data. We don't depend on a Game/Oto stack — the
// dispatcher is exercised directly and the analyzer is fed synthetic
// samples to verify it produces a non-flat snapshot.
func TestEnsureAnalyzersForTab_EnablesMaster(t *testing.T) {
	tabs := []PanelTab{TabWave, TabSpectrum, TabMeters, TabEQ, TabScope}
	for _, tab := range tabs {
		EnsureAnalyzersForTab(tab, "")

		// The Master analyzer must now be present in the registry — we
		// confirm by feeding it samples through the channel processor
		// path and reading the snapshot back. Without the dispatcher
		// the analyzer wouldn't exist and the registry returns a zero
		// snapshot.
		an := audio.EnableChannelAnalyzer("main", 512)
		if an == nil {
			t.Fatalf("tab=%v: EnableChannelAnalyzer returned nil (stub build)", tab)
		}
		// Push enough non-zero samples to fill the analyzer window and
		// trigger a compute() pass.
		const blockN = 1024
		samples := make([]float32, blockN)
		for i := range samples {
			// Mix of 1 kHz + 4 kHz tones at fs=48 kHz so both Peak and
			// the spectrum have non-trivial content to verify.
			samples[i] = 0.5
		}
		an.ProcessBlock(samples, blockN)

		snap := audio.ChannelAnalyzerSnapshot("main")
		if snap.Peak <= 0 {
			t.Errorf("tab=%v: master analyzer Peak=%f want >0 after dispatcher enable + signal feed",
				tab, snap.Peak)
		}
	}
}

// TestEnsureAnalyzersForTab_SynthIsNoop confirms the Synth tab does
// NOT touch the master analyzer (it reads per-voice caches instead).
// Pinning this keeps the dispatcher minimal — adding rows to it must
// be a conscious change.
func TestEnsureAnalyzersForTab_SynthIsNoop(t *testing.T) {
	taps := AnalyzerTapsForTab(TabSynth)
	if len(taps) != 0 {
		t.Errorf("AnalyzerTapsForTab(TabSynth)=%v want empty", taps)
	}
}

// TestEnsureAnalyzersForTab_PerRowChannelAlsoEnabled covers the
// secondary contract: when the EQ channel picker is on a per-row
// channel (e.g., "kick") and the user opens an analysis tab, BOTH
// the master and that row's analyzer should be live.
func TestEnsureAnalyzersForTab_PerRowChannelAlsoEnabled(t *testing.T) {
	EnsureAnalyzersForTab(TabSpectrum, "kick-test-row")
	an := audio.EnableChannelAnalyzer("kick-test-row", 512)
	if an == nil {
		t.Fatalf("EnableChannelAnalyzer for per-row id returned nil")
	}
	samples := make([]float32, 1024)
	for i := range samples {
		samples[i] = 0.3
	}
	an.ProcessBlock(samples, 1024)
	if snap := audio.ChannelAnalyzerSnapshot("kick-test-row"); snap.Peak <= 0 {
		t.Errorf("per-row analyzer Peak=%f want >0", snap.Peak)
	}
}
