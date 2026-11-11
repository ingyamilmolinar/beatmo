//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestColorPickerSeedsCurrentColorFromRow verifies the row color picker is
// seeded with the row's live color so the matching palette swatch renders its
// "selected" ring (drumview_overlay_color_comp.go draws the ring on the swatch
// equal to props.CurrentColor). Without this wiring the picker opens with no
// indication of which color is currently applied to the node.
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
