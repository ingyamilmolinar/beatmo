//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestLayoutResizeZone_ID(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)
	if got := z.ID(); got != "layout-resize" {
		t.Errorf("ID() = %q, want %q", got, "layout-resize")
	}
}

func TestLayoutResizeZone_Layout(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)

	r := image.Rect(10, 20, 300, 400)
	z.Layout(r)

	if z.rect != r {
		t.Errorf("rect = %v, want %v", z.rect, r)
	}
	if z.dirty {
		t.Error("dirty should be false after Layout")
	}
}

func TestLayoutResizeZone_NeedsLayout(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)

	if !z.NeedsLayout() {
		t.Error("new zone should need layout")
	}

	z.Layout(image.Rect(0, 0, 100, 100))
	if z.NeedsLayout() {
		t.Error("zone should not need layout after Layout()")
	}
}

func TestLayoutResizeZone_Invalidate(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)
	z.Layout(image.Rect(0, 0, 100, 100))

	if z.NeedsLayout() {
		t.Fatal("precondition: should not need layout after Layout()")
	}

	z.Invalidate()
	if !z.NeedsLayout() {
		t.Error("should need layout after Invalidate()")
	}
}

func TestLayoutResizeZone_Update_NilHandler(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)
	// Should not panic with nil handler.
	z.Update()
}

func TestLayoutResizeZone_Update_Dragging(t *testing.T) {
	assertDefaultParityState(t)

	dv := &DrumView{}
	h := &LayoutResizeHandler{dv: dv, dragging: true}
	z := newLayoutResizeZone(h)

	// Should return early without accessing cursor or DrumView bounds.
	z.Update()
}

func TestLayoutResizeZone_HitAreas_NilHandler(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)

	areas := z.HitAreas()
	if areas != nil {
		t.Errorf("HitAreas() with nil handler = %v, want nil", areas)
	}
}

func TestLayoutResizeZone_HitAreas_NilWidgets(t *testing.T) {
	assertDefaultParityState(t)

	dv := &DrumView{} // dv.widgets is nil
	h := &LayoutResizeHandler{dv: dv}
	z := newLayoutResizeZone(h)

	areas := z.HitAreas()
	if areas != nil {
		t.Errorf("HitAreas() with nil widgets = %v, want nil", areas)
	}
}

func TestLayoutResizeZone_HitAreas_DisabledResize(t *testing.T) {
	assertDefaultParityState(t)

	// Force mobile profile where EnableLayoutResize is false.
	forceSmallScreenForTest = true
	defer func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	}()
	UpdateProfile()

	if Profile().EnableLayoutResize {
		t.Fatal("precondition: EnableLayoutResize should be false on mobile")
	}

	// Provide a non-nil widgets so the nil check passes but the feature
	// flag check triggers the early return.
	wb := NewWidgetBoard(image.Rect(0, 0, 800, 400), []float64{1, 3}, []float64{3, 3, 3})
	dv := &DrumView{widgets: wb}
	h := &LayoutResizeHandler{dv: dv}
	z := newLayoutResizeZone(h)

	areas := z.HitAreas()
	if areas != nil {
		t.Errorf("HitAreas() with EnableLayoutResize=false = %v, want nil", areas)
	}
}

func TestLayoutResizeZone_Draw(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)
	// Draw is a no-op; verify it does not panic with nil screen.
	z.Draw(nil)
}

func TestLayoutResizeZone_HandleKey(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)

	if got := z.HandleKey(ebiten.KeyEnter); got != InputIgnored {
		t.Errorf("HandleKey() = %v, want InputIgnored", got)
	}
}

func TestLayoutResizeZone_HandleChars(t *testing.T) {
	assertDefaultParityState(t)
	z := newLayoutResizeZone(nil)

	if got := z.HandleChars([]rune{'a', 'b'}); got != InputIgnored {
		t.Errorf("HandleChars() = %v, want InputIgnored", got)
	}
	// Also test with nil slice.
	if got := z.HandleChars(nil); got != InputIgnored {
		t.Errorf("HandleChars(nil) = %v, want InputIgnored", got)
	}
}

func TestLayoutResizeHitHandler_OnWheel(t *testing.T) {
	assertDefaultParityState(t)

	dv := &DrumView{}
	h := &LayoutResizeHandler{dv: dv}
	hh := &layoutResizeHitHandler{handler: h, axis: "col", idx: 0}

	if got := hh.OnWheel(10, 20, 3); got != InputIgnored {
		t.Errorf("OnWheel() = %v, want InputIgnored", got)
	}
}
