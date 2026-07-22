//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNavigationPanelWiresControls(t *testing.T) {
	var prev, next, pick, unit, gotoBeat, gotoTime int
	p := &NavigationPanel{}
	p.SetCallbacks(NavPanelCallbacks{
		PlayheadStep:  func() int { return 96 },
		BPM:           func() int { return 120 },
		Div:           func() int { return 8 },
		TimelineBeats: func() int { return 64 },
		JumpAmount:    func() int { return 4 },
		JumpUnitBeats: func() bool { return true },
		OnPrev:        func() { prev++ },
		OnNext:        func() { next++ },
		OnPickAmount:  func() { pick++ },
		OnToggleUnit:  func() { unit++ },
		OnGotoBeat:    func() { gotoBeat++ },
		OnGotoTime:    func() { gotoTime++ },
	})
	p.Layout(image.Rect(0, 0, 320, 5*touchMinTargetPx))
	if p.PanelHeight() != 5*touchMinTargetPx {
		t.Fatalf("PanelHeight=%d want %d", p.PanelHeight(), 5*touchMinTargetPx)
	}
	// Drawing must not panic and must render (pixel presence).
	dst := ebiten.NewImage(320, 5*touchMinTargetPx)
	p.Draw(dst)
	// The *Button controls (prev/amount/next/goto-beat/goto-time) fire their
	// callbacks via OnClick.
	fired := map[string]*int{"prev": &prev, "next": &next, "pick": &pick, "gotoBeat": &gotoBeat, "gotoTime": &gotoTime}
	for _, b := range p.Buttons() {
		if b.OnClick != nil {
			b.OnClick()
		}
	}
	for name, c := range fired {
		if *c == 0 {
			t.Fatalf("control %q OnClick never fired a callback", name)
		}
	}
	// The unit toggle is a SegmentedControl: with Beats (unit=true) active,
	// tapping the Subdiv segment fires OnToggleUnit.
	suppressClicksUntilRelease = false
	seg1 := p.unitSeg.SegmentRect(1)
	p.HandleInput((seg1.Min.X+seg1.Max.X)/2, (seg1.Min.Y+seg1.Max.Y)/2, true)
	if unit == 0 {
		t.Fatal("tapping the Subdiv segment must fire OnToggleUnit")
	}
}

// TestNavigationPanelHandleInputFiresOncePerPress guards the Task 3 fix:
// HandleInput must delegate to Button.HandleInputResult so a held press fires
// OnClick exactly once (on the held==1 edge), not once per pressed frame.
// The pre-fix body fired b.OnClick on every frame where pressed==true.
func TestNavigationPanelHandleInputFiresOncePerPress(t *testing.T) {
	suppressClicksUntilRelease = false
	var prev int
	p := &NavigationPanel{}
	p.SetCallbacks(NavPanelCallbacks{
		JumpAmount:    func() int { return 4 },
		JumpUnitBeats: func() bool { return true },
		OnPrev:        func() { prev++ },
	})
	p.Layout(image.Rect(0, 0, 320, 5*touchMinTargetPx))
	// Center of the prev button (jump row, first quarter cell).
	pr := p.prevBtn.Rect()
	cx, cy := (pr.Min.X+pr.Max.X)/2, (pr.Min.Y+pr.Max.Y)/2

	// Three consecutive pressed frames without release = one held press.
	// A correct edge-firing handler fires OnPrev exactly once.
	p.HandleInput(cx, cy, true)
	p.HandleInput(cx, cy, true)
	p.HandleInput(cx, cy, true)
	if prev != 1 {
		t.Fatalf("held press fired OnPrev %d times, want 1 (repeat-fire regression)", prev)
	}

	// Release, then a fresh press must fire again (held counter reset).
	p.HandleInput(cx, cy, false)
	p.HandleInput(cx, cy, true)
	if prev != 2 {
		t.Fatalf("second press fired OnPrev to %d, want 2 (release did not reset edge)", prev)
	}
}
