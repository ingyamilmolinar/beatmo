//go:build test

package ui

import (
	"image"
	"io"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// A slim hairline; the touch grab comes from the tall thumb, not the width.
func TestScrollbarWidthMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), g, logger)

	r := dv.scrollBarRect()
	w := r.Dx()
	if w != mobileScrollbarWidth {
		t.Fatalf("expected scrollbar width=%d on mobile, got %d", mobileScrollbarWidth, w)
	}
}

// Content and menu scrollers share one width, so none reads thicker.
func TestMobileScrollbarsUniformWidth(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	if got := ScrollbarStyleForPlatform().Width; got != mobileScrollbarWidth {
		t.Fatalf("content scrollbar width = %d, want %d", got, mobileScrollbarWidth)
	}
	if got := dropdownScrollbarStyle().Width; got != mobileScrollbarWidth {
		t.Fatalf("dropdown/menu scrollbar style width = %d, want %d", got, mobileScrollbarWidth)
	}
	if got := dropdownScrollbarWidth(); got != mobileScrollbarWidth {
		t.Fatalf("dropdown/menu scrollbar inset width = %d, want %d", got, mobileScrollbarWidth)
	}
}

// TestScrollbarThumbMinHeightMobile verifies the scroll thumb has a minimum height of 44px on mobile.
func TestScrollbarThumbMinHeightMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), g, logger)

	// Add many rows so the thumb would normally be very small
	for len(dv.Rows) < 50 {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: roster.Kick,
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Volume:     1,
		})
	}

	thumb := dv.scrollThumbRect()
	if thumb.Empty() {
		t.Fatal("expected non-empty thumb rect with many rows")
	}
	h := thumb.Dy()
	if h < 44 {
		t.Fatalf("expected thumb height >= 44 on mobile, got %d", h)
	}
}

// TestScrollbarWidthDesktop verifies the scrollbar stays narrow on desktop (regression).
func TestScrollbarWidthDesktop(t *testing.T) {
	assertDefaultParityState(t)
	// Ensure desktop mode (not small screen)
	old := forceSmallScreenForTest
	forceSmallScreenForTest = false
	defer func() { forceSmallScreenForTest = old }()

	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), g, logger)

	r := dv.scrollBarRect()
	w := r.Dx()
	if w != 6 {
		t.Fatalf("expected scrollbar width=6 on desktop, got %d", w)
	}
}
