package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// SetMasterVolume sets the master output volume (0..1) and syncs the
// transport-bar slider so the UI matches. Mirrors the slider callback in
// drumview_ctor.go.
func (g *Game) SetMasterVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	audio.SetMainVolume(v)
	if g.drum != nil && g.drum.mainVolSlider() != nil {
		g.drum.mainVolSlider().Value = v
	}
	emitMasterVolumeChange(v)
}

// SetEQHPF enables/disables the master high-pass filter and sets its cutoff.
func (g *Game) SetEQHPF(enabled bool, cutoffHz float64) {
	if g.drum == nil {
		return
	}
	if cutoffHz < 20 {
		cutoffHz = 20
	}
	if cutoffHz > 2000 {
		cutoffHz = 2000
	}
	g.drum.setActiveHPF(enabled, cutoffHz)
	g.drum.applyEQ()
	g.drum.eqCurveDirty = true
	g.drum.syncFilterButtonStyles()
}

// SetEQLPF enables/disables the master low-pass filter and sets its cutoff.
func (g *Game) SetEQLPF(enabled bool, cutoffHz float64) {
	if g.drum == nil {
		return
	}
	if cutoffHz < 1000 {
		cutoffHz = 1000
	}
	if cutoffHz > 20000 {
		cutoffHz = 20000
	}
	g.drum.setActiveLPF(enabled, cutoffHz)
	g.drum.applyEQ()
	g.drum.eqCurveDirty = true
	g.drum.syncFilterButtonStyles()
}

// SetEQBandGain sets the gain (in dB) for a band on the given channel.
// channel: "main" (or empty) for the master channel, or an instrument id.
func (g *Game) SetEQBandGain(channel string, band int, gainDB float64) {
	if g.drum == nil {
		return
	}
	gains, muted := g.drum.loadChannelBandState(channel)
	if band < 0 || band >= len(gains) {
		return
	}
	gains[band] = gainDB
	if g.drum.eqPanelZone != nil && len(g.drum.eqPanelZone.bandGainsDB) == len(gains) {
		copy(g.drum.eqPanelZone.bandGainsDB, gains)
		copy(g.drum.eqPanelZone.bandMuted, muted)
	}
	g.drum.applyEQ()
	g.drum.eqCurveDirty = true
}
