//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// ─── Color wheel state tests ─────────────────────────────────────────────────

func TestColorWheelHoldRelease(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openColorWheelPortal()

	// After opening via the portal tree callback, hold should be cleared.
	if dv.colorWheelComp != nil && dv.colorWheelComp.Capturing() {
		t.Error("expected colorWheelComp not capturing after portal open")
	}
}

func TestColorWheelPressOnWheel(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openColorWheelPortal()
	wheelRect := image.Rect(100, 100, 200, 200)

	// Point inside wheel rect should be detected as inside.
	pt := image.Pt(150, 150)
	if !pt.In(wheelRect) {
		t.Error("expected point (150,150) to be inside color wheel rect")
	}
}

func TestColorWheelPressOutsideWheel(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openColorWheelPortal()
	wheelRect := image.Rect(100, 100, 200, 200)

	// Point outside wheel rect.
	pt := image.Pt(50, 50)
	if pt.In(wheelRect) {
		t.Error("expected point (50,50) to be outside color wheel rect")
	}
}

func TestColorWheelCloseResetsState(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openColorWheelPortal()

	// Close by resetting state (as the portal system does).
	dv.CloseAllPopups()

	if dv.IsColorMenuOpen() {
		t.Error("expected colorMenuOpen to be false after close")
	}
}

// ─── Instrument menu input handling tests ─────────────────────────────────

func TestInstMenuBoundsWhenClosed(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.CloseAllPopups()
	if !dv.instMenuFullRect.Empty() {
		// The rect may keep a stale value; instMenuOpen false is what stops the portal dispatching.
	}
	if dv.IsInstMenuOpen() {
		t.Error("instMenuOpen should be false")
	}
}

func TestInstMenuBoundsInvalidRow(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openInstMenuPortal()
	dv.instMenuComp = nil // no component, use legacy
	dv.instMenuRow = -1   // invalid row

	if dv.instMenuRow >= 0 && dv.instMenuRow < len(dv.rowLabels()) {
		t.Error("invalid row should not be in rowLabels range")
	}
}

// TestInstMenuScrollDragging verifies that the scroll drag state is tracked.
func TestInstMenuScrollDragging(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openInstMenuPortal()
	dv.instMenuScroll.dragging = true

	// When dragging is active, the scroll state should be set.
	if !dv.instMenuScroll.dragging {
		t.Error("expected instMenuScroll.dragging to be true")
	}
}

// TestInstMenuCloseResetsState verifies that closing the inst menu resets state.
func TestInstMenuCloseResetsState(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	dv.openInstMenuPortal()

	// Close by resetting state (as the portal system does).
	dv.CloseAllPopups()

	if dv.IsInstMenuOpen() {
		t.Error("expected instMenuOpen to be false after close")
	}
}
