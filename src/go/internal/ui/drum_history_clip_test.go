package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestDrumHistoryMaskMatchesInstrumentPanel(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 360, 800, 720), graph, logger)
	dv.SetBounds(image.Rect(0, 360, 800, 720))
	dv.calcLayout()

	dst := ebiten.NewImage(800, 720)
	dv.Draw(dst, nil, 0, nil, 0)

	rect := dv.panelMaskRect
	if rect.Empty() {
		t.Fatalf("panel mask rect empty")
	}
	if rect.Min.X != dv.Bounds.Min.X {
		t.Fatalf("panel mask min x mismatch: got %d want %d", rect.Min.X, dv.Bounds.Min.X)
	}
	// The panel mask extends to the rack widget's right edge OR is clamped to
	// `Bounds.Min.X + labelW + controlsW` (whichever is smaller — see the
	// clamp at drumview_draw.go:204). The rack widget rect comes from the
	// widget grid, while labelW/controlsW are derived from RowControlBtnSize;
	// their sum can be smaller than the grid cell when the design profile
	// shrinks RowControlBtnSize. Both behaviors are valid clamps.
	maxRackX := dv.widgetRects[WidgetRack].Max.X
	if maxRackX == 0 {
		maxRackX = dv.timelineRect.Min.X
	}
	maskRight := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	wantMaxX := maxRackX
	if maskRight < wantMaxX {
		wantMaxX = maskRight
	}
	if rect.Max.X != wantMaxX {
		t.Fatalf("panel mask max x mismatch: got %d want %d (rack=%d maskRight=%d)", rect.Max.X, wantMaxX, maxRackX, maskRight)
	}
	if rect.Min.Y != dv.Bounds.Min.Y+dv.headerH {
		t.Fatalf("panel mask min y mismatch: got %d want %d", rect.Min.Y, dv.Bounds.Min.Y+dv.headerH)
	}
	if rect.Max.Y != dv.Bounds.Max.Y {
		t.Fatalf("panel mask max y mismatch: got %d want %d", rect.Max.Y, dv.Bounds.Max.Y)
	}
}
