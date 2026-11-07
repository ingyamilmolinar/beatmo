//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Full strip chrome before the analyser has ticked, synthesised as silence from the live rows, as the browser shows
// from frame one.
func TestLevelsIdleStateSynthesizedFromRows(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum
	dv.Rows[0].Name = "Kick"
	dv.Rows[0].Instrument = roster.Kick

	st := dv.eqPanelZone.idleLevelsState()
	if st == nil {
		t.Fatal("idleLevelsState returned nil")
	}
	if len(st.Instruments) == 0 {
		t.Fatal("idle state has no instrument channels — strips would not render")
	}
	if st.Instruments[0].ID != channel.Of(roster.Kick) || st.Instruments[0].Name != "Kick" {
		t.Fatalf("idle channel = %+v, want the live row's id/name", st.Instruments[0])
	}
	// -inf like the analyser's 20·log10(0), never 0 dB, which draws full-scale meters.
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
