//go:build test

package ui

import (
	"image"
	"testing"
)

// topRightButtonCase is one top-right grid button and the portal menu it toggles.
type topRightButtonCase struct {
	name   string
	rect   func(g *Game) image.Rectangle
	menuID string
}

func topRightButtonCases() []topRightButtonCase {
	return []topRightButtonCase{
		{"settings", (*Game).gridHelpButtonRect, settingsOverlayID},
		{"file", (*Game).gridFileButtonRect, overflowMenuID},
		{"logs", (*Game).gridLogsButtonRect, logsPanelID},
		{"theme", (*Game).gridThemeButtonRect, themeOverlayID},
	}
}

// realTouchPath drives ONE real touch (globalTouchState → touch override →
// gesture detector → Game.Update) that presses at path[0], moves through the
// rest of path (one frame per point) and lifts at the last point.
func realTouchPath(t *testing.T, g *Game, path ...image.Point) {
	t.Helper()
	mock := newMockTouchState()
	mock.addTouch(1, path[0].X, path[0].Y)
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	for _, p := range path {
		mock.moveTouch(1, p.X, p.Y)
		_ = g.Update()
	}
	mock.removeTouch(1)
	// Lift frame fires the GestureTap; settle a few frames after it.
	for i := 0; i < 6; i++ {
		_ = g.Update()
	}
}

func newSettledMobileGame(t *testing.T) *Game {
	t.Helper()
	g := newMobileGame(t)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	return g
}

// TestTopRightButtonsMobileRealTouch: a plain real-touch tap on the middle of
// each top-right button opens its menu and creates no node.
func TestTopRightButtonsMobileRealTouch(t *testing.T) {
	for _, c := range topRightButtonCases() {
		t.Run(c.name, func(t *testing.T) {
			g := newSettledMobileGame(t)
			r := c.rect(g)
			if r.Empty() {
				t.Fatalf("%s button rect empty", c.name)
			}
			before := len(g.graph.Nodes)
			mid := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			realTouchPath(t, g, mid, mid)
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("tap on %s button created a node beneath it (nodes %d -> %d)", c.name, before, after)
			}
			if !g.drum.portal().Has(c.menuID) {
				t.Errorf("tap on %s button did not open its menu %q", c.name, c.menuID)
			}
		})
	}
}

// TestTopRightButtonsMobileEdgeTapWithLiftDrift reproduces the reported mobile
// bug: "the top-right buttons cannot be used — if I tap, a node appears below".
//
// A finger's contact centroid lands a few px BELOW the target the user aims at,
// and drifts a few more px while lifting. The press below lands inside the
// button's 44px touch target (Button already expands its hit rect to
// TouchMinTarget on mobile) but the grid dispatcher routed by the 30px VISIBLE
// rect, so the button never fired; and the release-time GestureTap — routed by
// a separate, narrower "reserved zone" evaluated at the LIFT point — fell into
// the grid and created a node under the button. One gesture, two owners.
//
// Contract: a gesture whose press lands in a button's touch target belongs to
// that button for its whole lifetime — it opens the menu, and nothing beneath
// the button (the grid canvas) sees the tap, wherever the finger lifts.
func TestTopRightButtonsMobileEdgeTapWithLiftDrift(t *testing.T) {
	for _, c := range topRightButtonCases() {
		for _, drift := range []int{0, 4, 9} {
			c, drift := c, drift
			t.Run(c.name, func(t *testing.T) {
				g := newSettledMobileGame(t)
				r := c.rect(g)
				hit := r
				if m := TouchMinTarget(); m > r.Dy() {
					hit = r.Inset(-(m - r.Dy()) / 2)
				}
				// Press inside the touch target, just under the visible rect.
				press := image.Pt(r.Min.X+r.Dx()/2, r.Max.Y+2)
				if !press.In(hit) {
					t.Fatalf("setup: press %v must lie in the %s button's touch target %v", press, c.name, hit)
				}
				before := len(g.graph.Nodes)
				realTouchPath(t, g, press, press.Add(image.Pt(0, drift/2)), press.Add(image.Pt(0, drift)))
				if after := len(g.graph.Nodes); after != before {
					t.Errorf("drift=%d: tap on the %s button's touch target created a node below it (nodes %d -> %d)", drift, c.name, before, after)
				}
				if !g.drum.portal().Has(c.menuID) {
					t.Errorf("drift=%d: tap on the %s button's touch target did not open %q (top=%q)", drift, c.name, c.menuID, g.drum.portal().TopID())
				}
			})
		}
	}
}

// TestTopRightButtonsMobileSecondTapClosesMenu: every top-right button is a
// toggle — a second real tap closes the menu its first tap opened, and neither
// tap creates a node. (The theme button was missing from the toggle-reconcile
// table, so its first press stayed latched and the second tap was swallowed.)
func TestTopRightButtonsMobileSecondTapClosesMenu(t *testing.T) {
	for _, c := range topRightButtonCases() {
		t.Run(c.name, func(t *testing.T) {
			g := newSettledMobileGame(t)
			r := c.rect(g)
			mid := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			before := len(g.graph.Nodes)
			realTouchPath(t, g, mid, mid)
			if !g.drum.portal().Has(c.menuID) {
				t.Fatalf("first tap did not open %q", c.menuID)
			}
			realTouchPath(t, g, mid, mid)
			if g.drum.portal().Has(c.menuID) {
				t.Errorf("second tap on the %s button did not close %q", c.name, c.menuID)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("toggling the %s button created a node (nodes %d -> %d)", c.name, before, after)
			}
		})
	}
}

// TestTopRightClusterOwnsWholeGestureWhenNoMenuStaysOpen pins the gesture
// latch itself: the press edge decides the owner ONCE. A tap that CLOSES a
// button's menu leaves no open menu to shield the grid, and a finger that
// drifts off the button before lifting reports its GestureTap below the
// cluster. That tap belongs to the button that took the press, never to the
// grid canvas beneath. (On mobile every point of the cluster is some button's
// 44px touch target — the targets overlap the 6px gaps — so there is no inert
// gap to probe here.)
func TestTopRightClusterOwnsWholeGestureWhenNoMenuStaysOpen(t *testing.T) {
	const drift = 9 // within the tap slop (tapMaxMovePx=10): still a tap
	for _, c := range topRightButtonCases() {
		t.Run(c.name, func(t *testing.T) {
			g := newSettledMobileGame(t)
			r := c.rect(g)
			mid := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			realTouchPath(t, g, mid, mid)
			if !g.drum.portal().Has(c.menuID) {
				t.Fatalf("%s: setup: first tap did not open %q", c.name, c.menuID)
			}
			before := len(g.graph.Nodes)
			press := image.Pt(mid.X, r.Max.Y+2)
			realTouchPath(t, g, press, press.Add(image.Pt(0, drift/2)), press.Add(image.Pt(0, drift)))
			if g.drum.portal().Has(c.menuID) {
				t.Errorf("%s: closing tap did not close %q", c.name, c.menuID)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("%s: closing tap that drifted off the button created a node below it (nodes %d -> %d)", c.name, before, after)
			}
		})
	}
}
