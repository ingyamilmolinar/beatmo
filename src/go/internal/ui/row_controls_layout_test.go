//go:build test

package ui

import (
	"image"
	"sort"
	"testing"

	log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// rowControlRects gathers the first row's control button rects (mute, solo,
// fx, menu) plus the label rect, after a layout pass.
type rowControlRects struct {
	label image.Rectangle
	mute  image.Rectangle
	solo  image.Rectangle
	fx    image.Rectangle
	menu  image.Rectangle
}

func firstRowControlRects(t *testing.T, g *Game) rowControlRects {
	t.Helper()
	groups := g.drum.rowRackZone.RowGroups()
	if len(groups) == 0 {
		t.Fatal("no row groups after layout")
	}
	gr := groups[0]
	if gr.Mute == nil || gr.Solo == nil || gr.FX == nil || gr.Menu == nil || gr.Label == nil {
		t.Fatal("row group missing a control button")
	}
	return rowControlRects{
		label: gr.Label.Rect(),
		mute:  gr.Mute.Rect(),
		solo:  gr.Solo.Rect(),
		fx:    gr.FX.Rect(),
		menu:  gr.Menu.Rect(),
	}
}

// TestRowControls_NoCramping_AllProfiles asserts that on both desktop and
// mobile the mute/solo/fx/ellipsis buttons each render at least the comfortable
// RowControlBtnSize, never overlap one another, and leave a non-empty label.
func TestRowControls_NoCramping_AllProfiles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"desktop", false, 1280, 800},
		{"mobile", true, 390, 844},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mobile {
				setupMobileTest(t, true)
			} else {
				assertDefaultParityState(t)
			}
			g := New(log.New(testLogOutput(), log.LevelError))
			t.Cleanup(g.CloseForTest)
			g.Layout(tc.w, tc.h)
			advanceFrames(g, 2)

			rc := firstRowControlRects(t, g)
			minSize := RowControlBtnSize()

			controls := []struct {
				name string
				r    image.Rectangle
			}{
				{"mute", rc.mute},
				{"solo", rc.solo},
				{"fx", rc.fx},
				{"menu(ellipsis)", rc.menu},
			}
			for _, c := range controls {
				if c.r.Empty() {
					t.Errorf("%s button has empty rect", c.name)
					continue
				}
				if c.r.Dx() < minSize {
					t.Errorf("%s width %d < RowControlBtnSize %d (cramped)", c.name, c.r.Dx(), minSize)
				}
			}

			// Controls must not overlap horizontally.
			sort.Slice(controls, func(i, j int) bool {
				return controls[i].r.Min.X < controls[j].r.Min.X
			})
			for i := 1; i < len(controls); i++ {
				if controls[i].r.Min.X < controls[i-1].r.Max.X {
					t.Errorf("control %s (Min.X=%d) overlaps %s (Max.X=%d)",
						controls[i].name, controls[i].r.Min.X,
						controls[i-1].name, controls[i-1].r.Max.X)
				}
			}

			// Label must keep room (non-empty, positive width).
			if rc.label.Empty() || rc.label.Dx() <= 0 {
				t.Errorf("label rect collapsed: %v", rc.label)
			}
		})
	}
}

// TestRowControls_EllipsisVisibleAndRightmost asserts the ellipsis (overflow)
// button is present on BOTH profiles and is the right-most control, sitting to
// the right of the label and the other controls and within bounds.
func TestRowControls_EllipsisVisibleAndRightmost(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"desktop", false, 1280, 800},
		{"mobile", true, 390, 844},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mobile {
				setupMobileTest(t, true)
			} else {
				assertDefaultParityState(t)
			}
			g := New(log.New(testLogOutput(), log.LevelError))
			t.Cleanup(g.CloseForTest)
			g.Layout(tc.w, tc.h)
			advanceFrames(g, 2)

			rc := firstRowControlRects(t, g)
			if rc.menu.Empty() {
				t.Fatalf("ellipsis (menu) button is hidden on %s; want visible", tc.name)
			}
			// Right of every other control and the label.
			for _, other := range []struct {
				name string
				r    image.Rectangle
			}{
				{"label", rc.label},
				{"mute", rc.mute},
				{"solo", rc.solo},
				{"fx", rc.fx},
			} {
				if !other.r.Empty() && rc.menu.Min.X < other.r.Max.X {
					t.Errorf("ellipsis Min.X=%d is left of %s Max.X=%d; ellipsis must be rightmost",
						rc.menu.Min.X, other.name, other.r.Max.X)
				}
			}
			// Within the drum bounds.
			if rc.menu.Max.X > g.drum.Bounds.Max.X {
				t.Errorf("ellipsis Max.X=%d exceeds drum bounds Max.X=%d", rc.menu.Max.X, g.drum.Bounds.Max.X)
			}
		})
	}
}

// TestRowControls_DesktopEllipsisRoomierThanLegacy is a regression witness: the
// desktop ellipsis used to get weight 1/16 (~6% of row width, far below the
// comfortable button size). It must now be at least RowControlBtnSize wide.
func TestRowControls_DesktopEllipsisRoomierThanLegacy(t *testing.T) {
	assertDefaultParityState(t)
	g := New(log.New(testLogOutput(), log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	advanceFrames(g, 2)

	rc := firstRowControlRects(t, g)
	if rc.menu.Empty() {
		t.Fatal("desktop ellipsis button empty")
	}
	if rc.menu.Dx() < RowControlBtnSize() {
		t.Errorf("desktop ellipsis width %d < RowControlBtnSize %d", rc.menu.Dx(), RowControlBtnSize())
	}
}
