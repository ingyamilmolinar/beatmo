//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// With BEATMO_DEBUG_LAYOUT on, the rack/timeline pill is the last thing drawn at its position, never under a
// background fill.
func TestColumnPillVisibleOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	t.Setenv("BEATMO_DEBUG_LAYOUT", "1")
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 800)

	dv := g.drum
	if dv == nil {
		t.Fatal("DrumView not initialized")
	}
	if dv.layoutHandler == nil {
		t.Fatal("layoutHandler not initialized")
	}

	// Get the column pill handle position
	hr := dv.layoutHandler.columnHandleRect(0)
	if hr.Empty() {
		t.Fatal("column handle rect is empty")
	}
	pillCX := (hr.Min.X + hr.Max.X) / 2
	pillCY := (hr.Min.Y + hr.Max.Y) / 2

	// Verify pill center is within drum view bounds
	if !image.Pt(pillCX, pillCY).In(dv.Bounds) {
		t.Fatalf("pill center (%d,%d) outside drum bounds %v", pillCX, pillCY, dv.Bounds)
	}

	// Track drawRect calls with sequence numbers
	type drawCall struct {
		seq    int
		rect   image.Rectangle
		col    color.Color
		filled bool
	}
	var calls []drawCall
	seq := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, drawCall{seq: seq, rect: r, col: c, filled: filled})
		seq++
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	// Draw the full DrumView
	screen := ebiten.NewImage(1200, 800)
	dv.Draw(screen, nil, 0, nil, 0)

	// The handle fills at AlphaStrong over a faint halo, so anything at ≥ AlphaStrong counts and the halo is ignored.
	pillPt := image.Pt(pillCX, pillCY)
	lastFilledSeq := -1
	var lastFilledColor color.Color
	for _, c := range calls {
		if !c.filled {
			continue
		}
		if !pillPt.In(c.rect) {
			continue
		}
		_, _, _, a := c.col.RGBA()
		if a>>8 >= uint32(genAlphaStrong) {
			lastFilledSeq = c.seq
			lastFilledColor = c.col
		}
	}

	if lastFilledSeq < 0 {
		t.Fatal("no opaque filled draw call covers the pill center")
	}

	// The handle is light gold; backgrounds and dividers are dark.
	lr, lg, lb, _ := lastFilledColor.RGBA()
	// Background fills are dark (R,G,B < 50 in 8-bit). The pill handle is light (R,G,B > 150).
	r8 := lr >> 8
	g8 := lg >> 8
	b8 := lb >> 8
	if r8 < 100 && g8 < 100 && b8 < 100 {
		t.Errorf("Column pill handle at (%d,%d) is covered by a dark opaque fill (R=%d,G=%d,B=%d seq=%d); "+
			"the pill should be drawn AFTER row content and rack mask so it remains visible",
			pillCX, pillCY, r8, g8, b8, lastFilledSeq)
	}
}
