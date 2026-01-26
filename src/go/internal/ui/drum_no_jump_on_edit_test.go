package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"image"
	"image/color"
	"testing"
)

// Ensure that editing a circuit during active playback does not change the
// drum view offset or the yellow timeline cursor position; only the future
// portion of the affected row may change.
func TestNoJumpOnEditDuringPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// 1x1 loop
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(1, 1, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Freeze header scale during playback.
	g.SetPlaying(true)

	// Choose a synthetic elapsed position in the middle for Draw.
	elapsed := 5.0

	// Capture initial cursor X by intercepting drawRect for colTimelineCursor.
	dst := ebiten.NewImage(800, 240)
	var curX1, curX2 int
	orig := drawRect
	t.Cleanup(func() { drawRect = orig })
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if color.RGBAModel.Convert(c).(color.RGBA) == colTimelineCursor {
				curX1 = r.Min.X
			}
		}
		orig(dst, r, c, filled)
	}
	g.drum.Draw(dst, map[int]int64{}, 0, nil, elapsed)
	drawRect = orig
	offBefore := g.drum.Offset

	// Edit: silence b; recompute paths.
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		n.Type = model.NodeTypeSilent
		g.graph.Nodes[b.ID] = n
		g.notifyPredictorNode(b.ID)
	}
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Draw again with the same elapsed value; cursor should be identical and
	// offset must not change.
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if color.RGBAModel.Convert(c).(color.RGBA) == colTimelineCursor {
				curX2 = r.Min.X
			}
		}
		orig(dst, r, c, filled)
	}
	g.drum.Draw(dst, map[int]int64{}, 0, nil, elapsed)
	drawRect = orig

	if curX1 != curX2 {
		t.Fatalf("timeline cursor jumped on edit: %d -> %d", curX1, curX2)
	}
	if g.drum.Offset != offBefore {
		t.Fatalf("drum view offset changed on edit: %d -> %d", offBefore, g.drum.Offset)
	}
}
