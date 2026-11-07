//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The colour the enable toggle and FX highlights tint to.
func TestFXPanelAccentIsInstrumentColor(t *testing.T) {
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	want := color.RGBA{190, 120, 60, 255}
	dv.Rows[0].Color = want

	instID := dv.Rows[0].Instrument
	audio.AddInsertEffect(instID, audio.EffectDistortion, nil)
	dv.toggleFXPanel(0)
	if !dv.IsFXPanelOpen() {
		t.Fatal("FX panel did not open")
	}

	got := color.RGBAModel.Convert(dv.fxPanelAccent()).(color.RGBA)
	if got != want {
		t.Errorf("fxPanelAccent() = %v, want instrument color %v", got, want)
	}
}
