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

	// first upload via button click
	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
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
	click(g, r.Min.X+1, r.Min.Y+1)
	if !g.drum.uploading {
		t.Fatalf("upload button inactive")
	}
	audio.ResetInstruments()
}

// When the instrument menu is open, clicking Upload should close the menu and
// still trigger a file selection in the same click. Previously the click was
// swallowed while closing the menu, leaving the Upload button unresponsive.
func TestUploadButtonWhileMenuOpen(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// Simulate instrument menu being open
	g.drum.instMenuOpen = true
	g.drum.instMenuRow = 0

	// Click Upload while menu is open
	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)

	if !g.drum.uploading {
		t.Fatalf("upload button inactive while menu open")
	}
}

// After uploading and choosing a custom instrument from the menu, the Upload
// button should still respond to clicks and begin another file selection.
func TestUploadButtonAfterSelectingViaMenu(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// First upload via button click and naming
	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
	g.drum.uploadCh <- uploadResult{path: "first.wav", err: nil}
	g.drum.Update()
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return []rune{'c'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.drum.Update()
	restore()

	// Open instrument menu by clicking the row label.
	lbl := g.drum.rowLabels[0].Rect()
	click(g, lbl.Min.X+1, lbl.Min.Y+1)

	if !g.drum.instMenuOpen {
		t.Fatalf("instrument menu did not open")
	}

	// Click the custom instrument option "c" in the menu.
	idx := -1
	for i, id := range g.drum.instOptions {
		if id == "c" {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("custom instrument not found in options: %v", g.drum.instOptions)
	}
	opt := g.drum.instMenuBtns[idx].Rect()
	click(g, opt.Min.X+1, opt.Min.Y+1)

	if g.drum.instMenuOpen {
		t.Fatalf("instrument menu did not close after selection")
	}

	// Click Upload again.
	r = g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)

	if !g.drum.uploading {
		t.Fatalf("upload button inactive after selecting custom instrument")
	}

	// Clean up the pending upload to avoid leaking goroutines.
	g.drum.uploadCh <- uploadResult{path: "second.wav", err: nil}
	g.drum.Update()
	audio.ResetInstruments()
}

func TestUploadButtonClickableTwice(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
	g.drum.uploadCh <- uploadResult{path: "first.wav", err: nil}
	g.drum.Update()
	// dismiss naming dialog
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.drum.Update()
	restore()

	click(g, r.Min.X+1, r.Min.Y+1)
	if !g.drum.uploading {
		t.Fatalf("second click did not trigger upload")
	}
}
