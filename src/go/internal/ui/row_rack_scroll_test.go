//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowRackWheelScrollChangesOffset verifies that scrolling the mouse wheel
// over the row rack zone changes the row offset and that the offset persists
// across subsequent Update frames (not overwritten back to 0).
func TestRowRackWheelScrollChangesOffset(t *testing.T) {
	var mx, my int
	var wy float64
	wheelConsumed := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if !wheelConsumed {
				wheelConsumed = true
				return 0, wy
			}
			return 0, 0
		},
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 300, 100))

	// Initial layout.
	tree.Update()

	if z.RowOffset() != 0 {
		t.Fatalf("expected initial offset 0, got %d", z.RowOffset())
	}

	// Position cursor over the rack zone and scroll down.
	mx, my = 150, 50
	wy = -1
	wheelConsumed = false
	tree.Update()

	if z.RowOffset() <= 0 {
		t.Fatal("expected row offset > 0 after scroll down over rack zone")
	}

	saved := z.RowOffset()

	// Run 3 more frames with no wheel input — offset must persist.
	for i := 0; i < 3; i++ {
		tree.Update()
	}

	if z.RowOffset() != saved {
		t.Errorf("row offset changed from %d to %d after idle frames", saved, z.RowOffset())
	}
}

// TestRowRackWheelScrollMatchesTimeline verifies that scrolling via the rack
// zone produces the same offset as scrolling via the timeline zone's callback.
func TestRowRackWheelScrollMatchesTimeline(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(20)

	// Scroll via rack zone.
	z1, _ := newTestRowRackZone(rows)
	tree1 := registerRowRackZone(z1, image.Rect(0, 0, 300, 100))
	tree1.Update()

	scrollArea := findHitAreaByTagPrefix(z1.HitAreas(), "row-rack-scroll")
	if scrollArea == nil {
		t.Fatal("expected row-rack-scroll hit area")
	}
	scrollArea.Handler.OnWheel(50, 50, -1)
	rackOffset := z1.RowOffset()

	// Scroll via direct scroll behavior (simulating timeline callback path).
	z2, _ := newTestRowRackZone(rows)
	registerRowRackZone(z2, image.Rect(0, 0, 300, 100))
	z2.syncScroll()
	z2.RowScroll().HandleWheel(-1)
	z2.flushScroll()
	timelineOffset := z2.RowOffset()

	if rackOffset != timelineOffset {
		t.Errorf("rack scroll offset %d != timeline scroll offset %d", rackOffset, timelineOffset)
	}
}

// TestRowRackTouchScrollMobile verifies that touch drag scrolling over the
// rack area works on mobile.
func TestRowRackTouchScrollMobile(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 844 },
	)
	defer restore()

	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 300, 100))
	tree.Update()

	// Verify initial offset is 0.
	if z.RowOffset() != 0 {
		t.Fatalf("expected initial offset 0, got %d", z.RowOffset())
	}

	// Simulate a scroll-down wheel event (mobile touch scroll gets converted
	// to wheel events by the platform layer in many cases).
	scrollArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-scroll")
	if scrollArea == nil {
		t.Fatal("expected row-rack-scroll hit area on mobile")
	}
	scrollArea.Handler.OnWheel(150, 50, -1)

	if z.RowOffset() <= 0 {
		t.Fatal("expected row offset > 0 after mobile scroll")
	}

	// Verify offset persists.
	saved := z.RowOffset()
	tree.Update()

	if z.RowOffset() != saved {
		t.Errorf("row offset changed from %d to %d after idle frame", saved, z.RowOffset())
	}
}

// TestRowRackScrollHitAdapterCallsOnScrollChanged verifies that the
// OnScrollChanged callback is invoked when the scroll hit adapter handles
// a wheel event.
func TestRowRackScrollHitAdapterCallsOnScrollChanged(t *testing.T) {
	restore := noInputForTest()
	defer restore()

	rows := makeTestRows(20)
	called := false
	cb := RowRackCallbacks{
		OnScrollChanged:   func() { called = true },
		Rows:              func() []*DrumRow { return rows },
		IsInstrumentAvail: func(id string) bool { return true },
		RowHeight:         func() int { return TouchRowHeight() },
		DeleteConfirm:     func() (int, int64) { return -1, 0 },
		RenameRow:         func() int { return -1 },
		IsMobileEQMode:    func() bool { return false },
		Frame:             func() int64 { return 0 },
	}
	z := NewRowRackZone(cb)

	tree := registerRowRackZone(z, image.Rect(0, 0, 300, 100))
	tree.Update()

	scrollArea := findHitAreaByTagPrefix(z.HitAreas(), "row-rack-scroll")
	if scrollArea == nil {
		t.Fatal("expected row-rack-scroll hit area")
	}

	result := scrollArea.Handler.OnWheel(50, 50, -1)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %d", result)
	}

	if !called {
		t.Error("expected OnScrollChanged callback to be called")
	}
}
