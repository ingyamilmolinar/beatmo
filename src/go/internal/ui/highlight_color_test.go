package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestNodeHighlightUsesRowColor(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.pendingStartRow = -1
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	rowColor := color.RGBA{10, 120, 200, 255}
	g.drum.SetRowColor(0, rowColor)
	start := g.start
	if start == nil {
		start = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if n == nil || start == nil {
		t.Fatalf("failed to add nodes")
	}
	g.addEdge(start, n)
	g.addEdge(n, start)
	g.updateBeatInfos()
	g.nodeAnimSet(n.ID, 1)
	g.quietFrames = 0

	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)

	if len(g.nodeSpriteCache) == 0 {
		t.Fatalf("expected node sprite cache to be populated")
	}
	sx1, _, sx2, _ := g.nodeScreenRect(n)
	rPx := int(math.Round((sx2 - sx1) * 0.5))
	if rPx < 1 {
		rPx = 1
	}
	fr, fg, fb, fa := rgba8(rowColor)
	br, bg, bb, ba := rgba8(rowColor)
	key := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: br, bg: bg, bb: bb, ba: ba}
	if g.nodeSpriteCache[key] == nil {
		t.Fatalf("expected highlight sprite with row-color border; key=%+v", key)
	}
}

// TestGridPaneSilentNodeUsesGreyHighlight verifies that a silent node in the
// grid pane uses colMuteHighlight (grey) for the glow/border, not the row color.
func TestGridPaneSilentNodeUsesGreyHighlight(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.pendingStartRow = -1
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	rowColor := color.RGBA{10, 120, 200, 255}
	g.drum.SetRowColor(0, rowColor)
	start := g.start
	if start == nil {
		start = g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	// Add a SILENT node.
	n := g.tryAddNode(1, 0, model.NodeTypeSilent)
	if n == nil || start == nil {
		t.Fatalf("failed to add nodes")
	}
	g.addEdge(start, n)
	g.addEdge(n, start)
	g.updateBeatInfos()
	g.nodeAnimSet(n.ID, 1)
	g.quietFrames = 0

	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)

	if len(g.nodeSpriteCache) == 0 {
		t.Fatalf("expected node sprite cache to be populated")
	}
	sx1, _, sx2, _ := g.nodeScreenRect(n)
	rPx := int(math.Round((sx2 - sx1) * 0.5))
	if rPx < 1 {
		rPx = 1
	}
	// Silent node should use colMuteHighlight as the border color, not the row color.
	muteHL := colMuteHighlight
	br, bg, bb, ba := rgba8(muteHL)
	// The fill comes from the row color (base instrument color).
	fr, fg, fb, fa := rgba8(rowColor)
	key := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: br, bg: bg, bb: bb, ba: ba}
	if g.nodeSpriteCache[key] == nil {
		// Check if it used row color instead (the bug).
		rbr, rbg, rbb, rba := rgba8(rowColor)
		rowKey := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: rbr, bg: rbg, bb: rbb, ba: rba}
		if g.nodeSpriteCache[rowKey] != nil {
			t.Fatalf("silent node used row color for border — should use colMuteHighlight")
		}
		t.Fatalf("expected highlight sprite with colMuteHighlight border; key=%+v cache_len=%d", key, len(g.nodeSpriteCache))
	}
}

func TestTimelineHighlightUsesWhiteFlash(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.drum.SetInstrument("snare")
	ensureInstrumentAvailable(t, g, "snare")
	rowColor := color.RGBA{80, 40, 180, 255}
	g.drum.SetRowColor(0, rowColor)
	g.drum.SetLength(1)
	g.drum.Rows[0].Steps[0] = true
	g.drum.Offset = 0

	highlighted := [][]highlightEntry{{{idx: 0, val: encodeHighlight(10, false)}}}
	hlExpected := color.RGBAModel.Convert(colHighlight).(color.RGBA)

	var captured []color.RGBA
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		if filled && rgba == hlExpected && r.Min.Y >= g.drum.timelineRect.Min.Y {
			captured = append(captured, rgba)
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	screen := ebiten.NewImage(g.winW, g.winH)
	g.drum.calcLayout()
	g.drum.Draw(screen, highlighted, g.frame, nil, 0)

	if len(captured) == 0 {
		t.Fatalf("expected timeline highlight to use colHighlight (white flash); captured=%v", captured)
	}
}
