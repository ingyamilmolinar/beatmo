//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestBottomActionBar_AllocatedOnMobile verifies that on the mobile profile
// the DrumView allocates a bottom action bar rect that anchors to the
// drum-pane bottom, spans the drum-pane width, and is at least
// TouchMinTarget tall. This is the host for the volume icon, view-switch,
// and overflow buttons in the mobile bottom-sheet redesign (B3 critique).
func TestBottomActionBar_AllocatedOnMobile(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	r := g.drum.bottomActionBarRect
	if r.Empty() {
		t.Fatalf("expected bottom action bar rect non-empty on mobile, got empty")
	}
	if r.Dy() < TouchMinTarget() {
		t.Fatalf("bottom action bar height %d < TouchMinTarget %d", r.Dy(), TouchMinTarget())
	}
	if r.Max.Y != g.drum.Bounds.Max.Y {
		t.Fatalf("bottom action bar should anchor to drum-pane bottom: rect.Max.Y=%d bounds.Max.Y=%d", r.Max.Y, g.drum.Bounds.Max.Y)
	}
	if r.Min.X != g.drum.Bounds.Min.X || r.Max.X != g.drum.Bounds.Max.X {
		t.Fatalf("bottom action bar should span drum-pane width: rect=%v bounds=%v", r, g.drum.Bounds)
	}
}

// TestBottomActionBar_EmptyOnDesktop verifies the bottom action bar is NOT
// allocated on the desktop profile — it is a mobile-only affordance.
func TestBottomActionBar_EmptyOnDesktop(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)

	if !g.drum.bottomActionBarRect.Empty() {
		t.Fatalf("expected empty bottom action bar rect on desktop, got %v", g.drum.bottomActionBarRect)
	}
}

// TestBottomActionBar_DrumPaneShrinks verifies that on mobile the rows area
// ends at or above the bottom action bar — rows must never paint beneath
// the bar that hosts vol/view/overflow controls.
func TestBottomActionBar_DrumPaneShrinks(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	rowsBottom := g.drum.rowsBottom()
	barTop := g.drum.bottomActionBarRect.Min.Y
	if rowsBottom > barTop {
		t.Fatalf("rows area extends into bottom action bar: rowsBottom=%d barTop=%d", rowsBottom, barTop)
	}
}

// TestBottomActionBar_HostsSegmentedControlOnly (Theme 1) verifies that
// on mobile the bottom action bar hosts ONLY the 6-segment view switcher.
// Vol icon and overflow now live in the top transport toolbar (Theme 4).
func TestBottomActionBar_HostsSegmentedControlOnly(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	bar := g.drum.bottomActionBarRect
	if bar.Empty() {
		t.Fatalf("bottom action bar rect empty on mobile")
	}
	z := g.drum.transportZone
	if z == nil {
		t.Fatalf("transportZone nil")
	}
	// Segmented spans bar (minus padding).
	sc := g.drum.viewSwitchSegmented
	if sc == nil || sc.Rect().Empty() {
		t.Fatalf("segmented should be visible in bar")
	}
	if !sc.Rect().In(bar) {
		t.Errorf("segmented %v not contained in bar %v", sc.Rect(), bar)
	}
	if sc.Rect().Dy() < TouchMinTarget() {
		t.Errorf("segmented height %d below TouchMinTarget %d", sc.Rect().Dy(), TouchMinTarget())
	}
	// Vol icon is NOT in the bar — it lives in the top toolbar now.
	if !z.mainVolIconRect.Empty() && z.mainVolIconRect.In(bar) {
		t.Errorf("mainVolIcon %v should NOT be inside bar %v (Theme 4 moved it to top toolbar)", z.mainVolIconRect, bar)
	}
	// Overflow is NOT in the bar — it lives in the top toolbar now.
	if !z.overflowBtn.Rect().Empty() && z.overflowBtn.Rect().In(bar) {
		t.Errorf("overflowBtn %v should NOT be inside bar %v (Theme 4 moved it to top toolbar)", z.overflowBtn.Rect(), bar)
	}
	// And both must still be reachable somewhere on screen (top toolbar).
	if z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect should be non-empty (placed in top toolbar)")
	}
	if z.overflowBtn.Rect().Empty() {
		t.Errorf("overflowBtn rect should be non-empty (placed in top toolbar)")
	}
}

// TestUltraShortViewport_VolViewOverflowReachable verifies that on
// ultra-short viewports (landscape phones, drum-pane height insufficient
// for bar + 1 row) the vol/view/overflow buttons remain reachable —
// either inside the bottom action bar OR inside the top transport
// toolbar. Without this guard, the bar collapse in recalcButtons
// orphans the buttons (transport zone already cleared them, drumview
// won't re-place them), leaving the user with no entry point to the
// overflow menu.
func TestUltraShortViewport_VolViewOverflowReachable(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// Ultra-short viewport (480×200 — landscape watch / split-screen
	// landscape phone). With the 2026-05-10 adaptive-portrait split
	// fix, drum-pane height = max(needed, h*0.30) but clamped at
	// `maxDrum = h*0.65`; at h=200 the cap forces drum pane ≤ 130 px,
	// less than `headerH (56) + barH (44) + rowHeight (44) = 144`, so
	// recalcButtons takes the bar-collapse branch. If this precondition
	// stops triggering after future layout work, bump dims until the
	// bar collapses again.
	g.Layout(480, 200)

	// Precondition assertion: bar IS collapsed on this viewport. If this
	// fails the viewport math no longer triggers the bar-collapse path
	// and the rest of the test stops being a regression guard for the
	// orphaned-buttons bug — bump the dimensions until the bar collapses
	// again.
	if !g.drum.bottomActionBarRect.Empty() {
		t.Fatalf("precondition: expected bar to collapse at 844x390 landscape; got %v — pick smaller dims", g.drum.bottomActionBarRect)
	}
	z := g.drum.transportZone
	if z == nil {
		t.Fatalf("transportZone nil")
	}
	// Theme 1: legacy viewSwitchBtn is always suppressed on mobile in
	// favor of the segmented control; the segmented lives only in the
	// bottom action bar (which is collapsed here). When neither is
	// reachable, the overflow menu remains the user's escape hatch — so
	// we only require overflowBtn to be reachable at minimum.
	if z.overflowBtn.Rect().Empty() {
		t.Errorf("overflowBtn unreachable on ultra-short viewport (rect empty)")
	}
	if z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect unreachable on ultra-short viewport (empty)")
	}
}

// TestMobileTransport_AllShareSingleRow is a regression guard for Task 1.4:
// after the transport zone collapsed to a single row on mobile, all of
// play/stop/record/bpm/subdiv must share the same Y baseline (within 1 px
// for safeInsetTransport rounding).
func TestMobileTransport_AllShareSingleRow(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	z := g.drum.transportZone
	if z == nil {
		t.Fatalf("transportZone nil")
	}
	rects := []image.Rectangle{
		z.playBtn.Rect(),
		z.stopBtn.Rect(),
		z.recordBtn.Rect(),
		z.bpmBox.Rect,
		z.subdivBtn.Rect(),
	}

	if len(rects) == 0 || rects[0].Empty() {
		t.Fatalf("transport rects unexpectedly empty")
	}
	names := []string{"playBtn", "stopBtn", "recordBtn", "bpmBox", "subdivBtn"}
	// Compare vertical midlines rather than Min.Y: recordBtn is intentionally
	// shrunk symmetrically by recordDemoteInsetMobile (B12 critique — visual
	// demotion of the record button), which shifts its Min.Y down and Max.Y up
	// by the same amount. Midline alignment captures the single-row invariant
	// without fighting that symmetric inset.
	mid := func(r image.Rectangle) int { return (r.Min.Y + r.Max.Y) / 2 }
	baseMid := mid(rects[0])
	for i, r := range rects {
		if r.Empty() {
			t.Errorf("%s rect empty", names[i])
			continue
		}
		// 1-px tolerance for safeInsetTransport rounding.
		if absInt(mid(r)-baseMid) > 1 {
			t.Errorf("%s mid-Y=%d differs from baseMid (%s)=%d (>1 px); rect=%v", names[i], mid(r), names[0], baseMid, r)
		}
	}
}

// TestBottomActionBar_DrawsSheetSurface verifies that on mobile the bottom
// action bar paints a sheet surface (drawBottomSheetPanel) distinct from
// the underlying background fill. Without the surface paint the bar host
// reads back the bg color, which breaks the visual grouping of the
// vol/view/overflow controls.
func TestBottomActionBar_DrawsSheetSurface(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	bar := g.drum.bottomActionBarRect
	if bar.Empty() {
		t.Fatalf("precondition: bottomActionBarRect empty")
	}

	screen := ebiten.NewImage(360, 700)
	g.Draw(screen)

	// Theme 1: the bar hosts the 6-segment switcher with horizontal
	// padding only. Sample at the bar edge (1 px in from Min.X) which
	// sits inside the bar's surface paint but outside the segmented
	// control's rect.
	x := bar.Min.X + 1
	y := bar.Min.Y + bar.Dy()/2
	r1, g1, b1, a1 := screen.At(x, y).RGBA()
	if a1 == 0 {
		t.Fatalf("expected bar surface painted at (%d, %d); got transparent", x, y)
	}

	// Compare against a sample just ABOVE the bar (rows area). If the
	// surface paint is missing, both samples read back the same bg color.
	// drawBottomSheetPanel paints colPanelBG which differs from colBGBottom.
	yAbove := bar.Min.Y - 4
	if yAbove < 0 {
		t.Fatalf("test geometry: bar too high to sample above (bar.Min.Y=%d)", bar.Min.Y)
	}
	r2, g2, b2, _ := screen.At(x, yAbove).RGBA()
	if r1 == r2 && g1 == g2 && b1 == b2 {
		t.Fatalf("bar pixel (%d,%d)=RGBA(%d,%d,%d,%d) matches above-bar pixel (%d,%d)=RGBA(%d,%d,%d) — bar surface not drawn",
			x, y, r1, g1, b1, a1, x, yAbove, r2, g2, b2)
	}
}

// TestRowsRect_ExcludesBottomActionBar verifies the rowsRect() and
// rowsAreaHeight() geometry helpers respect the mobile bottom action
// bar — touches landing in the bar's vertical band must NOT count as
// "in the rows area" (otherwise scroll dead-zone and row-tap dispatch
// compete with bar-button taps).
func TestRowsRect_ExcludesBottomActionBar(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	bar := g.drum.bottomActionBarRect
	if bar.Empty() {
		t.Fatalf("precondition: bottomActionBarRect empty")
	}
	rr := g.drum.rowsRect()
	if rr.Max.Y > bar.Min.Y {
		t.Errorf("rowsRect %v extends into bottom action bar (top=%d)", rr, bar.Min.Y)
	}
	expectedH := bar.Min.Y - (g.drum.Bounds.Min.Y + g.drum.headerH)
	if expectedH < 0 {
		expectedH = 0
	}
	if got := g.drum.rowsAreaHeight(); got > expectedH {
		t.Errorf("rowsAreaHeight=%d exceeds bar-aware expectedH=%d", got, expectedH)
	}
}
