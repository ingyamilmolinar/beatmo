//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestUploadWAVAddsInstrument(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	audio.RegisterWAVDialogFunc = func(id string) error { return audio.RegisterWAV(id, "") }

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.uploadBtn.Min.X + 1, g.drum.uploadBtn.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update()
	pressed = false
	g.Update()

	if g.drum.Rows[0].Instrument != "user_wav" {
		t.Fatalf("expected instrument to switch to user_wav, got %s", g.drum.Rows[0].Instrument)
	}
	audio.ResetInstruments()
}
