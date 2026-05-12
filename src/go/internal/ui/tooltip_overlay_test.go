package ui

import (
	"image"
	"testing"
)

// TestTooltipOverlayLayoutClampsAndDraws exercises the new lightweight
// TooltipOverlay portal helper used by the chain-panel hover dwell flow.
// The overlay must:
//   - Layout a rectangle below its anchor sized to the caption-scaled text.
//   - Clamp to the screen bounds (Max.X never escapes the screen).
//   - Flip above the anchor when there is no room below.
//   - Report HitAreas() == nil (it is purely decorative).
//   - Report ShouldClose() == false initially, true after Close().
func TestTooltipOverlayLayoutClampsAndDraws(t *testing.T) {
	o := NewTooltipOverlay("hi")
	if o == nil {
		t.Fatal("NewTooltipOverlay returned nil")
	}
	if o.HitAreas() != nil {
		t.Fatalf("HitAreas() = %v; want nil", o.HitAreas())
	}
	if o.ShouldClose() {
		t.Fatal("ShouldClose() = true initially; want false")
	}

	// Anchor at (100,100..150,120). Screen is 200x200 so there IS room below.
	anchor := image.Rect(100, 100, 150, 120)
	screen := image.Rect(0, 0, 200, 200)
	o.Layout(anchor, screen)

	captionScale := FontSizeCaption / FontSizeBody
	wantMinW := int(float64(TextWidth("hi"))*captionScale) + 8
	wantMinH := int(float64(TextHeight())*captionScale) + 6
	if o.rect.Dx() < wantMinW {
		t.Errorf("rect width %d < minimum %d", o.rect.Dx(), wantMinW)
	}
	if o.rect.Dy() < wantMinH {
		t.Errorf("rect height %d < minimum %d", o.rect.Dy(), wantMinH)
	}
	if o.rect.Min.Y < anchor.Max.Y {
		t.Errorf("rect.Min.Y %d should be >= anchor.Max.Y %d (below anchor)", o.rect.Min.Y, anchor.Max.Y)
	}
	if o.rect.Max.X > screen.Max.X {
		t.Errorf("rect.Max.X %d escapes screen.Max.X %d", o.rect.Max.X, screen.Max.X)
	}

	// Clamp test: anchor near the right edge — Max.X must stay within screen.
	near := image.Rect(190, 10, 199, 30)
	o.Layout(near, screen)
	if o.rect.Max.X > screen.Max.X {
		t.Errorf("after right-edge layout, rect.Max.X %d escapes screen %d", o.rect.Max.X, screen.Max.X)
	}

	// Flip-above test: anchor near the bottom — overlay must flip above.
	bot := image.Rect(50, 190, 100, 199)
	o.Layout(bot, screen)
	if o.rect.Min.Y >= bot.Min.Y {
		t.Errorf("expected flip above for bottom-edge anchor; rect.Min.Y=%d anchor.Min.Y=%d", o.rect.Min.Y, bot.Min.Y)
	}

	// Close behavior.
	o.Close()
	if !o.ShouldClose() {
		t.Fatal("ShouldClose() = false after Close(); want true")
	}
}
