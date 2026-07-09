//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildTwoGroupsCircuit builds two spatially-separated, committed 2-node
// groups: "Alpha" over buildGroupLoopCircuit's a/b pair (grid (0,0)/(1,0)),
// and "Beta" over a fresh c/d pair several rows below (grid (0,5)/(1,5)) so
// their member bounding boxes — and therefore their badges — don't naturally
// collide, unless a test deliberately wants them to (TestGroupBadgesDoNotOverlap
// builds its own tighter pair instead).
func buildTwoGroupsCircuit(t *testing.T) (g *Game, aID, bID, cID, dID model.NodeID, gid1, gid2 model.GroupID) {
	t.Helper()
	g, aID, bID = buildGroupLoopCircuit(t)
	c := g.tryAddNode(0, 5, model.NodeTypeRegular)
	d := g.tryAddNode(1, 5, model.NodeTypeRegular)
	g.addEdge(c, d)
	g.updateBeatInfos()

	var err error
	gid1, err = g.graph.CreateGroup("Alpha", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup Alpha: %v", err)
	}
	gid2, err = g.graph.CreateGroup("Beta", []model.NodeID{c.ID, d.ID})
	if err != nil {
		t.Fatalf("CreateGroup Beta: %v", err)
	}
	return g, aID, bID, c.ID, d.ID, gid1, gid2
}

// badgeRectFor finds the computed badge rect for gid, failing the test if
// absent.
func badgeRectFor(t *testing.T, g *Game, gid model.GroupID) image.Rectangle {
	t.Helper()
	for _, b := range g.groupBadgeRects() {
		if b.gid == gid {
			return b.rect
		}
	}
	t.Fatalf("no badge rect computed for group %v", gid)
	return image.Rectangle{}
}

// TestGroupBadgeClickOpensMenu is task-group-badges brief test 4 (desktop
// path): with 2 committed groups, a real gridClick on the second group's
// badge rect center must open ITS menu (non-provisional), not touch the
// first group, and close the sidebar if it was open.
func TestGroupBadgeClickOpensMenu(t *testing.T) {
	g, aID, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)
	// Open the sidebar first so we can confirm the badge click closes it.
	g.sidebar.Open(g.nodesByID[aID])
	if !g.sidebar.IsOpen() {
		t.Fatalf("setup: sidebar must be open")
	}

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	gridClick(g, cx, cy)

	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid2 {
		t.Fatalf("badge click must open the menu for gid=%v, got open=%v id=%v", gid2, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if g.groupMenu.provisional {
		t.Fatalf("badge-opened menu must be non-provisional")
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("badge click must close an already-open sidebar")
	}
}

// TestGroupBadgeTapOpensMenu_Touch is task-group-badges brief test 5: the
// touch path (handleTapInGrid, driven directly the way existing touch tests
// do) must resolve a tap on a group's badge to opening that group's menu,
// mirroring the desktop click path. (The sidebar-open-closes-on-outside-tap
// behavior is pre-existing and orthogonal — handleTapInGrid's badge check
// sits after that guard, matching the brief's "top of the node/empty
// resolution" placement, so this test starts with the sidebar closed.)
func TestGroupBadgeTapOpensMenu_Touch(t *testing.T) {
	g, _, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	g.handleTapInGrid(cx, cy)

	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid2 {
		t.Fatalf("badge tap must open the menu for gid=%v, got open=%v id=%v", gid2, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if g.groupMenu.provisional {
		t.Fatalf("badge-opened menu must be non-provisional")
	}
}

// TestGroupBadgesDoNotOverlap is task-group-badges brief test 6: two groups
// whose member bounding boxes sit close enough together that their naively-
// anchored badge rects would collide must resolve to pairwise non-
// intersecting rects (the "push down below the collider" rule).
func TestGroupBadgesDoNotOverlap(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()

	// Two adjacent 2-node groups, one row apart — close enough that their
	// top-right-anchored badges would naturally collide before resolution.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(0, 1, model.NodeTypeRegular)
	d := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(c, d)
	g.updateBeatInfos()

	if _, err := g.graph.CreateGroup("Alpha", []model.NodeID{a.ID, b.ID}); err != nil {
		t.Fatalf("CreateGroup Alpha: %v", err)
	}
	if _, err := g.graph.CreateGroup("Beta", []model.NodeID{c.ID, d.ID}); err != nil {
		t.Fatalf("CreateGroup Beta: %v", err)
	}

	rects := g.groupBadgeRects()
	if len(rects) != 2 {
		t.Fatalf("want 2 badge rects, got %d", len(rects))
	}
	if rects[0].rect.Overlaps(rects[1].rect) {
		t.Fatalf("badge rects must never overlap, got %v and %v", rects[0].rect, rects[1].rect)
	}
}

// TestGroupColorsDistinctAndConsistent is task-group-badges brief test 7:
// groupColor cycles distinctly across consecutive GroupIDs, is deterministic
// (same gid -> same color across calls), and the ring-color helper
// (groupRingColor) — which is what the node-layer draw path actually calls —
// derives its RGB channel from the SAME groupColor value, only varying alpha
// by whether the GroupMenu is open for that gid.
func TestGroupColorsDistinctAndConsistent(t *testing.T) {
	g, aID, bID, _, _, gid1, _ := buildTwoGroupsCircuit(t)

	c1 := g.groupColor(gid1)
	c2 := g.groupColor(gid1 + 1)
	if c1 == c2 {
		t.Fatalf("groupColor for consecutive gids must differ, both = %v", c1)
	}
	if again := g.groupColor(gid1); again != c1 {
		t.Fatalf("groupColor must be deterministic, got %v then %v", c1, again)
	}

	// Faint (no menu open for gid1) vs strong (menu open for gid1), both
	// reading the SAME base RGB as groupColor(gid1).
	faint := g.groupRingColor(gid1)
	if faint.A != AlphaSubtle {
		t.Fatalf("ring color with no menu open must use AlphaSubtle, got %d", faint.A)
	}
	if faint.R != c1.R || faint.G != c1.G || faint.B != c1.B {
		t.Fatalf("ring RGB must match groupColor RGB, ring=%v groupColor=%v", faint, c1)
	}

	g.groupMenu.OpenAt(gid1, 10, 10)
	strong := g.groupRingColor(gid1)
	if strong.A != AlphaStrong {
		t.Fatalf("ring color for the OPEN menu's group must use AlphaStrong, got %d", strong.A)
	}
	if strong.R != c1.R || strong.G != c1.G || strong.B != c1.B {
		t.Fatalf("ring RGB must match groupColor RGB even at AlphaStrong, ring=%v groupColor=%v", strong, c1)
	}
	_ = aID
	_ = bID
}

// TestBadgeShiftPressDoesNotArmMarquee is task-group-badges brief test 8: a
// Shift+press landing on a group badge must behave exactly like a plain
// badge click (open that group's menu) and must NOT arm the marquee gesture
// or create any group.
func TestBadgeShiftPressDoesNotArmMarquee(t *testing.T) {
	g, aID, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)
	groupsBefore := len(g.graph.AllGroups())
	g.sidebar.Open(g.nodesByID[aID])

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2

	// shiftDrag with identical start/end coordinates models a Shift+press
	// (and release) with no movement, i.e. a "Shift+click" gesture.
	shiftDrag(g, cx, cy, cx, cy)

	if g.marquee.active {
		t.Fatalf("marquee must not be armed by a shift+press on a badge")
	}
	if got := len(g.graph.AllGroups()); got != groupsBefore {
		t.Fatalf("shift+press on a badge must not create a group, want %d got %d", groupsBefore, got)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid2 {
		t.Fatalf("badge click behavior must win over the marquee arm: want menu open for gid=%v, got open=%v id=%v",
			gid2, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("badge click must close the sidebar even via the shift-branch path")
	}
}
