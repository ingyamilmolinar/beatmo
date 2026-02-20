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
	// The panel mask extends to the rack widget's right edge. On desktop, the
	// Track button sits between the rack boundary and the timeline start, so
	// panelMaskRect.Max.X may differ from timelineRect.Min.X.
	wantMaxX := dv.widgetRects[WidgetRack].Max.X
	if wantMaxX == 0 {
		wantMaxX = dv.timelineRect.Min.X
	}
	if rect.Max.X != wantMaxX {
		t.Fatalf("panel mask max x mismatch: got %d want %d", rect.Max.X, wantMaxX)
	}
	if rect.Min.Y != dv.Bounds.Min.Y+dv.headerH {
		t.Fatalf("panel mask min y mismatch: got %d want %d", rect.Min.Y, dv.Bounds.Min.Y+dv.headerH)
	}
	if rect.Max.Y != dv.Bounds.Max.Y {
		t.Fatalf("panel mask max y mismatch: got %d want %d", rect.Max.Y, dv.Bounds.Max.Y)
	}
}
