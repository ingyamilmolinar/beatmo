package ui

// synth_graph_glow.go — the trigger glow's clock (spec 2026-10-24 §4.7). A
// sequencer hit or Preview on the instrument the Synth tab shows starts it;
// envelopeGlow (synth_graph_pane.go) turns the elapsed time into an intensity
// from the note's own loudness. The trigger is a SYNC handoff
// (onInstrumentTriggered) from the highlight consumer and the Preview button —
// no audio taps, no goroutines.

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

// tick expires the glow after glowMaxSec (envelopeGlow ends it sooner when the
// note fades). Returns true while it lasts: the caller requests a redraw.
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

// onInstrumentTriggered is the SYNC trigger sink: called from the highlight
// consumer (a sequencer hit on an audible row) and from the Preview button. It
// starts the glow only when the Synth tab is showing this instrument; every
// other case is a no-op. Runs on the UI goroutine under seqMu — it must not
// call back into Game.
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
