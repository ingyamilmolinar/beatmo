//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestUploadWAVRegistersInstrument(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// simulate upload selection
	g.drum.uploading = true
	g.drum.uploadCh <- uploadResult{path: "dummy.wav", err: nil}
	g.drum.Update() // process channel

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return []rune{'u'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.drum.Update() // process naming
	restore()

	if g.drum.Rows[0].Instrument != "u" {
		t.Fatalf("expected instrument to be u, got %s", g.drum.Rows[0].Instrument)
	}
	audio.ResetInstruments()
}

func TestUploadWAVMultipleAllowsInstrumentChange(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	audio.Register("foo", nil)
	g.drum.AddInstrument("foo")
	audio.Register("bar", nil)
	g.drum.AddInstrument("bar")

	if g.drum.Rows[0].Instrument != "bar" {
		t.Fatalf("expected instrument to be bar, got %s", g.drum.Rows[0].Instrument)
	}

	before := g.drum.Rows[0].Instrument
	g.drum.CycleInstrument()
	if g.drum.Rows[0].Instrument == before {
		t.Fatalf("instrument did not change after cycling")
	}
	audio.ResetInstruments()
}

func TestUploadButtonWorksAfterSelectingCustom(t *testing.T) {
        g := New(testLogger)
        g.Layout(640, 480)
        g.drum.recalcButtons()

        // first upload
        g.drum.uploading = true
        g.drum.uploadCh <- uploadResult{path: "first.wav", err: nil}
        g.drum.Update()
        restore := SetInputForTest(
                func() (int, int) { return 0, 0 },
                func(b ebiten.MouseButton) bool { return false },
                func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
                func() []rune { return []rune{'a'} },
                func() (float64, float64) { return 0, 0 },
                func() (int, int) { return 0, 0 },
        )
        g.drum.Update()
        restore()

        if g.drum.Rows[0].Instrument != "a" {
                t.Fatalf("expected first instrument 'a', got %s", g.drum.Rows[0].Instrument)
        }

        // simulate clicking upload button again
        g.drum.uploadBtn.OnClick()
        if !g.drum.uploading {
                t.Fatalf("upload button inactive")
        }
        audio.ResetInstruments()
}
