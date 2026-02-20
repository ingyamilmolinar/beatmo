//go:build test

package ui

import (
	"image"
	"io"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTouchScrollBasicDrag verifies that a vertical drag returns 0 within the
// dead zone, then returns proportional deltas once past it.
func TestTouchScrollBasicDrag(t *testing.T) {
	var ts TouchScroller
	ts.Begin(100, 100)

	if !ts.Active() {
		t.Fatal("expected Active() after Begin")
	}

	// Move within dead zone (distance < 8px) — should return 0.
	d := ts.Move(100, 105)
	if d != 0 {
		t.Fatalf("expected 0 within dead zone, got %f", d)
	}

	// Move past dead zone vertically (distance = 50px from start).
	// This locks direction to vertical but still returns 0 for the lock frame.
	d = ts.Move(100, 150)
	if d != 0 {
		t.Fatalf("expected 0 on direction-lock frame, got %f", d)
	}

	// Subsequent moves should return proportional deltas.
	d = ts.Move(100, 160)
	if d != 10 {
		t.Fatalf("expected delta=10, got %f", d)
	}

	d = ts.Move(100, 155)
	if d != -5 {
		t.Fatalf("expected delta=-5, got %f", d)
	}
}

// TestTouchScrollMomentum verifies that End() returns velocity and
// UpdateMomentum() applies friction until the velocity drops below threshold.
func TestTouchScrollMomentum(t *testing.T) {
	var ts TouchScroller
	ts.Begin(100, 100)

	// Move past dead zone to lock vertical direction.
	ts.Move(100, 115)

	// Establish velocity with a series of moves.
	ts.Move(100, 130)
	ts.Move(100, 150) // velocity = 20 (last delta)

	vel := ts.End()
	if vel != 20 {
		t.Fatalf("expected velocity=20 from End(), got %f", vel)
	}

	if ts.Active() {
		t.Fatal("expected Active()=false after End()")
	}
	if !ts.HasMomentum() {
		t.Fatal("expected HasMomentum()=true immediately after End()")
	}

	// UpdateMomentum should return decaying deltas.
	prev := math.Abs(vel)
	frames := 0
	for ts.HasMomentum() {
		d := ts.UpdateMomentum()
		curr := math.Abs(d)
		if frames > 0 && curr >= prev {
			t.Fatalf("frame %d: momentum should decay, prev=%f curr=%f", frames, prev, curr)
		}
		prev = curr
		frames++
		if frames > 500 {
			t.Fatal("momentum did not stop within 500 frames")
		}
	}

	// After momentum stops, UpdateMomentum should return 0.
	d := ts.UpdateMomentum()
	if d != 0 {
		t.Fatalf("expected 0 after momentum stopped, got %f", d)
	}
	if ts.HasMomentum() {
		t.Fatal("expected HasMomentum()=false after momentum stopped")
	}
}

// TestTouchScrollClamp verifies that Reset() clears all state.
func TestTouchScrollClamp(t *testing.T) {
	var ts TouchScroller

	// Put the scroller into an active state with velocity.
	ts.Begin(50, 50)
	ts.Move(50, 70)
	ts.Move(50, 100)
	ts.End()

	if !ts.HasMomentum() {
		t.Fatal("expected momentum before Reset()")
	}

	ts.Reset()

	if ts.Active() {
		t.Fatal("expected Active()=false after Reset()")
	}
	if ts.HasMomentum() {
		t.Fatal("expected HasMomentum()=false after Reset()")
	}

	// UpdateMomentum should return 0 after Reset.
	d := ts.UpdateMomentum()
	if d != 0 {
		t.Fatalf("expected UpdateMomentum()=0 after Reset(), got %f", d)
	}
}

// TestTouchScrollHorizontalIgnored verifies that a primarily horizontal drag
// produces no vertical scroll and End() returns 0.
func TestTouchScrollHorizontalIgnored(t *testing.T) {
	var ts TouchScroller
	ts.Begin(100, 100)

	// Move horizontally past dead zone: dx=30, dy=2 → horizontal lock.
	d := ts.Move(130, 102)
	if d != 0 {
		t.Fatalf("expected 0 for horizontal move, got %f", d)
	}

	// Further moves should still return 0 (direction locked to horizontal).
	d = ts.Move(160, 105)
	if d != 0 {
		t.Fatalf("expected 0 for continued horizontal move, got %f", d)
	}

	vel := ts.End()
	if vel != 0 {
		t.Fatalf("expected End()=0 for horizontal drag, got %f", vel)
	}

	if ts.HasMomentum() {
		t.Fatal("expected no momentum after horizontal drag")
	}
}

// TestTouchScrollDirectionLock verifies that once direction is locked to
// vertical, subsequent horizontal movement still returns vertical-only deltas.
func TestTouchScrollDirectionLock(t *testing.T) {
	var ts TouchScroller
	ts.Begin(100, 100)

	// Move vertically past dead zone to lock vertical direction.
	d := ts.Move(100, 115)
	if d != 0 {
		t.Fatalf("expected 0 on lock frame, got %f", d)
	}

	// Now move horizontally (but keep Y the same) — delta should be 0
	// because Y didn't change, not because direction changed.
	d = ts.Move(130, 115)
	if d != 0 {
		t.Fatalf("expected delta=0 when Y unchanged, got %f", d)
	}

	// Move with both X and Y change — only vertical delta should be reported.
	d = ts.Move(160, 125)
	if d != 10 {
		t.Fatalf("expected delta=10 (vertical only), got %f", d)
	}
}

// TestTouchScrollInDrumView is an integration test that verifies the DrumView's
// applyTouchScrollDelta method correctly adjusts rowOffset and clamps it.
func TestTouchScrollInDrumView(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), g, logger)

	// Add enough rows to require scrolling. With forceSmallScreenForTest=true,
	// row height is 48px. The rows area is about 600-64-0=536px, so ~11 visible
	// rows. We add 20 rows total to ensure scrolling is needed.
	for len(dv.Rows) < 20 {
		dv.Rows = append(dv.Rows, &DrumRow{
			Name:       "Row",
			Instrument: "kick",
			Steps:      make([]bool, 8),
			CellTypes:  make([]model.NodeType, 8),
			Volume:     1,
		})
	}

	totalRows := len(dv.Rows)
	rh := dv.rowHeight()
	visRows := dv.visibleRows()

	if totalRows <= visRows {
		t.Fatalf("test setup: need more rows than visible: total=%d vis=%d", totalRows, visRows)
	}

	// Initial rowOffset should be 0.
	if dv.rowOffset != 0 {
		t.Fatalf("expected initial rowOffset=0, got %d", dv.rowOffset)
	}

	// Apply a positive delta large enough to scroll by multiple rows.
	dv.syncRowScroll()
	dv.rowScroll.applyPixelDelta(float64(rh * 3))
	dv.flushRowScroll()
	if dv.rowOffset <= 0 {
		t.Fatalf("expected rowOffset>0 after positive delta, got %d", dv.rowOffset)
	}
	if dv.rowOffset != 3 {
		t.Fatalf("expected rowOffset=3 after 3-row delta, got %d", dv.rowOffset)
	}

	// Apply a large negative delta — should clamp to 0.
	dv.syncRowScroll()
	dv.rowScroll.applyPixelDelta(float64(-rh * 100))
	dv.flushRowScroll()
	if dv.rowOffset != 0 {
		t.Fatalf("expected rowOffset clamped to 0, got %d", dv.rowOffset)
	}

	// Apply a huge positive delta — should clamp to maxOffset.
	dv.syncRowScroll()
	dv.rowScroll.applyPixelDelta(float64(rh * 100))
	dv.flushRowScroll()
	maxOff := totalRows - visRows
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset != maxOff {
		t.Fatalf("expected rowOffset clamped to maxOff=%d, got %d", maxOff, dv.rowOffset)
	}
}
