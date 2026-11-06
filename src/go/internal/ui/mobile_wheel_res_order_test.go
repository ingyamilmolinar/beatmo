// src/go/internal/ui/mobile_wheel_res_order_test.go
package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// openResLadderPopup opens a wheel popup over a continuous param that has a
// full step ladder, so the resolution strip is laid out.
func openResLadderPopup(t *testing.T) (*MobileWheelPopup, *KnobStepBadge) {
	t.Helper()
	def := audio.ParamDef{Name: "cutoff", Min: 20, Max: 20000, Unit: "Hz"}
	k := &Knob{Endless: true}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	badge := NewKnobStepBadge(def)
	k.StepMul = badge.Step()

	w := NewMobileWheelPopup()
	w.Open(WheelBinding{
		Knob: k, Badge: badge, Def: def,
		OnChange: func() {}, OnResolution: func() {}, Title: func() string { return "Cutoff" },
	}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)
	if w.resRect.Empty() {
		t.Fatal("resolution strip not laid out")
	}
	return w, badge
}

// TestWheelResolutionLadderCoarsestOnTop pins the resolution strip to the same
// orientation as the value barrel above it: bigger numbers on top. rungRects()
// stays indexed by LADDER rung (fine→coarse), so the coarsest rung — the last
// index — must occupy the TOP pill and each finer rung must sit lower.
func TestWheelResolutionLadderCoarsestOnTop(t *testing.T) {
	w, badge := openResLadderPopup(t)
	rects := w.rungRects()
	if len(rects) != badge.LadderLen() {
		t.Fatalf("rungRects must stay indexed by ladder rung: got %d rects for %d rungs",
			len(rects), badge.LadderLen())
	}
	if len(rects) < 2 {
		t.Fatalf("need a multi-rung ladder to test ordering, got %d", len(rects))
	}
	for i := 1; i < len(rects); i++ {
		if rects[i].Min.Y >= rects[i-1].Min.Y {
			t.Errorf("coarser rung %d (step %v) must draw ABOVE finer rung %d (step %v): y %d vs %d",
				i, badge.Rungs()[i], i-1, badge.Rungs()[i-1], rects[i].Min.Y, rects[i-1].Min.Y)
		}
	}
}

// TestWheelResolutionTopPillSelectsCoarsest proves the hit-test moved with the
// pills: tapping the topmost pill selects the coarsest step, not the finest.
func TestWheelResolutionTopPillSelectsCoarsest(t *testing.T) {
	w, badge := openResLadderPopup(t)
	rects := w.rungRects()

	top := rects[0]
	for _, r := range rects {
		if r.Min.Y < top.Min.Y {
			top = r
		}
	}
	w.HandleInput(top.Min.X+top.Dx()/2, top.Min.Y+top.Dy()/2, true)

	want := badge.LadderLen() - 1
	if badge.Index() != want {
		t.Fatalf("tapping the top pill must select the coarsest rung %d (step %v), got %d (step %v)",
			want, badge.Rungs()[want], badge.Index(), badge.Step())
	}
}
