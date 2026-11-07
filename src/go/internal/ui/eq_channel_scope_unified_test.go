//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// One shared channel pill for every tab; no scope-owned selector or hit area.
func TestChainPanelZoneHasNoInstrumentButton(t *testing.T) {
	assertDefaultParityState(t)

	z := NewChainPanelZone(ChainCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))
	for _, a := range z.HitAreas() {
		if a.Tag == "scope-inst-btn" {
			t.Fatalf("hit area %q must be removed; the EQ panel's eqChannelBtn handles instrument selection", a.Tag)
		}
	}
}

// A channel picked on the shared pill reaches the scope's instrument (its fallback when there is no
// ScopeService, as under -tags test).
func TestEQChannelChangeUpdatesScopeInstrument(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.eqPanelZone == nil || dv.eqPanelZone.chainZone == nil {
		t.Fatal("DrumView must construct both eqPanelZone and chainZone")
	}
	if dv.eqPanelZone.callbacks.OnChannelChange == nil {
		t.Fatal("EQPanelZone OnChannelChange callback should be wired by NewDrumView")
	}

	// Seed a row so the dropdown would offer something other than Master.
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: roster.Snare,
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}

	// Drive the production callback as if the user picked the Snare entry.
	dv.eqPanelZone.callbacks.OnChannelChange(channel.Of(roster.Snare))

	if got := dv.eqPanelZone.chainZone.instrumentID; got != channel.Of(roster.Snare) {
		t.Fatalf("chainZone.instrumentID = %q after EQ channel change; want %q", got, "snare")
	}

	// Switching back to master must propagate too.
	dv.eqPanelZone.callbacks.OnChannelChange(channel.Master)
	if got := dv.eqPanelZone.chainZone.instrumentID; got != channel.Master {
		t.Fatalf("chainZone.instrumentID = %q after switch to master; want %q", got, "main")
	}
}
