//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPortalOpenClose(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	if portal.IsOpen() {
		t.Fatal("portal should start empty")
	}

	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(10, 10, 100, 100), Handler: &testHitHandler{}, Tag: "popup-btn"},
		},
	}
	portal.Open(PortalEntry{
		ID:      "test-popup",
		Overlay: overlay,
		Modal:   false,
		Anchor:  image.Rect(50, 50, 60, 60),
	})

	if !portal.IsOpen() {
		t.Fatal("portal should be open after Open()")
	}
	if portal.StackLen() != 1 {
		t.Fatalf("expected stack len 1, got %d", portal.StackLen())
	}

	// Hit areas should be registered in the index.
	hits := idx.At(50, 50)
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit from portal overlay, got %d", len(hits))
	}

	portal.Close("test-popup")
	if portal.IsOpen() {
		t.Fatal("portal should be closed after Close()")
	}

	// Hit areas should be removed.
	hits = idx.At(50, 50)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits after close, got %d", len(hits))
	}
}

func TestPortalModalBlocking(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	// Add a zone area.
	idx.Update("zone-a", []HitArea{
		{Rect: image.Rect(0, 0, 800, 600), ZIndex: 100, Handler: &testHitHandler{}, Tag: "zone"},
	})

	// Open a modal overlay.
	overlay := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 400, 400), Handler: &testHitHandler{}, Tag: "modal-btn"},
		},
	}
	portal.Open(PortalEntry{
		ID:      "modal-1",
		Overlay: overlay,
		Modal:   true,
		Anchor:  image.Rect(300, 300, 310, 310),
	})

	if !portal.HasModal() {
		t.Fatal("portal should report modal")
	}

	// Inside modal: only modal areas.
	hits := idx.At(300, 300)
	if len(hits) != 1 {
		t.Fatalf("expected 1 modal hit, got %d", len(hits))
	}
	if hits[0].Tag != "modal-btn" {
		t.Errorf("expected modal-btn, got %q", hits[0].Tag)
	}

	// Outside modal: nothing (zone blocked).
	hits = idx.At(10, 10)
	if len(hits) != 0 {
		t.Fatalf("expected 0 hits outside modal, got %d", len(hits))
	}

	// Close modal: zone areas visible again.
	portal.Close("modal-1")
	hits = idx.At(10, 10)
	if len(hits) != 1 {
		t.Fatalf("expected zone hit after modal close, got %d", len(hits))
	}
}

func TestPortalCloseTop(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	portal.Open(PortalEntry{
		ID:      "a",
		Overlay: &testPortalOverlay{},
		Modal:   false,
	})
	portal.Open(PortalEntry{
		ID:      "b",
		Overlay: &testPortalOverlay{},
		Modal:   false,
	})
	if portal.StackLen() != 2 {
		t.Fatalf("expected 2, got %d", portal.StackLen())
	}
	if portal.TopID() != "b" {
		t.Errorf("expected top 'b', got %q", portal.TopID())
	}

	portal.CloseTop()
	if portal.StackLen() != 1 {
		t.Fatalf("expected 1 after CloseTop, got %d", portal.StackLen())
	}
	if portal.TopID() != "a" {
		t.Errorf("expected top 'a', got %q", portal.TopID())
	}

	portal.CloseTop()
	if portal.IsOpen() {
		t.Fatal("expected empty after two CloseTop calls")
	}
}

func TestPortalStackOrdering(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	// Two overlapping overlays.
	portal.Open(PortalEntry{
		ID: "bottom",
		Overlay: &testPortalOverlay{
			hitAreas: []HitArea{
				{Rect: image.Rect(0, 0, 100, 100), Handler: &testHitHandler{}, Tag: "bottom-btn"},
			},
		},
	})
	portal.Open(PortalEntry{
		ID: "top",
		Overlay: &testPortalOverlay{
			hitAreas: []HitArea{
				{Rect: image.Rect(0, 0, 100, 100), Handler: &testHitHandler{}, Tag: "top-btn"},
			},
		},
	})

	hits := idx.At(50, 50)
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	// Top overlay should have higher z-index and appear first.
	if hits[0].Tag != "top-btn" {
		t.Errorf("expected top-btn first, got %q", hits[0].Tag)
	}
}

func TestPortalMultiModalClose(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	portal.Open(PortalEntry{ID: "modal-1", Overlay: &testPortalOverlay{}, Modal: true})
	portal.Open(PortalEntry{ID: "modal-2", Overlay: &testPortalOverlay{}, Modal: true})

	if !portal.HasModal() {
		t.Fatal("should have modal")
	}

	// Close top modal.
	portal.CloseTop()
	if !portal.HasModal() {
		t.Fatal("should still have modal after closing top")
	}

	// Close remaining modal.
	portal.CloseTop()
	if portal.HasModal() {
		t.Fatal("should not have modal after closing all")
	}
}

// testPortalOverlay is a minimal PortalOverlay for testing.
type testPortalOverlay struct {
	hitAreas     []HitArea
	layoutCalled bool
	drawCalled   bool
	shouldClose  bool
}

func (o *testPortalOverlay) Layout(anchor, screenBounds image.Rectangle) {
	o.layoutCalled = true
}

func (o *testPortalOverlay) HitAreas() []HitArea {
	return o.hitAreas
}

func (o *testPortalOverlay) Draw(_ *ebiten.Image) {
	o.drawCalled = true
}

func (o *testPortalOverlay) ShouldClose() bool {
	return o.shouldClose
}

// ---------------------------------------------------------------------------
// Portal system gap tests
// ---------------------------------------------------------------------------

// TestPortalLayoutRelayoutsAll verifies that Layout() calls Layout on all
// overlays in the stack.
func TestPortalLayoutRelayoutsAll(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	ov1 := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(10, 10, 100, 100), Handler: &testHitHandler{}, Tag: "a"},
		},
	}
	ov2 := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(200, 200, 300, 300), Handler: &testHitHandler{}, Tag: "b"},
		},
	}

	portal.Open(PortalEntry{
		ID:      "a",
		Overlay: ov1,
		Anchor:  image.Rect(50, 50, 60, 60),
	})
	portal.Open(PortalEntry{
		ID:      "b",
		Overlay: ov2,
		Anchor:  image.Rect(250, 250, 260, 260),
	})

	// Reset layoutCalled since Open() already calls Layout once.
	ov1.layoutCalled = false
	ov2.layoutCalled = false

	portal.Layout()

	if !ov1.layoutCalled {
		t.Error("overlay 'a' Layout was not called during portal.Layout()")
	}
	if !ov2.layoutCalled {
		t.Error("overlay 'b' Layout was not called during portal.Layout()")
	}
}

// TestPortalOnCloseCallback verifies OnClose is called when an overlay is
// closed by ID.
func TestPortalOnCloseCallback(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	closed := false
	portal.Open(PortalEntry{
		ID:      "x",
		Overlay: &testPortalOverlay{},
		OnClose: func() { closed = true },
	})

	portal.Close("x")

	if !closed {
		t.Error("OnClose callback was not called on Close()")
	}
}

// TestPortalOnCloseCallbackOnCloseTop verifies OnClose is called when the
// topmost overlay is closed via CloseTop().
func TestPortalOnCloseCallbackOnCloseTop(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	closed := false
	portal.Open(PortalEntry{
		ID:      "top-cb",
		Overlay: &testPortalOverlay{},
		OnClose: func() { closed = true },
	})

	portal.CloseTop()

	if !closed {
		t.Error("OnClose callback was not called on CloseTop()")
	}
}

// TestPortalCleanupClosedAutoRemoves verifies that CleanupClosed removes
// overlays whose ShouldClose() returns true and fires OnClose.
func TestPortalCleanupClosedAutoRemoves(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	closed := false
	ov := &testPortalOverlay{shouldClose: true}
	portal.Open(PortalEntry{
		ID:      "auto-close",
		Overlay: ov,
		OnClose: func() { closed = true },
	})

	if !portal.IsOpen() {
		t.Fatal("portal should be open before cleanup")
	}

	portal.CleanupClosed()

	if portal.IsOpen() {
		t.Error("portal should be empty after CleanupClosed removes self-closing overlay")
	}
	if !closed {
		t.Error("OnClose should be called during CleanupClosed")
	}
}

// TestPortalCleanupClosedPartial verifies that CleanupClosed only removes
// overlays that request closing, leaving others intact.
func TestPortalCleanupClosedPartial(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	ovA := &testPortalOverlay{shouldClose: false}
	ovB := &testPortalOverlay{shouldClose: true}

	portal.Open(PortalEntry{
		ID:      "a",
		Overlay: ovA,
	})
	portal.Open(PortalEntry{
		ID:      "b",
		Overlay: ovB,
	})

	if portal.StackLen() != 2 {
		t.Fatalf("expected 2 overlays, got %d", portal.StackLen())
	}

	portal.CleanupClosed()

	if portal.StackLen() != 1 {
		t.Fatalf("expected 1 overlay after cleanup, got %d", portal.StackLen())
	}
	if portal.TopID() != "a" {
		t.Errorf("expected remaining overlay 'a', got %q", portal.TopID())
	}
	if !portal.Has("a") {
		t.Error("overlay 'a' should still be in the stack")
	}
	if portal.Has("b") {
		t.Error("overlay 'b' should have been removed")
	}
}

// TestPortalModalSwitching verifies that closing the top modal still leaves
// a lower modal active.
func TestPortalModalSwitching(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	// Add a zone-level hit area.
	idx.Update("zone-bg", []HitArea{
		{Rect: image.Rect(0, 0, 800, 600), ZIndex: 100, Handler: &testHitHandler{}, Tag: "bg"},
	})

	ov1 := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(100, 100, 200, 200), Handler: &testHitHandler{}, Tag: "modal-1-btn"},
		},
	}
	ov2 := &testPortalOverlay{
		hitAreas: []HitArea{
			{Rect: image.Rect(300, 300, 400, 400), Handler: &testHitHandler{}, Tag: "modal-2-btn"},
		},
	}

	portal.Open(PortalEntry{ID: "modal-1", Overlay: ov1, Modal: true})
	portal.Open(PortalEntry{ID: "modal-2", Overlay: ov2, Modal: true})

	if !portal.HasModal() {
		t.Fatal("should have modal with two modal overlays")
	}

	// Close top modal (modal-2).
	portal.CloseTop()

	if !portal.HasModal() {
		t.Error("should still have modal after closing top (modal-1 remains)")
	}

	// The hit index should now filter to modal-1's areas.
	hits := idx.At(150, 150) // inside modal-1
	if len(hits) == 0 {
		t.Error("expected hits inside modal-1 after closing modal-2")
	}

	// Outside both modals: zone bg should be blocked by remaining modal.
	hits = idx.At(500, 500)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits outside modal-1, got %d", len(hits))
	}
}

// TestPortalDuplicateIdReplaces verifies that opening an overlay with a
// duplicate ID replaces the existing one rather than stacking duplicates.
func TestPortalDuplicateIdReplaces(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	portal.Open(PortalEntry{
		ID:      "x",
		Overlay: &testPortalOverlay{},
	})
	portal.Open(PortalEntry{
		ID:      "x",
		Overlay: &testPortalOverlay{},
	})

	if portal.StackLen() != 1 {
		t.Errorf("expected 1 entry (duplicate replaced), got %d", portal.StackLen())
	}
	if portal.TopID() != "x" {
		t.Errorf("expected top 'x', got %q", portal.TopID())
	}
}

// TestPortalOnCloseCallbackDuringCleanup verifies OnClose fires exactly once
// when an overlay self-closes via ShouldClose and CleanupClosed runs.
func TestPortalOnCloseCallbackDuringCleanup(t *testing.T) {
	idx := &HitIndex{}
	portal := NewOverlayPortal(idx)
	portal.SetScreenBounds(image.Rect(0, 0, 800, 600))

	closeCount := 0
	ov := &testPortalOverlay{shouldClose: true}
	portal.Open(PortalEntry{
		ID:      "self-close",
		Overlay: ov,
		OnClose: func() { closeCount++ },
	})

	portal.CleanupClosed()

	if closeCount != 1 {
		t.Errorf("OnClose should fire exactly once during CleanupClosed, got %d", closeCount)
	}

	// Double cleanup should not re-fire.
	portal.CleanupClosed()
	if closeCount != 1 {
		t.Errorf("OnClose should not fire again on second CleanupClosed, got %d", closeCount)
	}
}
