package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Simulate holding the mouse down on a row delete button and ensure only one row
// is deleted even if the button row shifts under the cursor.
func TestDeleteRow_NoCascadeOnHeldPress(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(ebiten.NewImage(600, 200).Bounds(), nil, logger)
	// Add two more rows (total 3)
	dv.AddRow()
	dv.AddRow()
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowDeleteBtns) < 2 {
		t.Fatalf("need at least 2 delete buttons")
	}
	// Click coordinates over row 1 delete button
	r := dv.rowDeleteBtns[1].Rect()
	mx, my := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	// First click enters confirm mode (2-click delete confirmation)
	if !dv.rowDeleteBtns[1].Handle(mx, my, true) {
		t.Fatalf("expected handle on first press")
	}
	_ = dv.rowDeleteBtns[1].Handle(mx, my, false) // release
	if len(dv.Rows) != 3 {
		t.Fatalf("first click should not delete; rows=%d", len(dv.Rows))
	}
	if dv.deleteConfirmRow != 1 {
		t.Fatalf("expected deleteConfirmRow=1, got %d", dv.deleteConfirmRow)
	}
	// Second click within timeout should delete row 1
	if !dv.rowDeleteBtns[1].Handle(mx, my, true) {
		t.Fatalf("expected handle on second press")
	}
	if len(dv.Rows) != 2 {
		t.Fatalf("expected one row deleted; rows=%d", len(dv.Rows))
	}
	// While still pressed at same coordinates, calling Handle on the next button
	// (now occupying the same area) should be ignored by the global guard.
	// Guard lives in ui package; Button.Handle will return false.
	if dv.rowDeleteBtns[1].Handle(mx, my, true) {
		t.Fatalf("unexpected second delete on the same held press")
	}
	if len(dv.Rows) != 2 {
		t.Fatalf("second delete triggered; rows=%d", len(dv.Rows))
	}
	// Release resets guard; ensure release state clears
	_ = dv.rowDeleteBtns[1].Handle(mx, my, false)
}
