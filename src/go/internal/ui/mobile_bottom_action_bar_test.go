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

// TestBottomActionBar_HostsVolViewOverflow verifies that on mobile the
// volume icon, view-switch, and overflow-menu buttons are placed inside
// the DrumView.bottomActionBarRect (not in the top transport toolbar)
// and that each rect meets the minimum touch-target height.
func TestBottomActionBar_HostsVolViewOverflow(t *testing.T) {
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
	cases := []struct {
		name string
		rect image.Rectangle
	}{
		{"mainVolIcon", z.mainVolIconRect},
		{"viewSwitchBtn", z.viewSwitchBtn.Rect()},
		{"overflowBtn", z.overflowBtn.Rect()},
	}
	for _, c := range cases {
		if c.rect.Empty() {
			t.Errorf("%s rect empty on mobile; expected to live in bottom action bar", c.name)
			continue
		}
		if !c.rect.In(bar) {
			t.Errorf("%s rect %v not contained in bottom action bar %v", c.name, c.rect, bar)
		}
		if c.rect.Dy() < TouchMinTarget() {
			t.Errorf("%s height %d below TouchMinTarget %d", c.name, c.rect.Dy(), TouchMinTarget())
		}
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

	// Sample a point inside the bar between the cell rects (in the
	// horizontal padding gap so we don't read a button's fill). With the
	// bar split into 3 cells of 120 px, the gap between cells 0 and 1
	// (around x=120) lies in the inset region between mainVolIcon and
	// viewSwitchBtn.
	cellGapX := bar.Min.X + bar.Dx()/3 // boundary between cell 0 and cell 1
	x := cellGapX
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
