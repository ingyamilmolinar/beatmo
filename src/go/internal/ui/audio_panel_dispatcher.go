package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// audio_panel_dispatcher.go — cross-tab analyzer-enable rules for the
// audio panel (Wave / Spectrum / Levels / EQ / Chain / Synth).
//
// Background: the analyzer for a given channel ID is only created on
// demand via audio.EnableChannelAnalyzer / EnablePreEQAnalyzer /
// EnableSynthAnalyzer. Before this file existed, the enable was tied
// to which EQ *channel* the user had selected — so opening the
// Spectrum, Levels, or Chain tab without first interacting with the
// channel picker left the master analyzer disabled and the panel
// rendered flat output during playback (the original blackout bug).
//
// This file owns the rule "when the user activates an analysis tab
// that reads the Master analyzer, ensure the Master analyzer is
// enabled". The eq_panel_zone.go onTab closure calls
// EnsureAnalyzersForTab on every tab transition so the next frame
// has data to draw.
//
// The dispatcher is intentionally a small, pure-ish function set so
// it's trivial to unit test (audio_panel_dispatcher_test.go) without
// instantiating the whole Game.

// audioPanelMasterChannelID is the ID used by the master / "main"
// channel across the audio package and the analyzer registry.
const audioPanelMasterChannelID = "main"

// tabReadsMasterAnalyzer reports whether the given tab renders data
// sourced from the master analyzer (and therefore needs the master
// analyzer enabled before its draw runs).
//
// Wave / Spectrum / Levels / EQ all read the analyzer state for the
// active channel (which defaults to master when no per-row channel
// has been selected). Chain reads scope tap data and does not need
// the master analyzer per se — but the master metering shown on its
// header row does, so we enable it for symmetry. Synth reads its
// own per-voice cache and does not require the analyzer at all.
func tabReadsMasterAnalyzer(tab PanelTab) bool {
	switch tab {
	case TabWave, TabSpectrum, TabMeters, TabEQ, TabScope:
		return true
	default:
		return false
	}
}

// AnalyzerTapsForTab returns the list of analyzer IDs that need to be
// enabled when the given tab becomes active. Pure function; deterministic;
// exported so tests can assert the rule without invoking audio side
// effects.
//
// Today every tab in the readsMaster set uses the master analyzer at
// "main"; future split-tab views may add per-row IDs and this is the
// hook to extend.
func AnalyzerTapsForTab(tab PanelTab) []string {
	if !tabReadsMasterAnalyzer(tab) {
		return nil
	}
	return []string{audioPanelMasterChannelID}
}

// EnsureAnalyzersForTab enables the analyzers required by the given
// tab. Idempotent — repeated calls are a no-op once the analyzer is
// created. activeChannelID is the channel currently selected in the
// EQ channel picker; when it's non-empty and not "main", we also
// enable that channel's analyzer so per-row EQ stays responsive.
//
// Callers: the onTab closure inside (*EQPanelZone)/NewEQPanelZone and
// the boot path that instantiates DrumView with TabEQ pre-selected.
// The function never panics on stub audio builds — the audio.Enable*
// helpers are no-ops when no oto context is alive.
func EnsureAnalyzersForTab(tab PanelTab, activeChannelID string) {
	for _, id := range AnalyzerTapsForTab(tab) {
		_ = audio.EnableChannelAnalyzer(id, 512)
		_ = audio.EnablePreEQAnalyzer(id, 512)
		_ = audio.EnableSynthAnalyzer(id, 512)
		audio.SetAnalyzerEnabled(id, true)
	}
	if activeChannelID != "" && activeChannelID != audioPanelMasterChannelID {
		_ = audio.EnableChannelAnalyzer(activeChannelID, 512)
		_ = audio.EnablePreEQAnalyzer(activeChannelID, 512)
		_ = audio.EnableSynthAnalyzer(activeChannelID, 512)
		audio.SetAnalyzerEnabled(activeChannelID, true)
	}
}
