//go:build test

package ui

import (
	"image"
	"io"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestScrollbarWidthMobile verifies the scrollbar is wider on mobile.
func TestScrollbarWidthMobile(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), g, logger)

	r := dv.scrollBarRect()
	w := r.Dx()
	if w != 16 {
		t.Fatalf("expected scrollbar width=16 on mobile, got %d", w)
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
			Instrument: "kick",
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
