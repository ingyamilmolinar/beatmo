//go:build test

package ui

import (
	"image"
	"testing"
)

func TestHitIndexBasicQuery(t *testing.T) {
	idx := &HitIndex{}
	handler := &testHitHandler{}
	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(10, 10, 50, 50), ZIndex: 100, Handler: handler, Tag: "btn"},
	})

	hits := idx.At(25, 25)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Tag != "btn" {
		t.Errorf("expected tag 'btn', got %q", hits[0].Tag)
	}

	// Miss.
	hits = idx.At(0, 0)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits outside area, got %d", len(hits))
	}
}

func TestHitIndexZOrdering(t *testing.T) {
	idx := &HitIndex{}
	h1 := &testHitHandler{}
	h2 := &testHitHandler{}
	// Two overlapping areas at different z-indexes.
	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(0, 0, 100, 100), ZIndex: 100, Handler: h1, Tag: "low"},
		{Rect: image.Rect(0, 0, 100, 100), ZIndex: 150, Handler: h2, Tag: "high"},
	})

	hits := idx.At(50, 50)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].Tag != "high" {
		t.Errorf("expected highest z-index first, got %q", hits[0].Tag)
	}
	if hits[1].Tag != "low" {
		t.Errorf("expected lowest z-index second, got %q", hits[1].Tag)
	}
}

func TestHitIndexModalFiltering(t *testing.T) {
	idx := &HitIndex{}
	h1 := &testHitHandler{}
	h2 := &testHitHandler{}

	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(0, 0, 100, 100), ZIndex: 100, Handler: h1, Tag: "zone"},
	})
	idx.UpdatePortal("overlay-1", []HitArea{
		{Rect: image.Rect(20, 20, 80, 80), ZIndex: 300, Handler: h2, Tag: "overlay"},
	})

	// Without modal: both visible.
	hits := idx.At(50, 50)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits without modal, got %d", len(hits))
	}

	// With modal: only overlay visible.
	idx.SetModal("overlay-1")
	hits = idx.At(50, 50)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit with modal, got %d", len(hits))
	}
	if hits[0].Tag != "overlay" {
		t.Errorf("expected overlay hit, got %q", hits[0].Tag)
	}

	// Click outside overlay returns nothing when modal.
	hits = idx.At(5, 5)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits outside modal, got %d", len(hits))
	}

	// Clear modal.
	idx.ClearModal()
	hits = idx.At(50, 50)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits after clearing modal, got %d", len(hits))
	}
}

func TestHitIndexTouchExpansion(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	idx := &HitIndex{}
	handler := &testHitHandler{}
	// Small 10x10 button.
	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(100, 100, 110, 110), ZIndex: 100, Handler: handler, Tag: "tiny", Touch: true},
	})

	// Touch expansion should make the area larger.
	expand := TouchMinTarget()
	if expand <= 0 {
		t.Fatal("expected positive TouchMinTarget on small screen")
	}

	// Hit slightly outside the original rect but inside expanded.
	hits := idx.At(100-expand+1, 105)
	if len(hits) != 1 {
		t.Fatalf("expected touch-expanded hit, got %d hits", len(hits))
	}
}

func TestHitIndexClipRectCutsOutsideButton(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	idx := &HitIndex{}
	handler := &testHitHandler{}

	// Button rect sits outside the clip rect (mimics len+/- buttons
	// positioned to the right of the shrunk timeline zone rect).
	clipRect := image.Rect(100, 100, 300, 200)
	buttonRect := image.Rect(304, 140, 340, 170) // 4px right of clip

	// With ClipRect: touch expansion intersects with clip, producing empty hit.
	idx.Update("zone-a", []HitArea{
		{Rect: buttonRect, ZIndex: 100, Handler: handler, Tag: "clipped-btn", Touch: true, ClipRect: clipRect},
	})
	cx := (buttonRect.Min.X + buttonRect.Max.X) / 2
	cy := (buttonRect.Min.Y + buttonRect.Max.Y) / 2
	hits := idx.At(cx, cy)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits when ClipRect excludes button, got %d", len(hits))
	}

	// Without ClipRect: same button is hittable.
	idx.Update("zone-a", []HitArea{
		{Rect: buttonRect, ZIndex: 100, Handler: handler, Tag: "unclipped-btn", Touch: true},
	})
	hits = idx.At(cx, cy)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit without ClipRect, got %d", len(hits))
	}
	if hits[0].Tag != "unclipped-btn" {
		t.Errorf("expected tag 'unclipped-btn', got %q", hits[0].Tag)
	}
}

func TestHitIndexPortalAddRemove(t *testing.T) {
	idx := &HitIndex{}
	handler := &testHitHandler{}

	idx.UpdatePortal("popup-1", []HitArea{
		{Rect: image.Rect(0, 0, 50, 50), ZIndex: 300, Handler: handler, Tag: "popup"},
	})
	hits := idx.At(25, 25)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit after add, got %d", len(hits))
	}

	idx.RemovePortal("popup-1")
	hits = idx.At(25, 25)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits after remove, got %d", len(hits))
	}
}

func TestHitIndexUpdateReplacesExisting(t *testing.T) {
	idx := &HitIndex{}
	handler := &testHitHandler{}

	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(0, 0, 50, 50), ZIndex: 100, Handler: handler, Tag: "old"},
	})
	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(60, 60, 100, 100), ZIndex: 100, Handler: handler, Tag: "new"},
	})

	// Old area should be gone.
	hits := idx.At(25, 25)
	if len(hits) != 0 {
		t.Fatalf("old area should be removed, got %d hits", len(hits))
	}

	// New area should be present.
	hits = idx.At(80, 80)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit for new area, got %d", len(hits))
	}
	if hits[0].Tag != "new" {
		t.Errorf("expected tag 'new', got %q", hits[0].Tag)
	}
}

// testHitHandler is a minimal HitHandler for testing.
type testHitHandler struct {
	pressCount   int
	dragCount    int
	releaseCount int
	wheelCount   int
	lastX, lastY int
	pressResult  InputResult
}

func (h *testHitHandler) OnPress(x, y int) InputResult {
	h.pressCount++
	h.lastX, h.lastY = x, y
	return h.pressResult
}

func (h *testHitHandler) OnDrag(x, y int) {
	h.dragCount++
	h.lastX, h.lastY = x, y
}

func (h *testHitHandler) OnRelease(x, y int) {
	h.releaseCount++
	h.lastX, h.lastY = x, y
}

func (h *testHitHandler) OnWheel(x, y, steps int) InputResult {
	h.wheelCount++
	return InputConsumed
}
