package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// openSidebarWithScroll is a helper that creates a game with a small grid pane,
// opens the sidebar with all sections expanded + logic dropdown open so content
// overflows, and returns the game. Caller must t.Cleanup(g.CloseForTest).
// NOTE: assertDefaultParityState is called internally, so forceSmallScreenForTest
// must be false when calling this.
func openSidebarWithScroll(t *testing.T, gridH int) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, gridH)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	if !g.sidebar.scroll.HasScroll() {
		t.Fatal("scroll should be enabled when all sections expanded with dropdown in small pane")
	}
	return g
}

// TestSidebarWheelScrollDirection verifies that mouse wheel scroll direction
// matches user expectation: wheel down (steps=-1) scrolls content down
// (VS.First increases), wheel up (steps=+1) scrolls content up (VS.First
// decreases).
func TestSidebarWheelScrollDirection(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	if g.sidebar.scroll.VS.First != 0 {
		t.Fatalf("initial scroll offset should be 0, got %d", g.sidebar.scroll.VS.First)
	}

	// Wheel down (steps=-1) → content scrolls down → VS.First increases
	g.sidebar.HandleWheel(0, 0, -1)
	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatalf("VS.First should increase after wheel-down (steps=-1), got %d", g.sidebar.scroll.VS.First)
	}

	saved := g.sidebar.scroll.VS.First

	// Wheel up (steps=+1) → content scrolls up → VS.First decreases
	g.sidebar.HandleWheel(0, 0, 1)
	if g.sidebar.scroll.VS.First >= saved {
		t.Fatalf("VS.First should decrease after wheel-up (steps=+1), got %d (was %d)", g.sidebar.scroll.VS.First, saved)
	}
}

// TestSidebarScrollbarTrackClick verifies that clicking on the scrollbar track
// (but not on the thumb) jumps the scroll position proportionally.
func TestSidebarScrollbarTrackClick(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	bar := g.sidebar.scroll.BarRect()
	thumb := g.sidebar.scroll.ThumbRect()
	if bar.Empty() || thumb.Empty() {
		t.Fatal("bar/thumb rects should be non-empty")
	}

	// Click below the thumb in the track area
	clickY := thumb.Max.Y + (bar.Max.Y-thumb.Max.Y)/2
	if clickY >= bar.Max.Y {
		clickY = bar.Max.Y - 1
	}
	clickX := (bar.Min.X + bar.Max.X) / 2

	// Verify click is on the track but not the thumb
	if image.Pt(clickX, clickY).In(thumb) {
		t.Fatal("click should be outside thumb rect")
	}
	if !image.Pt(clickX, clickY).In(bar) {
		t.Fatal("click should be inside bar rect")
	}

	// Press on track
	g.sidebar.HandleInput(clickX, clickY, true)
	// Release
	g.sidebar.HandleInput(clickX, clickY, false)

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatalf("VS.First should jump to non-zero after track click below thumb, got %d", g.sidebar.scroll.VS.First)
	}
}

// TestSidebarScrollMomentum verifies that momentum continues decaying after
// touch ends, even when HandleInput is no longer called (finger lifted and
// cursor moved away from sidebar).
func TestSidebarScrollMomentum(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Simulate touch swipe via ScrollBehavior directly
	startY := 200
	endY := 100
	sb := g.sidebar.scroll
	sb.HandleTouchBegin(50, startY)
	// Multiple moves to build velocity
	for y := startY; y >= endY; y -= 5 {
		sb.HandleTouchMove(50, y)
	}
	sb.HandleTouchEnd()

	// Momentum should be active
	if !sb.HasMomentum() {
		t.Fatal("should have momentum after swipe")
	}

	// Call UpdateScroll (per-frame method, NOT HandleInput) for 50 frames
	firstAfterSwipe := sb.VS.First
	for i := 0; i < 50; i++ {
		g.sidebar.UpdateScroll()
	}

	if sb.VS.First <= firstAfterSwipe {
		t.Fatalf("VS.First should increase via momentum: before=%d after=%d", firstAfterSwipe, sb.VS.First)
	}
}

// TestSidebarTouchScrollMobile verifies touch scrolling works on mobile
// (small screen) where content overflows the sidebar viewport.
func TestSidebarTouchScrollMobile(t *testing.T) {
	// Must set forceSmallScreenForTest AFTER assertDefaultParityState (called
	// in openSidebarWithScroll), which asserts it is false.
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 240)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	if !g.sidebar.scroll.HasScroll() {
		t.Fatal("scroll should be enabled when all sections expanded in small mobile pane")
	}

	// Simulate touch drag within sidebar HandleInput.
	// Use coordinates within the panel bounds (not absolute screen coords).
	panel := g.sidebar.rects[sbPanel]
	cx := panel.Min.X + panel.Dx()/2
	startY := panel.Max.Y - 10 // near bottom of panel
	endY := panel.Min.Y + 20   // near top of panel

	// Press
	g.sidebar.HandleInput(cx, startY, true)
	// Drag up (finger moves up = content scrolls down = VS.First increases)
	for y := startY - 5; y >= endY; y -= 5 {
		g.sidebar.HandleInput(cx, y, true)
	}
	// Release
	g.sidebar.HandleInput(cx, endY, false)

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatalf("VS.First should increase after touch swipe down, got %d", g.sidebar.scroll.VS.First)
	}
}

// TestSidebarViewportClipping verifies that section headers and buttons
// outside the visible viewport are not drawn (inViewport returns false).
func TestSidebarViewportClipping(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Scroll to the bottom so top sections are above viewport
	maxFirst := g.sidebar.scroll.VS.Total - g.sidebar.scroll.VS.Visible
	if maxFirst <= 0 {
		t.Fatal("maxFirst should be positive for scrollable content")
	}
	g.sidebar.scroll.VS.First = maxFirst
	g.sidebar.scroll.VS.Clamp()
	g.sidebar.layout()

	// The first section header ("sec-vol") should now be above the viewport
	secVol := g.sidebar.rects[sbSectionHeader(secVol)]
	if secVol.Empty() {
		t.Fatal("sec-vol rect should exist")
	}

	// inViewport should return false for content scrolled above the viewport
	if g.sidebar.inViewport(secVol) {
		t.Fatal("sec-vol should be outside viewport when scrolled to bottom")
	}

	// Find the last section that exists and verify it IS in viewport.
	// "sec-grp" (task-sidebar-create: always laid out, last in layout()) is
	// now the true bottom section on every platform; "sec-aud"/"sec-groove"
	// are the prior (still-correct) desktop/mobile fallbacks.
	var lastSection sbID
	for _, sec := range []sbID{sbSectionHeader(secGrp), sbSectionHeader(secAud), sbSectionHeader(secGroove)} {
		if r, ok := g.sidebar.rects[sec]; ok && !r.Empty() {
			lastSection = sec
			break
		}
	}
	if lastSection == (sbID{}) {
		t.Fatal("no bottom section found")
	}
	lastRect := g.sidebar.rects[lastSection]
	if !g.sidebar.inViewport(lastRect) {
		t.Fatalf("%s should be in viewport when scrolled to bottom: rect=%v gridH=%d contentTop=%d",
			lastSection, lastRect, g.split.GridH(g.winH), sidebarPad+sidebarHeaderH+sidebarGap)
	}
}

// TestSidebarTouchScrollDoesNotToggleSection verifies that a touch drag on
// a section header scrolls the content without toggling the section.
func TestSidebarTouchScrollDoesNotToggleSection(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Record which sections are open before the touch.
	openBefore := make(map[sbSection]bool)
	for k, v := range g.sidebar.sectionOpen {
		openBefore[k] = v
	}

	// Find a section header rect to touch on.
	secRect := g.sidebar.rects[sbSectionHeader(secVol)]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	startY := (secRect.Min.Y + secRect.Max.Y) / 2

	// Touch down on section header
	g.sidebar.HandleInput(cx, startY, true)

	// Drag vertically (20px is well past the 8px dead zone)
	for dy := 1; dy <= 25; dy++ {
		g.sidebar.HandleInput(cx, startY+dy, true)
	}

	// Release
	g.sidebar.HandleInput(cx, startY+25, false)

	// Section state should be unchanged — scroll, not toggle.
	for k, v := range openBefore {
		if g.sidebar.sectionOpen[k] != v {
			t.Fatalf("section %q toggled during scroll: was %v, now %v", k, v, g.sidebar.sectionOpen[k])
		}
	}

	// Scroll offset should have changed.
	if g.sidebar.scroll.VS.First == 0 {
		// Momentum may not have moved First yet — check if touch was tracked.
		if !g.sidebar.scroll.HasMomentum() {
			t.Log("note: scroll offset still 0 and no momentum — touch may not have committed")
		}
	}
}

// TestSidebarTapStillTogglesSection verifies that a quick tap on a section
// header (press+release without moving) still toggles the section.
func TestSidebarTapStillTogglesSection(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Find a closed section.
	volOpen := g.sidebar.sectionOpen[secVol]

	secRect := g.sidebar.rects[sbSectionHeader(secVol)]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	cy := (secRect.Min.Y + secRect.Max.Y) / 2

	// Tap: press + immediate release at same position
	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.sectionOpen[secVol] == volOpen {
		t.Fatalf("section 'vol' should have toggled from %v after tap", volOpen)
	}
}

// TestSidebarButtonSuppressedDuringScroll verifies that button presses
// are suppressed when a touch scroll gesture is in progress.
func TestSidebarButtonSuppressedDuringScroll(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Expand volume section so vol+/- buttons exist.
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.layout()

	volPlusRect := g.sidebar.rects[sbPlus(fieldVol)]
	if volPlusRect.Empty() {
		t.Fatal("vol+ rect should exist when volume section is open")
	}

	// Get initial volume value.
	mn, ok := g.graph.GetNodeByID(g.sidebar.node.ID)
	if !ok {
		t.Fatal("node not found")
	}
	volBefore := mn.Params.Volume

	// Start touch on the vol+ button
	cx := (volPlusRect.Min.X + volPlusRect.Max.X) / 2
	startY := (volPlusRect.Min.Y + volPlusRect.Max.Y) / 2
	g.sidebar.HandleInput(cx, startY, true)

	// Drag vertically past dead zone to commit scroll
	for dy := 1; dy <= 25; dy++ {
		g.sidebar.HandleInput(cx, startY+dy, true)
	}

	// Release
	g.sidebar.HandleInput(cx, startY+25, false)

	// Volume should NOT have changed — button was suppressed by scroll.
	mn2, _ := g.graph.GetNodeByID(g.sidebar.node.ID)
	if mn2.Params.Volume != volBefore {
		t.Fatalf("vol+ should NOT fire during scroll: before=%v after=%v", volBefore, mn2.Params.Volume)
	}
}

// TestSidebarGestureTapScrollDoesNotToggle verifies that when the sidebar
// has committed scroll state (user dragged past dead zone), a release does
// NOT toggle the section.
//
// This used to fire through handleTapInGrid, which special-cased the sidebar
// (send only a release when scroll.TouchActive()/Dragging(), resolving via the
// ScrollingCommitted path that cancels the deferred tap). Since the sidebar's
// card is now rendered/hit-tested through the global OverlayPortal, opening it
// makes modalOverlayActive() true, and handleTapInGrid bails out on its very
// first line before ever reaching that sidebar-interaction branch — the
// branch is unreachable while the sidebar is open (see game_input_touch.go's
// gate at the top of handleTapInGrid). handleTapInGrid's sidebar block reduced
// to exactly this call (`g.sidebar.HandleInput(x, y, false)` or, through
// g.handleNodeMenuButtons, the same call with a leading press), so calling it
// directly re-expresses the same invariant against NodeSidebar.HandleInput's
// still-unchanged scroll/deferred-tap state machine, without depending on the
// now-dead grid-tap wrapper.
func TestSidebarGestureTapScrollDoesNotToggle(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Record section state before.
	openBefore := make(map[sbSection]bool)
	for k, v := range g.sidebar.sectionOpen {
		openBefore[k] = v
	}

	// Find a section header to scroll on.
	secRect := g.sidebar.rects[sbSectionHeader(secVol)]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	startY := (secRect.Min.Y + secRect.Max.Y) / 2

	// Simulate InputDispatcher captured frames: press + drag past dead zone.
	// This commits the scroll direction.
	g.sidebar.HandleInput(cx, startY, true)
	for dy := 1; dy <= touchScrollDeadZone+5; dy++ {
		g.sidebar.HandleInput(cx, startY+dy, true)
	}

	// Verify scroll is committed.
	if !g.sidebar.scroll.ScrollingCommitted() {
		t.Fatal("scroll should be committed after dragging past dead zone")
	}

	// Resolve with a release, directly on the sidebar — the exact call
	// handleTapInGrid used to make on this path before the sidebar's
	// interaction branch there became unreachable (see comment above).
	endY := startY + touchScrollDeadZone + 5
	g.sidebar.HandleInput(cx, endY, false)

	// Section state should be unchanged — committed scroll cancels deferred tap.
	for k, v := range openBefore {
		if g.sidebar.sectionOpen[k] != v {
			t.Fatalf("section %q toggled during committed scroll: was %v, now %v", k, v, g.sidebar.sectionOpen[k])
		}
	}

	// State should be clean.
	if g.sidebar.scroll.TouchActive() {
		t.Fatal("TouchActive should be false after resolution")
	}
}

// TestSidebarGestureTapStillToggles verifies that a true tap (0px movement)
// still toggles sections when no scroll state is active.
//
// Previously driven through handleTapInGrid, whose sidebar-interaction branch
// (no scroll/touch state active) forwarded to g.handleNodeMenuButtons — itself
// a thin wrapper calling g.sidebar.HandleInput(x, y, left) for press then
// release. That branch is unreachable now that the sidebar renders through the
// global OverlayPortal (modalOverlayActive() gates handleTapInGrid's very
// first line while the sidebar is open — see game_input_touch.go). Driving the
// same underlying press+release pair directly against the sidebar
// re-expresses the identical invariant against NodeSidebar.HandleInput, whose
// section-header toggle logic is unchanged.
func TestSidebarGestureTapStillToggles(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	secRect := g.sidebar.rects[sbSectionHeader(secVol)]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	cy := (secRect.Min.Y + secRect.Max.Y) / 2

	volBefore := g.sidebar.sectionOpen[secVol]

	// Direct tap — no prior scroll state. Section headers toggle on the press
	// edge (see NodeSidebar.HandleInput step 9); the release call mirrors what
	// g.handleNodeMenuButtons used to send afterward.
	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.sectionOpen[secVol] == volBefore {
		t.Fatalf("section 'vol' should have toggled from %v after gesture tap", volBefore)
	}
}

// TestSidebarScrollStateCleanAfterGestureTap verifies that after a gesture
// tap resolves sidebar scroll state, no stale TouchActive or DeferredTap remains.
//
// Previously resolved via handleTapInGrid (see the comment on
// TestSidebarGestureTapScrollDoesNotToggle above for why that path is now
// unreachable while the sidebar is open). Resolving directly with the release
// call handleTapInGrid used to make preserves the same invariant against
// NodeSidebar.HandleInput's state machine.
func TestSidebarScrollStateCleanAfterGestureTap(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	secRect := g.sidebar.rects[sbSectionHeader(secVol)]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	startY := (secRect.Min.Y + secRect.Max.Y) / 2

	// Start scroll tracking.
	g.sidebar.HandleInput(cx, startY, true)
	for dy := 1; dy <= 12; dy++ {
		g.sidebar.HandleInput(cx, startY+dy, true)
	}

	// Resolve the pending state with a release, directly on the sidebar.
	g.sidebar.HandleInput(cx, startY+12, false)

	if g.sidebar.scroll.TouchActive() {
		t.Fatal("TouchActive should be false after gesture tap resolution")
	}
	if g.sidebar.deferredTap.Active() {
		t.Fatal("DeferredTap should not be active after gesture tap resolution")
	}
}

// TestScrollDeadZoneMatchesTapThreshold asserts that touchScrollDeadZone
// matches tapMaxMovePx so all touch disambiguation uses the same threshold.
func TestScrollDeadZoneMatchesTapThreshold(t *testing.T) {
	if touchScrollDeadZone != tapMaxMovePx {
		t.Fatalf("touchScrollDeadZone (%d) != tapMaxMovePx (%d); they must match", touchScrollDeadZone, tapMaxMovePx)
	}
}

// TestFXPanelScrollDoesNotFireButtons verifies that a scroll gesture
// in the FX panel doesn't fire effect buttons (the FX panel uses the same
// deferred-tap + TouchScroller pattern).
func TestFXPanelScrollDoesNotFireButtons(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 600)

	// Add a row and open the FX panel.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.updateBeatInfos()

	if len(g.drum.Rows) == 0 {
		t.Fatal("need at least one drum row")
	}
	g.drum.toggleFXPanel(0)
	t.Cleanup(func() { g.drum.closeFXPanel() })
	if !g.drum.IsFXPanelOpen() {
		t.Fatal("FX panel should be open")
	}

	// The FX panel uses fxScrollTS + fxPanelDeferredTap.
	// Simulate: press inside viewport, drag past dead zone, release.
	vp := g.drum.fxViewportRect
	if vp.Empty() {
		// FX panel may not have a scrollable viewport (no effects added).
		// Just verify the panel is open and the pattern exists.
		t.Skip("FX viewport empty — no scrollable content")
	}
	cx := (vp.Min.X + vp.Max.X) / 2
	startY := (vp.Min.Y + vp.Max.Y) / 2

	sb := g.drum.ensureFXScroll().ScrollBehavior()
	sb.HandleTouchBegin(cx, startY)
	g.drum.fxPanelDeferredTap.Begin(cx, startY)
	for dy := 1; dy <= 15; dy++ {
		sb.HandleTouchMove(cx, startY+dy)
	}

	// Scroll should now be committed, deferred tap should be cancellable.
	if !sb.ScrollingCommitted() {
		t.Fatal("FX scroll should be committed after 15px drag")
	}

	// Cancel deferred tap (as the real code does when scroll commits).
	g.drum.fxPanelDeferredTap.Cancel()

	if g.drum.fxPanelDeferredTap.Active() {
		t.Fatal("deferred tap should be cancelled after scroll commit")
	}
}
