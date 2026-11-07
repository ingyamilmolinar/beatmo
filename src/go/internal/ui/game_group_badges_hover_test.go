//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Lighter than gridClick/shiftDrag, which also drive press state.
func stubCursorAt(x, y int) func() {
	return SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
}

func TestBadgeHoverComputesHighlight(t *testing.T) {
	g, _, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	restore := stubCursorAt(cx, cy)
	defer restore()

	gid, ok := g.hoveredGroupBadge()
	if !ok || gid != gid2 {
		t.Fatalf("hoveredGroupBadge() over gid2's badge = (%v,%v), want (%v,true)", gid, ok, gid2)
	}

	restore()
	restore = stubCursorAt(-1000, -1000) // far off any badge
	defer restore()
	if _, ok := g.hoveredGroupBadge(); ok {
		t.Fatalf("hoveredGroupBadge() off every badge must return ok=false")
	}
}

// The OPEN menu's group beats a merely hovered badge, as groupRingColor's "open menu draws strong".
func TestGroupExternalHighlightGidPrefersOpenMenu(t *testing.T) {
	g, _, _, _, _, gid1, gid2 := buildTwoGroupsCircuit(t)

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	restore := stubCursorAt(cx, cy)
	defer restore()

	// No menu open: hovered badge's group wins.
	gid, ok := g.groupExternalHighlightGid()
	if !ok || gid != gid2 {
		t.Fatalf("no menu open: groupExternalHighlightGid() = (%v,%v), want (%v,true)", gid, ok, gid2)
	}

	g.groupMenu.OpenAt(gid1, 10, 10)
	gid, ok = g.groupExternalHighlightGid()
	if !ok || gid != gid1 {
		t.Fatalf("menu open for gid1: groupExternalHighlightGid() = (%v,%v), want (%v,true)", gid, ok, gid1)
	}
}

// A brighter alpha bucket of the same groupColor RGB, never a new colour literal.
func TestGroupBadgeBrightensFillOnHover(t *testing.T) {
	g, _, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	restore := stubCursorAt(cx, cy)
	defer restore()

	fill := g.groupBadgeFillColor(gid2)
	if fill.A != AlphaStrong {
		t.Fatalf("hovered badge fill alpha = %d, want AlphaStrong (%d)", fill.A, AlphaStrong)
	}

	restore()
	restore = stubCursorAt(-1000, -1000)
	defer restore()
	fillCold := g.groupBadgeFillColor(gid2)
	if fillCold.A != AlphaMedium {
		t.Fatalf("un-hovered badge fill alpha = %d, want AlphaMedium (%d)", fillCold.A, AlphaMedium)
	}
}
