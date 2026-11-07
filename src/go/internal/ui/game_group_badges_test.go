//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Several rows apart so their badges never collide unless a test wants them to.
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

// Drives handleEditor directly: while the sidebar is open, the tree's click-outside closes it before the
// grid's release-time badge check, a separate interaction this test does not assert.
func desktopClickViaEditor(g *Game, x, y int) {
	down := true
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	g.handleEditor()
	down = false
	g.handleEditor()
}

// The second group's badge opens ITS menu, leaves the first group alone and closes the sidebar.
func TestGroupBadgeClickOpensMenu(t *testing.T) {
	g, aID, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)
	// Open the sidebar first so we can confirm the badge click closes it.
	g.sidebar.Open(g.nodesByID[aID])
	if !g.sidebar.IsOpen() {
		t.Fatalf("setup: sidebar must be open")
	}

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2
	desktopClickViaEditor(g, cx, cy)

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

// The touch path mirrors the desktop click. Starts with the sidebar closed: its outside-tap close is orthogonal.
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

// Naively anchored badges would collide; the push-down rule must separate them.
func TestGroupBadgesDoNotOverlap(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.updateBeatInfos()

	// One row apart, close enough to collide before resolution.
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

// Distinct across consecutive ids and deterministic; groupRingColor shares its RGB and varies only alpha.
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

// Behaves like a plain badge click: no marquee armed, no group created.
func TestBadgeShiftPressDoesNotArmMarquee(t *testing.T) {
	g, aID, _, _, _, _, gid2 := buildTwoGroupsCircuit(t)
	groupsBefore := len(g.graph.AllGroups())
	g.sidebar.Open(g.nodesByID[aID])

	rect := badgeRectFor(t, g, gid2)
	cx, cy := (rect.Min.X+rect.Max.X)/2, (rect.Min.Y+rect.Max.Y)/2

	// Identical start and end: a Shift+click.
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
