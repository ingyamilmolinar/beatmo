//go:build test

package ui

import (
	"image"
	"testing"
)

func trailingTestButtons() []*Button {
	preview := NewSpecButton("Preview", ComponentButtonSecondary, nil)
	save := NewSpecButton("Save", ComponentButtonPrimary, nil)
	overflow := NewSpecButton("", ComponentButtonSecondary, nil)
	overflow.Icon = string(IconOverflow)
	return []*Button{preview, save, overflow}
}

// TestStickyBarTrailingActionsMobileLayout verifies the sticky bar lays trailing
// per-tab action buttons into its right cluster on mobile: all get non-empty
// rects inside the bar, right-aligned, never overlapping the channel pill, and
// each is registered as a tappable hit area.
func TestStickyBarTrailingActionsMobileLayout(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Fatal("did not enter mobile profile")
	}

	b := NewAudioStickyBar(130, func() {}, func(PanelTab) {})
	btns := trailingTestButtons()
	b.SetTrailingActions(btns)

	barRect := image.Rect(0, 0, 390, stickyBarHeight())
	b.Layout(barRect)

	chMax := b.ChannelBtn().Rect().Max.X
	prevMinX := -1
	for i, btn := range btns {
		r := btn.Rect()
		if r.Empty() {
			t.Fatalf("trailing button %d has empty rect on mobile", i)
		}
		if !r.In(barRect) {
			t.Errorf("trailing button %d rect %v not inside bar %v", i, r, barRect)
		}
		if r.Min.X < chMax {
			t.Errorf("trailing button %d (minX=%d) overlaps the channel pill (maxX=%d)", i, r.Min.X, chMax)
		}
		// Buttons are laid left-to-right in array order (preview, save, overflow).
		if r.Min.X <= prevMinX {
			t.Errorf("trailing buttons not laid left-to-right: button %d minX=%d not right of prev %d", i, r.Min.X, prevMinX)
		}
		prevMinX = r.Min.X
	}

	// Rightmost button (overflow) hugs the right edge.
	if got := btns[len(btns)-1].Rect().Max.X; got > barRect.Max.X || got < barRect.Max.X-16 {
		t.Errorf("rightmost trailing button Max.X=%d not near right edge %d", got, barRect.Max.X)
	}

	// Every trailing button is a hit area.
	hitCount := 0
	for _, ha := range b.HitAreas() {
		for _, btn := range btns {
			if ha.Rect == btn.Rect() {
				hitCount++
			}
		}
	}
	if hitCount < len(btns) {
		t.Errorf("only %d/%d trailing buttons registered as hit areas", hitCount, len(btns))
	}
}

// TestStickyBarTrailingActionsMobileTouchMin verifies the relocated action
// buttons meet the finger target via Touch+ClipRect expansion (the ~32px pills
// live in a 40px bar; the expanded hit rect must reach MinTarget), mirroring the
// channel pill's discipline.
func TestStickyBarTrailingActionsMobileTouchMin(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Fatal("did not enter mobile profile")
	}

	b := NewAudioStickyBar(130, func() {}, func(PanelTab) {})
	btns := trailingTestButtons()
	b.SetTrailingActions(btns)
	b.Layout(image.Rect(0, 0, 390, stickyBarHeight()))

	minTarget := Profile().MinTarget
	found := 0
	for _, ha := range b.HitAreas() {
		isTrailing := false
		for _, btn := range btns {
			if ha.Rect == btn.Rect() {
				isTrailing = true
			}
		}
		if !isTrailing {
			continue
		}
		found++
		if !ha.Touch {
			t.Errorf("trailing hit area %v not flagged Touch", ha.Rect)
		}
		if ha.ClipRect.Dy() < minTarget {
			t.Errorf("trailing hit area ClipRect height %d < touch-min %d", ha.ClipRect.Dy(), minTarget)
		}
	}
	if found < len(btns) {
		t.Errorf("only %d/%d trailing hit areas found", found, len(btns))
	}
}

// TestStickyBarTrailingActionsDesktopIgnored verifies trailing actions are a
// mobile-only affordance: on desktop the right cluster is the tab strip, so
// trailing buttons receive no rect.
func TestStickyBarTrailingActionsDesktopIgnored(t *testing.T) {
	forceSmallScreenForTest = false
	UpdateProfile()
	if Profile().IsMobile() {
		t.Skip("test environment defaults to mobile; desktop path unverifiable")
	}

	b := NewAudioStickyBar(130, func() {}, func(PanelTab) {})
	btns := trailingTestButtons()
	b.SetTrailingActions(btns)
	b.Layout(image.Rect(0, 0, 1000, stickyBarHeight()))

	for i, btn := range btns {
		if !btn.Rect().Empty() {
			t.Errorf("desktop: trailing button %d got a rect %v, want empty", i, btn.Rect())
		}
	}
}
