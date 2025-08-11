//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestUploadWAVMultipleAllowsInstrumentChange(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	ids := []string{"foo", "bar"}
	idx := 0
	audio.RegisterWAVDialogFunc = func() (string, error) {
		id := ids[idx]
		idx++
		audio.Register(id, nil)
		return id, nil
	}

	click := func(x, y int) {
		pressed := true
		restore := SetInputForTest(
			func() (int, int) { return x, y },
			func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 640, 480 },
		)
		g.Update()
		pressed = false
		g.Update()
		restore()
	}

	// upload two custom instruments
	click(g.drum.uploadBtn.Min.X+1, g.drum.uploadBtn.Min.Y+1)
	click(g.drum.uploadBtn.Min.X+1, g.drum.uploadBtn.Min.Y+1)

	if g.drum.Rows[0].Instrument != "bar" {
		t.Fatalf("expected instrument to be bar, got %s", g.drum.Rows[0].Instrument)
	}

	before := g.drum.Rows[0].Instrument
	click(g.drum.instBtn.Min.X+1, g.drum.instBtn.Min.Y+1)
	if g.drum.Rows[0].Instrument == before {
		t.Fatalf("instrument did not change after cycling")
	}
	audio.ResetInstruments()
}
