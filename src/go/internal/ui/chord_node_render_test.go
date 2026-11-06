//go:build test

package ui

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/eventlogger"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// interceptChordIcon counts drawChordIcon calls (the ebitestub does not
// rasterise vector icon paths, so pixels cannot show an icon; the PNG capture
// in the scene catalog is the visual proof).
func interceptChordIcon(t *testing.T) *int {
	t.Helper()
	calls := new(int)
	orig := drawChordIcon
	drawChordIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		*calls++
		orig(dst, r, col)
	}
	t.Cleanup(func() { drawChordIcon = orig })
	return calls
}

func TestIconChordIsRegistered(t *testing.T) {
	calls := interceptChordIcon(t)
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconChord, image.Rect(0, 0, 24, 24), color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("IconChord not registered in drawIconByID")
	}
	if *calls != 1 {
		t.Fatalf("IconChord dispatched to drawChordIcon %d times", *calls)
	}
}

func countNonTransparent(img *ebiten.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0 {
				n++
			}
		}
	}
	return n
}

// A chord node carries the chord glyph on the grid; a regular node gets no
// state overlay at all.
func TestNodeStateOverlayMarksChordNodes(t *testing.T) {
	g := newTestGameForUndo(t)
	calls := interceptChordIcon(t)
	fill, border := color.NRGBA{200, 120, 80, 255}, color.NRGBA{240, 200, 160, 255}
	reg := ebiten.NewImage(64, 64)
	g.drawNodeStateOverlay(reg, 8, 8, 56, 56, model.NodeTypeRegular, fill, border)
	if countNonTransparent(reg) != 0 || *calls != 0 {
		t.Fatal("a regular node must get no state overlay")
	}
	chord := ebiten.NewImage(64, 64)
	g.drawNodeStateOverlay(chord, 8, 8, 56, 56, model.NodeTypeChord, fill, border)
	if *calls != 1 {
		t.Fatalf("a chord node drew the chord glyph %d times, want 1", *calls)
	}
	// A tiny chord node still gets a visible (edge) mark.
	tiny := ebiten.NewImage(16, 16)
	g.drawNodeStateOverlay(tiny, 6, 6, 10, 10, model.NodeTypeChord, fill, border)
	if countNonTransparent(tiny) == 0 {
		t.Fatal("a tiny chord node draws no mark")
	}
}

// The coordinate badge leads with the chord name on a chord node.
func TestCoordBadgeNamesTheChord(t *testing.T) {
	g, node := chordSidebarGame(t, roster.OrganChurch)
	g.setNodeType(node.ID, model.NodeTypeChord)
	g.setNodeChord(node.ID, model.ChordSpec{Quality: "m7", Inversion: 1})
	got := g.coordBadgeText(node)
	want := g.nodeChordLabel(node.ID) + " · "
	if !strings.HasPrefix(got, want) || !strings.Contains(want, "m7/") {
		t.Fatalf("badge %q, want prefix %q (a slash chord name)", got, want)
	}
}

// A sounding chord cell in the drum view carries a mark a regular cell does
// not.
func TestDrumCellMarksChordCells(t *testing.T) {
	fill := color.NRGBA{200, 120, 80, 255}
	reg := ebiten.NewImage(20, 24)
	drawDrumCellTyped(reg, image.Rect(0, 0, 20, 24), true, fill, model.NodeTypeRegular)
	chord := ebiten.NewImage(20, 24)
	drawDrumCellTyped(chord, image.Rect(0, 0, 20, 24), true, fill, model.NodeTypeChord)
	differ := false
	for y := 0; y < 24 && !differ; y++ {
		for x := 0; x < 20; x++ {
			if reg.At(x, y) != chord.At(x, y) {
				differ = true
				break
			}
		}
	}
	if !differ {
		t.Fatal("a chord cell renders identically to a regular cell")
	}
	// An OFF chord cell (not sounding) is not marked.
	offReg := ebiten.NewImage(20, 24)
	drawDrumCellTyped(offReg, image.Rect(0, 0, 20, 24), false, fill, model.NodeTypeRegular)
	offChord := ebiten.NewImage(20, 24)
	drawDrumCellTyped(offChord, image.Rect(0, 0, 20, 24), false, fill, model.NodeTypeChord)
	for y := 0; y < 24; y++ {
		for x := 0; x < 20; x++ {
			if offReg.At(x, y) != offChord.At(x, y) {
				t.Fatal("an off chord cell must look like any off cell")
			}
		}
	}
}

// The played tier freezes a chord cell AS a chord (its type byte), through
// the real playback path: play past the chord cell, then collapse the node —
// the past keeps its chord mark.
func TestPlayedTierKeepsChordCellType(t *testing.T) {
	g := newLaneCircuitGame(t)
	id := chordTestNodeAt(t, g, 2)
	clock := laneTestNow(g)
	tl := g.timelineService()
	for i := 0; i < 2000 && tl.PlayedMax(0) < 4; i++ {
		advanceLaneClock(clock, 0.01)
		_ = g.Update()
	}
	if tl.PlayedMax(0) < 2 {
		t.Fatalf("playback never passed the chord cell (played max %d)", tl.PlayedMax(0))
	}
	g.setNodeType(id, model.NodeTypeRegular)
	if _, typ, ok := tl.Played(0, 2); !ok || typ != model.NodeTypeChord {
		t.Fatalf("played cell type = %s (ok=%v), want chord", typ.String(), ok)
	}
}

// The event log names the chord.
func TestNodeParamsLogLineNamesTheChord(t *testing.T) {
	line := eventlogger.Format(hooks.Event{Kind: hooks.EventNodeParamsChanged,
		Payload: hooks.NodeParamsPayload{ID: 3, Chord: "C#m7"}})
	if !strings.Contains(line, "chord C#m7") {
		t.Fatalf("log line %q does not name the chord", line)
	}
}
