//go:build test

package ui

import (
	"image"
	"testing"

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
