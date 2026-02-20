//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// ─── ColorWheelOverlay input handling tests ─────────────────────────────────

// TestColorWheelOverlayLegacyHoldRelease verifies that the legacy hold state
// is cleared when the mouse is released, and InputCaptured is returned while
// held.
func TestColorWheelOverlayLegacyHoldRelease(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &ColorWheelOverlay{dv: dv}

	// Simulate legacy hold state (no component).
	dv.colorWheelComp = nil
	dv.colorHold = true
	dv.colorMenuOpen = true

	// While held, any input should return InputCaptured.
	result := overlay.HandleInput(50, 50, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured during hold, got %v", result)
	}

	// Release should clear hold.
	result = overlay.HandleInput(50, 50, false)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on release, got %v", result)
	}
	if dv.colorHold {
		t.Error("expected colorHold to be false after release")
	}
}

// TestColorWheelOverlayPressOnWheel verifies that pressing inside the color
// wheel rect returns InputConsumed (no hold state change in legacy mode).
func TestColorWheelOverlayPressOnWheel(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &ColorWheelOverlay{dv: dv}

	dv.colorWheelComp = nil
	dv.colorHold = false
	dv.colorMenuOpen = true
	dv.colorWheelRect = image.Rect(100, 100, 200, 200)

	// Press inside wheel rect.
	result := overlay.HandleInput(150, 150, true)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on press inside wheel, got %v", result)
	}
}

// TestColorWheelOverlayPressOutsideWheel verifies that pressing outside the
// wheel rect (and not in hold state) returns InputIgnored.
func TestColorWheelOverlayPressOutsideWheel(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &ColorWheelOverlay{dv: dv}

	dv.colorWheelComp = nil
	dv.colorHold = false
	dv.colorMenuOpen = true
	dv.colorWheelRect = image.Rect(100, 100, 200, 200)

	// Press outside wheel rect.
	result := overlay.HandleInput(50, 50, true)
	if result != InputIgnored {
		t.Errorf("expected InputIgnored on press outside wheel, got %v", result)
	}
}

// TestColorWheelOverlayCloseResetsState verifies that Close() clears both
// component and legacy state.
func TestColorWheelOverlayCloseResetsState(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &ColorWheelOverlay{dv: dv}

	dv.colorMenuOpen = true
	dv.colorHold = true
	overlay.Close()

	if dv.colorMenuOpen {
		t.Error("expected colorMenuOpen to be false after Close()")
	}
	if dv.colorHold {
		t.Error("expected colorHold to be false after Close()")
	}
}

// ─── InstrumentMenuOverlay input handling tests ─────────────────────────────

// TestInstMenuOverlayInputBoundsWhenClosed verifies that InputBounds returns
// an empty rect when the instrument menu is not open.
func TestInstMenuOverlayInputBoundsWhenClosed(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &InstrumentMenuOverlay{dv: dv}

	dv.instMenuOpen = false
	bounds := overlay.InputBounds()
	if !bounds.Empty() {
		t.Errorf("expected empty bounds when menu is closed, got %v", bounds)
	}
}

// TestInstMenuOverlayInputBoundsInvalidRow verifies that InputBounds returns
// empty when the menu row index is out of range (legacy path).
func TestInstMenuOverlayInputBoundsInvalidRow(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &InstrumentMenuOverlay{dv: dv}

	dv.instMenuOpen = true
	dv.instMenuComp = nil // no component, use legacy
	dv.instMenuRow = -1   // invalid row

	bounds := overlay.InputBounds()
	if !bounds.Empty() {
		t.Errorf("expected empty bounds for invalid row, got %v", bounds)
	}
}

// TestInstMenuOverlayHandleInputDragging verifies that HandleInput returns
// InputCaptured during an active scroll drag.
func TestInstMenuOverlayHandleInputDragging(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &InstrumentMenuOverlay{dv: dv}

	dv.instMenuOpen = true
	dv.instMenuScroll.dragging = true

	result := overlay.HandleInput(100, 100, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured during drag, got %v", result)
	}
}

// TestInstMenuOverlayHandleInputInstHoldRelease verifies that instHold is
// cleared on mouse release.
func TestInstMenuOverlayHandleInputInstHoldRelease(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &InstrumentMenuOverlay{dv: dv}

	dv.instMenuOpen = true
	dv.instHold = true

	// While held, should return InputCaptured.
	result := overlay.HandleInput(100, 100, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured during instHold, got %v", result)
	}

	// Release should clear instHold.
	result = overlay.HandleInput(100, 100, false)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on release, got %v", result)
	}
	if dv.instHold {
		t.Error("expected instHold to be false after release")
	}
}

// TestInstMenuOverlayClose verifies that Close() resets all menu state.
func TestInstMenuOverlayClose(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
	dv.Length = 8

	overlay := &InstrumentMenuOverlay{dv: dv}

	dv.instMenuOpen = true
	dv.instHold = true
	overlay.Close()

	if dv.instMenuOpen {
		t.Error("expected instMenuOpen to be false after Close()")
	}
	if dv.instHold {
		t.Error("expected instHold to be false after Close()")
	}
}
