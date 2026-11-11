package ui

import (
	"image"
	"testing"
)

func TestScrollBehavior_Wheel(t *testing.T) {
	sb := NewScrollBehavior(DefaultScrollbarStyle, 24)
	sb.VS.Total = 20
	sb.VS.Visible = 5
	sb.VS.View = image.Rect(0, 0, 200, 120)

	// Wheel down (negative steps) should increase First
	changed := sb.HandleWheel(-3)
	if !changed || sb.VS.First != 3 {
		t.Fatalf("wheel down: changed=%v first=%d want 3", changed, sb.VS.First)
	}

	// Wheel up should decrease First
	changed = sb.HandleWheel(2)
	if !changed || sb.VS.First != 1 {
		t.Fatalf("wheel up: changed=%v first=%d want 1", changed, sb.VS.First)
	}

	// Wheel beyond top clamps
	sb.HandleWheel(10)
	if sb.VS.First != 0 {
		t.Fatalf("clamp top: first=%d want 0", sb.VS.First)
	}

	// Wheel beyond bottom clamps
	sb.HandleWheel(-100)
	if sb.VS.First != 15 {
		t.Fatalf("clamp bottom: first=%d want 15", sb.VS.First)
	}

	// Dirty flag
	if !sb.Dirty() {
		t.Fatal("expected dirty after wheel")
	}
	sb.ClearDirty()
	if sb.Dirty() {
		t.Fatal("expected not dirty after clear")
	}
}

func TestScrollBehavior_WheelNoScroll(t *testing.T) {
	sb := NewScrollBehavior(DefaultScrollbarStyle, 24)
	sb.VS.Total = 3
	sb.VS.Visible = 5 // no scrolling needed

	changed := sb.HandleWheel(-1)
	if changed {
		t.Fatal("expected no change when no scroll needed")
	}
}

func TestScrollBehavior_DragSequence(t *testing.T) {
	sb := NewScrollBehavior(DropdownScrollbarStyle, 24)
	sb.VS.Total = 20
	sb.VS.Visible = 5
	sb.VS.View = image.Rect(190, 0, 200, 120)

	// Thumb should be present
	thumb := sb.ThumbRect()
	if thumb.Empty() {
		t.Fatal("thumb should not be empty")
	}

	// Start drag at thumb center
	midY := (thumb.Min.Y + thumb.Max.Y) / 2
	started := sb.HandleDragStart(midY)
	if !started {
		t.Fatal("drag should start within thumb")
	}
	if !sb.Dragging() {
		t.Fatal("should be dragging")
	}

	// Drag down
	sb.HandleDragTo(midY + 30)
	if sb.VS.First == 0 {
		t.Fatal("drag down should have advanced First")
	}

	// End drag
	sb.HandleDragEnd()
	if sb.Dragging() {
		t.Fatal("should not be dragging after end")
	}
}

func TestScrollBehavior_DragOutsideThumb(t *testing.T) {
	sb := NewScrollBehavior(DropdownScrollbarStyle, 24)
	sb.VS.Total = 20
	sb.VS.Visible = 5
	sb.VS.View = image.Rect(190, 0, 200, 120)

	// Click outside thumb
	started := sb.HandleDragStart(sb.ThumbRect().Max.Y + 50)
	if started {
		t.Fatal("drag should not start outside thumb")
	}
}

func TestScrollBehavior_TouchScrollPixelConversion(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.VS.Total = 10
	sb.VS.Visible = 4
	sb.VS.View = image.Rect(0, 0, 400, 208)

	sb.HandleTouchBegin(100, 200)

	// Move UP beyond dead zone to lock direction (returns 0 on lock frame).
	// Finger up = scroll down = First increases.
	sb.HandleTouchMove(100, 188) // 12px up > 8px dead zone, locks direction
	// The lock frame sets lastY=188, subsequent moves measure delta from there.
	// Move 52px UP (1 item height) from current lastY.
	changed := sb.HandleTouchMove(100, 188-52)
	if !changed || sb.VS.First != 1 {
		t.Fatalf("touch scroll: changed=%v first=%d want 1", changed, sb.VS.First)
	}
}

func TestScrollBehavior_TouchSubItemAccumulator(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.VS.Total = 10
	sb.VS.Visible = 4
	sb.VS.View = image.Rect(0, 0, 400, 208)

	sb.HandleTouchBegin(100, 300)
	// Lock direction (finger moves UP)
	sb.HandleTouchMove(100, 288) // locks vertical

	// Small upward moves that individually don't cross an item boundary
	sb.HandleTouchMove(100, 288-20)
	if sb.VS.First != 0 {
		t.Fatalf("expected no scroll yet, first=%d", sb.VS.First)
	}
	// Another small upward move that crosses the boundary with accumulated delta
	changed := sb.HandleTouchMove(100, 288-20-40)
	if !changed || sb.VS.First != 1 {
		t.Fatalf("accumulator: changed=%v first=%d want 1", changed, sb.VS.First)
	}
}

func TestScrollBehavior_MomentumDecay(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.VS.Total = 20
	sb.VS.Visible = 4
	sb.VS.View = image.Rect(0, 0, 400, 208)

	sb.HandleTouchBegin(100, 500)
	sb.HandleTouchMove(100, 488) // lock direction (upward)
	// Large upward swipe to build velocity (finger moves up = scroll down)
	sb.HandleTouchMove(100, 488-200)
	sb.HandleTouchEnd()

	if !sb.HasMomentum() {
		t.Fatal("expected momentum after swipe")
	}

	// Run momentum for several frames
	prevFirst := sb.VS.First
	for i := 0; i < 50; i++ {
		sb.UpdateMomentum()
	}
	if sb.VS.First <= prevFirst {
		t.Fatalf("momentum should advance scroll: prev=%d now=%d", prevFirst, sb.VS.First)
	}

	// Eventually momentum should stop
	for i := 0; i < 200; i++ {
		sb.UpdateMomentum()
	}
	if sb.HasMomentum() {
		t.Fatal("momentum should have decayed to zero")
	}
}

func TestScrollBehavior_BoundaryClamping(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.VS.Total = 5
	sb.VS.Visible = 3
	sb.VS.View = image.Rect(0, 0, 400, 156)

	// Scroll past end
	sb.HandleWheel(-100)
	if sb.VS.First != 2 {
		t.Fatalf("expected clamped to 2, got %d", sb.VS.First)
	}

	// Scroll past beginning
	sb.HandleWheel(100)
	if sb.VS.First != 0 {
		t.Fatalf("expected clamped to 0, got %d", sb.VS.First)
	}
}

func TestScrollBehavior_TouchScrollCommitted(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.VS.Total = 10
	sb.VS.Visible = 4
	sb.VS.View = image.Rect(0, 0, 400, 208)

	if sb.ScrollingCommitted() {
		t.Fatal("should not be committed before touch")
	}

	sb.HandleTouchBegin(100, 100)
	if sb.ScrollingCommitted() {
		t.Fatal("should not be committed before dead zone")
	}

	sb.HandleTouchMove(100, 120) // past dead zone, vertical
	if !sb.ScrollingCommitted() {
		t.Fatal("should be committed after passing dead zone vertically")
	}

	sb.HandleTouchEnd()
	if sb.ScrollingCommitted() {
		t.Fatal("should not be committed after touch end")
	}
}

func TestScrollBehavior_ResetTouch(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 52)
	sb.HandleTouchBegin(100, 100)
	sb.HandleTouchMove(100, 120)
	sb.HandleTouchEnd()

	sb.ResetTouch()
	if sb.HasMomentum() {
		t.Fatal("expected no momentum after reset")
	}
	if sb.TouchActive() {
		t.Fatal("expected not active after reset")
	}
}

func TestScrollBehavior_DrawEmpty(t *testing.T) {
	// Draw with empty state should not panic
	sb := NewScrollBehavior(DefaultScrollbarStyle, 24)
	sb.Draw(nil) // no scroll needed → early return
}

func TestScrollBehavior_ZeroItemHeight(t *testing.T) {
	sb := NewScrollBehavior(MobileScrollbarStyle, 0) // zero item height
	sb.VS.Total = 10
	sb.VS.Visible = 4

	sb.HandleTouchBegin(100, 100)
	sb.HandleTouchMove(100, 120)
	changed := sb.HandleTouchMove(100, 200)
	if changed {
		t.Fatal("expected no change with zero item height")
	}
}
