//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// stubCursorAt stubs the cursor position (no buttons/keys) for hover-only
// checks — lighter than gridClick/shiftDrag, which also drive press state.
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

// TestBadgeHoverComputesHighlight is task-enabled-status brief Part B test 1
// (RED): with the cursor stubbed directly over group 2's badge rect,
// hoveredGroupBadge() must resolve to gid2; moved off every badge, it must
// resolve to none.
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

// TestGroupExternalHighlightGidPrefersOpenMenu is task-enabled-status brief
// Part B test 2 (structural): groupExternalHighlightGid resolves the OPEN
// GroupMenu's group over a merely-hovered badge (matches groupRingColor's
// existing "open menu draws strong" precedent), and hoveredGroupBadge alone
// resolves when no menu is open. Hoisted once per frame by drawGridNodes (see
// its call site), not per node — this test exercises the resolver directly
// rather than duplicating a pixel test.
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

	// Open gid1's menu (different group than the hovered badge): the open
	// menu's group must win over the hover.
	g.groupMenu.OpenAt(gid1, 10, 10)
	gid, ok = g.groupExternalHighlightGid()
	if !ok || gid != gid1 {
		t.Fatalf("menu open for gid1: groupExternalHighlightGid() = (%v,%v), want (%v,true)", gid, ok, gid1)
	}
}

// TestGroupBadgeBrightensFillOnHover pins the cheap hover affordance: the
// badge fill color for the hovered group's badge is a brighter alpha bucket
// than the un-hovered fill (AlphaStrong vs AlphaMedium), never a new color
// literal — same groupColor RGB, only the alpha bucket differs.
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
