package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The analyzer for a channel exists only on demand, so activating a tab must enable what it reads
// before the next frame, never via channel selection (that drew flat output).

// Chain needs the master only for its header metering; Synth reads its own voice cache.
func tabReadsMasterAnalyzer(tab PanelTab) bool {
	switch tab {
	case TabWave, TabSpectrum, TabMeters, TabEQ, TabScope:
		return true
	default:
		return false
	}
}

// Pure, so tests assert the rule without audio side effects.
func AnalyzerTapsForTab(tab PanelTab) []channel.ID {
	if !tabReadsMasterAnalyzer(tab) {
		return nil
	}
	return []channel.ID{channel.Master}
}

// On WASM each instrument's AnalyserNode is wired on demand, so without a tap per row the
// per-instrument views stayed blank while audio played. nil for tabs that read no analyzer.
func AnalyzerInstrumentTapsForTab(tab PanelTab, instrumentIDs []channel.ID) []channel.ID {
	if !tabReadsMasterAnalyzer(tab) {
		return nil
	}
	taps := make([]channel.ID, 0, 1+len(instrumentIDs))
	taps = append(taps, channel.Master)
	for _, id := range instrumentIDs {
		if id.IsZero() || id == channel.Master {
			continue
		}
		taps = append(taps, id)
	}
	return taps
}

// Idempotent and safe on stub builds. Callers: the onTab closure, the boot path, and import (rows
// change without a tab transition).
func EnsureAnalyzersForTab(tab PanelTab, activeChannelID channel.ID, instrumentIDs ...channel.ID) {
	enable := func(id channel.ID) {
		_ = audio.EnableChannelAnalyzer(id, 512)
		_ = audio.EnablePreEQAnalyzer(id, 512)
		_ = audio.EnableSynthAnalyzer(id, 512)
		audio.SetAnalyzerEnabled(id, true)
	}
	for _, id := range AnalyzerInstrumentTapsForTab(tab, instrumentIDs) {
		enable(id)
	}
	if !activeChannelID.IsZero() && activeChannelID != channel.Master {
		enable(activeChannelID)
	}
}
