//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRowHeight_DoesNotScale verifies the per-row pixel height is the
// fixed profile baseline on both desktop and mobile. The historical
// rowZoom multiplier (driven by the +/− chip and the rows-area pinch)
// has been retired — the chips now resize the *visible beat length*
// of the active drum view via `dv.changeLength`. Per-row dimensions
// must NEVER change in response to a chip tap.
func TestRowHeight_DoesNotScale(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum

	if got := dv.rowHeight(); got != TouchRowHeight() {
		t.Errorf("rowHeight()=%d must equal TouchRowHeight()=%d (no per-instance scaling)",
			got, TouchRowHeight())
	}

	if dv.rowZoomIncBtn == nil || dv.rowZoomDecBtn == nil {
		t.Fatal("zoom chip buttons must be constructed")
	}
	beforeRH := dv.rowHeight()
	if cb := dv.rowZoomIncBtn.OnClick; cb != nil {
		cb()
	}
	advanceFrames(g, 2)
	if cb := dv.rowZoomDecBtn.OnClick; cb != nil {
		cb()
	}
	advanceFrames(g, 2)
	if got := dv.rowHeight(); got != beforeRH {
		t.Errorf("rowHeight changed after chip clicks: before=%d after=%d (chips must NOT touch row dimensions)",
			beforeRH, got)
	}
}

// TestLengthChip_IncreasesVisibleBeats verifies the + chip grows the
// timeline by one beat (`dv.timelineUnitsPerBeat` subdivisions). The
// user contract: chips control the horizontal beat span of the
// active drum view, never per-row dimensions or the pane's vertical
// size.
func TestLengthChip_IncreasesVisibleBeats(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)
	dv := g.drum

	if dv.rowZoomIncBtn == nil || dv.rowZoomIncBtn.OnClick == nil {
		t.Fatal("inc chip / OnClick must be constructed")
	}
	// Pre-shrink Length to give room to grow — default boot already
	// sits at the cell-width-derived max for narrow mobile viewports,
	// where + chips would no-op against the clamp. SetLength bypasses
	// clampLength so we can drop well below maxLen.
	beforePerBeat := dv.timelineUnitsPerBeat
	dv.SetLength(max1(beforePerBeat) * 2)
	advanceFrames(g, 2)
	beforeLen := dv.Length

	dv.rowZoomIncBtn.OnClick()
	advanceFrames(g, 2)

	if !dv.userAdjustedLength {
		t.Errorf("userAdjustedLength flag must flip true after a chip tap")
	}
	wantDelta := max1(beforePerBeat)
	gotDelta := dv.Length - beforeLen
	if gotDelta <= 0 || gotDelta > wantDelta {
		t.Errorf("+ chip: Length delta=%d, expected (0, %d] beat-units (beforeLen=%d, perBeat=%d)",
			gotDelta, wantDelta, beforeLen, beforePerBeat)
	}
}

// TestLengthChip_DecreasesVisibleBeats is the symmetric assertion for
// the − chip.
func TestLengthChip_DecreasesVisibleBeats(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)
	dv := g.drum

	// Set Length to a known value above the floor (one beat) but well
	// within the cell-width-derived max so neither clamp engages.
	beforePerBeat := dv.timelineUnitsPerBeat
	dv.SetLength(max1(beforePerBeat) * 2)
	advanceFrames(g, 2)
	beforeLen := dv.Length

	dv.rowZoomDecBtn.OnClick()
	advanceFrames(g, 2)

	wantDelta := max1(beforePerBeat)
	gotDelta := beforeLen - dv.Length
	if gotDelta <= 0 || gotDelta > wantDelta {
		t.Errorf("− chip: Length delta=%d, expected (0, %d] beat-units", gotDelta, wantDelta)
	}
}

// TestLengthChip_RowDimensionsAndPaneUnchanged verifies the chip
// callbacks never alter row dimensions OR the drum-view pane height.
// The chip's only effect is on `dv.Length`.
func TestLengthChip_RowDimensionsAndPaneUnchanged(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	advanceFrames(g, 2)
	dv := g.drum

	beforeRH := dv.rowHeight()
	beforePaneH := dv.Bounds.Dy()
	beforeSplitY := g.split.Y

	dv.rowZoomIncBtn.OnClick()
	advanceFrames(g, 2)
	if got := dv.rowHeight(); got != beforeRH {
		t.Errorf("rowHeight changed after + chip: before=%d after=%d", beforeRH, got)
	}
	if got := dv.Bounds.Dy(); got != beforePaneH {
		t.Errorf("drum pane height changed after + chip: before=%d after=%d", beforePaneH, got)
	}
	if g.split.Y != beforeSplitY {
		t.Errorf("split.Y changed after + chip: before=%d after=%d", beforeSplitY, g.split.Y)
	}

	dv.rowZoomDecBtn.OnClick()
	advanceFrames(g, 2)
	if got := dv.rowHeight(); got != beforeRH {
		t.Errorf("rowHeight changed after − chip: before=%d after=%d", beforeRH, got)
	}
	if got := dv.Bounds.Dy(); got != beforePaneH {
		t.Errorf("drum pane height changed after − chip: before=%d after=%d", beforePaneH, got)
	}
	if g.split.Y != beforeSplitY {
		t.Errorf("split.Y changed after − chip: before=%d after=%d", beforeSplitY, g.split.Y)
	}
}

// TestRowZoom_ChipPlacedOnMobile verifies the chip rects are non-empty
// on mobile (so users can tap them).
func TestRowZoom_ChipPlacedOnMobile(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum
	if dv.rowZoomIncBtn == nil || dv.rowZoomIncBtn.Rect().Empty() {
		t.Errorf("mobile: timeline-length inc chip rect should be non-empty")
	}
	if dv.rowZoomDecBtn == nil || dv.rowZoomDecBtn.Rect().Empty() {
		t.Errorf("mobile: timeline-length dec chip rect should be non-empty")
	}
}

// TestRowZoom_ChipAbsentOnDesktop verifies the chips are mobile-only.
func TestRowZoom_ChipAbsentOnDesktop(t *testing.T) {
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	advanceFrames(g, 2)
	dv := g.drum
	if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
		t.Errorf("desktop: timeline-length inc chip should be empty, got %v", dv.rowZoomIncBtn.Rect())
	}
	if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
		t.Errorf("desktop: timeline-length dec chip should be empty, got %v", dv.rowZoomDecBtn.Rect())
	}
}

// TestPinchInRowsZone_DoesNotChangeRowDimensions verifies that the
// rows-area pinch gesture is absorbed (does NOT scale row height) —
// the legacy "pinch to zoom rows" behavior was retired with the chip
// refactor.
func TestPinchInRowsZone_DoesNotChangeRowDimensions(t *testing.T) {
	assertDefaultParityState(t)
	setupMobileTest(t, true)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)
	advanceFrames(g, 2)
	dv := g.drum
	beforeRH := dv.rowHeight()

	rr := dv.rowsRect()
	if rr.Empty() {
		t.Fatal("rowsRect empty — cannot test pinch")
	}
	cx := (rr.Min.X + rr.Max.X) / 2
	cy := (rr.Min.Y + rr.Max.Y) / 2

	g.handleTouchPinch(cx, cy, 100)
	g.handleTouchPinch(cx, cy, 150)

	if got := dv.rowHeight(); got != beforeRH {
		t.Errorf("rows-area pinch changed rowHeight: before=%d after=%d (must be inert)",
			beforeRH, got)
	}
}
