//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The Levels tab must render its full strip chrome even before the analyzer
// service has ticked (native desktop at boot) — the browser build shows
// per-channel strips at -inf from frame one, while native drew a dead
// border-only rect (A5 leftover, 2026-07-04 critique). The zone synthesizes
// a silent state from the live row set.
func TestLevelsIdleStateSynthesizedFromRows(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = "kick"

	st := dv.eqPanelZone.idleLevelsState()
	if st == nil {
		t.Fatal("idleLevelsState returned nil")
	}
	if len(st.Instruments) == 0 {
		t.Fatal("idle state has no instrument channels — strips would not render")
	}
	if st.Instruments[0].ID != "kick" || st.Instruments[0].Name != "Kick" {
		t.Fatalf("idle channel = %+v, want the live row's id/name", st.Instruments[0])
	}
	// Silence must read as -inf (matching the real analyzer's 20*log10(0)),
	// never as a zero-value 0 dB — which would render full-scale meters.
	if !math.IsInf(st.Instruments[0].PeakDB, -1) || !math.IsInf(st.Master.PeakDB, -1) {
		t.Fatalf("idle peaks must be -Inf, got inst=%v master=%v", st.Instruments[0].PeakDB, st.Master.PeakDB)
	}

	// And the renderer must actually paint strip chrome from it.
	dst := ebiten.NewImage(800, 300)
	rect := image.Rect(0, 0, 800, 300)
	rects := collectFilledRects(t, func() {
		drawLevelsMultiChannel(dst, rect, st, nil, nil)
	})
	ink := 0
	for _, r := range rects {
		if r.Rect.In(rect) {
			ink++
		}
	}
	if ink == 0 {
		t.Fatal("idle Levels state painted no strip chrome — dead-panel regression")
	}
}
