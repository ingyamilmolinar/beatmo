//go:build test

package ui

import (
	"image"
	"testing"
)

// TestScopePanelZoneHasNoInstrumentButton enforces that the Scope tab does
// not own its own instrument selector — the Wave Analyzer panel must expose a
// single shared instrument pill (eqChannelBtn) for every tab. The duplicate
// "Kick" button and "scope-inst-btn" hit area must not exist.
func TestScopePanelZoneHasNoInstrumentButton(t *testing.T) {
	assertDefaultParityState(t)

	z := NewScopePanelZone(ScopeCallbacks{})
	z.Layout(image.Rect(0, 0, 1000, 300))
	for _, a := range z.HitAreas() {
		if a.Tag == "scope-inst-btn" {
			t.Fatalf("hit area %q must be removed; the EQ panel's eqChannelBtn handles instrument selection", a.Tag)
		}
	}
}

// TestEQChannelChangeUpdatesScopeInstrument verifies that selecting an
// instrument from the shared eqChannelBtn dropdown propagates to the Scope
// zone's instrumentID (the WASM fallback path the Scope zone reads when no
// audio.ScopeService is available, which is the case under -tags test).
//
// The production wiring lives in drumview_ctor.go's OnChannelChange callback;
// today that callback only updates setEQActiveChannel + AnalyzerService, so
// switching channel from the EQ pill leaves the scope viewing the wrong
// instrument. After the fix the same callback also forwards to scope.
func TestEQChannelChangeUpdatesScopeInstrument(t *testing.T) {
	assertDefaultParityState(t)

	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if dv.eqPanelZone == nil || dv.eqPanelZone.scopeZone == nil {
		t.Fatal("DrumView must construct both eqPanelZone and scopeZone")
	}
	if dv.eqPanelZone.callbacks.OnChannelChange == nil {
		t.Fatal("EQPanelZone OnChannelChange callback should be wired by NewDrumView")
	}

	// Seed a row so the dropdown would offer something other than Master.
	dv.Rows = []*DrumRow{{
		Name:       "Snare",
		Instrument: "snare",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}

	// Drive the production callback as if the user picked the Snare entry.
	dv.eqPanelZone.callbacks.OnChannelChange("snare")

	if got := dv.eqPanelZone.scopeZone.instrumentID; got != "snare" {
		t.Fatalf("scopeZone.instrumentID = %q after EQ channel change; want %q", got, "snare")
	}

	// Switching back to master must propagate too.
	dv.eqPanelZone.callbacks.OnChannelChange("main")
	if got := dv.eqPanelZone.scopeZone.instrumentID; got != "main" {
		t.Fatalf("scopeZone.instrumentID = %q after switch to master; want %q", got, "main")
	}
}
