package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Test that the cache blit applies (-pad+dx, -pad+dy) so cached content aligns
// with the intended top-pane coordinates across reuse and small pans.
func TestEdgeCacheBlitOffsets(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// One horizontal edge
	a := g.tryAddNode(0, 0, 0)
	b := g.tryAddNode(1, 0, 0)
	g.addEdge(a, b)

	// Capture blit offsets
	blits := [][2]int{}
	old := blitCache
	blitCache = func(dst, src *ebiten.Image, ox, oy int) {
		blits = append(blits, [2]int{ox, oy})
		// still perform the draw to keep state sane
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(ox), float64(oy))
		dst.DrawImage(src, &op)
	}
	defer func() { blitCache = old }()

	img := ebiten.NewImage(800, g.split.Y)
	// First draw: build cache; expect (-pad, -pad)
	g.drawGridPane(img)
	if len(blits) == 0 {
		t.Fatalf("expected a cache blit on first draw")
	}
	pad := g.edgeCachePad
	if blits[0][0] != -pad || blits[0][1] != -pad {
		t.Fatalf("first blit wrong: got (%d,%d) want (%d,%d)", blits[0][0], blits[0][1], -pad, -pad)
	}

	// Clear, pan camera by +5,+3 (within pad), expect reuse with (-pad+dx, -pad+dy)
	blits = blits[:0]
	g.cam.OffsetX += 5
	g.cam.OffsetY += 3
	g.cam.Snap()
	g.drawGridPane(img)
	if len(blits) == 0 {
		t.Fatalf("expected a cache reuse blit after small pan")
	}
	if blits[0][0] != -pad+5 || blits[0][1] != -pad+3 {
		t.Fatalf("reuse blit wrong: got (%d,%d) want (%d,%d)", blits[0][0], blits[0][1], -pad+5, -pad+3)
	}
}
