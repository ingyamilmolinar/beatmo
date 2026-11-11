//go:build test

package ui

import (
	"image"
	"testing"
)

func TestSubdivMenu_ScrollsWhenOptionsOverflow(t *testing.T) {
	assertDefaultParityState(t)
	c := NewSubdivMenuComponent()
	c.SetScreenBounds(image.Rect(0, 0, 200, 120)) // short → overflow
	manyOpts := []int{1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 48, 64}
	c.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(10, 0, 60, 20),
		Current:    4,
		Options:    manyOpts,
		RowHeight:  24,
		OnSelect:   func(int) {},
		OnClose:    func() {},
	})
	c.Open()

	scroll := c.MenuScrollForTest()
	if scroll == nil {
		t.Fatal("subdiv MenuScroll is nil after Open")
	}
	if !scroll.HasScroll() {
		t.Fatalf("expected HasScroll()=true with %d options in a short card", len(manyOpts))
	}

	// Scroll down via the component's OWN wheel entry point (the production
	// path: HandleWheel → rebuildButtons), not the raw MenuScroll. The last
	// option's button must move up into the card.
	last := c.Buttons()[len(c.Buttons())-1].Rect()
	for i := 0; i < len(manyOpts); i++ {
		c.HandleWheel(0, 0, -1)
	}
	last2 := c.Buttons()[len(c.Buttons())-1].Rect()
	if last2.Min.Y >= last.Min.Y {
		t.Fatalf("last option did not move up after scroll: before=%d after=%d", last.Min.Y, last2.Min.Y)
	}
}
