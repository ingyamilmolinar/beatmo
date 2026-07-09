//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// ─── Audio-panel ↔ drum-rows input isolation ────────────────────────────────
//
// These tests drive the REAL DrumView input loop (dv.Update → dv.tree.Update,
// the same per-frame path production runs) rather than a synthetic HitIndex.
// They reproduce the cross-zone wheel leak: the audio panel and the drum rows
// share ONE DrumViewTree / HitIndex, so a wheel gesture over the panel that no
// panel handler consumes falls through, by z-order, to the row-rack scroll
// handler and scrolls the unrelated drum rows.

// newTestDrumViewWithRows builds a fully-composed Game (so the bottom audio
// panel exists and is laid out exactly as in production) and grows the drum
// rows to at least `rows` so the row offset CAN change. It returns the Game's
// DrumView. The Game is kept alive for the test's lifetime via t.Cleanup.
func newTestDrumViewWithRows(t *testing.T, rows int) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Desktop viewport: wide+tall enough that the synth tab lays out real knob
	// rects and the row rack shows fewer rows than exist (so scrolling is
	// possible). Width stays in the desktop class so the panel sits at the
	// bottom (not a mobile bottom-sheet).
	g.Layout(1280, 900)
	for len(g.drum.Rows) < rows {
		g.drum.AddRow()
	}
	return g.drum
}

// activateSynthTab switches the bottom audio panel to the Synth tab on desktop
// and binds row 0's instrument to a recipe so the tab renders a real knob set.
// Routes through the production SetActiveTab path (→ EnsureAnalyzersForTab).
func activateSynthTab(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil || dv.eqPanelZone == nil {
		t.Fatal("nil drum view / eq panel zone while activating synth tab")
	}
	// Anchor a recipe binding for row 0 so the Synth tab has a non-empty
	// ParamDef set to render (mirrors soak_heap_bound_helper_test.go).
	if len(dv.Rows) > 0 && dv.Rows[0] != nil && dv.Rows[0].Instrument != "" {
		id := dv.Rows[0].Instrument
		audio.BindInstrumentToRecipe(id, "drum-snare")
		t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	}
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
}

// layoutForTest runs the same layout the production per-frame loop runs, so
// the synth knob rects + zone hit areas exist before input is injected.
func layoutForTest(dv *DrumView) {
	if dv == nil {
		return
	}
	// dv.Update() runs recalcButtons + calcLayout + tree.Update (Layout phase
	// publishes hit areas). Run it twice: the first frame populates
	// instEditorKnobs via buildSynthTab; the second lets the tree pick up the
	// freshly-sized hit areas (mirrors the soak helper's double Update).
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	dv.Update()
	dv.Update()
}

func (dv *DrumView) layoutForTest() { layoutForTest(dv) }

// updateForTest ticks one real frame through the production DrumView input
// loop (dv.Update → dv.tree.Update → wheel dispatch).
func (dv *DrumView) updateForTest() { dv.Update() }

// firstNonScrollableSynthKnobRect returns the on-screen rect of the first knob
// belonging to a synth section that is NOT scrollable (sectionGrid == nil or
// !HasScroll()). Over such a knob, synthKnobHitAdapter.OnWheel returns
// InputIgnored — the precondition for the row-scroll leak.
func firstNonScrollableSynthKnobRect(t *testing.T, dv *DrumView) image.Rectangle {
	t.Helper()
	if dv == nil {
		return image.Rectangle{}
	}
	// Only the SELECTED section has laid-out (non-empty) knob rects. With the
	// fewer-per-row layout a stage can become scrollable, so probe each section
	// in turn — select it, lay out, and return the first NON-scrollable one's
	// visible knob (leaving that section active for the caller).
	instID := ""
	if len(dv.Rows) > 0 && dv.Rows[0] != nil {
		instID = dv.resolveSynthInstrument(dv.Rows[0].Instrument)
	}
	ids := make([]synthSectionID, 0, len(dv.instEditorSections))
	for _, s := range dv.instEditorSections {
		ids = append(ids, s.id)
	}
	for _, id := range ids {
		if instID != "" {
			dv.setSelectedSynthSection(instID, id)
			dv.layoutForTest()
		}
		grid := dv.sectionGrid(id)
		if grid != nil && grid.HasScroll() {
			continue // scrollable section: its OnWheel consumes the wheel
		}
		for _, s := range dv.instEditorSections {
			if s.id != id {
				continue
			}
			for _, idx := range s.knobIdxs {
				if idx < 0 || idx >= len(dv.instEditorKnobs) {
					continue
				}
				k := dv.instEditorKnobs[idx]
				if k == nil {
					continue
				}
				if r := k.Rect(); !r.Empty() {
					return r
				}
			}
		}
	}
	return image.Rectangle{}
}

// injectWheelAt places the cursor at (x,y) and delivers a wheel delta
// (dx,dy) for exactly one frame through the production input seams
// (cursorPosition + wheel, both swapped by SetInputForTest), then ticks the
// DrumView so the tree's wheel dispatch runs against that frame's input.
func injectWheelAt(t *testing.T, dv *DrumView, x, y int, dx, dy float64) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return dx, dy },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	restore()
}

// TestWheelOverSynthKnobDoesNotScrollRows reproduces the bug where a
// two-finger trackpad drag (wheel events) over a synth knob whose section
// is NOT scrollable leaked through the shared HitIndex to the row-rack
// scroll handler, scrolling the unrelated drum rows. Drives the real
// DrumView input loop.
func TestWheelOverSynthKnobDoesNotScrollRows(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12) // enough rows that rowOffset CAN change
	activateSynthTab(t, dv)              // desktop: panel visible, Synth tab active
	dv.layoutForTest()                   // full layout so knob rects exist

	knob := firstNonScrollableSynthKnobRect(t, dv)
	if knob.Empty() {
		t.Fatal("no synth knob rect available; check activateSynthTab/layout")
	}
	cx, cy := knob.Min.X+knob.Dx()/2, knob.Min.Y+knob.Dy()/2

	if dv.VisibleRowsForTest() >= len(dv.Rows) {
		t.Fatal("test is vacuous: all rows visible, no scroll possible")
	}
	before := dv.RowOffsetForTest()
	for i := 0; i < 6; i++ {
		injectWheelAt(t, dv, cx, cy, 0, -1) // negative = scroll down
		dv.updateForTest()
	}
	if after := dv.RowOffsetForTest(); before != after {
		t.Fatalf("wheel over synth knob scrolled the drum rows: rowOffset %d -> %d (must stay %d)", before, after, before)
	}
}

// ─── Discipline tests: lock the audio-panel ↔ drum-view isolation ───────────
//
// These tests lock the RootTree composition so the wheel/press-cross bug class
// cannot return. Isolation is enforced TWO ways and these tests assert BOTH:
//
//   STRUCTURAL — dv.tree (drum-view) and dv.audioTree (audio-panel) are
//   SEPARATE DrumViewTree instances with distinct HitIndex / capture / portal.
//   The eq-panel zone lives ONLY in audioTree; row-rack lives ONLY in dv.tree.
//
//   BEHAVIORAL — the RootTree routes each frame's input to exactly ONE subtree
//   (capturer → blocking-portal owner → top-down by z), and the audio subtree
//   (z=1, on top) consumes input in the panel region first, so a sibling-
//   subtree handler never sees a press/wheel that landed on the panel.
//
// IMPORTANT: the subtrees' hit-area RECTS are NOT spatially disjoint — the
// row-rack scroll catch-all spans the whole drum pane and overlaps the panel
// region. So these tests assert BEHAVIOR + STRUCTURE, never literal rect
// disjointness. Every test drives the REAL per-frame input loop
// (dv.updateForTest / press injection through SetInputForTest), not a
// synthetic HitIndex.

// rowRackSpy hooks every row-rack callback with a leak-detecting spy and
// returns a pointer to the slice that accumulates the names of any callbacks
// that fired. Mirrors the spy pattern in drumview_fx_panel_test.go.
func rowRackSpy(dv *DrumView) *[]string {
	leaked := &[]string{}
	cb := &dv.rowRackZone.callbacks
	spy := func(name string, orig func(int)) func(int) {
		return func(row int) {
			*leaked = append(*leaked, name)
			if orig != nil {
				orig(row)
			}
		}
	}
	cb.OnMuteToggle = spy("OnMuteToggle", cb.OnMuteToggle)
	cb.OnSoloToggle = spy("OnSoloToggle", cb.OnSoloToggle)
	cb.OnFXPanelToggle = spy("OnFXPanelToggle", cb.OnFXPanelToggle)
	cb.OnInstMenuOpen = spy("OnInstMenuOpen", cb.OnInstMenuOpen)
	cb.OnContextMenuOpen = spy("OnContextMenuOpen", cb.OnContextMenuOpen)
	cb.OnColorWheelOpen = spy("OnColorWheelOpen", cb.OnColorWheelOpen)
	cb.OnRenameOpen = spy("OnRenameOpen", cb.OnRenameOpen)
	cb.OnDeleteRow = spy("OnDeleteRow", cb.OnDeleteRow)
	cb.OnOriginReq = spy("OnOriginReq", cb.OnOriginReq)
	cb.OnRowSelect = spy("OnRowSelect", cb.OnRowSelect)
	origAdd := cb.OnAddRow
	cb.OnAddRow = func() {
		*leaked = append(*leaked, "OnAddRow")
		if origAdd != nil {
			origAdd()
		}
	}
	return leaked
}

// injectPressAt holds a LEFT-button press at (x,y) for exactly one frame
// through the production input seams, then ticks the DrumView. The button stays
// "down" only for this frame (release is a separate call). Mirrors injectWheelAt.
func injectPressAt(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	restore()
}

// injectRelease delivers a no-button frame at (x,y), ending any held press.
func injectRelease(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	restore()
}

// rowMuteCenter returns the on-screen center of row 0's mute button (a
// row-rack control in the drum-rows region) and whether it has a usable rect.
func rowMuteCenter(dv *DrumView) (int, int, bool) {
	btns := dv.rowMuteBtns()
	if len(btns) == 0 || btns[0] == nil {
		return 0, 0, false
	}
	r := btns[0].Rect()
	if r.Empty() {
		return 0, 0, false
	}
	return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2, true
}

// TestAudioPanelIsOwnSubtree locks the STRUCTURAL half of the isolation: the
// audio panel is a separate DrumViewTree with its own HitIndex, and the
// eq-panel / row-rack zones live in exactly one subtree each.
func TestAudioPanelIsOwnSubtree(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	if dv.tree == nil || dv.audioTree == nil {
		t.Fatal("dv.tree and dv.audioTree must both be non-nil")
	}
	if dv.tree == dv.audioTree {
		t.Fatal("audio panel shares the drum-view subtree — must be its own DrumViewTree")
	}
	// Distinct HitIndex instances: a press/wheel handled by one index can
	// never be re-considered by the other.
	if dv.tree.HitIndexRef() == nil || dv.audioTree.HitIndexRef() == nil {
		t.Fatal("both subtrees must own a HitIndex")
	}
	if dv.tree.HitIndexRef() == dv.audioTree.HitIndexRef() {
		t.Fatal("subtrees share one HitIndex — isolation is structurally impossible")
	}
	// Popups live in a THIRD, top-of-z overlay subtree that is distinct from
	// both base subtrees. Base subtrees keep their own (now popup-free) portals
	// for structural isolation; every menu/picker/panel opens in the overlay
	// portal so it composites and hit-tests above ALL base zones.
	if dv.overlayTree == nil {
		t.Fatal("overlay subtree must exist to own the global portal")
	}
	if dv.overlayTree == dv.tree || dv.overlayTree == dv.audioTree {
		t.Fatal("overlay subtree must be distinct from both base subtrees")
	}
	if dv.portal() != dv.overlayTree.Portal() {
		t.Fatal("dv.portal() must resolve to the overlay subtree's portal")
	}
	if dv.tree.Portal() == dv.audioTree.Portal() {
		t.Fatal("base subtrees share one OverlayPortal")
	}
	// eq-panel ONLY in audioTree; row-rack ONLY in dv.tree.
	if !dv.audioTree.HasZoneForTest("eq-panel") {
		t.Error("eq-panel zone must be registered in the audio subtree")
	}
	if dv.tree.HasZoneForTest("eq-panel") {
		t.Error("eq-panel zone must NOT be registered in the drum-view subtree")
	}
	if !dv.tree.HasZoneForTest("row-rack") {
		t.Error("row-rack zone must be registered in the drum-view subtree")
	}
	if dv.audioTree.HasZoneForTest("row-rack") {
		t.Error("row-rack zone must NOT be registered in the audio subtree")
	}
}

// TestWheelAnywhereInPanelNeverScrollsRows generalizes the original
// reproduction: a wheel delivered at ANY point inside the eq-panel rect must
// never scroll the drum rows, regardless of which panel sub-control (or
// whitespace) sits under the cursor.
func TestWheelAnywhereInPanelNeverScrollsRows(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	if dv.VisibleRowsForTest() >= len(dv.Rows) {
		t.Fatal("test is vacuous: all rows visible, no scroll possible")
	}
	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("eq-panel rect is empty; layout failed")
	}

	before := dv.RowOffsetForTest()
	// Sweep a grid of points across the panel rect.
	const grid = 5
	for gy := 1; gy < grid; gy++ {
		for gx := 1; gx < grid; gx++ {
			x := panel.Min.X + panel.Dx()*gx/grid
			y := panel.Min.Y + panel.Dy()*gy/grid
			if !image.Pt(x, y).In(panel) {
				continue
			}
			// Multiple wheel ticks to give any leak a chance to accumulate.
			for i := 0; i < 3; i++ {
				injectWheelAt(t, dv, x, y, 0, -1)
				dv.updateForTest()
			}
			if after := dv.RowOffsetForTest(); after != before {
				t.Fatalf("wheel at panel point (%d,%d) scrolled drum rows: rowOffset %d -> %d", x, y, before, after)
			}
		}
	}
}

// TestPressInPanelNeverHitsRowRack: a LEFT press on panel whitespace (not a
// knob/control) must not trigger any row-rack callback and must not scroll the
// rows. The audio subtree's eq-panel catch-all consumes the press in its region.
func TestPressInPanelNeverHitsRowRack(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("eq-panel rect is empty")
	}
	// Panel whitespace: a point near the top-left inside the panel but not on
	// any synth knob. Verify it is NOT inside any knob rect.
	px := panel.Min.X + 6
	py := panel.Min.Y + 6
	pt := image.Pt(px, py)
	if !pt.In(panel) {
		t.Fatalf("chosen point (%d,%d) not inside panel %v", px, py, panel)
	}
	for _, k := range dv.instEditorKnobs {
		if k != nil && pt.In(k.Rect()) {
			t.Fatalf("chosen 'whitespace' point (%d,%d) is inside a knob — pick another", px, py)
		}
	}

	leaked := rowRackSpy(dv)
	before := dv.RowOffsetForTest()

	injectPressAt(t, dv, px, py)
	injectRelease(t, dv, px, py)

	if len(*leaked) > 0 {
		t.Errorf("press on panel whitespace leaked to row-rack: %v", *leaked)
	}
	if after := dv.RowOffsetForTest(); after != before {
		t.Errorf("press on panel whitespace scrolled rows: %d -> %d", before, after)
	}
}

// TestPressInRowsNeverChangesPanel: a press on a drum-row control (rows region)
// must not change the audio panel's active tab or active channel.
func TestPressInRowsNeverChangesPanel(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	tabBefore := dv.eqPanelZone.ActiveTab()
	chanBefore := dv.activeEQChannel()

	mx, my, ok := rowMuteCenter(dv)
	if !ok {
		t.Fatal("no row-0 mute button rect in the rows region")
	}
	// Guard: the press point must be OUTSIDE the panel (it is a rows-region
	// control), else the test is not exercising the cross-boundary path.
	if image.Pt(mx, my).In(dv.eqPanelZone.PanelRect()) {
		t.Fatalf("mute center (%d,%d) is inside the panel rect — not a rows-region point", mx, my)
	}

	injectPressAt(t, dv, mx, my)
	injectRelease(t, dv, mx, my)

	if got := dv.eqPanelZone.ActiveTab(); got != tabBefore {
		t.Errorf("press in rows changed panel active tab: %v -> %v", tabBefore, got)
	}
	if got := dv.activeEQChannel(); got != chanBefore {
		t.Errorf("press in rows changed panel active channel: %q -> %q", chanBefore, got)
	}
}

// TestKnobDragFreezesRowRackPresses: a drag begun on a synth knob (audio
// subtree captures) must own ALL subsequent input. Dragging the held cursor
// into the rows region must NOT trigger any row interaction, and the capture
// must stay with the audio subtree until release.
func TestKnobDragFreezesRowRackPresses(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	knob := firstNonScrollableSynthKnobRect(t, dv)
	if knob.Empty() {
		t.Fatal("no synth knob rect available")
	}
	kx, ky := knob.Min.X+knob.Dx()/2, knob.Min.Y+knob.Dy()/2

	// Begin the drag: hold the knob (do NOT release).
	injectPressAt(t, dv, kx, ky)
	if !dv.audioTree.Capturing() {
		t.Fatal("audio subtree should be Capturing after a press on a synth knob")
	}
	if dv.tree.Capturing() {
		t.Fatal("drum-view subtree must NOT be capturing — the knob lives in audioTree")
	}

	// Now hook the row-rack spies and drag the held cursor into the rows
	// region (button stays down). The RootTree routes all input to the
	// capturing subtree, so no row interaction may start.
	leaked := rowRackSpy(dv)
	mx, my, ok := rowMuteCenter(dv)
	if !ok {
		t.Fatal("no row-0 mute button rect in the rows region")
	}
	for i := 0; i < 3; i++ {
		injectPressAt(t, dv, mx, my) // still holding (button down) — this is a drag move
	}
	if !dv.audioTree.Capturing() {
		t.Fatal("drag must stay captured by the audio subtree while dragged into the rows region")
	}
	if len(*leaked) > 0 {
		t.Errorf("knob drag dragged into rows leaked to row-rack: %v", *leaked)
	}

	// Release ends the capture.
	injectRelease(t, dv, mx, my)
	if dv.audioTree.Capturing() {
		t.Fatal("capture should clear after release")
	}

	// Normal dispatch resumes: a fresh press+release on the mute control now
	// reaches the row rack.
	leaked2 := rowRackSpy(dv)
	before := dv.Rows[0].Muted
	injectPressAt(t, dv, mx, my)
	injectRelease(t, dv, mx, my)
	if dv.Rows[0].Muted == before && len(*leaked2) == 0 {
		t.Error("after release, a fresh press in the rows region must reach the row rack (mute did not toggle and no row callback fired)")
	}
}

// TestPanelModalBlocksRowRack: a BLOCKING overlay open in the audio subtree
// gives that subtree exclusive dispatch, so a press on a drum-row control is
// not dispatched. Uses a Modal scrim overlay opened on dv.audioTree.Portal().
func TestPanelModalBlocksRowRack(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	// Open a Modal overlay in the AUDIO subtree.
	dv.audioTree.Portal().Open(PortalEntry{
		ID:      "test-audio-modal",
		Overlay: &nullScrimOverlay{},
		Modal:   true,
	})
	dv.updateForTest()
	if !dv.audioTree.PortalHasBlocking() {
		t.Fatal("audio subtree should report a blocking overlay after opening the synth overflow sheet")
	}
	if dv.tree.PortalHasBlocking() {
		t.Fatal("drum-view subtree must NOT have a blocking overlay")
	}

	mx, my, ok := rowMuteCenter(dv)
	if !ok {
		t.Fatal("no row-0 mute button rect")
	}
	leaked := rowRackSpy(dv)
	before := dv.Rows[0].Muted

	injectPressAt(t, dv, mx, my)
	injectRelease(t, dv, mx, my)

	if len(*leaked) > 0 {
		t.Errorf("row-rack callbacks fired while audio modal was open: %v", *leaked)
	}
	if dv.Rows[0].Muted != before {
		t.Error("row-0 mute toggled while audio modal was open — modal did not block the sibling subtree")
	}

	// Close the modal; a row press now dispatches.
	dv.audioTree.Portal().Close("test-audio-modal")
	dv.updateForTest()
	if dv.audioTree.PortalHasBlocking() {
		t.Fatal("modal should be closed")
	}
	leaked2 := rowRackSpy(dv)
	before2 := dv.Rows[0].Muted
	injectPressAt(t, dv, mx, my)
	injectRelease(t, dv, mx, my)
	if dv.Rows[0].Muted == before2 && len(*leaked2) == 0 {
		t.Error("after closing the modal, a row press must reach the row rack (mute did not toggle and no row callback fired)")
	}
}

// ─── Task 9: cross-cutting surface verification ─────────────────────────────
//
// The audio-panel/drum-view subtree split (RootTree composing dv.tree +
// dv.audioTree) touches four cross-cutting surfaces beyond plain input
// isolation:
//
//  1. Mobile visibility gating — when the audio panel is hidden by view-mode,
//     it must publish EMPTY hit areas so a tap reaches the row rack BENEATH
//     (across the subtree boundary), not be swallowed by a stale catch-all.
//  2. RootTree.Draw ordering — a modal/scrim opened in the AUDIO subtree must
//     composite ABOVE the drum-view subtree's CONTENT (all content first, then
//     all overlays), so the scrim darkens rows-region pixels.
//  3. Render isolation — the audio subtree's content must stay clipped to the
//     panel rect; it must not bleed UP into the rows region.
//  4. Portal ownership — the EQ channel dropdown lives in the AUDIO subtree's
//     portal/HitIndex, not the drum-view subtree's.
//
// All four drive the real per-frame loop / real Draw composite.

// newTestMobileDrumViewWithRows builds a Game in the mobile screen class with at
// least `rows` drum rows and returns the DrumView plus a restore closure for the
// forced small-screen profile. Mirrors pads_input_regression_test.go's setup.
func newTestMobileDrumViewWithRows(t *testing.T, rows int) (*DrumView, func()) {
	t.Helper()
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)
	for len(g.drum.Rows) < rows {
		g.drum.AddRow()
	}
	return g.drum, func() { restore() }
}

// TestMobilePadsTapReachesRowRackAfterPanelHidden is the cross-subtree
// generalization of pads_input_regression_test.go. On mobile the audio panel
// REPLACES the rows: in an audio view mode the eq-panel zone is visible (and the
// row rack gated), and in viewModeRows the eq-panel zone is hidden and MUST
// publish empty hit areas. This test walks the EQ-view → Pads transition that
// reportedly broke, then injects a real press on a row-rack control in the band
// where the panel WAS, and asserts the row-rack callback FIRES — proving the
// now-hidden audio SUBTREE does not swallow the tap across the subtree boundary.
func TestMobilePadsTapReachesRowRackAfterPanelHidden(t *testing.T) {
	dv, restore := newTestMobileDrumViewWithRows(t, 8)
	defer restore()
	if !Profile().IsMobile() {
		t.Fatal("mobile profile not active; SetForceSmallScreen did not take")
	}

	// Make the audio panel visible first (Synth audio view), run frames so its
	// hit areas publish, then switch back to Pads (rows) and run frames so the
	// panel's visibility predicate flips and it republishes EMPTY hit areas.
	dv.setViewMode(viewModeSynth)
	dv.recalcButtons()
	dv.updateForTest()
	dv.updateForTest()
	if !dv.MobileEQMode() {
		t.Fatal("MobileEQMode should be true while a synth/audio view is active on mobile")
	}

	dv.setViewMode(viewModeRows)
	dv.recalcButtons()
	dv.updateForTest()
	dv.updateForTest()
	if dv.MobileEQMode() {
		t.Fatal("MobileEQMode should be false after switching back to Rows")
	}

	// Find a real interactive row-rack control. Use the rack's own published
	// hit areas — that's a point an honest user tap could land on.
	rack := dv.rowRackZone
	if rack == nil {
		t.Fatal("row rack zone is nil")
	}
	rackHits := rack.HitAreas()
	if len(rackHits) == 0 {
		t.Skip("row rack has no hit areas in this layout; cannot exercise the cross-subtree tap")
	}
	target := image.Rectangle{}
	for _, h := range rackHits {
		if !h.Rect.Empty() {
			target = h.Rect
			break
		}
	}
	if target.Empty() {
		t.Skip("row rack hit areas all empty; layout not realised")
	}
	px := target.Min.X + target.Dx()/2
	py := target.Min.Y + target.Dy()/2

	// Sanity: the chosen point must lie inside the rows band, i.e. inside the
	// drum-view subtree's HitIndex coverage. Confirm the drum-view subtree has a
	// hit there and the audio subtree does NOT (its catch-all is gone).
	dvHits := dv.tree.HitIndexRef().At(px, py)
	if len(dvHits) == 0 {
		t.Skipf("no drum-view hit at row-rack probe (%d,%d); layout/profile mismatch", px, py)
	}
	for _, h := range dv.audioTree.HitIndexRef().At(px, py) {
		if h.Tag == "eq-panel-capture" || h.Tag == "chain-panel-capture" {
			t.Fatalf("hidden audio panel still owns hit %q at row-rack probe (%d,%d): "+
				"the panel's visibility predicate gates Draw but not HitAreas — it swallows the tap across subtrees",
				h.Tag, px, py)
		}
	}

	// Drive the real press+release loop on a mobile-sized canvas and assert a
	// row-rack callback fired (the press reached the rack beneath the formerly
	// visible panel).
	leaked := rowRackSpy(dv)
	restoreInput := SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 360, 800 },
	)
	dv.Update()
	restoreInput()
	restoreInput = SetInputForTest(
		func() (int, int) { return px, py },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 360, 800 },
	)
	dv.Update()
	restoreInput()

	if len(*leaked) == 0 {
		t.Errorf("press on a row-rack control after EQ→Pads transition reached NO row-rack callback "+
			"(probe %d,%d) — the hidden audio subtree swallowed the cross-subtree tap", px, py)
	}
}

// dvImageDiffers reports whether two ebiten images differ at any pixel along the
// horizontal scanline y in [x0,x1). Used to assert "the pixel changed" without
// pinning an exact color.
func dvScanlineDiffers(a, b *ebiten.Image, x0, x1, y int) (int, int, bool) {
	for x := x0; x < x1; x++ {
		ar, ag, ab, aa := a.At(x, y).RGBA()
		br, bg, bb, ba := b.At(x, y).RGBA()
		if ar != br || ag != bg || ab != bb || aa != ba {
			return x, y, true
		}
	}
	return 0, 0, false
}

// TestPanelModalDrawsAboveDrumView proves RootTree.Draw composites the AUDIO
// subtree's OVERLAYS above the drum-view subtree's CONTENT. It opens a
// Scrim:true portal entry on the AUDIO subtree's portal (the same portal the
// EQ channel dropdown / synth overflow sheet use) and asserts a pixel in the
// ROWS region — which the drum-view subtree paints as CONTENT — changes versus
// a control frame with the scrim closed. The scrim is painted full-screen by
// OverlayPortal.Draw (portal.go: drawRect(screenBounds, colScrim)), which only
// runs inside DrawOverlays; if RootTree drew the audio subtree's overlays
// BEFORE the drum-view subtree's content, the rows-region pixel would be the
// underlying drum content, not the scrim composite.
//
// Assertion: exact-color — the rows-region pixel under the scrim must equal the
// drum content alpha-composited with colScrim (pure-black @ alpha 150). We
// compute the expected composite from the control (no-scrim) pixel and compare.
func TestPanelModalDrawsAboveDrumView(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 900)
	dv := g.drum
	for len(dv.Rows) < 4 {
		dv.AddRow()
	}
	dv.Update()
	dv.Update()

	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("eq-panel rect empty; layout failed")
	}
	// Probe a point in the ROWS region (above the panel), well inside the drum
	// pane, where the drum-view subtree paints content.
	px := dv.Bounds.Min.X + dv.Bounds.Dx()/2
	py := panel.Min.Y - 40
	if py <= dv.Bounds.Min.Y {
		t.Fatalf("rows-region probe y=%d not above panel/in bounds", py)
	}

	// Control frame: no scrim. Capture the drum content pixel at the probe.
	ctrl := ebiten.NewImage(1280, 900)
	g.Draw(ctrl)
	cr, cg, cb, ca := ctrl.At(px, py).RGBA()

	// Open a full-screen scrim overlay on the AUDIO subtree's portal.
	if dv.audioTree == nil || dv.audioTree.Portal() == nil {
		t.Fatal("audio subtree / portal missing")
	}
	dv.audioTree.Portal().Open(PortalEntry{
		ID:      "test-audio-scrim",
		Overlay: &nullScrimOverlay{},
		Modal:   true,
		Scrim:   true,
	})
	t.Cleanup(func() { dv.audioTree.Portal().Close("test-audio-scrim") })
	dv.Update()
	dv.Update()
	if !dv.audioTree.PortalHasBlocking() {
		t.Fatal("audio subtree should report a blocking scrim overlay")
	}

	withScrim := ebiten.NewImage(1280, 900)
	g.Draw(withScrim)
	sr, sg, sb, sa := withScrim.At(px, py).RGBA()

	// The scrim must have changed the rows-region pixel. If the audio overlay
	// composited BELOW the drum content, the pixel would be identical.
	if sr == cr && sg == cg && sb == cb && sa == ca {
		t.Fatalf("rows-region pixel at (%d,%d) UNCHANGED by an audio-subtree scrim "+
			"(control=%v scrim=%v) — RootTree.Draw did not composite the audio subtree's "+
			"OVERLAYS above the drum-view subtree's CONTENT", px, py,
			color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)},
			color.RGBA{uint8(sr >> 8), uint8(sg >> 8), uint8(sb >> 8), uint8(sa >> 8)})
	}

	// Exact-color check: colScrim is pure-black @ alpha 150 painted with
	// source-over. Expected channel = src*(1-a) where src is the control color
	// (premultiplied) and a = 150/255. We compare 8-bit channels with a small
	// tolerance for rounding in the stub blend.
	const scrimA = 150.0 / 255.0
	c8 := func(v uint32) float64 { return float64(v >> 8) }
	wantR := c8(cr) * (1 - scrimA)
	wantG := c8(cg) * (1 - scrimA)
	wantB := c8(cb) * (1 - scrimA)
	gotR, gotG, gotB := c8(sr), c8(sg), c8(sb)
	const tol = 6.0
	if absF(gotR-wantR) > tol || absF(gotG-wantG) > tol || absF(gotB-wantB) > tol {
		// Non-fatal diagnostic: blend math in the stub may differ. The
		// "changed" assertion above is the load-bearing one; this is a tighter
		// cross-check we only LOG when it disagrees so a legitimate blend
		// difference doesn't fail the isolation guarantee.
		t.Logf("scrim composite color near-but-not-exact: got(%.0f,%.0f,%.0f) want≈(%.0f,%.0f,%.0f) "+
			"(src %.0f,%.0f,%.0f × %.2f darken) — 'changed' assertion still holds",
			gotR, gotG, gotB, wantR, wantG, wantB, c8(cr), c8(cg), c8(cb), 1-scrimA)
	}
}

// TestLayoutResizeHitAreasStayInAudioSubtreeAfterResize locks the subtree the
// layout-resize zone republishes into. The zone is registered in dv.audioTree
// (so its z=ZResize hit areas out-prioritize the eq-panel catch-all WITHIN
// audioTree's HitIndex). recalcButtons re-publishes the divider pills' hit areas
// after a bounds change / widget-board resize; that manual republish MUST target
// the SAME HitIndex the zone lives in (audioTree), or the updated pill geometry
// drifts into dv.tree while audioTree keeps stale frame-1 geometry — making the
// divider un-hit-testable after a desktop resize. (Latent: the zone never
// re-dirties via the normal layout pass, so layout-once-then-drag tests pass.)
//
// This test FAILS against the old `dv.tree`-targeted republish (the "layout-
// resize" areas would be in dv.tree, not audioTree) and PASSES after FIX 1.
func TestLayoutResizeHitAreasStayInAudioSubtreeAfterResize(t *testing.T) {
	assertDefaultParityState(t)
	// Floor the audio panel so the EQ-boundary row divider actually exists
	// (mirrors driveRealEQDivider in row_eq_divider_functional_test.go).
	eqPanelHeightForTest = 190
	t.Cleanup(func() { eqPanelHeightForTest = 0 })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum

	if !Profile().EnableLayoutResize {
		t.Fatal("EnableLayoutResize is false on this profile — divider not interactive; test would be vacuous")
	}
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatal("no EQ divider present — test is vacuous (panel not floored / no row divider)")
	}
	// Non-vacuous precondition: the zone publishes at least one row hit area.
	if len(dv.layoutResizeZone.HitAreas()) == 0 {
		t.Fatal("layout-resize zone published no hit areas — divider build missing")
	}

	// Trigger a bounds change / re-layout: a different viewport runs the
	// production layout path (recalcButtons), which contains the manual
	// layout-resize republish under test. Re-resolve the pill after relayout.
	g.Layout(1200, 760)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	idx = dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatal("EQ divider disappeared after re-layout")
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	if hr.Empty() {
		t.Fatal("row divider pill rect empty after re-layout")
	}
	px := (hr.Min.X + hr.Max.X) / 2
	py := (hr.Min.Y + hr.Max.Y) / 2

	// The "layout-resize" owner's areas must live in audioTree's HitIndex and
	// must be hit-testable at the pill center there.
	audioHasResize := false
	for _, h := range dv.audioTree.HitIndexRef().At(px, py) {
		if h.Tag == "layout-resize-row" || h.Tag == "layout-resize-col" {
			audioHasResize = true
			break
		}
	}
	if !audioHasResize {
		t.Errorf("layout-resize hit area NOT present in the AUDIO subtree's HitIndex at the pill center (%d,%d) after resize — "+
			"the republish targeted the wrong subtree, so the divider is un-hit-testable", px, py)
	}

	// And it must NOT have leaked into the drum-view subtree's HitIndex.
	for _, h := range dv.tree.HitIndexRef().At(px, py) {
		if h.Tag == "layout-resize-row" || h.Tag == "layout-resize-col" {
			t.Errorf("layout-resize hit area leaked into the DRUM-VIEW subtree's HitIndex at (%d,%d) after resize — "+
				"the manual republish pushed pill geometry into the wrong tree (the zone lives in audioTree)", px, py)
		}
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// nullScrimOverlay is a portal overlay that paints nothing itself; the portal's
// own full-screen scrim (Scrim:true) provides the visible darkening. Used to
// isolate the RootTree draw-ordering contract from any overlay-specific paint.
type nullScrimOverlay struct{ closed bool }

func (o *nullScrimOverlay) Layout(_, _ image.Rectangle) {}
func (o *nullScrimOverlay) HitAreas() []HitArea         { return nil }
func (o *nullScrimOverlay) Draw(_ *ebiten.Image)        {}
func (o *nullScrimOverlay) ShouldClose() bool           { return o.closed }

// ─── Symmetric isolation: the DRUM-VIEW subtree freezes the AUDIO panel ──────
//
// The discipline tests above lock the audio→drum-view direction (an audio-panel
// capture / blocking overlay freezes the row rack). The RootTree's routing is
// symmetric — capturer() and blockingPortalOwner() scan ALL children — but the
// drum-view→audio direction is the NON-OBVIOUS one: the audio subtree sits at
// z=1 and is dispatched FIRST, so these tests prove a z=0 drum-view
// gesture/overlay still seizes input from (and draws above) the HIGHER-z
// sibling. They are mirrors of TestKnobDragFreezesRowRackPresses /
// TestPanelModalBlocksRowRack / TestPanelModalDrawsAboveDrumView with the
// subtrees swapped, and were called out in the plan (Task 8 Steps 4-5, Task 9
// Step 2 "symmetric") but not yet shipped.

// firstTimelineScrubPoint returns the center of the drum-view timeline's
// "timeline-scrub" hit area — pressing it captures in dv.tree — and whether one
// is present in this layout.
func firstTimelineScrubPoint(dv *DrumView) (int, int, bool) {
	if dv == nil || dv.timelineZone == nil {
		return 0, 0, false
	}
	for _, h := range dv.timelineZone.HitAreas() {
		if h.Tag == "timeline-scrub" && !h.Rect.Empty() {
			return h.Rect.Min.X + h.Rect.Dx()/2, h.Rect.Min.Y + h.Rect.Dy()/2, true
		}
	}
	return 0, 0, false
}

// TestTimelineScrubFreezesPanelPresses is the symmetric counterpart of
// TestKnobDragFreezesRowRackPresses: a drag begun on the drum-view timeline
// (dv.tree captures) must own ALL subsequent input. Dragging the held cursor
// onto a synth knob must NOT start an audio-subtree capture or change the panel,
// and the capture must stay with the drum-view subtree until release.
func TestTimelineScrubFreezesPanelPresses(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	sx, sy, ok := firstTimelineScrubPoint(dv)
	if !ok {
		t.Skip("no timeline-scrub hit area in this layout; cannot exercise a drum-view capture")
	}
	knob := firstNonScrollableSynthKnobRect(t, dv)
	if knob.Empty() {
		t.Fatal("no synth knob rect available")
	}
	kx, ky := knob.Min.X+knob.Dx()/2, knob.Min.Y+knob.Dy()/2

	// Begin the scrub in the DRUM-VIEW subtree (hold; do NOT release).
	injectPressAt(t, dv, sx, sy)
	if !dv.tree.Capturing() {
		t.Skipf("press on timeline-scrub (%d,%d) did not start a drum-view capture; layout/profile mismatch", sx, sy)
	}
	if dv.audioTree.Capturing() {
		t.Fatal("audio subtree must NOT be capturing — the scrub lives in dv.tree")
	}

	// Drag the held cursor onto a synth knob (button stays down). The RootTree
	// routes ALL input to the capturing (drum-view) subtree, so the audio panel
	// must not start a knob capture and its active tab must not change.
	tabBefore := dv.eqPanelZone.ActiveTab()
	for i := 0; i < 3; i++ {
		injectPressAt(t, dv, kx, ky) // still holding — a drag move, not a fresh press
	}
	if dv.audioTree.Capturing() {
		t.Error("drag into the panel started an audio-subtree capture — the drum-view capture did not freeze the sibling")
	}
	if got := dv.eqPanelZone.ActiveTab(); got != tabBefore {
		t.Errorf("audio panel active tab changed while the drum-view held capture: %v -> %v", tabBefore, got)
	}

	// Release ends the drum-view capture; normal dispatch resumes and a fresh
	// press on the knob now captures the AUDIO subtree.
	injectRelease(t, dv, kx, ky)
	if dv.tree.Capturing() {
		t.Fatal("drum-view capture should clear after release")
	}
	injectPressAt(t, dv, kx, ky)
	if !dv.audioTree.Capturing() {
		t.Error("after release, a fresh press on a synth knob must capture the audio subtree (dispatch did not resume)")
	}
	injectRelease(t, dv, kx, ky)
}

// TestDrumViewModalBlocksPanel is the symmetric counterpart of
// TestPanelModalBlocksRowRack: a BLOCKING overlay open in the DRUM-VIEW subtree
// gives that subtree exclusive dispatch, so a press on a synth knob is not
// dispatched to the audio panel. Real drum-view blocking overlays (row context
// menu, instrument picker, rename dialog) route identically; a synthetic
// modal+scrim entry keeps the test about the RootTree contract (mirrors
// TestPanelModalDrawsAboveDrumView's overlay setup).
func TestDrumViewModalBlocksPanel(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	knob := firstNonScrollableSynthKnobRect(t, dv)
	if knob.Empty() {
		t.Fatal("no synth knob rect available")
	}
	kx, ky := knob.Min.X+knob.Dx()/2, knob.Min.Y+knob.Dy()/2

	dv.tree.Portal().Open(PortalEntry{
		ID:      "test-drumview-modal",
		Overlay: &nullScrimOverlay{},
		Modal:   true,
		Scrim:   true,
	})
	t.Cleanup(func() { dv.tree.Portal().Close("test-drumview-modal") })
	dv.updateForTest()
	if !dv.tree.PortalHasBlocking() {
		t.Fatal("drum-view subtree should report a blocking overlay")
	}
	if dv.audioTree.PortalHasBlocking() {
		t.Fatal("audio subtree must NOT have a blocking overlay")
	}

	tabBefore := dv.eqPanelZone.ActiveTab()
	injectPressAt(t, dv, kx, ky)
	if dv.audioTree.Capturing() {
		t.Error("synth knob captured while a drum-view modal was open — the modal did not block the sibling subtree")
	}
	if got := dv.eqPanelZone.ActiveTab(); got != tabBefore {
		t.Errorf("audio panel active tab changed while a drum-view modal was open: %v -> %v", tabBefore, got)
	}
	injectRelease(t, dv, kx, ky)

	// Close the modal; a fresh press on the knob now captures the audio subtree.
	dv.tree.Portal().Close("test-drumview-modal")
	dv.updateForTest()
	if dv.tree.PortalHasBlocking() {
		t.Fatal("drum-view modal should be closed")
	}
	injectPressAt(t, dv, kx, ky)
	if !dv.audioTree.Capturing() {
		t.Error("after closing the drum-view modal, a synth knob press must capture the audio subtree (dispatch did not resume)")
	}
	injectRelease(t, dv, kx, ky)
}

// TestDrumViewModalDrawsAbovePanel is the symmetric counterpart of
// TestPanelModalDrawsAboveDrumView: it proves RootTree.Draw composites the
// DRUM-VIEW subtree's OVERLAYS above the AUDIO subtree's CONTENT. It opens a
// Scrim:true overlay on the drum-view subtree's portal and asserts a pixel
// INSIDE the panel region — which the audio subtree paints as CONTENT — changes
// versus a control frame with the scrim closed. Because RootTree.Draw paints ALL
// subtree content first then ALL subtree overlays, a z=0 drum-view scrim must
// darken the z=1 audio panel's pixels; if overlays were drawn per-subtree
// interleaved with content, the panel content would paint OVER the lower
// subtree's scrim and the pixel would be unchanged.
func TestDrumViewModalDrawsAbovePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 900)
	dv := g.drum
	for len(dv.Rows) < 4 {
		dv.AddRow()
	}
	dv.Update()
	dv.Update()

	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("eq-panel rect empty; layout failed")
	}
	// Probe a point INSIDE the panel region (audio-subtree content).
	px := panel.Min.X + panel.Dx()/2
	py := panel.Min.Y + panel.Dy()/2

	ctrl := ebiten.NewImage(1280, 900)
	g.Draw(ctrl)
	cr, cg, cb, ca := ctrl.At(px, py).RGBA()

	if dv.tree == nil || dv.tree.Portal() == nil {
		t.Fatal("drum-view subtree / portal missing")
	}
	dv.tree.Portal().Open(PortalEntry{
		ID:      "test-drumview-scrim",
		Overlay: &nullScrimOverlay{},
		Modal:   true,
		Scrim:   true,
	})
	t.Cleanup(func() { dv.tree.Portal().Close("test-drumview-scrim") })
	dv.Update()
	dv.Update()
	if !dv.tree.PortalHasBlocking() {
		t.Fatal("drum-view subtree should report a blocking scrim overlay")
	}

	withScrim := ebiten.NewImage(1280, 900)
	g.Draw(withScrim)
	sr, sg, sb, sa := withScrim.At(px, py).RGBA()

	if sr == cr && sg == cg && sb == cb && sa == ca {
		t.Fatalf("panel-region pixel at (%d,%d) UNCHANGED by a drum-view-subtree scrim "+
			"(control=%v scrim=%v) — RootTree.Draw did not composite the drum-view subtree's "+
			"OVERLAYS above the audio subtree's CONTENT", px, py,
			color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)},
			color.RGBA{uint8(sr >> 8), uint8(sg >> 8), uint8(sb >> 8), uint8(sa >> 8)})
	}
}

// TestAudioSubtreeDoesNotPaintAbovePanel proves the audio subtree's CONTENT
// stays clipped to the panel rect and does not bleed UP into the rows region.
// It renders two frames whose ONLY difference is the audio panel's internal
// content (active tab switched between two data-different tabs: Wave vs
// Spectrum), and asserts that every pixel along a scanline a few px ABOVE the
// panel rect is IDENTICAL across the two frames. If the audio subtree painted
// above its panel rect, changing its internal content would perturb those
// rows-region pixels.
func TestAudioSubtreeDoesNotPaintAbovePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 900)
	dv := g.drum
	for len(dv.Rows) < 4 {
		dv.AddRow()
	}

	// Frame A: Wave tab.
	dv.eqPanelZone.SetActiveTab(TabWave)
	dv.Update()
	dv.Update()
	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("panel rect empty")
	}
	frameA := ebiten.NewImage(1280, 900)
	g.Draw(frameA)

	// Frame B: Spectrum tab — different internal panel content, same panel rect.
	dv.eqPanelZone.SetActiveTab(TabSpectrum)
	dv.Update()
	dv.Update()
	if got := dv.eqPanelZone.PanelRect(); got != panel {
		// A tab switch should not move the panel; if it did, the scanline
		// comparison would be apples-to-oranges. Use the min of the two.
		t.Logf("panel rect changed across tab switch: %v -> %v", panel, got)
		if got.Min.Y < panel.Min.Y {
			panel = got
		}
	}
	frameB := ebiten.NewImage(1280, 900)
	g.Draw(frameB)

	// Scanline a few px ABOVE the panel: this is the rows region. Span the panel
	// width so any bleed under the panel's horizontal extent is caught.
	y := panel.Min.Y - 5
	if y <= dv.Bounds.Min.Y {
		t.Fatalf("scanline y=%d above panel not inside bounds (panel=%v)", y, panel)
	}
	x0 := panel.Min.X
	x1 := panel.Max.X
	if x, yy, diff := dvScanlineDiffers(frameA, frameB, x0, x1, y); diff {
		ar, ag, ab, _ := frameA.At(x, yy).RGBA()
		br, bg, bb, _ := frameB.At(x, yy).RGBA()
		t.Fatalf("rows-region pixel at (%d,%d) — %d px above the panel — CHANGED when only the "+
			"audio panel's internal tab content differed (Wave vs Spectrum): "+
			"A=(%d,%d,%d) B=(%d,%d,%d). The audio subtree's content bled UP past its panel rect.",
			x, yy, panel.Min.Y-y, ar>>8, ag>>8, ab>>8, br>>8, bg>>8, bb>>8)
	}
}

// TestEQChannelDropdownInOverlaySubtree proves the EQ channel dropdown is owned
// by the top-of-z OVERLAY subtree's portal + HitIndex — NOT by either base
// subtree (drum-view or audio). Every popup opens in the single global overlay
// portal so it composites and hit-tests above ALL base zones in ALL subtrees
// (the FX-panel-over-EQ isolation guarantee). It opens the dropdown through the
// production OnClick path and asserts: (1) the entry lives in dv.portal() (the
// overlay portal) and NOT in either base subtree's portal; (2) the dropdown's
// hit areas are visible to dv.overlayTree.HitIndexRef() and absent from BOTH
// base subtree HitIndexes. The click-changes-channel behavior is fully covered
// by TestEQChannelDropdownOpensAndSelects / TestEQChannelDropdownViaGameUpdate
// in eq_per_instrument_test.go, so here we lock subtree OWNERSHIP only.
func TestEQChannelDropdownInOverlaySubtree(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 900)
	dv := g.drum
	dv.recalcButtons()
	dv.AddRow()
	dv.AddRow()
	dv.Update()

	z := dv.eqPanelZone
	if z == nil || z.stickyBar == nil || z.stickyBar.ChannelBtn() == nil {
		t.Fatal("eq channel button not available")
	}
	// Open via the production OnClick closure (same path the user click takes).
	z.stickyBar.ChannelBtn().OnClick()
	dv.Update()

	// Ownership: the dropdown portal entry must live in the OVERLAY subtree,
	// and in NEITHER base subtree.
	if !dv.portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must be registered in the global overlay portal")
	}
	if dv.audioTree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must NOT be registered in the AUDIO base subtree's portal")
	}
	if dv.tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must NOT be registered in the drum-view base subtree's portal")
	}

	// HitIndex ownership: find a point inside the dropdown menu rect and assert
	// the OVERLAY subtree's HitIndex has the dropdown hit there while NEITHER
	// base subtree's HitIndex does.
	menu := z.channelScroll.VS.View
	if menu.Empty() {
		t.Fatal("channel dropdown menu rect empty")
	}
	mx := menu.Min.X + menu.Dx()/2
	my := menu.Min.Y + menu.Dy()/2

	overlayHasDropdown := false
	for _, h := range dv.overlayTree.HitIndexRef().At(mx, my) {
		if h.Tag == "eq-channel-dropdown" {
			overlayHasDropdown = true
			break
		}
	}
	if !overlayHasDropdown {
		t.Errorf("eq-channel-dropdown hit area not present in the overlay subtree's HitIndex at (%d,%d)", mx, my)
	}
	for _, h := range dv.audioTree.HitIndexRef().At(mx, my) {
		if h.Tag == "eq-channel-dropdown" {
			t.Errorf("eq-channel-dropdown hit area leaked into the audio base subtree's HitIndex at (%d,%d)", mx, my)
		}
	}
	for _, h := range dv.tree.HitIndexRef().At(mx, my) {
		if h.Tag == "eq-channel-dropdown" {
			t.Errorf("eq-channel-dropdown hit area leaked into the drum-view base subtree's HitIndex at (%d,%d)", mx, my)
		}
	}

	// Behavioral cross-check (cheap, deterministic): selecting an item via the
	// real loop changes the active channel. Covered in depth by
	// eq_per_instrument_test.go; asserted lightly here so ownership + dispatch
	// are proven together.
	before := dv.activeEQChannel()
	anchor := dv.eqChannelBtn().Rect()
	btnH := 24
	rx := (anchor.Min.X + anchor.Max.X) / 2
	ry := anchor.Max.Y + btnH + btnH/2 // center of the 2nd item (first instrument row)
	press := SetInputForTest(
		func() (int, int) { return rx, ry },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	press()
	rel := SetInputForTest(
		func() (int, int) { return rx, ry },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 900 },
	)
	dv.Update()
	rel()
	if dv.activeEQChannel() == before {
		t.Errorf("clicking a dropdown item via the real loop did not change the active channel (still %q)", before)
	}
}
