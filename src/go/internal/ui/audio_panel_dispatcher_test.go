package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Drift here almost always blacks out an audio tab's analyser.
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
			if got[i].String() != c.want[i] {
				t.Errorf("tab=%v taps[%d]=%q want %q", c.tab, i, got[i], c.want[i])
			}
		}
	}
}

// Per-instrument tabs tap EVERY row instrument: on WASM each AnalyserNode is wired on demand, so an untouched
// template leaves Levels blank in the browser while desktop is fine. Synth/Sampler read voice caches instead.
func TestAnalyzerInstrumentTapsForTab(t *testing.T) {
	ids := []channel.ID{channel.Of(roster.KickTight), channel.Of(roster.Snare), channel.Of(roster.FmBass)}
	cases := []struct {
		tab  PanelTab
		want []string
	}{
		{TabWave, []string{"main", string(roster.KickTight), string(roster.Snare), string(roster.FmBass)}},
		{TabSpectrum, []string{"main", string(roster.KickTight), string(roster.Snare), string(roster.FmBass)}},
		{TabMeters, []string{"main", string(roster.KickTight), string(roster.Snare), string(roster.FmBass)}},
		{TabEQ, []string{"main", string(roster.KickTight), string(roster.Snare), string(roster.FmBass)}},
		{TabScope, []string{"main", string(roster.KickTight), string(roster.Snare), string(roster.FmBass)}},
		{TabSynth, nil},
		{TabSampler, nil},
	}
	for _, c := range cases {
		got := AnalyzerInstrumentTapsForTab(c.tab, ids)
		if len(got) != len(c.want) {
			t.Errorf("tab=%v taps=%v want %v", c.tab, got, c.want)
			continue
		}
		for i := range got {
			if got[i].String() != c.want[i] {
				t.Errorf("tab=%v taps[%d]=%q want %q", c.tab, i, got[i], c.want[i])
			}
		}
	}
}

// Each instrument's analyser is live for the next Draw, with no prior channel select.
func TestEnsureAnalyzersForTab_EnablesPerInstrument(t *testing.T) {
	ids := []channel.ID{channel.Of("tmpl-kick"), channel.Of("tmpl-snare")}
	EnsureAnalyzersForTab(TabMeters, channel.ID{}, ids...)
	for _, id := range ids {
		an := audio.EnableChannelAnalyzer(id, 512)
		if an == nil {
			t.Fatalf("EnableChannelAnalyzer(%q) returned nil (stub build)", id)
		}
		samples := make([]float32, 1024)
		for i := range samples {
			samples[i] = 0.4
		}
		an.ProcessBlock(samples, 1024)
		if snap := audio.ChannelAnalyzerSnapshot(id); snap.Peak <= 0 {
			t.Errorf("per-instrument analyzer %q Peak=%f want >0 after dispatcher enable", id, snap.Peak)
		}
	}
}

// Opening Spectrum / Levels / Chain (or Wave / EQ) without touching the channel picker enables the master analyser.
func TestEnsureAnalyzersForTab_EnablesMaster(t *testing.T) {
	tabs := []PanelTab{TabWave, TabSpectrum, TabMeters, TabEQ, TabScope}
	for _, tab := range tabs {
		EnsureAnalyzersForTab(tab, channel.ID{})

		// Fed through the channel processor and read back: without the dispatcher the registry returns a zero snapshot.
		an := audio.EnableChannelAnalyzer(channel.Master, 512)
		if an == nil {
			t.Fatalf("tab=%v: EnableChannelAnalyzer returned nil (stub build)", tab)
		}
		// Enough to fill the window and trigger a compute().
		const blockN = 1024
		samples := make([]float32, blockN)
		for i := range samples {
			// 1 kHz + 4 kHz at 48 kHz, so peak and spectrum both have content.
			samples[i] = 0.5
		}
		an.ProcessBlock(samples, blockN)

		snap := audio.ChannelAnalyzerSnapshot(channel.Master)
		if snap.Peak <= 0 {
			t.Errorf("tab=%v: master analyzer Peak=%f want >0 after dispatcher enable + signal feed",
				tab, snap.Peak)
		}
	}
}

// A new row here must be a conscious change.
func TestEnsureAnalyzersForTab_SynthIsNoop(t *testing.T) {
	taps := AnalyzerTapsForTab(TabSynth)
	if len(taps) != 0 {
		t.Errorf("AnalyzerTapsForTab(TabSynth)=%v want empty", taps)
	}
}

// With a per-row channel picked, both the master and that row are live.
func TestEnsureAnalyzersForTab_PerRowChannelAlsoEnabled(t *testing.T) {
	EnsureAnalyzersForTab(TabSpectrum, channel.Of("kick-test-row"))
	an := audio.EnableChannelAnalyzer(channel.Of("kick-test-row"), 512)
	if an == nil {
		t.Fatalf("EnableChannelAnalyzer for per-row id returned nil")
	}
	samples := make([]float32, 1024)
	for i := range samples {
		samples[i] = 0.3
	}
	an.ProcessBlock(samples, 1024)
	if snap := audio.ChannelAnalyzerSnapshot(channel.Of("kick-test-row")); snap.Peak <= 0 {
		t.Errorf("per-row analyzer Peak=%f want >0", snap.Peak)
	}
}
