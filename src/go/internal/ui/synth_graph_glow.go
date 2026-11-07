package ui

// A sync handoff from the highlight consumer and the Preview button: no audio taps, no goroutines.

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synthGlow is the per-DrumView trigger-glow state.
type synthGlow struct {
	glowStart time.Time
	glowing   bool

	now func() time.Time // injectable clock; nil = time.Now
}

func (g *synthGlow) clock() time.Time {
	if g.now != nil {
		return g.now()
	}
	return time.Now()
}

// graphGlow returns the DrumView's glow state (lazily created).
func (dv *DrumView) graphGlow() *synthGlow {
	if dv.glow == nil {
		dv.glow = &synthGlow{}
	}
	return dv.glow
}

// start begins a glow now.
func (g *synthGlow) start() {
	g.glowing = true
	g.glowStart = g.clock()
}

// Returns true while it lasts, so the caller redraws.
func (g *synthGlow) tick() bool {
	if !g.glowing {
		return false
	}
	if g.clock().Sub(g.glowStart).Seconds() >= glowMaxSec {
		g.glowing = false
		return false
	}
	return true
}

// Runs on the UI goroutine under seqMu: must not call back into Game.
func (dv *DrumView) onInstrumentTriggered(instID instrument.ID) {
	if dv == nil || instID == "" || dv.eqPanelZone == nil {
		return
	}
	if dv.eqPanelZone.ActiveTab() != TabSynth {
		return
	}
	if dv.resolveSynthInstrument(dv.synthTabActiveInstrument()) != dv.resolveSynthInstrument(instID) {
		return
	}
	dv.graphGlow().start()
	dv.eqPanelZone.Invalidate()
}
