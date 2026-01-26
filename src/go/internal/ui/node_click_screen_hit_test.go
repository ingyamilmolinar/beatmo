package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"testing"
)

// Clicking within the on-screen rect of a node should select it and open the popup,
// even when the grid snap would map to an adjacent subdivision.
func TestClickNodeByScreenRectOpensPopup(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	// Click slightly off-center to avoid perfect grid-snap alignment
	mx := int((x1+x2)/2) + 3
	my := int((y1+y2)/2) + 2
	left := false
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()
	// Press + release
	left = true
	_ = g.Update()
	left = false
	_ = g.Update()
	if g.sel != n {
		t.Fatalf("expected node selected via screen rect click")
	}
	if !g.nodeMenuOpen || g.nodeMenuNode != n {
		t.Fatalf("expected popup open for clicked node")
	}
}
