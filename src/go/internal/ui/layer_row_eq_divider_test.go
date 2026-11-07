//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Draw-only chrome over the LayoutResizeHandler, mirroring the grid↔drum splitter.

func TestRowEQDividerLayer_VisibleDesktopHiddenMobile(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	layer := newRowEQDividerLayer(dv)
	if !layer.Visible() {
		t.Fatalf("row-eq divider must be visible on desktop")
	}
	// Must NOT depend on the debug layout-guides flag.
	if Profile().ShowLayoutGuides {
		t.Fatalf("precondition: ShowLayoutGuides should default off on desktop")
	}

	// On mobile, layout resize is disabled, so the divider must not show.
	forceSmallScreenForTest = true
	UpdateProfile()
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	if !Profile().IsMobile() {
		t.Fatalf("precondition: profile should be mobile after forceSmallScreenForTest")
	}
	if layer.Visible() {
		t.Errorf("row-eq divider must be hidden on mobile (layout resize disabled)")
	}
}

func TestRowEQDividerLayer_DrawsFullWidthLineAndPill(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider (full-width audio panel below)")
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	if hr.Empty() {
		t.Fatalf("EQ divider handle rect is empty")
	}
	boundaryY := (hr.Min.Y + hr.Max.Y) / 2
	cx := (hr.Min.X + hr.Max.X) / 2

	layer := newRowEQDividerLayer(dv)
	rec := &drawCallRecorder{}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	rec.record(t, func() { layer.Draw(dst) })

	wantAccent := color.RGBAModel.Convert(colAccent).(color.RGBA)
	width := dv.Bounds.Dx()

	var foundLine, foundPill bool
	for _, c := range rec.calls {
		if c.Kind != drawCallRect {
			continue
		}
		straddlesBoundary := c.Rect.Min.Y <= boundaryY && c.Rect.Max.Y >= boundaryY
		if !straddlesBoundary {
			continue
		}
		// The horizon line spans the full drum-view width and uses colAccent.
		if c.Color == wantAccent && c.Rect.Dx() >= width {
			foundLine = true
		}
		// The pill handle is a narrow rect centered on the boundary.
		if c.Rect.Dx() < width/2 && c.Rect.Min.X <= cx && c.Rect.Max.X >= cx {
			foundPill = true
		}
	}
	if !foundLine {
		t.Errorf("expected a full-width colAccent line at boundary y=%d", boundaryY)
	}
	if !foundPill {
		t.Errorf("expected a centered pill handle at boundary (cx=%d)", cx)
	}
}

// The panel is drawn floor-expanded, so its top edge sits ABOVE the widget-board boundary; the divider belongs on
// the VISIBLE edge, or it draws inside the panel.
func TestRowEQDivider_AnchoredToPanelTopEdge(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}

	widgetBoundaryY := dv.widgets.rowPos[idx+1] + dv.Bounds.Min.Y
	panelTopY := widgetBoundaryY - 80
	dv.eqRect = image.Rect(dv.Bounds.Min.X, panelTopY, dv.Bounds.Max.X, dv.Bounds.Max.Y)
	if panelTopY == widgetBoundaryY {
		t.Fatalf("test precondition: panel top (%d) must differ from widget boundary (%d)", panelTopY, widgetBoundaryY)
	}

	// The handle (single source for draw + hit + hover) must follow the panel top.
	hr := dv.layoutHandler.rowHandleRect(idx)
	handleY := (hr.Min.Y + hr.Max.Y) / 2
	if handleY != panelTopY {
		t.Errorf("rowHandleRect(%d) center y=%d, want panel top edge %d (got widget boundary? %d)",
			idx, handleY, panelTopY, widgetBoundaryY)
	}

	layer := newRowEQDividerLayer(dv)
	rec := &drawCallRecorder{}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	rec.record(t, func() { layer.Draw(dst) })

	wantAccent := color.RGBAModel.Convert(colAccent).(color.RGBA)
	width := dv.Bounds.Dx()
	var lineAtPanelTop, lineAtWidgetBoundary bool
	for _, c := range rec.calls {
		if c.Kind != drawCallRect || c.Color != wantAccent || c.Rect.Dx() < width {
			continue
		}
		if c.Rect.Min.Y <= panelTopY && c.Rect.Max.Y >= panelTopY {
			lineAtPanelTop = true
		}
		if c.Rect.Min.Y <= widgetBoundaryY && c.Rect.Max.Y >= widgetBoundaryY {
			lineAtWidgetBoundary = true
		}
	}
	if !lineAtPanelTop {
		t.Errorf("divider line must be drawn at the audio-panel top edge y=%d", panelTopY)
	}
	if lineAtWidgetBoundary {
		t.Errorf("divider line must NOT be drawn at the widget-board boundary y=%d (inside the panel)", widgetBoundaryY)
	}
}

func TestRowEQDivider_HitAreaAlignsWithVisibleDivider(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}
	panelTopY := dv.widgets.rowPos[idx+1] + dv.Bounds.Min.Y - 80
	dv.eqRect = image.Rect(dv.Bounds.Min.X, panelTopY, dv.Bounds.Max.X, dv.Bounds.Max.Y)

	// The resize zone publishes the grab area from the same handle rect.
	areas := dv.layoutResizeZone.HitAreas()
	var rowArea *HitArea
	for i := range areas {
		if areas[i].Tag == "layout-resize-row" {
			a := areas[i]
			rowArea = &a
			break
		}
	}
	if rowArea == nil {
		t.Fatalf("no layout-resize-row hit area published")
	}
	if !(rowArea.Rect.Min.Y <= panelTopY && rowArea.Rect.Max.Y >= panelTopY) {
		t.Errorf("row grab area %v does not straddle the panel top edge y=%d", rowArea.Rect, panelTopY)
	}

	// Hover/press detection must also resolve at the visible divider Y.
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2
	if axis, gi := dv.layoutHandler.detectRowDivider(cx, panelTopY); axis != "row" || gi != idx {
		t.Errorf("detectRowDivider at panel top (%d,%d) = (%q,%d), want (row,%d)", cx, panelTopY, axis, gi, idx)
	}
}

// A floor-expanded panel 80 px above the widget-board boundary, which the stub (eqPanelHeight=0) cannot produce.
func realisticEQPanel(t *testing.T, dv *DrumView) (idx, panelTopY int) {
	t.Helper()
	idx = dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}
	panelTopY = dv.widgets.rowPos[idx+1] + dv.Bounds.Min.Y - 80
	dv.eqRect = image.Rect(dv.Bounds.Min.X, panelTopY, dv.Bounds.Max.X, dv.Bounds.Max.Y)
	return idx, panelTopY
}

// Through the production hover path, layoutResizeZone.Update.
func TestRowEQDivider_HoverReachesPillAtPanelTop(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx, panelTopY := realisticEQPanel(t, dv)
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	restore := SetInputForTest(
		func() (int, int) { return cx, panelTopY },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	dv.layoutResizeZone.Update()
	if dv.layoutHoverAxis != "row" || dv.layoutHoverIdx != idx {
		t.Errorf("hover at the visible pill (%d,%d) did not register: axis=%q idx=%d, want (row,%d)",
			cx, panelTopY, dv.layoutHoverAxis, dv.layoutHoverIdx, idx)
	}
}

// The grab is the visible pill plus SpaceSM: it covers all the user sees and never spreads over the sticky-bar
// controls below.
func TestRowEQDivider_GrabAreaMatchesVisiblePill(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx, panelTopY := realisticEQPanel(t, dv)

	var rowArea *HitArea
	for _, a := range dv.layoutResizeZone.HitAreas() {
		if a.Tag == "layout-resize-row" {
			aa := a
			rowArea = &aa
		}
	}
	if rowArea == nil {
		t.Fatalf("no layout-resize-row hit area")
	}
	// Must straddle the visible panel top.
	if !(rowArea.Rect.Min.Y <= panelTopY && rowArea.Rect.Max.Y >= panelTopY) {
		t.Errorf("grab area %v does not straddle panel top y=%d", rowArea.Rect, panelTopY)
	}
	pill := dv.layoutHandler.rowHandleRect(idx)
	if pill.Empty() {
		t.Fatalf("EQ boundary pill is empty")
	}
	want := pill.Inset(-SpaceSM)
	if rowArea.Rect != want {
		t.Errorf("grab area %v != visible pill + SpaceSM forgiveness %v (pill=%v)", rowArea.Rect, want, pill)
	}
	// Sanity: the grab covers the full visible pill width/height.
	if rowArea.Rect.Dx() < pill.Dx() || rowArea.Rect.Dy() < pill.Dy() {
		t.Errorf("grab area %v does not fully cover the visible pill %v", rowArea.Rect, pill)
	}
}

// The divider grab sits at z=200 over the real panel, yet the sticky bar's channel and tab pills must still win
// off its handle.
func TestRowEQDivider_GrabDoesNotStealStickyBarControls(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx, panelTopY := realisticEQPanel(t, dv)
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	// Laid out at the floored rect so the sticky-bar hit areas sit on the divider's row.
	dv.eqPanelZone.Layout(dv.eqRect)
	panelAreas := dv.eqPanelZone.HitAreas()
	if len(panelAreas) == 0 {
		t.Fatalf("audio panel published no hit areas; cannot verify non-interference")
	}

	hi := &HitIndex{}
	hi.Update("eq-panel", panelAreas)
	hi.Update("layout-resize", dv.layoutResizeZone.HitAreas())

	// Every sticky-bar control must remain the top hit at its own center.
	checked := 0
	for _, a := range panelAreas {
		if a.Tag != "eq-channel-btn" && !strings.HasPrefix(a.Tag, "eq-tab-") {
			continue
		}
		ccx := (a.Rect.Min.X + a.Rect.Max.X) / 2
		ccy := (a.Rect.Min.Y + a.Rect.Max.Y) / 2
		hits := hi.At(ccx, ccy)
		if len(hits) == 0 {
			t.Errorf("no hit at sticky-bar control %q center (%d,%d)", a.Tag, ccx, ccy)
			continue
		}
		if hits[0].Tag == "layout-resize-row" {
			t.Errorf("divider grab STOLE sticky-bar control %q at (%d,%d) — top hit is layout-resize-row",
				a.Tag, ccx, ccy)
		}
		checked++
	}
	if checked == 0 {
		t.Fatalf("found no eq-tab-*/eq-channel-btn controls to verify against")
	}

	// And the divider still wins at its own handle center.
	hits := hi.At(cx, panelTopY)
	if len(hits) == 0 || hits[0].Tag != "layout-resize-row" {
		got := "none"
		if len(hits) > 0 {
			got = hits[0].Tag
		}
		t.Errorf("divider handle center (%d,%d) top hit = %q, want layout-resize-row", cx, panelTopY, got)
	}
	_ = idx
}

// Points across the whole pill hover; off it, where the tab controls live, nothing does.
func TestRowEQDivider_HoverWorksAcrossHandle(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx, panelTopY := realisticEQPanel(t, dv)
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	pill := dv.layoutHandler.rowHandleRect(idx)
	if pill.Empty() {
		t.Fatalf("EQ boundary pill is empty")
	}
	// Stay safely inside the pill (a few px short of its edges).
	hdx := pill.Dx()/2 - 2
	if hdx < 0 {
		hdx = 0
	}

	probe := func(mx, my int) (string, int) {
		dv.layoutHoverAxis, dv.layoutHoverIdx = "", -1
		restore := SetInputForTest(
			func() (int, int) { return mx, my },
			func(ebiten.MouseButton) bool { return false },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 1280, 720 },
		)
		dv.layoutResizeZone.Update()
		restore()
		return dv.layoutHoverAxis, dv.layoutHoverIdx
	}

	type pt struct{ dx, dy int }
	for _, p := range []pt{{0, 0}, {-hdx, 0}, {hdx, 0}, {0, 4}, {0, -4}} {
		if axis, hidx := probe(cx+p.dx, panelTopY+p.dy); axis != "row" || hidx != idx {
			t.Errorf("on-pill hover at offset (%+d,%+d) did not register (axis=%q idx=%d)",
				p.dx, p.dy, axis, hidx)
		}
	}

	offX := pill.Dx()/2 + SpaceSM + 12
	for _, p := range []pt{{-offX, 0}, {offX, 0}} {
		if axis, hidx := probe(cx+p.dx, panelTopY+p.dy); axis != "" || hidx != -1 {
			t.Errorf("off-pill hover at offset (%+d,%+d) should NOT register (axis=%q idx=%d)",
				p.dx, p.dy, axis, hidx)
		}
	}
}

// The audio-panel catch-all occupies the same region.
func TestRowEQDivider_PressWinsOverPanelAtTop(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	_, panelTopY := realisticEQPanel(t, dv)
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	hi := &HitIndex{}
	// The audio panel publishes a full-bounds catch-all over its rect.
	hi.Update("eq-panel", []HitArea{NewInputCaptureHitArea(dv.eqRect, ZEQPanel, "eq-panel-capture")})
	hi.Update("layout-resize", dv.layoutResizeZone.HitAreas())

	hits := hi.At(cx, panelTopY)
	if len(hits) == 0 {
		t.Fatalf("no hit at divider center (%d,%d)", cx, panelTopY)
	}
	if hits[0].Tag != "layout-resize-row" {
		t.Errorf("top hit at divider center is %q (z=%d), want layout-resize-row to win over the panel catch-all",
			hits[0].Tag, hits[0].ZIndex)
	}
}

func TestRowEQDivider_FollowsPanelTopAcrossRelayout(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}
	layer := newRowEQDividerLayer(dv)
	wantAccent := color.RGBAModel.Convert(colAccent).(color.RGBA)
	width := dv.Bounds.Dx()

	lineY := func() int {
		rec := &drawCallRecorder{}
		dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
		rec.record(t, func() { layer.Draw(dst) })
		for _, c := range rec.calls {
			if c.Kind == drawCallRect && c.Color == wantAccent && c.Rect.Dx() >= width {
				return c.Rect.Min.Y
			}
		}
		return -1
	}

	// Two distinct panel heights → two distinct top edges.
	for _, panelTop := range []int{dv.Bounds.Max.Y - 300, dv.Bounds.Max.Y - 180} {
		dv.eqRect = image.Rect(dv.Bounds.Min.X, panelTop, dv.Bounds.Max.X, dv.Bounds.Max.Y)
		handle := dv.layoutHandler.rowHandleRect(idx)
		handleY := (handle.Min.Y + handle.Max.Y) / 2
		if handleY != panelTop {
			t.Errorf("handle y=%d, want panel top %d", handleY, panelTop)
		}
		if got := lineY(); got != panelTop {
			t.Errorf("drawn line y=%d, want panel top %d", got, panelTop)
		}
	}
}

func TestRowEQDividerLayer_HoverBrightensLine(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}
	// Simulate the resize handler reporting hover over the EQ divider.
	dv.layoutHoverAxis = "row"
	dv.layoutHoverIdx = idx

	layer := newRowEQDividerLayer(dv)
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	// The hover lift eases in over several frames.
	for i := 0; i < 12; i++ {
		layer.Draw(dst)
	}
	rec := &drawCallRecorder{}
	rec.record(t, func() { layer.Draw(dst) })

	wantBright := color.RGBAModel.Convert(colAccentBright).(color.RGBA)
	width := dv.Bounds.Dx()
	var foundBright bool
	for _, c := range rec.calls {
		if c.Kind == drawCallRect && c.Color == wantBright && c.Rect.Dx() >= width {
			foundBright = true
		}
	}
	if !foundBright {
		t.Errorf("expected the accent line to ease up to its bright variant while hovering the EQ divider")
	}
}

// SplitterHandle is shared by the grid↔drum splitter and the EQ divider; both must drive its eased progress.

func TestSplitterHandle_PillGrowsAndBrightensWithHover(t *testing.T) {
	measure := func(anim float64) (glowArea int, glowAlpha uint8) {
		h := SplitterHandle{hoverAnim: anim}
		rec := &drawCallRecorder{}
		dst := ebiten.NewImage(400, 100)
		rec.record(t, func() { h.DrawHorizontalDivider(dst, 0, 400, 50) })
		for _, c := range rec.calls {
			if c.Kind != drawCallRect || c.Rect.Dx() >= 400 {
				continue // skip the full-width horizon-line rects
			}
			if a := c.Rect.Dx() * c.Rect.Dy(); a > glowArea {
				glowArea, glowAlpha = a, c.Color.A
			}
		}
		return
	}
	restArea, restAlpha := measure(0)
	hotArea, hotAlpha := measure(1)
	if restArea == 0 {
		t.Fatalf("no pill/glow rect drawn")
	}
	if hotArea <= restArea {
		t.Errorf("pill should GROW on hover: rest glow area %d, hover %d", restArea, hotArea)
	}
	if hotAlpha <= restAlpha {
		t.Errorf("glow should BRIGHTEN on hover: rest alpha %d, hover %d", restAlpha, hotAlpha)
	}
}

func TestSplitterHandle_AdvanceEasesInThenOut(t *testing.T) {
	var h SplitterHandle
	if h.HoverAnim() != 0 {
		t.Fatalf("rest hover progress = %v, want 0", h.HoverAnim())
	}
	h.Advance(true)
	p1 := h.HoverAnim()
	if p1 <= 0 || p1 >= 1 {
		t.Errorf("after one hovered frame progress = %v, want in (0,1) — must animate, not snap", p1)
	}
	prev := p1
	for i := 0; i < 30; i++ {
		h.Advance(true)
		if cur := h.HoverAnim(); cur < prev {
			t.Errorf("hover progress went backwards while hovered: %v -> %v", prev, cur)
		} else {
			prev = cur
		}
	}
	if h.HoverAnim() < 0.99 {
		t.Errorf("after sustained hover progress = %v, want ~1", h.HoverAnim())
	}
	h.Advance(false)
	if d := h.HoverAnim(); d >= 1 || d <= 0 {
		t.Errorf("first leave frame progress = %v, want in (0,1) — must ease out, not snap", d)
	}
	for i := 0; i < 30; i++ {
		h.Advance(false)
	}
	if h.HoverAnim() > 0.01 {
		t.Errorf("after sustained leave progress = %v, want ~0", h.HoverAnim())
	}
}

func TestSplitterHandle_HoverAnimStaysClamped(t *testing.T) {
	var h SplitterHandle
	for i := 0; i < 100; i++ {
		h.Advance(true)
		if p := h.HoverAnim(); p < 0 || p > 1 {
			t.Fatalf("hover progress escaped [0,1]: %v", p)
		}
	}
	for i := 0; i < 100; i++ {
		h.Advance(false)
		if p := h.HoverAnim(); p < 0 || p > 1 {
			t.Fatalf("hover progress escaped [0,1]: %v", p)
		}
	}
}

func TestMainSplitter_PillAnimatesOnHover(t *testing.T) {
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.split.horizontal = true
	g.split.Y = 360

	restore := SetInputForTest(
		func() (int, int) { return 640, 360 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	if g.split.handle.HoverAnim() != 0 {
		t.Fatalf("splitter hover starts at %v, want 0", g.split.handle.HoverAnim())
	}
	img := ebiten.NewImage(1280, 720)
	g.drawDivider(img)
	if p := g.split.handle.HoverAnim(); p <= 0 || p >= 1 {
		t.Errorf("after one hovered frame splitter hover = %v, want in (0,1)", p)
	}
	for i := 0; i < 12; i++ {
		g.drawDivider(img)
	}
	if p := g.split.handle.HoverAnim(); p < 0.99 {
		t.Errorf("sustained hover should settle the splitter pill at ~1, got %v", p)
	}
}

func TestRowEQDivider_PillAnimatesOnHover(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("no EQ divider")
	}
	dv.layoutHoverAxis, dv.layoutHoverIdx = "row", idx

	layer := dv.rowEQDividerLayer()
	if layer.handle.HoverAnim() != 0 {
		t.Fatalf("EQ divider hover starts at %v, want 0", layer.handle.HoverAnim())
	}
	dst := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	for i := 0; i < 12; i++ {
		layer.Draw(dst)
	}
	if p := layer.handle.HoverAnim(); p < 0.99 {
		t.Errorf("sustained hover should settle the EQ pill at ~1, got %v", p)
	}
	dv.layoutHoverAxis, dv.layoutHoverIdx = "", -1
	for i := 0; i < 12; i++ {
		layer.Draw(dst)
	}
	if p := layer.handle.HoverAnim(); p > 0.01 {
		t.Errorf("after leaving, EQ pill should ease back to ~0, got %v", p)
	}
}
