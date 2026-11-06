//go:build test

package ui

import (
	"image"
	"testing"
)

func TestMenuCardBandsNeverOverlapAndStayInside(t *testing.T) {
	panel := image.Rect(100, 50, 440, 400)
	for _, spec := range []MenuCardSpec{
		{Title: "Plain"},
		{Title: "With caption", Caption: "Light", CaptionRight: "Deep"},
		{Title: "With footer", Footer: []*Button{NewSpecButton("Cancel", ComponentButtonSecondary, nil), NewSpecButton("OK", ComponentButtonDestructive, nil)}},
		{Title: "Everything", IconID: IconSettings, Caption: "hint", Footer: []*Button{NewSpecButton("Reset all", ComponentButtonDestructive, nil)}},
	} {
		c := NewMenuCard(nil)
		c.Layout(panel, spec)
		if !c.TitleRect().In(panel) || !c.Body().In(panel) {
			t.Fatalf("%s: bands outside panel: title=%v body=%v panel=%v", spec.Title, c.TitleRect(), c.Body(), panel)
		}
		if c.TitleRect().Max.Y > c.Body().Min.Y {
			t.Fatalf("%s: body overlaps title", spec.Title)
		}
		if !c.Caption().Empty() && (c.Caption().Max.Y > c.Body().Min.Y || c.Caption().Min.Y < c.TitleRect().Max.Y) {
			t.Fatalf("%s: caption band misplaced: %v", spec.Title, c.Caption())
		}
		if !c.FooterRect().Empty() {
			if c.FooterRect().Min.Y < c.Body().Max.Y {
				t.Fatalf("%s: footer overlaps body", spec.Title)
			}
			for _, b := range spec.Footer {
				if !b.Rect().In(c.FooterRect()) {
					t.Fatalf("%s: footer button %v outside footer %v", spec.Title, b.Rect(), c.FooterRect())
				}
			}
		}
		if cb := c.CloseBtn(); desktopCloseVisible() {
			if cb == nil || !cb.Rect().In(c.TitleRect()) {
				t.Fatalf("%s: desktop close × must sit inside the title band", spec.Title)
			}
		} else if cb != nil {
			t.Fatalf("%s: mobile must have no close ×", spec.Title)
		}
		areas := c.HitAreas(ZOverlayMin, "t", settingsPanelHandler{})
		if areas[len(areas)-1].Tag != "t-panel" || areas[len(areas)-1].Rect != panel {
			t.Fatalf("%s: catch-all must be LAST and cover the panel: %+v", spec.Title, areas[len(areas)-1])
		}
		for _, a := range areas[:len(areas)-1] {
			if _, ok := a.Handler.(*buttonHitAdapter); !ok {
				t.Fatalf("%s: control areas must route through buttonHitAdapter: %+v", spec.Title, a)
			}
		}
	}
}

func TestMenuCardEntryFlags(t *testing.T) {
	e := menuCardEntry("x", nil, image.Rect(0, 0, 1, 1), false, nil)
	if e.Modal || !e.Scrim || e.ID != "x" {
		t.Fatalf("menu entry: %+v", e)
	}
	if e := menuCardEntry("y", nil, image.Rectangle{}, true, nil); !e.Modal || !e.Scrim {
		t.Fatalf("must-answer entry: %+v", e)
	}
}

// The swatch picker is the reference: its own solveSwatchGrid bands must equal
// the card's, so adopting the card changed nothing on screen.
func TestMenuCardReproducesSwatchPickerFrame(t *testing.T) {
	c := NewColorWheelComponent()
	c.SetProps(ColorWheelProps{AnchorRect: image.Rect(300, 300, 340, 330), Bounds: image.Rect(0, 0, 1280, 720), RowHeight: 32})
	c.Open()
	if c.card == nil {
		t.Fatal("picker has no card after Open")
	}
	if c.card.TitleRect() != c.titleRect {
		t.Fatalf("title band: card %v vs picker %v", c.card.TitleRect(), c.titleRect)
	}
	if c.card.Body() != c.gridRect {
		t.Fatalf("body: card %v vs picker grid %v", c.card.Body(), c.gridRect)
	}
	if cb := c.CloseBtn(); cb != nil && cb.Rect() != closeButtonRect(c.titleRect, SpaceXS) {
		t.Fatalf("close ×: %v vs %v", cb.Rect(), closeButtonRect(c.titleRect, SpaceXS))
	}
	if c.card.Panel() != c.WheelRect() {
		t.Fatalf("panel: %v vs %v", c.card.Panel(), c.WheelRect())
	}
}
