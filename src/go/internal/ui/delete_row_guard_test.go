package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Verify the two-click delete confirmation: first click sets confirm mode,
// second click within the timeout window actually deletes the row.
// On desktop the delete button has an empty rect (it lives in the overflow
// menu), so we invoke the callback directly instead of simulating Handle.
func TestDeleteRow_NoCascadeOnHeldPress(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(ebiten.NewImage(600, 200).Bounds(), nil, logger)
	// Add two more rows (total 3)
	dv.AddRow()
	dv.AddRow()
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowDeleteBtns()) < 2 {
		t.Fatalf("need at least 2 delete buttons")
	}

	// On desktop the delete button rect is hidden (empty).
	r := dv.rowDeleteBtns()[1].Rect()
	if !r.Empty() {
		t.Fatalf("expected delete button rect to be empty on desktop, got %v", r)
	}

	// First click enters confirm mode (2-click delete confirmation).
	dv.rowDeleteBtns()[1].OnClick()
	if len(dv.Rows) != 3 {
		t.Fatalf("first click should not delete; rows=%d", len(dv.Rows))
	}
	if dv.deleteConfirmRow != 1 {
		t.Fatalf("expected deleteConfirmRow=1, got %d", dv.deleteConfirmRow)
	}

	// Second click within timeout should delete row 1.
	dv.rowDeleteBtns()[1].OnClick()
	if len(dv.Rows) != 2 {
		t.Fatalf("expected one row deleted; rows=%d", len(dv.Rows))
	}
}
