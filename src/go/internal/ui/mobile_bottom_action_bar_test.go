//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Anchored to the drum-pane bottom, full width, at least TouchMinTarget tall.
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

// Rows never paint beneath the bar.
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

// Only the view switcher: volume and overflow live in the top transport toolbar.
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
	// The vol icon must still be reachable somewhere on screen (top toolbar).
	if z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect should be non-empty (placed in top toolbar)")
	}
}

// When the bar collapses on an ultra-short viewport its buttons must stay reachable somewhere, or the user
// has no way into the overflow menu.
func TestUltraShortViewport_VolViewOverflowReachable(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// At h=200 the drum pane is capped at 130 px, under the 144 that header + bar + one row need, so the bar
	// collapses; if layout work changes that, shrink the viewport until it does again.
	g.Layout(480, 200)

	// Without the collapse this test guards nothing: shrink the viewport until it collapses again.
	if !g.drum.bottomActionBarRect.Empty() {
		t.Fatalf("precondition: expected bar to collapse at 844x390 landscape; got %v — pick smaller dims", g.drum.bottomActionBarRect)
	}
	z := g.drum.transportZone
	if z == nil {
		t.Fatalf("transportZone nil")
	}
	// The segmented control lives only in the collapsed bar; the volume icon must still be in the top toolbar.
	if z.mainVolIconRect.Empty() {
		t.Errorf("mainVolIconRect unreachable on ultra-short viewport (empty)")
	}
}

// The single-row transport: play/stop/record/bpm/subdiv share one baseline (±1 px of inset rounding).
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
	// Midlines only; exact top/bottom agreement is TestTransportClusterSharesBounds's.
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

// A sheet surface distinct from the background, which groups the bar's controls visually.
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

	// 1 px inside the bar's edge: on its surface, outside the segmented control.
	x := bar.Min.X + 1
	y := bar.Min.Y + bar.Dy()/2
	r1, g1, b1, a1 := screen.At(x, y).RGBA()
	if a1 == 0 {
		t.Fatalf("expected bar surface painted at (%d, %d); got transparent", x, y)
	}

	// Without the surface both samples read the background; the sheet paints colPanelBG.
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

// A touch in the bar's band is not "in the rows area", or row scroll and taps compete with the bar's buttons.
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
