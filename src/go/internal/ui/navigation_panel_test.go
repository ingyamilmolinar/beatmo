//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestNavigationPanelWiresControls(t *testing.T) {
	var prev, next, pick, unit, gotoN int
	gotoAt := -1
	p := &NavigationPanel{}
	p.SetCallbacks(NavPanelCallbacks{
		Position:      func() PlayheadPos { return playheadPosAt(12, 0.5, 8) },
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
		OnGoto:        func(s int) { gotoN++; gotoAt = s },
	})
	h := p.PanelHeight()
	p.Layout(image.Rect(0, 0, 320, h))
	if h != 5*touchMinTargetPx+gotoWheelHeight() {
		t.Fatalf("PanelHeight=%d want %d", h, 5*touchMinTargetPx+gotoWheelHeight())
	}
	// Drawing must not panic and must render (pixel presence).
	dst := ebiten.NewImage(320, h)
	p.Draw(dst)
	// Go seeks to the wheel's pending position, seated at the playhead.
	fired := map[string]*int{"prev": &prev, "next": &next, "pick": &pick, "go": &gotoN}
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
	if gotoAt != 96 {
		t.Fatalf("Go seeked to %d, want the playhead step 96 (nothing scrolled)", gotoAt)
	}
	suppressClicksUntilRelease = false
	seg1 := p.unitSeg.SegmentRect(1)
	p.HandleInput((seg1.Min.X+seg1.Max.X)/2, (seg1.Min.Y+seg1.Max.Y)/2, true)
	if unit == 0 {
		t.Fatal("tapping the Subdiv segment must fire OnToggleUnit")
	}
}

// Delegating to Button.HandleInputResult fires once on the held==1 edge, not on every pressed frame.
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
