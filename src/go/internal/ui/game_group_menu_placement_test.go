package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Anchored at the badge (bbox top-right, the only anchor a real click produces), the panel must clear every
// member when a clean placement exists; anchored far away it must not move.
func TestGroupMenuAvoidsMemberNodes(t *testing.T) {
	sizes := []struct{ w, h int }{{800, 600}, {1280, 720}}
	for _, sz := range sizes {
		g, aID, bID, cID := buildGroupLoopCircuitSized(t, sz.w, sz.h)
		gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, cID})
		if err != nil {
			t.Fatalf("%dx%d: CreateGroup: %v", sz.w, sz.h, err)
		}

		// The same Floor/Ceil as groupMemberScreenBBox, so an edge that only touches is not an overlap.
		memberRects := make([]image.Rectangle, 0, 3)
		for _, id := range []model.NodeID{aID, bID, cID} {
			x1, y1, x2, y2 := g.nodeScreenRect(g.nodesByID[id])
			memberRects = append(memberRects, image.Rect(
				int(math.Floor(x1)), int(math.Floor(y1)),
				int(math.Ceil(x2)), int(math.Ceil(y2)),
			))
		}

		// Exactly what a badge click feeds layoutAt.
		bbox, ok := g.groupMemberScreenBBox([]model.NodeID{aID, bID, cID})
		if !ok {
			t.Fatalf("%dx%d: groupMemberScreenBBox: no members resolved", sz.w, sz.h)
		}
		acx, acy := bbox.Max.X, bbox.Min.Y
		g.groupMenu.OpenAtProvisional(gid, acx, acy)
		advanceFrames(g, 1)

		panel := g.groupMenu.panel
		for i, mr := range memberRects {
			if panel.Overlaps(mr) {
				t.Fatalf("%dx%d: panel %v overlaps member[%d] rect %v (anchored at badge position)", sz.w, sz.h, panel, i, mr)
			}
		}

		// With no obstacle, the panel lands at the anchor (pane clamp and gear avoidance aside).
		gridRect := g.split.GridRect(g.winW, g.winH)
		farX, farY := gridRect.Dx()-5, gridRect.Dy()-5
		g.groupMenu.OpenAtProvisional(gid, farX, farY)
		advanceFrames(g, 1)
		farPanel := g.groupMenu.panel
		for i, mr := range memberRects {
			if farPanel.Overlaps(mr) {
				t.Fatalf("%dx%d: panel %v overlaps member[%d] rect %v (anchored far away)", sz.w, sz.h, farPanel, i, mr)
			}
		}
	}
}

// The gear's reserved zone wins: the smaller, absolute exclusion, applied last.
func TestGroupMenuMemberAvoidRespectsGearZone(t *testing.T) {
	g, aID, bID, cID := buildGroupLoopCircuitSized(t, 800, 600)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, cID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	reserved := g.gridHelpReservedRect()
	if reserved.Empty() {
		t.Skip("no gear reserved rect in this fixture")
	}
	rcx, rcy := (reserved.Min.X+reserved.Max.X)/2, (reserved.Min.Y+reserved.Max.Y)/2

	g.groupMenu.OpenAtProvisional(gid, rcx, rcy)
	advanceFrames(g, 1)

	panel := g.groupMenu.panel
	if panel.Overlaps(reserved) {
		t.Fatalf("panel %v must not overlap the gear's reserved zone %v even after member avoidance", panel, reserved)
	}
}

// camScale <= 0 keeps the default zoom.
type groupMenuGeomCase struct {
	name    string
	winW    int
	winH    int
	camZoom float64
	coords  [][2]int
}

// Each tuned so a clean anchor-relative candidate genuinely exists, so the invariant is never skipped vacuously.
func groupMenuAvoidableGeomCases() []groupMenuGeomCase {
	return []groupMenuGeomCase{
		// At the right edge "right of" cannot fit; "left of anchor" clears it.
		{"right-edge members", 1280, 720, 20, [][2]int{{16, 4}, {16, 6}}},
		// In the gear's height band, where gear avoidance could push a clear panel back onto the members.
		{"under-gear members", 1280, 720, 20, [][2]int{{-2, -4}, {0, -4}}},
		// An L shape whose union bbox blocks every side; only per-member testing finds the empty quadrant.
		{"wide bbox with internal gap", 2000, 1200, 20, [][2]int{{-25, -8}, {25, -8}, {-25, 8}}},
		// The vertical twin of right-edge: "above anchor" is the clean escape.
		{"bottom-edge members", 1280, 720, 20, [][2]int{{-10, 4}, {10, 4}, {0, 3}}},
	}
}

// Camera zoom control, because NodeRadius scales with zoom and members need a real footprint.
func buildGroupMenuGeometry(t *testing.T, c groupMenuGeomCase) (*Game, model.GroupID, []model.NodeID) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(c.winW, c.winH)
	if c.camZoom > 0 {
		g.cam.Scale = c.camZoom
	}
	g.updateBeatInfos()

	ids := make([]model.NodeID, 0, len(c.coords))
	nodes := make([]*uiNode, 0, len(c.coords))
	for _, xy := range c.coords {
		n := g.tryAddNode(xy[0], xy[1], model.NodeTypeRegular)
		ids = append(ids, n.ID)
		nodes = append(nodes, n)
	}
	for i := range nodes {
		g.addEdge(nodes[i], nodes[(i+1)%len(nodes)])
	}
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.updateBeatInfos()

	gid, err := g.graph.CreateGroup("", ids)
	if err != nil {
		t.Fatalf("%s: CreateGroup: %v", c.name, err)
	}
	return g, gid, ids
}

// An INDEPENDENT recomputation, never calling placeGroupMenu, so a bug in its orchestration cannot agree with
// itself. It reuses only the generic geometry primitives.
func groupMenuCleanCandidatePredicate(g *Game, ids []model.NodeID) (anchor image.Point, size image.Point, anyClean bool, minOverlap int, members []image.Rectangle) {
	gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
	reserved := g.gridHelpReservedRect()
	rowH, gap := computeGroupMenuRowSizing(gridH, 0, 0)
	halfH := rowH / 2
	h := sidebarPad*2 + sidebarHeaderH + gap + groupMenuHalfRows*(halfH+gap) + groupMenuFullRows*(rowH+gap)
	w := groupMenuW
	if w > gridW {
		w = gridW
	}
	size = image.Pt(w, h)
	members = g.groupMemberScreenRects(ids, nil)
	bbox, _ := g.groupMemberScreenBBox(ids)
	anchor = image.Pt(bbox.Max.X, bbox.Min.Y) // badge position — see groupBadgeRects

	ax, ay := anchor.X, anchor.Y
	raw := [4]image.Rectangle{
		image.Rect(ax, ay, ax+w, ay+h),
		image.Rect(ax-w, ay, ax, ay+h),
		image.Rect(ax-w/2, ay, ax+w/2, ay+h),
		image.Rect(ax-w/2, ay-h, ax+w/2, ay),
	}
	minOverlap = -1
	for _, c := range raw {
		c = clampRectToPane(c, gridW, gridH)
		c = avoidReservedRect(c, reserved, gridW, gridH)
		overlap := 0
		for _, m := range members {
			overlap += rectIntersectArea(c, m)
		}
		if overlap == 0 {
			anyClean = true
		}
		if minOverlap == -1 || overlap < minOverlap {
			minOverlap = overlap
		}
	}
	return
}

// Within one panel dimension of the anchor on at least one axis: adjacent, never teleported.
func panelAdjacentToAnchor(panel image.Rectangle, anchor image.Point, size image.Point) bool {
	axisDist := func(p, lo, hi int) int {
		if p >= lo && p <= hi {
			return 0
		}
		if p < lo {
			return lo - p
		}
		return p - hi
	}
	xDist := axisDist(anchor.X, panel.Min.X, panel.Max.X)
	yDist := axisDist(anchor.Y, panel.Min.Y, panel.Max.Y)
	return xDist <= size.X || yDist <= size.Y
}

// No member is overlapped whenever a clean candidate exists.
func TestBadgeClickMenuNeverOverlapsMembersWhenAvoidable(t *testing.T) {
	for _, c := range groupMenuAvoidableGeomCases() {
		g, gid, ids := buildGroupMenuGeometry(t, c)
		anchor, _, anyClean, _, members := groupMenuCleanCandidatePredicate(g, ids)
		if !anyClean {
			t.Fatalf("%s: test setup: no clean anchor-relative candidate exists for this geometry (predicate says anyClean=false) — geometry must be re-tuned, this test doesn't exercise the invariant otherwise", c.name)
		}

		g.groupMenu.OpenAtProvisional(gid, anchor.X, anchor.Y)
		advanceFrames(g, 1)
		panel := g.groupMenu.panel

		for i, m := range members {
			if panel.Overlaps(m) {
				t.Errorf("%s: panel %v overlaps member[%d] %v even though a clean anchor-relative candidate exists (anchor=%v)", c.name, panel, i, m, anchor)
			}
		}
	}
}

// Adjacent whether or not a clean candidate existed: never teleported far from the badge.
func TestMenuStaysAdjacentToLabel(t *testing.T) {
	for _, c := range groupMenuAvoidableGeomCases() {
		g, gid, ids := buildGroupMenuGeometry(t, c)
		anchor, size, _, _, _ := groupMenuCleanCandidatePredicate(g, ids)

		g.groupMenu.OpenAtProvisional(gid, anchor.X, anchor.Y)
		advanceFrames(g, 1)
		panel := g.groupMenu.panel

		if !panelAdjacentToAnchor(panel, anchor, size) {
			t.Errorf("%s: panel %v is not adjacent to anchor %v (size=%v) — menu teleported away from the label", c.name, panel, anchor, size)
		}
	}
}

// No clean candidate exists: the minimum total overlap wins, while still clearing the gear and staying adjacent.
// Covering a member is the only allowed degradation.
func TestMenuOverlapsOnlyWhenNoRoom(t *testing.T) {
	c := groupMenuGeomCase{
		name: "surrounded, tiny pane", winW: 400, winH: 300, camZoom: 20,
		coords: [][2]int{{-3, -2}, {3, -2}, {-3, 2}, {3, 2}},
	}
	g, gid, ids := buildGroupMenuGeometry(t, c)
	anchor, size, anyClean, minOverlap, members := groupMenuCleanCandidatePredicate(g, ids)
	if anyClean {
		t.Fatalf("test setup: expected NO clean candidate for this surrounded/tiny geometry, got anyClean=true — geometry must be re-tuned to actually exercise the no-room fallback")
	}
	reserved := g.gridHelpReservedRect()

	g.groupMenu.OpenAtProvisional(gid, anchor.X, anchor.Y)
	advanceFrames(g, 1)
	panel := g.groupMenu.panel

	if panel.Overlaps(reserved) {
		t.Errorf("panel %v overlaps the gear's reserved zone %v even in the no-room fallback — gear zone must stay absolute", panel, reserved)
	}
	if !panelAdjacentToAnchor(panel, anchor, size) {
		t.Errorf("panel %v is not adjacent to anchor %v (size=%v) in the no-room fallback", panel, anchor, size)
	}
	overlap := 0
	for _, m := range members {
		overlap += rectIntersectArea(panel, m)
	}
	if overlap != minOverlap {
		t.Errorf("fallback panel overlap area=%d, want the independently-computed minimum=%d (panel=%v)", overlap, minOverlap, panel)
	}
}

// The priority order is fixed: the first candidate when free, the second when a member covers exactly the first.
func TestPlaceGroupMenuCandidateOrderDeterminism(t *testing.T) {
	gridW, gridH := 2000, 2000
	anchor := image.Pt(500, 500)
	size := image.Pt(300, 200)
	var noGear image.Rectangle

	t.Run("no obstacles: first candidate wins untouched", func(t *testing.T) {
		got := placeGroupMenu(anchor, size, nil, noGear, gridW, gridH)
		want := image.Rect(500, 500, 800, 700) // right of anchor
		if got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("first candidate blocked: falls through to second in priority order", func(t *testing.T) {
		blocker := image.Rect(500, 500, 800, 700)
		got := placeGroupMenu(anchor, size, []image.Rectangle{blocker}, noGear, gridW, gridH)
		want := image.Rect(200, 500, 500, 700) // left of anchor
		if got != want {
			t.Fatalf("got %v, want %v (candidate 1 was blocked, must fall through to candidate 2)", got, want)
		}
	})
}

// The union bbox is the whole square, so a bbox test rejects a centre candidate no member occupies;
// per-member testing must accept it.
func TestPlaceGroupMenuPerMemberNotBBox(t *testing.T) {
	gridW, gridH := 2000, 2000
	var noGear image.Rectangle
	tl := image.Rect(0, 0, 50, 50)
	tr := image.Rect(950, 0, 1000, 50)
	bl := image.Rect(0, 950, 50, 1000)
	// mid inside the first candidate forces the algorithms apart: per-member advances to the clean left candidate,
	// while bbox-only stays on candidate 0.
	mid := image.Rect(600, 550, 650, 600)
	members := []image.Rectangle{tl, tr, bl, mid}
	bbox := tl.Union(tr).Union(bl).Union(mid)
	if bbox != image.Rect(0, 0, 1000, 1000) {
		t.Fatalf("test setup: expected the union bbox to be the full 1000x1000 square, got %v", bbox)
	}

	anchor := image.Pt(500, 500) // bbox center — deep inside the coarse bbox
	size := image.Pt(300, 200)
	right := image.Rect(500, 500, 800, 700) // candidate 0 (right of anchor)
	left := image.Rect(200, 500, 500, 700)  // candidate 1 (left of anchor)

	// The right candidate overlaps a real member; the left only the coarse bbox.
	if !right.Overlaps(mid) {
		t.Fatalf("test setup: right candidate %v must overlap mid %v", right, mid)
	}
	if !left.Overlaps(bbox) {
		t.Fatalf("test setup: left candidate %v must overlap the coarse bbox %v to be a meaningful differentiator", left, bbox)
	}
	for i, m := range members {
		if left.Overlaps(m) {
			t.Fatalf("test setup: left candidate %v must NOT overlap any real member, but overlaps member[%d] %v", left, i, m)
		}
	}

	got := placeGroupMenu(anchor, size, members, noGear, gridW, gridH)
	if got != left {
		t.Fatalf("got %v, want %v (left is the first candidate clear of every real member; bbox-only logic would have stayed on the right candidate via the min-overlap tie-break)", got, left)
	}
}
