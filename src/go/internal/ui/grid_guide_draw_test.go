//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// setCursorForGuideTest pins the package cursor at (x,y) with no buttons held.
func setCursorForGuideTest(t *testing.T, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
}

func TestGuideCursorLabelShowsFraction(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.1*u)
	setCursorForGuideTest(t, sx, sy)
	advanceFrames(g, 2)
	img := ebiten.NewImage(1280, 720)
	g.Draw(img)
	if !strings.Contains(g.cursorLabel, "1/3") {
		t.Fatalf("cursor label %q must contain the 1/3 span fraction", g.cursorLabel)
	}
}

func TestGuideGhostDrawnAtCommitCell(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	u := g.grid.Unit()
	g.cam.Scale = float64(Profile().DensityValues().GuideSnapRadius) / (0.9 * u)
	sx, sy := guideScreenAt(g, 0.2*u, 4.1*u)
	setCursorForGuideTest(t, sx, sy)
	advanceFrames(g, 2)

	// Intercept drawRect (package function variable) to observe the ghost.
	var rects []image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rects = append(rects, r)
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	img := ebiten.NewImage(1280, 720)
	g.Draw(img)

	// Expected ghost center: screen position of cell (0,4). Filter to
	// ghost-sized rects — large rects (backgrounds, panels) would contain the
	// point trivially and mask a missing ghost.
	cx := int(g.cam.OffsetX)
	cy := int(g.cam.OffsetY+4*u*g.cam.Scale) + gridTopOffset()
	found := false
	for _, r := range rects {
		if r.Dx() <= 60 && r.Dy() <= 60 && image.Pt(cx, cy).In(r.Inset(-2)) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no drawRect covering ghost center (%d,%d); %d rects seen", cx, cy, len(rects))
	}
}

func TestGuideLayerHiddenOutsideGridPane(t *testing.T) {
	g, _, _ := buildGuideCircuit(t, 12)
	setCursorForGuideTest(t, 10, g.winH-10) // deep in the drum pane
	advanceFrames(g, 2)
	if g.guideLayerVisible() {
		t.Fatalf("guide must be hidden while the cursor is outside the grid pane")
	}
}

// TestCursorLabelHiddenOverHigherZSurfaces: the cursor coordinate label is an
// unclipped z=70 layer, so without its own occlusion check it paints on top
// of the sidebar (z=60), the group menu (z=62), and into the settings-gear
// corner — surfaces that render and handle input above the grid. The label
// must only appear over bare grid.
func TestCursorLabelHiddenOverHigherZSurfaces(t *testing.T) {
	g, a, b := buildGuideCircuit(t, 12)

	labelAt := func(x, y int) string {
		restore := SetInputForTest(
			func() (int, int) { return x, y },
			func(btn ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 0, 0 },
		)
		defer restore()
		advanceFrames(g, 2)
		img := ebiten.NewImage(1280, 720)
		g.Draw(img)
		return g.cursorLabel
	}

	// Control: bare grid space shows the label.
	u := g.grid.Unit()
	cx, cy := guideScreenAt(g, 20*u, 10*u)
	if labelAt(cx, cy) == "" {
		t.Fatalf("control: label must show over bare grid")
	}

	// 1. Over the open node sidebar (full-height left panel).
	g.selectNodeOpenSidebar(a)
	sx, sy := 50, gridTopOffset()+60
	if !g.sidebar.IsOpen() || !g.sidebar.Hit(sx, sy) {
		t.Fatalf("setup: probe point must be inside the open sidebar panel")
	}
	if lbl := labelAt(sx, sy); lbl != "" {
		t.Fatalf("label %q must be hidden over the open sidebar", lbl)
	}
	g.sidebar.Close()
	advanceFrames(g, 4) // let the closed-guard decay

	// 2. Over the settings gear button.
	gr := g.gridHelpButtonRect()
	gx, gy := (gr.Min.X+gr.Max.X)/2, (gr.Min.Y+gr.Max.Y)/2
	if lbl := labelAt(gx, gy); lbl != "" {
		t.Fatalf("label %q must be hidden over the settings gear", lbl)
	}

	// 3. Over an open group menu.
	gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	g.groupMenu.OpenAt(gid, 400, gridTopOffset()+20)
	mx, my := 420, gridTopOffset()+40
	if !g.groupMenu.IsOpen() || !g.groupMenu.Hit(mx, my) {
		t.Fatalf("setup: probe point must be inside the open group menu panel")
	}
	if lbl := labelAt(mx, my); lbl != "" {
		t.Fatalf("label %q must be hidden over the open group menu", lbl)
	}
}
