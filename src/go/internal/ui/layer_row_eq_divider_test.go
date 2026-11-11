//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// The production divider between the drum-rows panel and the audio-panel (EQ /
// tabs) section is draw-only chrome over the already-wired LayoutResizeHandler.
// It mirrors the main grid↔drum splitter's visuals (drawDivider): a full-width
// azure horizon line + a centered pill handle, brightening on hover. These
// tests pin its visibility gating and its draw output. The drag/resize behavior
// itself is covered by layout_resize_test.go +
// drumview_layout_resize_handler_test.go.

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

// TestRowEQDivider_AnchoredToPanelTopEdge is the load-bearing positioning
// guard. In production the audio panel is rendered with a floor-expanded height
// (PanelHeightAt), so its real top edge (dv.eqRect.Min.Y == Bounds.Max.Y - eqH)
// sits ABOVE the widget-board row boundary (widgets.rowPos[2]). The divider must
// sit on the VISIBLE boundary — the panel's top edge — not the widget-board
// boundary, otherwise it draws inside the panel content (the reported bug).
func TestRowEQDivider_AnchoredToPanelTopEdge(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()

	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		t.Fatalf("expected an EQ row divider")
	}

	// Simulate the production condition: a floor-expanded audio panel whose top
	// edge sits 80px ABOVE the widget-board boundary.
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

	// The drawn line must be at the panel top, NOT inside the panel at the
	// widget boundary.
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

// TestRowEQDivider_HitAreaAlignsWithVisibleDivider ensures the grab zone moves
// with the visible line — you can grab the divider where you see it.
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

// TestRowEQDivider_FollowsPanelTopAcrossRelayout asserts the divider tracks the
// audio panel's top edge as the panel is resized: when dv.eqRect changes (which
// the production drag path updates via recalcButtons), the visible line and the
// handle follow it — no stale position left behind.
// realisticEQPanel sets dv.eqRect to a floor-expanded audio panel whose top
// edge sits 80px above the widget-board boundary — the production condition the
// test stub (eqPanelHeight=0) otherwise can't reproduce. Returns the panel top Y.
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

// TestRowEQDivider_HoverReachesPillAtPanelTop drives the PRODUCTION hover path
// (layoutResizeZone.Update) with the cursor on the visible pill at the panel
// top edge. The user reports hover never fires there.
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

// TestRowEQDivider_GrabAreaMatchesVisiblePill asserts the divider's grab target
// is the VISIBLE pill plus the standard SpaceSM forgiveness — no more. It must
// cover everything the user can see (so the affordance is honest) but must NOT
// balloon into a wide band that bleeds over the sticky-bar controls below the
// divider (the invasive-hover complaint).
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
	// The grab is exactly the visible pill expanded by SpaceSM — it covers the
	// whole pill (honest affordance) and nothing wider (not invasive).
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

// TestRowEQDivider_GrabDoesNotStealStickyBarControls lays out the REAL audio
// panel at the floored top edge and proves the enlarged divider grab (z=200)
// never wins over the sticky-bar's channel pill or tab pills (lower z) — those
// controls stay clickable, while the divider still wins at its own handle.
func TestRowEQDivider_GrabDoesNotStealStickyBarControls(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDrumView(t, 1280, 720)
	dv.refreshWidgetLayout()
	idx, panelTopY := realisticEQPanel(t, dv)
	cx := (dv.Bounds.Min.X + dv.Bounds.Max.X) / 2

	// Lay the audio panel out at the floored rect so its sticky-bar tab/channel
	// hit areas are published at the divider's row.
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

// TestRowEQDivider_HoverWorksAcrossHandle drives the production hover path at
// several points across the visible handle (not just the exact centre), so the
// divider is forgiving to hover within the pill — while OFF the pill (where the
// sticky-bar tab controls live) hover must NOT register. The on-pill offsets are
// kept inside the pill's half-extents so they exercise the whole visible handle.
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

	// Off the pill — well past the SpaceSM forgiveness, where the sticky-bar tab
	// controls sit — hover must NOT register.
	offX := pill.Dx()/2 + SpaceSM + 12
	for _, p := range []pt{{-offX, 0}, {offX, 0}} {
		if axis, hidx := probe(cx+p.dx, panelTopY+p.dy); axis != "" || hidx != -1 {
			t.Errorf("off-pill hover at offset (%+d,%+d) should NOT register (axis=%q idx=%d)",
				p.dx, p.dy, axis, hidx)
		}
	}
}

// TestRowEQDivider_PressWinsOverPanelAtTop ensures a press on the divider at the
// panel top edge resolves to the resize handler, not the audio-panel catch-all
// that occupies the same region.
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
	// The hover lift is now animated, so the accent line eases up to its bright
	// variant over several frames — draw enough to let it settle.
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

// ── Shared animated SplitterHandle (glow + grow on hover) ──
// SplitterHandle is reused by BOTH the main grid↔drum splitter and the EQ
// divider. These pin the eased hover progress and that both consumers drive it.

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
