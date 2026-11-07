//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Clear of every avoid rect, the corner cluster, nodes and group badges.
func gridPointOutside(t *testing.T, g *Game, avoid ...image.Rectangle) image.Point {
	t.Helper()
	drum := g.split.DrumRect(g.winW, g.winH)
	for y := gridTopOffset() + 40; y < drum.Min.Y-10; y += 17 {
	next:
		for x := 20; x < g.winW-20; x += 23 {
			p := image.Pt(x, y)
			for _, r := range avoid {
				if p.In(r.Inset(-8)) {
					continue next
				}
			}
			if !g.split.InGridPane(x, y) || g.inTopRightButtonZone(x, y) || g.nodeAtScreen(x, y) != nil {
				continue
			}
			if _, ok := g.groupBadgeAt(x, y); ok {
				continue
			}
			return p
		}
	}
	t.Fatalf("no grid point outside %v", avoid)
	return image.Point{}
}

type gridPopupCase struct {
	name   string
	open   func(t *testing.T, g *Game)
	isOpen func(g *Game) bool
	panel  func(g *Game) image.Rectangle
}

// Every portal card menu plus the grid-local pop-ups; the four corner menus are covered separately.
func gridPopupCases() []gridPopupCase {
	var cases []gridPopupCase
	for _, m := range cardMenus() {
		m := m
		cases = append(cases, gridPopupCase{
			name:   "portal-" + m.name,
			open:   func(t *testing.T, g *Game) { m.open(g) },
			isOpen: m.isOpen,
			panel:  m.panel,
		})
	}
	cases = append(cases,
		gridPopupCase{
			name: "node-sidebar",
			open: func(t *testing.T, g *Game) {
				n := g.tryAddNode(3, 3, model.NodeTypeRegular)
				if n == nil {
					t.Fatal("setup: tryAddNode failed")
				}
				g.selectNodeOpenSidebar(n)
			},
			isOpen: func(g *Game) bool { return g.sidebar.IsOpen() },
			panel:  func(g *Game) image.Rectangle { return g.sidebar.InputBounds() },
		},
		gridPopupCase{
			name: "group-menu",
			open: func(t *testing.T, g *Game) {
				a := g.tryAddNode(2, 2, model.NodeTypeRegular)
				b := g.tryAddNode(3, 2, model.NodeTypeRegular)
				if a == nil || b == nil {
					t.Fatal("setup: tryAddNode failed")
				}
				gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
				if err != nil {
					t.Fatalf("setup: CreateGroup: %v", err)
				}
				x1, y1, _, _ := g.nodeScreenRect(a)
				g.openGroupMenuForBadge(gid, int(x1), int(y1))
			},
			isOpen: func(g *Game) bool { return g.groupMenu.IsOpen() },
			panel:  func(g *Game) image.Rectangle { return g.groupMenu.InputBounds() },
		},
	)
	return cases
}

// openPopupForOutsideTap opens c, lays it out, and returns the panel rect.
func openPopupForOutsideTap(t *testing.T, g *Game, c gridPopupCase) image.Rectangle {
	t.Helper()
	c.open(t, g)
	screen := ebiten.NewImage(g.winW, g.winH)
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}
	g.Draw(screen) // comp-style menus lay out in Draw
	_ = g.Update()
	if !c.isOpen(g) {
		t.Fatalf("setup: %s did not open", c.name)
	}
	return c.panel(g)
}

// Mobile has no close ×, so the outside tap IS the close, and the press that dismisses the menu owns the
// gesture: its release-time tap must not create a node.
func TestMobileTopRightMenuOutsideTapInGridClosesAndCreatesNoNode(t *testing.T) {
	panels := map[string]func(g *Game) image.Rectangle{}
	for _, m := range cardMenus() {
		panels[m.name] = m.panel
	}
	for _, c := range topRightButtonCases() {
		t.Run(c.name, func(t *testing.T) {
			g := newSettledMobileGame(t)
			r := c.rect(g)
			mid := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
			realTouchPath(t, g, mid, mid)
			g.Draw(ebiten.NewImage(g.winW, g.winH))
			_ = g.Update()
			if !g.drum.portal().Has(c.menuID) {
				t.Fatalf("setup: tap on %s button did not open %q", c.name, c.menuID)
			}
			panel := panels[c.name](g)
			p := gridPointOutside(t, g, panel)
			before := len(g.graph.Nodes)
			realTouchPath(t, g, p, p)
			if g.drum.portal().Has(c.menuID) {
				t.Errorf("outside tap at %v did not close %q (panel %v)", p, c.menuID, panel)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("outside tap at %v that closed %q created a node beneath it (nodes %d -> %d)", p, c.menuID, before, after)
			}
		})
	}
}

// On both platforms (real touch, real mouse): a grid tap outside any menu closes it and does nothing else.
func TestEveryPopupOutsideGridTapOnlyCloses(t *testing.T) {
	for _, c := range gridPopupCases() {
		c := c
		t.Run("mobile/"+c.name, func(t *testing.T) {
			g := newSettledMobileGame(t)
			panel := openPopupForOutsideTap(t, g, c)
			p := gridPointOutside(t, g, panel)
			before := len(g.graph.Nodes)
			realTouchPath(t, g, p, p)
			if c.isOpen(g) {
				t.Errorf("outside tap at %v did not close %s (panel %v)", p, c.name, panel)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("outside tap at %v with %s open created a node (nodes %d -> %d)", p, c.name, before, after)
			}
		})
		t.Run("desktop/"+c.name, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			advanceFrames(g, 2)
			panel := openPopupForOutsideTap(t, g, c)
			p := gridPointOutside(t, g, panel)
			before := len(g.graph.Nodes)
			tap := makeHoldTap(g, 1280, 720)
			tap(p.X, p.Y, 3)
			advanceFrames(g, 3)
			if c.isOpen(g) {
				t.Errorf("outside click at %v did not close %s (panel %v)", p, c.name, panel)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("outside click at %v with %s open created a node (nodes %d -> %d)", p, c.name, before, after)
			}
		})
	}
}

// A touchscreen on the desktop layout takes the same release-time GestureTap as a phone.
func TestEveryPopupOutsideGridTouchOnDesktopLayoutOnlyCloses(t *testing.T) {
	for _, c := range gridPopupCases() {
		c := c
		t.Run(c.name, func(t *testing.T) {
			assertDefaultParityState(t)
			resetTouchOverride()
			t.Cleanup(resetTouchOverride)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)
			advanceFrames(g, 2)
			panel := openPopupForOutsideTap(t, g, c)
			p := gridPointOutside(t, g, panel)
			before := len(g.graph.Nodes)
			realTouchPath(t, g, p, p)
			if c.isOpen(g) {
				t.Errorf("outside touch at %v did not close %s (panel %v)", p, c.name, panel)
			}
			if after := len(g.graph.Nodes); after != before {
				t.Errorf("outside touch at %v with %s open created a node (nodes %d -> %d)", p, c.name, before, after)
			}
		})
	}
}

// DrumView grabs the dispatcher's capture on a drum-pane hover while a dropdown is open; a capture that
// declines a NEW press must release it, or the press gets no owner and the grid tap creates a node.
func TestPopupOutsideGridTapAfterPointerRestedOnDrumPane(t *testing.T) {
	for _, layout := range []struct {
		name   string
		mobile bool
	}{{"mobile", true}, {"desktop", false}} {
		for _, c := range gridPopupCases() {
			layout, c := layout, c
			t.Run(layout.name+"/"+c.name, func(t *testing.T) {
				var g *Game
				if layout.mobile {
					g = newSettledMobileGame(t)
				} else {
					assertDefaultParityState(t)
					resetTouchOverride()
					t.Cleanup(resetTouchOverride)
					g = New(testLogger)
					t.Cleanup(g.CloseForTest)
					g.Layout(1280, 720)
					advanceFrames(g, 2)
				}
				panel := openPopupForOutsideTap(t, g, c)
				// Pointer rests (unpressed) on the drum pane for a few frames.
				drum := g.drum.Bounds
				hx, hy := drum.Min.X+drum.Dx()/2, drum.Min.Y+drum.Dy()/2
				restore := SetInputForTest(
					func() (int, int) { return hx, hy },
					func(ebiten.MouseButton) bool { return false },
					func(ebiten.Key) bool { return false },
					func() []rune { return nil },
					func() (float64, float64) { return 0, 0 },
					func() (int, int) { return g.winW, g.winH },
				)
				advanceFrames(g, 3)
				restore() // the stub would mask the touch override below
				if !c.isOpen(g) {
					t.Fatalf("setup: hovering the drum pane closed %s", c.name)
				}
				p := gridPointOutside(t, g, panel)
				before := len(g.graph.Nodes)
				realTouchPath(t, g, p, p)
				if c.isOpen(g) {
					t.Errorf("outside tap at %v did not close %s", p, c.name)
				}
				if after := len(g.graph.Nodes); after != before {
					t.Errorf("outside tap at %v with %s open (pointer had rested on the drum pane) created a node (nodes %d -> %d)", p, c.name, before, after)
				}
			})
		}
	}
}
