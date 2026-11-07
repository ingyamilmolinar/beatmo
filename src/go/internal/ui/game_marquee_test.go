package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Shift held throughout; frames advance through the real Update loop.
func shiftDrag(g *Game, x1, y1, x2, y2 int) {
	pos := [2]int{x1, y1}
	down := true
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2) // press
	pos = [2]int{x2, y2}
	advanceFrames(g, 3) // drag
	down = false
	advanceFrames(g, 2) // release
}

func TestMarqueeCreatesGroupAndOpensMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Compute a screen rect that encloses both nodes with margin.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	t.Logf("node a rect start=(%v,%v) node b rect end=(%v,%v)", ax1, ay1, bx2, by2)
	shiftDrag(g, int(ax1)-20, int(ay1)-20, int(bx2)+20, int(by2)+20)

	groups := g.graph.AllGroups()
	if len(groups) != 1 {
		t.Fatalf("want 1 group after marquee, got %d", len(groups))
	}
	if len(groups[0].NodeIDs) != 2 {
		t.Fatalf("want both nodes in group, got %v", groups[0].NodeIDs)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != groups[0].ID {
		t.Fatalf("group menu must open for the new group")
	}
	// No node was created/selected by the release (marquee != click).
	if len(g.nodes) != 2 {
		t.Fatalf("marquee must not add nodes, have %d", len(g.nodes))
	}
}

func TestMarqueeOverEmptySpaceIsNoOp(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	// Far corner away from both nodes.
	shiftDrag(g, 600, 400, 700, 480)
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("empty marquee must create no group, got %d", n)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must not open on empty marquee")
	}
}

func TestShiftClickWithoutDragIsNoOp(t *testing.T) {
	g, _, _ := buildGroupLoopCircuit(t)
	shiftDrag(g, 600, 400, 601, 401) // < 4px movement
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("tiny shift-click must be a no-op, got %d groups", n)
	}
}

func TestShiftDragFromNodeStillCreatesEdge(t *testing.T) {
	// Regression guard: link-drag (shift+drag STARTING ON a node) untouched.
	g, aID, bID := buildGroupLoopCircuit(t)
	g.deleteEdge(g.nodesByID[aID], g.nodesByID[bID]) // remove a->b, re-create via gesture
	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	bx1, by1, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int((ax1+ax2)/2), int((ay1+ay2)/2), int((bx1+bx2)/2), int((by1+by2)/2))
	if _, ok := g.graph.Edges[[2]model.NodeID{aID, bID}]; !ok {
		t.Fatalf("link drag from node must still create the edge")
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("link drag must not create a group, got %d", n)
	}
}

func gridClick(g *Game, x, y int) {
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
	advanceFrames(g, 2) // press
	down = false
	advanceFrames(g, 2) // release
}

// A plain click on empty grid space closes an open GroupMenu, as it closes the sidebar.
func TestGridClickClosesOpenGroupMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: group menu must be open")
	}
	// The grid pane's top-right: (700,500) would land in the drum pane, and the nodes and the menu sit top-left.
	gridClick(g, 700, 50)
	if g.groupMenu.IsOpen() {
		t.Fatalf("plain grid click must close the open group menu")
	}
}

// A click ON a node closes the menu and does nothing else: the dismissing press owns its gesture, so the
// node is neither selected nor opened.
func TestNodeClickClosesOpenGroupMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: group menu must be open")
	}

	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	cx, cy := int((ax1+ax2)/2), int((ay1+ay2)/2)
	gridClick(g, cx, cy)

	if g.groupMenu.IsOpen() {
		t.Fatalf("plain click on a node must close the open group menu")
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("a click that dismisses the group menu must not also open the node sidebar")
	}
}

// The button is down for every point but the last, so a gesture can leave the grid pane before its release.
func shiftDragTo(g *Game, downAt bool, points ...[2]int) {
	if len(points) == 0 {
		return
	}
	pos := points[0]
	down := downAt
	restore := SetInputForTest(
		func() (int, int) { return pos[0], pos[1] },
		func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
	for _, p := range points[1:] {
		pos = p
		advanceFrames(g, 2)
	}
}

// A marquee released outside the grid pane must be cancelled: otherwise it stays active (dead camera pan)
// and re-entering the pane replays a stale release into a phantom group.
func TestMarqueeCancelledWhenReleasedOutsideGridPane(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)

	// Encloses both nodes, so a replayed release would actually create a group.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	startX, startY := int(ax1)-20, int(ay1)-20
	dragX, dragY := int(bx2)+20, int(by2)+20

	if !g.split.InGridPane(startX, startY) || !g.split.InGridPane(dragX, dragY) {
		t.Fatalf("setup: press/drag points must be inside the grid pane, got InGridPane(start)=%v InGridPane(drag)=%v",
			g.split.InGridPane(startX, startY), g.split.InGridPane(dragX, dragY))
	}
	outsideX, outsideY := 700, 550 // drum pane for an 800x600 default 50/50 split (split.Y=300)
	if g.split.InGridPane(outsideX, outsideY) {
		t.Fatalf("setup: (%d,%d) must be outside the grid pane", outsideX, outsideY)
	}

	shiftDragTo(g, true,
		[2]int{startX, startY},
		[2]int{dragX, dragY},
		[2]int{outsideX, outsideY},
	)
	// Release explicitly while outside the grid pane.
	shiftDragReleaseAt(g, outsideX, outsideY)

	if g.marquee.active {
		t.Fatalf("marquee must be cancelled, not left active, when released outside the grid pane")
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("no group should be created when the marquee is cancelled outside the grid pane, got %d", n)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("group menu must not open when the marquee is cancelled outside the grid pane")
	}

	// Back into the grid pane with the button up must not resurrect the marquee.
	emptyX, emptyY := 700, 50 // empty grid space, away from both nodes
	shiftDragReleaseAt(g, emptyX, emptyY)

	if g.marquee.active {
		t.Fatalf("marquee must stay inactive after re-entering the grid pane with the button up")
	}
	if n := len(g.graph.AllGroups()); n != 0 {
		t.Fatalf("no phantom group should appear after re-entering the grid pane, got %d", n)
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("group menu must not open from a phantom stale-release replay")
	}
}

// A release over the open GroupMenu is consumed by the dispatcher before handleEditor sees it, so the marquee
// must be cancelled then, or the next plain click replays its stale rect into a phantom group.
func TestMarqueeCancelledWhenReleasedOverGroupMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 120, 80)
	advanceFrames(g, 1) // let the dispatcher register the now-open group menu
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: group menu must be open")
	}

	bounds := g.groupMenu.InputBounds()
	// The panel's side depends on where the members sit, so take a point along the far right edge (the panel is at
	// most groupMenuW wide) and check it against the panel, the gear and the members.
	gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
	gear := g.gridHelpButtonRect()
	memberBBox, _ := g.groupMemberScreenBBox([]model.NodeID{aID, bID})
	safeOutside := func(p image.Point) bool {
		return !p.In(bounds) && !p.In(gear) && !p.In(memberBBox) &&
			p.X >= 0 && p.X < gridW && p.Y >= 0 && p.Y < gridH
	}

	startX, startY := gridW-10, 50 // empty grid space, outside the panel
	if !safeOutside(image.Pt(startX, startY)) {
		t.Fatalf("setup: start point (%d,%d) must be outside the panel/gear/members, bounds=%v gear=%v members=%v", startX, startY, bounds, gear, memberBBox)
	}
	insideX, insideY := (bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2

	// The release lands inside the menu's InputBounds, so the dispatcher consumes that frame.
	shiftDrag(g, startX, startY, insideX, insideY)

	if g.marquee.active {
		t.Fatalf("marquee must be cancelled, not left dangling, when its release lands on the group menu")
	}
	if n := len(g.graph.AllGroups()); n != 1 {
		t.Fatalf("no phantom group should appear, want 1 (pre-existing) group, got %d", n)
	}

	// Must add a node, not replay the stale rect into a phantom group.
	clickX, clickY := gridW-10, 200
	if !safeOutside(image.Pt(clickX, clickY)) {
		t.Fatalf("setup: post-cancel click point (%d,%d) must be outside the panel/gear/members, bounds=%v gear=%v members=%v", clickX, clickY, bounds, gear, memberBBox)
	}
	gridClick(g, clickX, clickY)
	if n := len(g.graph.AllGroups()); n != 1 {
		t.Fatalf("plain click after a cancelled marquee must not create a phantom group, got %d groups", n)
	}
}

// Marqueeing exactly a committed group's node set reopens that group: no new group, no undo step, and
// closing the reopened menu keeps it.
func TestMarqueeSameSetReusesGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID) // provisional
	g.groupMenu.save()                        // commit it (task-groupsave Save flow)
	advanceFrames(g, 2)
	// Save keeps the menu open; close it to reach "committed, menu closed".
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: Save must keep the menu open")
	}
	if g.groupMenu.provisional {
		t.Fatalf("setup: provisional flag must be cleared after save")
	}
	g.groupMenu.Close()
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("setup: closing a saved (committed, clean) group must not delete it")
	}
	if got := len(g.graph.AllGroups()); got != 1 {
		t.Fatalf("setup: want 1 committed group, got %d", got)
	}
	undoBefore := len(g.undoManager.undo)

	// Pressed bottom-left, released top-right: same rect, but the press stays clear of the group's badge, which
	// floats past the top-right corner.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(by2)+20, int(bx2)+20, int(ay1)-20)

	if got := len(g.graph.AllGroups()); got != 1 {
		t.Fatalf("reusing an existing group must not create a new one, got %d groups", got)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gid {
		t.Fatalf("menu must reopen for the SAME group id=%v, got open=%v id=%v", gid, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if g.groupMenu.provisional {
		t.Fatalf("reopening an already-committed group must NOT mark it provisional")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("reopening an existing group must add zero undo steps, before=%d after=%d", undoBefore, got)
	}

	// Closing the reopened (non-provisional) menu must not delete the group.
	g.groupMenu.Close()
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("closing a reopened NON-provisional group must not delete it")
	}
}

// The same set marqueed twice before any Save leaves one group, still provisional.
func TestMarqueeSameSetTwiceProvisional(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gidA := marqueeCreateGroup(t, g, aID, bID)
	if !g.groupMenu.provisional {
		t.Fatalf("setup: first marquee-created group must be provisional")
	}

	// Bottom-left to top-right to stay clear of the badge.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(by2)+20, int(bx2)+20, int(ay1)-20)

	groups := g.graph.AllGroups()
	if len(groups) != 1 || groups[0].ID != gidA {
		t.Fatalf("want exactly 1 group with the SAME id=%v, got %+v", gidA, groups)
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != gidA {
		t.Fatalf("menu must be open for the same group id=%v, got open=%v id=%v", gidA, g.groupMenu.IsOpen(), g.groupMenu.GroupID())
	}
	if !g.groupMenu.provisional {
		t.Fatalf("re-marqueeing the same still-unsaved set must leave the group STILL provisional")
	}

	// Closing without ever saving must discard it (same as any provisional group).
	g.groupMenu.Close()
	if _, ok := g.graph.Group(gidA); ok {
		t.Fatalf("closing a still-provisional reopened group must discard it")
	}
}

// A subset or superset of a group's members is not a reuse match.
func TestMarqueeSubsetCreatesNewGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Far away, so the a/b marquee can never sweep it in.
	nc := g.tryAddNode(20, 20, model.NodeTypeRegular)
	if _, err := g.graph.CreateGroup("", []model.NodeID{aID, bID, nc.ID}); err != nil {
		t.Fatalf("setup CreateGroup: %v", err)
	}

	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(ay1)-20, int(bx2)+20, int(by2)+20)

	if got := len(g.graph.AllGroups()); got != 2 {
		t.Fatalf("a subset selection must create a NEW group, want 2 groups total, got %d", got)
	}
}

// Button up, Shift held: the continuation of a gesture after shiftDragTo.
func shiftDragReleaseAt(g *Game, x, y int) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	advanceFrames(g, 2)
}

// Fails the test unless the marquee produced exactly one two-member group with its menu open.
func marqueeCreateGroup(t *testing.T, g *Game, aID, bID model.NodeID) model.GroupID {
	t.Helper()
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, bx2, by2 := g.nodeScreenRect(g.nodesByID[bID])
	shiftDrag(g, int(ax1)-20, int(ay1)-20, int(bx2)+20, int(by2)+20)

	groups := g.graph.AllGroups()
	if len(groups) != 1 || len(groups[0].NodeIDs) != 2 {
		t.Fatalf("setup: want 1 group with 2 members, got %+v", groups)
	}
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: group menu must be open (provisional) after marquee-create")
	}
	return groups[0].ID
}

// Closing without Save discards the provisional group: no undo step, no rings.
func TestMarqueeGroupDiscardedOnCloseWithoutSave(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)

	g.groupMenu.Close() // same method the close button invokes

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must be closed")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("provisional group must be deleted on close-without-save")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("discard must not add an undo step, before=%d after=%d", undoBefore, got)
	}
	idx := g.groupIndexSnapshot()
	if idx.Member(aID) || idx.Member(bID) {
		t.Fatalf("group rings must be gone after discard")
	}
}

// The dismissing click only closes the menu, so no node is added.
func TestMarqueeGroupDiscardedOnGridClickOutside(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID)
	nodesBefore := len(g.nodes)

	// On the side of the panel opposite to wherever it landed.
	panel := g.groupMenu.panel
	clickX, clickY := 50, 50
	if image.Pt(clickX, clickY).In(panel) {
		clickX = panel.Max.X + 50
	}
	gridClick(g, clickX, clickY) // empty grid space, away from nodes/menu

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close on click-outside")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("provisional group must be deleted on click-outside")
	}
	if len(g.nodes) != nodesBefore {
		t.Fatalf("a click that dismisses the group menu must not also add a node, want %d nodes got %d", nodesBefore, len(g.nodes))
	}
}

// A click on a node discards the provisional group and opens no sidebar.
func TestMarqueeGroupDiscardedOnNodeClick(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID)

	ax1, ay1, ax2, ay2 := g.nodeScreenRect(g.nodesByID[aID])
	cx, cy := int((ax1+ax2)/2), int((ay1+ay2)/2)
	gridClick(g, cx, cy)

	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close on node click")
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("a node click that dismisses the group menu must not also open the sidebar")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("provisional group must be deleted on node-click close")
	}
}

// Rule edits then Save land as ONE undo step carrying membership and rule; the menu stays open.
func TestMarqueeGroupSavedPersists(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)

	g.groupMenu.setRuleEnabled(true)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, +1)
	advanceFrames(g, 2) // enqueueUI actions run inside Update

	g.groupMenu.save()
	advanceFrames(g, 2)

	if !g.groupMenu.IsOpen() {
		t.Fatalf("Save must keep the menu open (task-save-always)")
	}
	grp, ok := g.graph.Group(gid)
	if !ok {
		t.Fatalf("group must exist after save")
	}
	if len(grp.Rules) != 1 {
		t.Fatalf("saved group must keep its rule, got %+v", grp.Rules)
	}
	if got := len(g.undoManager.undo) - undoBefore; got != 1 {
		t.Fatalf("save must add exactly 1 undo step, got %d", got)
	}

	g.undoManager.Undo()
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("undo must remove the saved group")
	}
	g.undoManager.Redo()
	grp, ok = g.graph.Group(gid)
	if !ok {
		t.Fatalf("redo must restore the group")
	}
	if len(grp.Rules) != 1 {
		t.Fatalf("redo must restore the group's rule, got %+v", grp.Rules)
	}
}

// Provisional rule edits add no undo step until Save.
func TestProvisionalRuleEditsEmitNothing(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	marqueeCreateGroup(t, g, aID, bID)
	undoBefore := len(g.undoManager.undo)

	g.groupMenu.setRuleEnabled(true)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, +1)
	advanceFrames(g, 2)

	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("provisional rule edits must not add undo steps, before=%d after=%d", undoBefore, got)
	}
}

// A second marquee while a provisional menu is open must discard the first group, never leak it menuless and
// exportable into the graph.
func TestSecondMarqueeDiscardsPriorProvisionalGroup(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)

	gidA := marqueeCreateGroup(t, g, aID, bID)

	// A distinct region for a disjoint selection; the undo baseline is taken after this (committed) add. At -5,-5
	// the node sat inside group A's badge, so the press opened A instead of arming a marquee.
	nc := g.tryAddNode(-20, -20, model.NodeTypeRegular)
	undoBefore := len(g.undoManager.undo)
	cx1, cy1, cx2, cy2 := g.nodeScreenRect(nc)

	// The margin must clear c's guide-snap radius, measured from its centre: a Shift+press within it arms a
	// connect drag, which outranks the marquee.
	snapPx := int(g.guideSnapRadiusWorld()*g.cam.Scale) + 2
	p1 := image.Pt(int(cx1)-snapPx, int(cy1)-snapPx)
	p2 := image.Pt(int(cx2)+snapPx, int(cy2)+snapPx)

	// Any frame whose cursor lands in the open menu is consumed and cancels an active marquee, and shiftDrag
	// samples only the endpoints, so both must sit clear of the panel.
	panel := g.groupMenu.panel
	if p1.In(panel) || p2.In(panel) {
		t.Fatalf("setup: second marquee's press/release points must both fall outside the open menu panel=%v p1=%v p2=%v", panel, p1, p2)
	}
	// A press on group A's badge opens it instead of arming the marquee.
	if _, ok := g.groupBadgeAt(p1.X, p1.Y); ok {
		t.Fatalf("setup: press point %v must not land on group A's badge", p1)
	}
	// A press inside any node's snap radius arms a link drag; failing here names that cause directly.
	if gt := g.guideTargetAt(p1.X, p1.Y); gt.Kind == GuideNode {
		t.Fatalf("setup: press point %v must fall outside every node's guide snap radius (%.1fpx screen); it snapped to node %d and would arm a link-drag, not a marquee",
			p1, g.guideSnapRadiusWorld()*g.cam.Scale, gt.Node.ID)
	}

	shiftDrag(g, p1.X, p1.Y, p2.X, p2.Y)

	groups := g.graph.AllGroups()
	if len(groups) != 1 {
		t.Fatalf("want exactly 1 group after the second marquee discards the first, got %d: %+v", len(groups), groups)
	}
	if groups[0].ID == gidA {
		t.Fatalf("the surviving group must be the NEW (second) group, not the orphaned first one")
	}
	if _, ok := g.graph.Group(gidA); ok {
		t.Fatalf("first provisional group must be discarded, not orphaned into g.graph.Groups")
	}
	if !g.groupMenu.IsOpen() || g.groupMenu.GroupID() != groups[0].ID {
		t.Fatalf("menu must be open for the new (second) group, got open=%v id=%v want=%v",
			g.groupMenu.IsOpen(), g.groupMenu.GroupID(), groups[0].ID)
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("neither marquee-create (both still provisional/discarded) should add an undo step, before=%d after=%d", undoBefore, got)
	}
}

// A discarded provisional group frees its number, so the next default name is "Group 1" again. The second
// marquee picks a superset so the exact-match reuse cannot mask it.
func TestDiscardedProvisionalReleasesGroupNumber(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	// Far away, so a marquee enclosing all three is a different (superset) selection.
	nc := g.tryAddNode(20, 20, model.NodeTypeRegular)
	const wantTitle = "Group 1" // matches CreateGroup's fmt.Sprintf("Group %d", id) default

	gid1 := marqueeCreateGroup(t, g, aID, bID)
	if got := g.groupMenu.titleText(); got != wantTitle {
		t.Fatalf("first provisional title = %q, want %q", got, wantTitle)
	}
	g.groupMenu.Close() // same method the close button invokes
	if _, ok := g.graph.Group(gid1); ok {
		t.Fatalf("first provisional group must be deleted on close")
	}

	// Different (superset) set: {a, b, c} instead of {a, b}.
	ax1, ay1, _, _ := g.nodeScreenRect(g.nodesByID[aID])
	_, _, cx2, cy2 := g.nodeScreenRect(nc)
	shiftDrag(g, int(ax1)-20, int(ay1)-20, int(cx2)+20, int(cy2)+20)
	groups := g.graph.AllGroups()
	if len(groups) != 1 || len(groups[0].NodeIDs) != 3 {
		t.Fatalf("setup: want 1 group with 3 members (superset), got %+v", groups)
	}
	gid2 := groups[0].ID
	if got := g.groupMenu.titleText(); got != wantTitle {
		t.Fatalf("after first discard+remarquee, title = %q, want %q (number must be released)", got, wantTitle)
	}
	if groups[0].ID != 1 {
		t.Fatalf("want the reused ID 1, got %+v", groups)
	}
	g.groupMenu.Close()
	if _, ok := g.graph.Group(gid2); ok {
		t.Fatalf("second provisional group must be deleted on close")
	}

	// Repeated discard cycles never drift the number upward.
	gid3 := marqueeCreateGroup(t, g, aID, bID)
	if got := g.groupMenu.titleText(); got != wantTitle {
		t.Fatalf("after second discard+remarquee, title = %q, want %q (no monotonic drift)", got, wantTitle)
	}
	if groups := g.graph.AllGroups(); len(groups) != 1 || groups[0].ID != 1 {
		t.Fatalf("want exactly 1 group with ID 1, got %+v", groups)
	}
	_ = gid3
}

// A real Esc edge closes the menu and discards its provisional group without a trace.
func TestEscClosesAndDiscardsProvisionalMenu(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	undoBefore := len(g.undoManager.undo)
	gid := marqueeCreateGroup(t, g, aID, bID)

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	restore()

	if g.groupMenu.IsOpen() {
		t.Fatalf("Esc must close the provisional group menu")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("Esc must discard the provisional group, same as close/click-outside")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("Esc-discard must not add an undo step, before=%d after=%d", undoBefore, got)
	}

	// Number must be released: the next marquee reuses "Group 1".
	gid2 := marqueeCreateGroup(t, g, aID, bID)
	if got, want := g.groupMenu.titleText(), "Group 1"; got != want {
		t.Fatalf("after Esc-discard, next marquee title = %q, want %q", got, want)
	}
	_ = gid2
}

// Esc on a committed group's menu closes it without deleting the group.
func TestEscOnCommittedMenuClosesWithoutDelete(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid, err := g.graph.CreateGroup("Committed", []model.NodeID{aID, bID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 100, 100)
	if !g.groupMenu.IsOpen() {
		t.Fatalf("setup: menu must be open")
	}

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	restore()

	if g.groupMenu.IsOpen() {
		t.Fatalf("Esc must close the menu")
	}
	if _, ok := g.graph.Group(gid); !ok {
		t.Fatalf("Esc on a committed (non-provisional) menu must NOT delete the group")
	}
}

// A discarded provisional group with edited rules leaves the export byte-identical to before the marquee.
func TestDiscardLeavesExportByteIdentical(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	before, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes (before): %v", err)
	}

	marqueeCreateGroup(t, g, aID, bID)
	g.groupMenu.setRuleEnabled(true)
	g.groupMenu.stepRuleDelta(model.GroupParamPitch, +1)
	g.groupMenu.stepRuleEveryN(+1)
	advanceFrames(g, 2)
	g.groupMenu.Close()

	after, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes (after): %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("export bytes diverged after discard-without-save:\nbefore=%s\nafter=%s", before, after)
	}
}

// Both open is unreachable by gesture, so they are forced open to pin the rung order: Esc closes the
// z-above GroupMenu first.
func TestEscPrefersGroupMenuOverSidebar(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	gid := marqueeCreateGroup(t, g, aID, bID)
	g.sidebar.Open(g.nodesByID[aID])
	if !g.groupMenu.IsOpen() || !g.sidebar.IsOpen() {
		t.Fatalf("setup: both group menu and sidebar must be open, got menu=%v sidebar=%v", g.groupMenu.IsOpen(), g.sidebar.IsOpen())
	}

	restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
	g.handleGlobalShortcuts()
	restore()

	if g.groupMenu.IsOpen() {
		t.Fatalf("Esc must close the group menu (topmost, z=210) first")
	}
	if _, ok := g.graph.Group(gid); ok {
		t.Fatalf("group menu's Esc-close must discard the still-provisional group")
	}
	if !g.sidebar.IsOpen() {
		t.Fatalf("sidebar must remain open — a single Esc consumes only the topmost rung")
	}
}

// The Delete button on a provisional group releases its number like Close, Esc and click-out.
func TestDeleteButtonOnProvisionalReleasesGroupNumber(t *testing.T) {
	g, aID, bID := buildGroupLoopCircuit(t)
	const wantTitle = "Group 1"

	gid1 := marqueeCreateGroup(t, g, aID, bID)
	if got := g.groupMenu.titleText(); got != wantTitle {
		t.Fatalf("provisional title = %q, want %q", got, wantTitle)
	}
	undoBefore := len(g.undoManager.undo)
	g.groupMenu.deleteGroup() // the Delete button's handler (enqueued)
	advanceFrames(g, 2)
	if _, ok := g.graph.Group(gid1); ok {
		t.Fatalf("provisional group must be deleted by the Delete button")
	}
	if g.groupMenu.IsOpen() {
		t.Fatalf("menu must close after Delete")
	}
	if got := len(g.undoManager.undo); got != undoBefore {
		t.Fatalf("provisional Delete must be silent: undo depth %d -> %d", undoBefore, got)
	}

	// The number must be released: a fresh marquee mints "Group 1" again.
	gid2 := marqueeCreateGroup(t, g, aID, bID)
	if got := g.groupMenu.titleText(); got != wantTitle {
		t.Fatalf("after Delete-button discard, remarquee title = %q, want %q (number must be released)", got, wantTitle)
	}
	if gid2 != gid1 {
		t.Fatalf("want reused tail ID %d, got %d", gid1, gid2)
	}
	g.groupMenu.Close()
}
