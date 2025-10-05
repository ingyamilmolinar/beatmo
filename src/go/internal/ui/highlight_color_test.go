package ui

import (
	"image"
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestNodeHighlightUsesRowColor(t *testing.T) {
	prev := os.Getenv("RENDER_SAFE")
	if err := os.Setenv("RENDER_SAFE", "1"); err != nil {
		t.Fatalf("setenv: %v", err)
	}
	defer func() {
		if prev == "" {
			os.Unsetenv("RENDER_SAFE")
		} else {
			os.Setenv("RENDER_SAFE", prev)
		}
	}()

	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)
	g.drum.instMu.Lock()
	g.drum.instAvail = map[string]bool{"snare": true}
	g.drum.instMu.Unlock()
	g.drum.Rows[0].Instrument = "snare"
	rowColor := color.RGBA{10, 120, 200, 255}
	g.drum.Rows[0].Color = rowColor
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatalf("failed to add node")
	}
	g.nodeAnim[n.ID] = 1

	var captured []color.RGBA
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		if !filled && rgba == rowColor {
			captured = append(captured, rgba)
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	screen := ebiten.NewImage(g.winW, g.winH)
	g.Draw(screen)

	if len(captured) == 0 {
		t.Fatalf("expected node highlight to use row color; captured=%v", captured)
	}
}

func TestTimelineHighlightUsesRowColor(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.SetUseSequencerForTest(false)
	g.drum.instMu.Lock()
	g.drum.instAvail = map[string]bool{"snare": true}
	g.drum.instMu.Unlock()
	g.drum.Rows[0].Instrument = "snare"
	rowColor := color.RGBA{80, 40, 180, 255}
	g.drum.Rows[0].Color = rowColor
	g.drum.Rows[0].Steps = []bool{true}
	g.drum.Length = 1
	g.drum.Offset = 0

	highlighted := map[int]int64{makeBeatKey(0, 0): encodeHighlight(10, false)}

	var captured []color.RGBA
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		rgba := color.RGBAModel.Convert(c).(color.RGBA)
		if filled && rgba == rowColor && r.Min.Y >= g.drum.timelineRect.Min.Y {
			captured = append(captured, rgba)
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	screen := ebiten.NewImage(g.winW, g.winH)
	g.drum.calcLayout()
	g.drum.Draw(screen, highlighted, g.frame, nil, 0)

	if len(captured) == 0 {
		t.Fatalf("expected timeline highlight to use row color; captured=%v", captured)
	}
}
