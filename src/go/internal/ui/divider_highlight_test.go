package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"testing"
)

func TestDividerThickDefaultAndHover(t *testing.T) {
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Default (cursor far from divider)
	img := ebiten.NewImage(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 10, g.split.Y - 20 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
	g.Draw(img)
	restore()
	if g.dividerHover {
		t.Fatalf("divider marked hover away from line")
	}
	if g.dividerThick < 1.5 || g.dividerThick > 2.5 {
		t.Fatalf("divider thick default=%v", g.dividerThick)
	}

	// Hover near divider
	restore = SetInputForTest(
		func() (int, int) { return 10, g.split.Y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
	g.Draw(img)
	restore()
	if !g.dividerHover {
		t.Fatalf("divider not hover near line")
	}
	if g.dividerThick < 2.5 || g.dividerThick > 3.5 {
		t.Fatalf("divider thick hover=%v", g.dividerThick)
	}
}

// ebiten stubs are linked for tests; creating an Image is cheap.
