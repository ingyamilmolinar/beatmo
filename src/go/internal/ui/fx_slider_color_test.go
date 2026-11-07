//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The owning row's instrument colour, not the fixed light blue.
func TestFXPanelSlidersFollowInstrumentColor(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}

	rowCol := color.RGBA{190, 120, 60, 255}
	dv.Rows[0].Color = rowCol

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)

	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}
	if len(dv.fxPanelSliders) == 0 {
		t.Fatal("no sliders in FX panel")
	}

	want := rowShadeToRGBA(rowCol)
	for i, sl := range dv.fxPanelSliders {
		if sl.FillCol != want {
			t.Errorf("fxPanelSliders[%d].FillCol = %v, want instrument color %v", i, sl.FillCol, want)
		}
	}
}
