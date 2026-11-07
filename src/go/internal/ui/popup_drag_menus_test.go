package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Through the real dispatcher, never a hand-called HandleInput.
type dragRig struct {
	g    *Game
	x, y int
	down bool
}

func newDragRig(t *testing.T, w, h int) *dragRig {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)

	r := &dragRig{g: g}
	restore := SetInputForTest(
		func() (int, int) { return r.x, r.y },
		func(b ebiten.MouseButton) bool { return r.down && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	return r
}

// moveTo positions the pointer and runs one frame.
func (r *dragRig) moveTo(x, y int) {
	r.x, r.y = x, y
	_ = r.g.Update()
}

// dragGrip presses the grip at from, walks to to in steps, and releases.
func (r *dragRig) dragGrip(from, to image.Point) {
	r.x, r.y = from.X, from.Y
	r.down = true
	_ = r.g.Update() // press frame
	steps := 5
	for i := 1; i <= steps; i++ {
		r.moveTo(
			from.X+(to.X-from.X)*i/steps,
			from.Y+(to.Y-from.Y)*i/steps,
		)
	}
	r.down = false
	_ = r.g.Update() // release frame
}

func openNodeMenuForDrag(t *testing.T, r *dragRig) *uiNode {
	t.Helper()
	n := r.g.tryAddNode(2, 1, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("tryAddNode returned nil")
	}
	r.g.updateBeatInfos()
	r.g.sel = n
	n.Selected = true
	r.g.sidebar.Open(n)
	_ = r.g.Update()
	return n
}

func TestNodeMenuGripDragMovesCard(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	before := r.g.sidebar.rects[sbPanel]
	grip := r.g.sidebar.rects[sbGrip]
	if grip.Empty() {
		t.Fatal(`sidebar must publish rects[sbGrip]`)
	}
	if !grip.In(before) {
		t.Fatalf("grip %v must sit inside the card %v", grip, before)
	}

	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	to := from.Add(image.Pt(120, 60))
	r.dragGrip(from, to)

	after := r.g.sidebar.rects[sbPanel]
	if after == before {
		t.Fatalf("card did not move: still %v", before)
	}
	if got, want := after.Min.Sub(before.Min), image.Pt(120, 60); got != want {
		t.Fatalf("card should track the cursor delta: want %v got %v", want, got)
	}
	if after.Dx() != before.Dx() || after.Dy() != before.Dy() {
		t.Fatalf("drag must not resize the card: %v → %v", before, after)
	}
}

// Not a one-frame nudge the next layout re-anchors.
func TestNodeMenuGripDragPinSurvivesFrames(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(140, 40)))
	dropped := r.g.sidebar.rects[sbPanel]

	for i := 0; i < 30; i++ {
		_ = r.g.Update()
	}
	if got := r.g.sidebar.rects[sbPanel]; got != dropped {
		t.Fatalf("pinned card drifted over 30 frames: %v → %v", dropped, got)
	}
}

// Pinned to the screen: a grid pan does not take the card with its node.
func TestNodeMenuGripDragSurvivesCameraPan(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(100, 50)))
	dropped := r.g.sidebar.rects[sbPanel]

	r.g.cam.OffsetX += 90
	r.g.cam.OffsetY += 45
	_ = r.g.Update()

	if got := r.g.sidebar.rects[sbPanel]; got != dropped {
		t.Fatalf("pinned card followed the camera: %v → %v", dropped, got)
	}
}

// TestNodeMenuReopenResetsToDefault: the pin is per-open, per the design.
func TestNodeMenuReopenResetsToDefault(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	n := openNodeMenuForDrag(t, r)
	def := r.g.sidebar.rects[sbPanel]

	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(130, 70)))
	if r.g.sidebar.rects[sbPanel] == def {
		t.Fatal("precondition failed: card never moved")
	}

	r.g.sidebar.Close()
	_ = r.g.Update()
	r.g.sidebar.Open(n)
	_ = r.g.Update()

	if got := r.g.sidebar.rects[sbPanel]; got != def {
		t.Fatalf("reopen should restore the default anchored position: want %v got %v", def, got)
	}
}

// Consumed, never reaching the grid editor.
func TestNodeMenuGripPressCreatesNoNode(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)
	before := len(r.g.nodes) // the UI-side node slice, as eq_wheel_popup_isolation_test.go does

	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(110, 55)))

	if after := len(r.g.nodes); after != before {
		t.Fatalf("grip drag changed the node count: %d → %d", before, after)
	}
}

// The grip sits above the scroll viewport.
func TestNodeMenuGripDragDoesNotScroll(t *testing.T) {
	r := newDragRig(t, 640, 250) // short pane → content overflows
	openNodeMenuForDrag(t, r)
	r.g.sidebar.ExpandAllSections()
	_ = r.g.Update()

	before := r.g.sidebar.scroll.VS.First
	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(20, 60)))

	if after := r.g.sidebar.scroll.VS.First; after != before {
		t.Fatalf("grip drag scrolled the content: %d → %d", before, after)
	}
}

// The layer clips to the pane, so a card outside it would be scissored away.
func TestNodeMenuStaysInGridPaneWhenDragged(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	gridPane := image.Rect(0, 0, r.g.split.GridW(r.g.winW), r.g.split.GridH(r.g.winH))
	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, image.Pt(4000, 4000))

	if got := r.g.sidebar.rects[sbPanel]; !got.In(gridPane) {
		t.Fatalf("dragged card %v escaped the grid pane %v", got, gridPane)
	}
}

func openGroupMenuForDrag(t *testing.T, r *dragRig) model.GroupID {
	t.Helper()
	a := r.g.tryAddNode(2, 1, model.NodeTypeRegular)
	b := r.g.tryAddNode(3, 1, model.NodeTypeRegular)
	c := r.g.tryAddNode(4, 1, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil {
		t.Fatal("could not build the group's nodes")
	}
	r.g.updateBeatInfos()
	gid, err := r.g.graph.CreateGroup("Drums", []model.NodeID{a.ID, b.ID, c.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	_, sy1, sx2, _ := r.g.nodeScreenRect(c)
	r.g.groupMenu.OpenAt(gid, int(sx2)+24, int(sy1))
	_ = r.g.Update()
	return gid
}

// TestGroupMenuGripDragMovesCard: the headline behavior for the second menu.
func TestGroupMenuGripDragMovesCard(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	before := r.g.groupMenu.panel
	grip := r.g.groupMenu.rects[gmGrip]
	if grip.Empty() {
		t.Fatal(`group menu must publish rects[sbGrip]`)
	}
	if !grip.In(before) {
		t.Fatalf("grip %v must sit inside the card %v", grip, before)
	}

	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	// Up-left: the card already touches the pane floor at 1280x720, so a downward drag would test clamping, not
	// tracking.
	r.dragGrip(from, from.Add(image.Pt(-90, -80)))

	after := r.g.groupMenu.panel
	if got, want := after.Min.Sub(before.Min), image.Pt(-90, -80); got != want {
		t.Fatalf("card should track the cursor delta: want %v got %v", want, got)
	}
}

// Auto placement dodges members; a pinned card stays where it was dropped.
func TestGroupMenuPinnedIgnoresMemberAvoidance(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	grip := r.g.groupMenu.rects[gmGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(-120, 60)))
	dropped := r.g.groupMenu.panel

	// Pan so the members slide under the dropped card.
	r.g.cam.OffsetX -= 140
	for i := 0; i < 5; i++ {
		_ = r.g.Update()
	}
	if got := r.g.groupMenu.panel; got != dropped {
		t.Fatalf("pinned group card was displaced by member avoidance: %v → %v", dropped, got)
	}
}

// TestGroupMenuReopenResetsToDefault: pin is per-open here too.
func TestGroupMenuReopenResetsToDefault(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	gid := openGroupMenuForDrag(t, r)
	def := r.g.groupMenu.panel
	anchorX, anchorY := r.g.groupMenu.anchorX, r.g.groupMenu.anchorY

	grip := r.g.groupMenu.rects[gmGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, from.Add(image.Pt(-100, 80)))
	if r.g.groupMenu.panel == def {
		t.Fatal("precondition failed: card never moved")
	}

	r.g.groupMenu.Close()
	_ = r.g.Update()
	r.g.groupMenu.OpenAt(gid, anchorX, anchorY)
	_ = r.g.Update()

	if got := r.g.groupMenu.panel; got != def {
		t.Fatalf("reopen should restore the default placement: want %v got %v", def, got)
	}
}

// Without it the dispatcher drops the capture mid-gesture.
func TestGroupMenuCapturingDuringDrag(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	grip := r.g.groupMenu.rects[gmGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.x, r.y = from.X, from.Y
	r.down = true
	_ = r.g.Update()
	r.moveTo(from.X-40, from.Y+30)

	if !r.g.groupMenu.Capturing() {
		t.Fatal("group menu must report Capturing() while a grip drag is in flight")
	}
	r.down = false
	_ = r.g.Update()
	if r.g.groupMenu.Capturing() {
		t.Fatal("Capturing() must clear on release")
	}
}

func TestGroupMenuStaysInGridPaneWhenDragged(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	gridPane := image.Rect(0, 0, r.g.split.GridW(r.g.winW), r.g.split.GridH(r.g.winH))
	grip := r.g.groupMenu.rects[gmGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, image.Pt(-4000, 4000))

	if got := r.g.groupMenu.panel; !got.In(gridPane) {
		t.Fatalf("dragged group card %v escaped the grid pane %v", got, gridPane)
	}
}

// panOK gates on menuHit(cursor), so the drag state must gate it once the cursor leaves the card. The drag goes
// to a far corner where the card clamps and the cursor separates from it; a modest drag never would.
func TestGripDragDoesNotPanCamera(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openNodeMenuForDrag(t, r)

	beforeX, beforeY := r.g.cam.OffsetX, r.g.cam.OffsetY

	grip := r.g.sidebar.rects[sbGrip]
	from := image.Pt(grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2)
	r.dragGrip(from, image.Pt(4000, 4000))

	if r.g.cam.OffsetX != beforeX || r.g.cam.OffsetY != beforeY {
		t.Fatalf("camera panned during a grip drag: (%v,%v) → (%v,%v)",
			beforeX, beforeY, r.g.cam.OffsetX, r.g.cam.OffsetY)
	}
}

// A helper checking only the sidebar would regress the group menu silently.
func TestGridMenuDraggingReportsBothMenus(t *testing.T) {
	r := newDragRig(t, 1280, 720)
	openGroupMenuForDrag(t, r)

	if r.g.gridMenuDragging() {
		t.Fatal("no drag in flight yet")
	}
	grip := r.g.groupMenu.rects[gmGrip]
	r.x, r.y = grip.Min.X+grip.Dx()/2, grip.Min.Y+grip.Dy()/2
	r.down = true
	_ = r.g.Update()
	r.moveTo(r.x-60, r.y+40)

	if !r.g.gridMenuDragging() {
		t.Fatal("gridMenuDragging must report the group menu's drag")
	}
	r.down = false
	_ = r.g.Update()
	if r.g.gridMenuDragging() {
		t.Fatal("gridMenuDragging must clear on release")
	}
}
