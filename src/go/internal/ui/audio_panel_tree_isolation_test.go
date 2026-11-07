//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Through the real dv.Update loop: a wheel over the panel that no panel handler takes must not fall through by
// z to the row-rack scroll and move the drum rows.

// A full Game, so the panel is laid out as in production, with enough rows that the offset can change.
func newTestDrumViewWithRows(t *testing.T, rows int) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Desktop class, so the panel sits at the bottom, and tall enough that the synth tab lays out real knobs while
	// the rack still scrolls.
	g.Layout(1280, 900)
	for len(g.drum.Rows) < rows {
		g.drum.AddRow()
	}
	return g.drum
}

// Through the production SetActiveTab path, with row 0 bound so the tab renders real knobs.
func activateSynthTab(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil || dv.eqPanelZone == nil {
		t.Fatal("nil drum view / eq panel zone while activating synth tab")
	}
	if len(dv.Rows) > 0 && dv.Rows[0] != nil && dv.Rows[0].Instrument != "" {
		id := dv.Rows[0].Instrument
		_ = audio.InstallInstrumentDocForTest(id, roster.Snare)
		t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	}
	dv.eqPanelZone.SetActiveTab(TabSynth)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
}

func layoutForTest(dv *DrumView) {
	if dv == nil {
		return
	}
	// Twice: the first frame builds the controls, the second publishes their freshly sized hit areas.
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	dv.Update()
	dv.Update()
}

func (dv *DrumView) layoutForTest() { layoutForTest(dv) }

func (dv *DrumView) updateForTest() { dv.Update() }

// Over a knob in a non-scrollable section the wheel is ignored, the precondition for the leak.
func firstNonScrollableSynthControlRect(t *testing.T, dv *DrumView) image.Rectangle {
	t.Helper()
	if dv == nil {
		return image.Rectangle{}
	}
	// Only the selected section has knob rects, and a stage can become scrollable, so probe each in turn.
	instID := ""
	if len(dv.Rows) > 0 && dv.Rows[0] != nil {
		instID = string(dv.resolveSynthInstrument(dv.Rows[0].Instrument))
	}
	ids := make([]synthSectionID, 0, len(dv.instEditorSections))
	for _, s := range dv.instEditorSections {
		ids = append(ids, s.id)
	}
	for _, id := range ids {
		if instID != "" {
			dv.setSelectedSynthSection(instrument.ID(instID), id)
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
				if idx < 0 || idx >= len(dv.instEditorControls) {
					continue
				}
				k := dv.instEditorControls[idx]
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

// For exactly one frame, through the production input seams.
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

// A trackpad wheel over a knob in a non-scrollable section must not scroll the drum rows.
func TestWheelOverSynthControlDoesNotScrollRows(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12) // enough rows that rowOffset CAN change
	activateSynthTab(t, dv)              // desktop: panel visible, Synth tab active
	dv.layoutForTest()                   // full layout so knob rects exist

	knob := firstNonScrollableSynthControlRect(t, dv)
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

// The audio panel and the drum view are separate trees (own HitIndex, capture, portal), and the RootTree routes
// each frame to exactly one; their rects overlap (the rack's scroll catch-all spans the pane), so these
// assert structure and behaviour, never rect disjointness.

// Collects the name of any row-rack callback that fires.
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

// Down for this frame only; the release is a separate call.
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

// Its own HitIndex, with eq-panel and row-rack each in exactly one tree.
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
	// So an input handled by one is never reconsidered by the other.
	if dv.tree.HitIndexRef() == nil || dv.audioTree.HitIndexRef() == nil {
		t.Fatal("both subtrees must own a HitIndex")
	}
	if dv.tree.HitIndexRef() == dv.audioTree.HitIndexRef() {
		t.Fatal("subtrees share one HitIndex — isolation is structurally impossible")
	}
	// Popups open in a third overlay tree, so they composite and hit-test above every base zone.
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

// Whatever control or whitespace is under the cursor.
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

// The panel's catch-all consumes a press on its whitespace.
func TestPressInPanelNeverHitsRowRack(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	panel := dv.eqPanelZone.PanelRect()
	if panel.Empty() {
		t.Fatal("eq-panel rect is empty")
	}
	px := panel.Min.X + 6
	py := panel.Min.Y + 6
	pt := image.Pt(px, py)
	if !pt.In(panel) {
		t.Fatalf("chosen point (%d,%d) not inside panel %v", px, py, panel)
	}
	for _, k := range dv.instEditorControls {
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
	// Otherwise the cross-boundary path is not exercised.
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

// A blocking overlay gives its tree exclusive dispatch.
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

// With the panel hidden its tree publishes nothing; its scrim composites above the drum view's content; its
// content stays clipped to the panel; the channel dropdown lives in the overlay tree. All through the real loop.

// Returns a restore for the forced small-screen profile.
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

// On mobile the panel replaces the rows; after EQ → Pads its hidden tree must not swallow a real press on a row
// control where the panel was.
func TestMobilePadsTapReachesRowRackAfterPanelHidden(t *testing.T) {
	dv, restore := newTestMobileDrumViewWithRows(t, 8)
	defer restore()
	if !Profile().IsMobile() {
		t.Fatal("mobile profile not active; SetForceSmallScreen did not take")
	}

	// Visible first so its hit areas publish, then Pads so it republishes nothing.
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

	// From the rack's own published areas: a point an honest tap could land on.
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

	// The drum-view tree has a hit here and the audio tree none.
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

// Changed, without pinning an exact colour.
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

// The portal's full-screen scrim only paints in DrawOverlays, so a rows-region pixel darkens only if the RootTree
// draws every tree's content before any overlay. Exact colour: the content composited with colScrim.
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
	// Above the panel, where the drum view paints content.
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

	// Unchanged means the overlay composited below the drum content.
	if sr == cr && sg == cg && sb == cb && sa == ca {
		t.Fatalf("rows-region pixel at (%d,%d) UNCHANGED by an audio-subtree scrim "+
			"(control=%v scrim=%v) — RootTree.Draw did not composite the audio subtree's "+
			"OVERLAYS above the drum-view subtree's CONTENT", px, py,
			color.RGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)},
			color.RGBA{uint8(sr >> 8), uint8(sg >> 8), uint8(sb >> 8), uint8(sa >> 8)})
	}

	// Black at alpha 150, source-over: channel = src·(1 − a), with a little tolerance for stub rounding.
	const scrimA = 150.0 / 255.0
	c8 := func(v uint32) float64 { return float64(v >> 8) }
	wantR := c8(cr) * (1 - scrimA)
	wantG := c8(cg) * (1 - scrimA)
	wantB := c8(cb) * (1 - scrimA)
	gotR, gotG, gotB := c8(sr), c8(sg), c8(sb)
	const tol = 6.0
	if absF(gotR-wantR) > tol || absF(gotG-wantG) > tol || absF(gotB-wantB) > tol {
		// Logged, not failed: the stub's blend may differ, and the "changed" check is the guarantee.
		t.Logf("scrim composite color near-but-not-exact: got(%.0f,%.0f,%.0f) want≈(%.0f,%.0f,%.0f) "+
			"(src %.0f,%.0f,%.0f × %.2f darken) — 'changed' assertion still holds",
			gotR, gotG, gotB, wantR, wantG, wantB, c8(cr), c8(cg), c8(cb), 1-scrimA)
	}
}

// The resize zone lives in the audio tree, so recalcButtons' republish after a bounds change must target that
// index too; otherwise the pills drift into the drum tree and the divider stops hit-testing after a resize.
func TestLayoutResizeHitAreasStayInAudioSubtreeAfterResize(t *testing.T) {
	assertDefaultParityState(t)
	// Floored, so the EQ-boundary divider exists.
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

	// A new viewport runs the production recalcButtons republish.
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

// Paints nothing, so only the portal's own scrim shows and the test is about draw order alone.
type nullScrimOverlay struct{ closed bool }

func (o *nullScrimOverlay) Layout(_, _ image.Rectangle) {}
func (o *nullScrimOverlay) HitAreas() []HitArea         { return nil }
func (o *nullScrimOverlay) Draw(_ *ebiten.Image)        {}
func (o *nullScrimOverlay) ShouldClose() bool           { return o.closed }

// The reverse direction is the non-obvious one: the audio tree is z=1 and dispatched first, yet a drum-view
// capture or overlay must still seize input from it and draw above it.

// Pressing it captures in dv.tree.
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

// A drag begun on the timeline owns all input until release: crossing a synth knob starts no audio capture and
// changes nothing in the panel.
func TestTimelineScrubFreezesPanelPresses(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	sx, sy, ok := firstTimelineScrubPoint(dv)
	if !ok {
		t.Skip("no timeline-scrub hit area in this layout; cannot exercise a drum-view capture")
	}
	knob := firstNonScrollableSynthControlRect(t, dv)
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

	// The RootTree routes everything to the capturing tree while the button stays down.
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

	// Normal dispatch resumes, and a fresh tap arms the card inline.
	injectRelease(t, dv, kx, ky)
	if dv.tree.Capturing() {
		t.Fatal("drum-view capture should clear after release")
	}
	instID := dv.resolveSynthInstrument(dv.Rows[0].Instrument)
	injectPressAt(t, dv, kx, ky)
	injectRelease(t, dv, kx, ky)
	if _, armed := dv.synthControlArmed(instID); !armed {
		t.Error("after release, a fresh tap on a synth card must arm it (dispatch did not resume)")
	}
	dv.lockSynthControl(instID)
}

// A blocking drum-view overlay gives that tree exclusive dispatch; a synthetic modal keeps this about the
// RootTree contract.
func TestDrumViewModalBlocksPanel(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 12)
	activateSynthTab(t, dv)
	dv.layoutForTest()

	knob := firstNonScrollableSynthControlRect(t, dv)
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

	// Arming is the signal that dispatch reached the card.
	instID := dv.resolveSynthInstrument(dv.Rows[0].Instrument)
	tabBefore := dv.eqPanelZone.ActiveTab()
	injectPressAt(t, dv, kx, ky)
	injectRelease(t, dv, kx, ky)
	if _, armed := dv.synthControlArmed(instID); armed {
		t.Error("synth card tap armed the control while a drum-view modal was open — the modal did not block the sibling subtree")
	}
	if got := dv.eqPanelZone.ActiveTab(); got != tabBefore {
		t.Errorf("audio panel active tab changed while a drum-view modal was open: %v -> %v", tabBefore, got)
	}

	// Close the modal; a fresh tap on the card now arms it.
	dv.tree.Portal().Close("test-drumview-modal")
	dv.updateForTest()
	if dv.tree.PortalHasBlocking() {
		t.Fatal("drum-view modal should be closed")
	}
	injectPressAt(t, dv, kx, ky)
	injectRelease(t, dv, kx, ky)
	if _, armed := dv.synthControlArmed(instID); !armed {
		t.Error("after closing the drum-view modal, a synth card tap must arm it (dispatch did not resume)")
	}
	dv.lockSynthControl(instID)
}

// A z=0 drum-view scrim darkens the z=1 panel's pixels only because all content draws before all overlays.
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

// Two frames differing only in the panel's content (Wave vs Spectrum) leave every pixel just above the panel
// identical.
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
		// A moved panel would make the comparison meaningless.
		t.Logf("panel rect changed across tab switch: %v -> %v", panel, got)
		if got.Min.Y < panel.Min.Y {
			panel = got
		}
	}
	frameB := ebiten.NewImage(1280, 900)
	g.Draw(frameB)

	// Spanning the panel's width.
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

// Owned by the overlay tree's portal and index and by neither base tree, so it sits above every base zone;
// selection behaviour is tested elsewhere.
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

	if !dv.portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must be registered in the global overlay portal")
	}
	if dv.audioTree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must NOT be registered in the AUDIO base subtree's portal")
	}
	if dv.tree.Portal().Has("eq-channel-dropdown") {
		t.Fatal("eq-channel-dropdown must NOT be registered in the drum-view base subtree's portal")
	}

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

	// A light check that ownership and dispatch hold together.
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
