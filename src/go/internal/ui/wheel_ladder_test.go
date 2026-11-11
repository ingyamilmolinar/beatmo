package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func mkWheelForLadder(t *testing.T) (*MobileWheelPopup, *Knob, *KnobStepBadge) {
	t.Helper()
	def := audio.ParamDef{Name: "cutoff", Label: "Cutoff", Min: 20, Max: 16000, Unit: "Hz"}
	k := &Knob{Endless: true, StepMul: 50}
	b := NewKnobStepBadge(def)
	w := NewMobileWheelPopup()
	w.Open(WheelBinding{Knob: k, Badge: b, Def: def,
		Title: func() string { return "Cutoff" },
		Gloss: func() string { return "bright or dull" },
	}, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600), 0)
	return w, k, b
}

// Every ladder rung gets a tappable rect; tapping one jumps straight to it
// and syncs the knob's StepMul.
func TestWheelLadderTapJumpsToRung(t *testing.T) {
	w, k, b := mkWheelForLadder(t)
	rungs := w.rungRects()
	if len(rungs) != b.LadderLen() {
		t.Fatalf("rung rects %d != ladder %d", len(rungs), b.LadderLen())
	}
	target := 0 // finest rung (top)
	if b.Index() == target {
		target = b.LadderLen() - 1
	}
	p := rungs[target].Min.Add(image.Pt(2, 2))
	w.HandleInput(p.X, p.Y, true)
	w.HandleInput(p.X, p.Y, false)
	if b.Index() != target {
		t.Fatalf("tap on rung %d landed on %d", target, b.Index())
	}
	if k.StepMul != b.Step() {
		t.Fatalf("StepMul %v not synced to rung step %v", k.StepMul, b.Step())
	}
}

// Plain cursor-over (the portalHoverer seam) tracks the hovered rung and
// clears when the cursor leaves the popup entirely.
func TestWheelLadderHoverTracksRung(t *testing.T) {
	w, _, _ := mkWheelForLadder(t)
	rungs := w.rungRects()
	p := rungs[0].Min.Add(image.Pt(2, 2))
	w.HoverAt(p.X, p.Y)
	if w.hoverRung != 0 {
		t.Fatalf("hoverRung = %d, want 0", w.hoverRung)
	}
	w.HoverAt(w.rect.Min.X-5, w.rect.Min.Y-5)
	if w.hoverRung != -1 {
		t.Fatalf("hover off-popup must clear, got %d", w.hoverRung)
	}
}

// The ladder's vertical ORDER (coarsest rung at the top, so the strip reads the
// same way as the value barrel beside it) is pinned by
// TestWheelResolutionLadderCoarsestOnTop and TestWheelResolutionTopPillSelectsCoarsest
// in mobile_wheel_res_order_test.go. The two tests above deliberately stay
// order-agnostic: they index rungRects() by LADDER rung, which is the contract
// that survives the flip.
