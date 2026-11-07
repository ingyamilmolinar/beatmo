//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Seeded with the row's live colour, so its swatch shows the selected ring.
func TestColorPickerSeedsCurrentColorFromRow(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)
	dv.calcLayout()

	want := color.RGBA{190, 120, 60, 255}
	dv.Rows[0].Color = want

	dv.openColorPickerForRow(0)

	if dv.colorWheelComp == nil {
		t.Fatal("colorWheelComp should be constructed")
	}
	got := dv.colorWheelComp.Props().CurrentColor
	if got == nil {
		t.Fatalf("color picker CurrentColor should be seeded from the row color %v; got nil", want)
	}
	if !colorsEqual(got, want) {
		t.Errorf("color picker CurrentColor = %v, want row color %v", got, want)
	}
}
