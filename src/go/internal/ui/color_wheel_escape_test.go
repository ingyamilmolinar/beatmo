package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestColorWheelEscCancels(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 640, 200), nil, game_log.New(nil, game_log.LevelError))
	dv.calcLayout()
	before := dv.colorKey(dv.Rows[0].Color)
	// Open wheel
	dv.rowColorBtns[0].OnClick()
	dv.Update()
	if !dv.colorMenuOpen {
		t.Fatalf("wheel not open")
	}
	// Send Esc
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 200 },
	)
	dv.Update()
	restore()
	if dv.colorMenuOpen {
		t.Fatalf("wheel still open after Esc")
	}
	after := dv.colorKey(dv.Rows[0].Color)
	if after != before {
		t.Fatalf("color changed on Esc: %s -> %s", before, after)
	}
}
