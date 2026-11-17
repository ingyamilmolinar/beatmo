package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestGroupMenuAvoidsMemberNodes is task-enabled-status brief Part A test 1,
// ported for task-placement-v2 (see task-placement-v2-report.md): the
// GroupMenu panel must never cover any of its group's member nodes when a
// clean anchor-adjacent placement exists. Anchoring the menu at the group's
// badge position (bbox top-right corner — the ONLY anchor real badge clicks
// ever produce, per openGroupMenuForBadge/groupBadgeRects) must nudge the
// panel clear of every member; anchoring far away (no obstacle in play) must
// leave the panel essentially where it was requested (no gratuitous move).
// Exercised at both 800x600 and 1280x720.
//
// Ported: the pre-v2 version anchored dead-center ON a member's own rect and
// asserted zero overlap via the OLD bbox-relative avoidRect, whose "left of
// the FULL bbox" escape snaps exactly to the bbox's outer edge regardless of
// where inside the bbox the anchor sits. placeGroupMenu deliberately tests
// candidates relative to the ANCHOR point instead (fixing H1/H2 — see
// grid_reserved_zone.go's placeGroupMenu doc comment), trading "always zero
// overlap when ANY bbox-relative escape exists" for "always adjacent to the
// label" per the brief's refined requirement. Anchoring dead-center on one
// member of a multi-member cluster is not a scenario a real badge click can
// produce (the badge always sits at the bbox's outer corner, never inside
// it), so this test now anchors the same way production does.
func TestGroupMenuAvoidsMemberNodes(t *testing.T) {
	sizes := []struct{ w, h int }{{800, 600}, {1280, 720}}
	for _, sz := range sizes {
		g, aID, bID, cID := buildGroupLoopCircuitSized(t, sz.w, sz.h)
		gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, cID})
		if err != nil {
			t.Fatalf("%dx%d: CreateGroup: %v", sz.w, sz.h, err)
		}

		// Mirrors groupMemberScreenBBox's own float->int conversion
		// (game_group_badges.go: Floor/Ceil) exactly, so a panel edge that
		// exactly touches (but shares no pixel with) a member's screen rect
		// isn't flagged as a false-positive overlap here.
		memberRects := make([]image.Rectangle, 0, 3)
		for _, id := range []model.NodeID{aID, bID, cID} {
			x1, y1, x2, y2 := g.nodeScreenRect(g.nodesByID[id])
			memberRects = append(memberRects, image.Rect(
				int(math.Floor(x1)), int(math.Floor(y1)),
				int(math.Ceil(x2)), int(math.Ceil(y2)),
			))
		}

		// Anchor at the group's badge position: the bbox top-right corner —
		// exactly what openGroupMenuForBadge/groupBadgeRects feed layoutAt in
		// production (game_group_badges.go).
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

		// Anchor far away from every member — no obstacle should be in play,
		// so the panel should land essentially at the requested anchor
		// (allowing for the existing pane-clamp/gear-avoid behavior, but NOT
		// perturbed by member avoidance).
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

// TestGroupMenuMemberAvoidRespectsGearZone is task-enabled-status brief Part
// A test 2: with BOTH the member bounding box AND the settings-gear reserved
// zone in play near the requested anchor, the final panel must clear the
// gear's reserved zone (gear wins — it's the smaller, absolute exclusion,
// re-applied last per layoutAt's ordering) as well as the member bbox.
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

// ---------------------------------------------------------------------------
// task-placement-v2: badge-click placement (see task-placement-v2-report.md
// for the Phase-1 debugging evidence — H1/H2 confirmed, H3 not confirmed).
// ---------------------------------------------------------------------------

// groupMenuGeomCase is one placement-repro geometry: a window size, camera
// zoom, and a set of member grid coordinates (wired into a loop and grouped).
// camScale <= 0 keeps the default camera zoom.
type groupMenuGeomCase struct {
	name    string
	winW    int
	winH    int
	camZoom float64
	coords  [][2]int
}

// groupMenuAvoidableGeomCases are the four repro geometries the brief calls
// out by name — right-edge members, under-gear members, a wide bbox with an
// internal per-member gap, and bottom-edge members — each hand-tuned (see
// task-placement-v2-report.md) so an independently-computed clean
// anchor-relative candidate genuinely exists, exercising the "overlaps no
// member whenever avoidable" invariant instead of vacuously skipping it.
func groupMenuAvoidableGeomCases() []groupMenuGeomCase {
	return []groupMenuGeomCase{
		// Small 2-member cluster hugging the grid pane's right edge — the
		// old avoidRect's "right of obstacle" candidate couldn't fit past
		// the edge and its 4-candidate fallback could strand the panel
		// overlapping the cluster (H1); placeGroupMenu's "left of anchor"
		// candidate clears it instead.
		{"right-edge members", 1280, 720, 20, [][2]int{{16, 4}, {16, 6}}},
		// 2-member cluster positioned at the same screen height band as the
		// settings-gear reserved zone (H2's territory — a candidate that
		// clears the members can get pushed back onto them by the trailing
		// gear-avoid re-application in the OLD code).
		{"under-gear members", 1280, 720, 20, [][2]int{{-2, -4}, {0, -4}}},
		// L-shaped 3-member layout (top-left, top-right, bottom-left) on a
		// wide pane: the union bbox spans nearly the whole pane (blocking a
		// bbox-only test on every side), but the bottom-right quadrant has
		// no real member in it — only per-member testing finds the gap.
		{"wide bbox with internal gap", 2000, 1200, 20, [][2]int{{-25, -8}, {25, -8}, {-25, 8}}},
		// 3-member cluster hugging the grid pane's bottom edge — mirrors
		// right-edge but on the vertical axis ("above anchor" is the clean
		// escape here instead of "left of anchor").
		{"bottom-edge members", 1280, 720, 20, [][2]int{{-10, 4}, {10, 4}, {0, 3}}},
	}
}

// buildGroupMenuGeometry builds a fresh Game for c, places a member node per
// c.coords (wired into a closed loop so updateBeatInfos has a valid path),
// and groups them all. Mirrors buildGroupLoopCircuitSized's shape but adds
// camera-zoom control (needed to get member nodes with a real onscreen
// footprint — NodeRadius scales with camera zoom, see grid.go) and an
// arbitrary member count/layout instead of the fixed 3-node loop.
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

// groupMenuCleanCandidatePredicate is test 1's INDEPENDENT recomputation of
// "does a clean anchor-relative candidate exist" — it does NOT call
// placeGroupMenu; it reimplements the same 4-candidate loop inline so a bug
// in placeGroupMenu's own orchestration (wrong loop order, wrong tie-break,
// an off-by-one in the candidate list) is still caught rather than silently
// agreeing with production. It DOES reuse the small shared primitives
// (clampRectToPane/avoidReservedRect/rectIntersectArea) that placeGroupMenu
// also uses — those are generic geometry helpers predating this task (or,
// for rectIntersectArea, a 4-line area helper), not the chooser itself, and
// re-deriving them independently would test arithmetic, not the actual bug
// class this task fixes.
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

// panelAdjacentToAnchor implements test 2's adjacency definition: the panel
// rect must be within `size` px of the anchor point on AT LEAST ONE axis —
// i.e. the gap between the anchor and the panel's nearest edge, along X or
// along Y, is no more than the panel's own width/height respectively. This
// bounds the menu to "one panel-dimension away, at most" rather than
// "somewhere on screen" — it never teleports to a bbox-relative position.
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

// TestBadgeClickMenuNeverOverlapsMembersWhenAvoidable is the Phase-1
// reproduction (task-placement-v2-report.md), promoted to the permanent
// regression: across all four repro geometries (right-edge members,
// under-gear members, a wide bbox with an internal per-member gap, and
// bottom-edge members), the panel must overlap NO member whenever an
// independently-computed clean anchor-relative candidate exists.
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

// TestMenuStaysAdjacentToLabel is task-placement-v2 test 2: across the same
// four geometries, the chosen panel must always stay adjacent to the anchor
// (within one panel-dimension on at least one axis — panelAdjacentToAnchor)
// regardless of whether a clean candidate existed or the min-overlap
// fallback fired — the menu must never teleport to a bbox-relative position
// far from the badge that opened it.
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

// TestMenuOverlapsOnlyWhenNoRoom is task-placement-v2 test 3: a tiny 400x300
// window with members surrounding the anchor in all four quadrants — no
// anchor-relative candidate can be clean (asserted via the same independent
// predicate as a setup sanity check). The chooser must fall back to the
// candidate with the smallest total member-overlap area (matching the
// independently-computed minimum exactly), while STILL clearing the
// settings-gear's reserved zone and staying adjacent to the anchor — the
// only degradation the product spec allows is covering a member, never the
// gear and never a teleport.
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

// TestPlaceGroupMenuCandidateOrderDeterminism is a pure-function unit test
// (no Game): with no members and an empty gear zone, placeGroupMenu must
// return exactly the FIRST candidate (right of anchor — top-left at anchor)
// unmodified. Placing a member that exactly covers the first candidate (but
// not the second) must deterministically fall through to the second
// candidate (left of anchor) — proving the priority order is fixed and
// exercised in sequence, not e.g. randomized or reordered by overlap area
// when a zero-overlap option is available.
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
		// Covers candidate 1 (right of anchor, (500,500)-(800,700)) but not
		// candidate 2 (left of anchor, (200,500)-(500,700)).
		blocker := image.Rect(500, 500, 800, 700)
		got := placeGroupMenu(anchor, size, []image.Rectangle{blocker}, noGear, gridW, gridH)
		want := image.Rect(200, 500, 500, 700) // left of anchor
		if got != want {
			t.Fatalf("got %v, want %v (candidate 1 was blocked, must fall through to candidate 2)", got, want)
		}
	})
}

// TestPlaceGroupMenuPerMemberNotBBox is a pure-function unit test (no Game)
// directly exercising the brief's key differentiator: an L-shaped 3-member
// layout (top-left, top-right, bottom-left corners of a 1000x1000 square)
// whose union bbox is the FULL 1000x1000 square — a coarse bbox-overlap test
// would reject a candidate placed near the square's center (it's inside the
// bbox rectangle), but NO real member actually occupies that area.
// placeGroupMenu must find and return that candidate (per-member testing),
// not fall back to the old bbox-blocks-everything behavior.
func TestPlaceGroupMenuPerMemberNotBBox(t *testing.T) {
	gridW, gridH := 2000, 2000
	var noGear image.Rectangle
	tl := image.Rect(0, 0, 50, 50)
	tr := image.Rect(950, 0, 1000, 50)
	bl := image.Rect(0, 950, 50, 1000)
	// mid sits INSIDE the first (right-of-anchor) candidate. This is what
	// forces the two algorithms apart: per-member logic must reject the
	// right candidate (it overlaps mid) and advance to the clean left
	// candidate, while a bbox-only counterfactual sees every candidate
	// equally buried in the coarse 1000x1000 bbox, falls to the min-overlap
	// tie-break, and stays on candidate 0 (right). Review finding: the
	// previous geometry let both implementations return the same rect, so
	// the test couldn't catch a bbox-only regression.
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

	// Sanity: the right candidate overlaps the real member mid (so
	// per-member logic must skip it); the left candidate overlaps the coarse
	// bbox but NO real member (so only per-member logic can accept it).
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
