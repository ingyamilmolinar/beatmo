//go:build test

package ui

import (
	"runtime"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func waitForUploadNaming(t *testing.T, g *Game) {
	t.Helper()
	for i := 0; i < 20; i++ {
		g.drum.Update()
		if g.drum.naming {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("upload click did not enter naming state (uploading=%v naming=%v)", g.drum.uploading, g.drum.naming)
}

func TestUploadWAVRegistersInstrument(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()
	initial := g.drum.Rows[0].Instrument

	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return []rune{'u'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.drum.Update() // process naming
	restore()

	if g.drum.Rows[0].Instrument != initial {
		t.Fatalf("expected row instrument to remain %q, got %q", initial, g.drum.Rows[0].Instrument)
	}
	found := false
	for _, id := range g.drum.instOptions {
		if id == "u" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected new instrument 'u' to be available")
	}
}

func TestUploadWAVMultipleAllowsInstrumentChange(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
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
}

func TestUploadButtonWorksAfterSelectingCustom(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()
	initial := g.drum.Rows[0].Instrument

	// first upload via button click
	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
	waitForUploadNaming(t, g)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return []rune{'a'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.drum.Update()
	restore()

	if g.drum.Rows[0].Instrument != initial {
		t.Fatalf("expected row instrument to remain %q, got %q", initial, g.drum.Rows[0].Instrument)
	}
	found := false
	for _, id := range g.drum.instOptions {
		if id == "a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected instrument 'a' to be listed after upload")
	}

	// simulate clicking upload button again
	restore2 := SetInputForTest(
		func() (int, int) { return r.Min.X + 1, r.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore2)
	g.drum.Update()
	if !g.drum.uploading {
		t.Fatalf("upload button inactive")
	}
	restore2()
	g.drum.Update()
}

// When the instrument menu is open, clicking Upload should close the menu and
// still trigger a file selection in the same click. Previously the click was
// swallowed while closing the menu, leaving the Upload button unresponsive.
func TestUploadButtonWhileMenuOpen(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// Open instrument menu via row label button.
	if len(g.drum.rowLabels) == 0 {
		t.Fatalf("missing row labels for instrument menu")
	}
	g.drum.rowLabels[0].OnClick()
	if !g.drum.instMenuOpen {
		t.Fatalf("instrument menu did not open")
	}

	// Click Upload while menu is open
	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)

	if g.drum.instMenuOpen {
		t.Fatalf("instrument menu still open after upload click")
	}
	if !g.drum.uploading && !g.drum.naming {
		t.Fatalf("upload not triggered while menu open")
	}
}

// After uploading and choosing a custom instrument from the menu, the Upload
// button should still respond to clicks and begin another file selection.

func TestUploadButtonClickableTwice(t *testing.T) {
	prevSuppress := suppressClicksUntilRelease
	withDefaultAudio(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prevSuppress })
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	r := g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
	waitForUploadNaming(t, g)
	// dismiss naming dialog
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.drum.Update()
	restore()
	if g.drum.naming {
		t.Fatalf("naming still active after Escape")
	}

	r = g.drum.uploadBtn.Rect()
	click(g, r.Min.X+1, r.Min.Y+1)
	g.drum.Update()
	if !g.drum.uploading && !g.drum.naming {
		t.Fatalf("second click did not trigger upload")
	}
}

func TestUploadButtonIgnoredWhileNaming(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)

	if g.drum.uploading {
		t.Fatalf("unexpected uploading state during naming")
	}
	g.drum.uploadBtn.OnClick()
	if g.drum.uploading {
		t.Fatalf("upload started while naming")
	}
	if !g.drum.naming {
		t.Fatalf("naming canceled unexpectedly after upload click")
	}
}

func TestUploadButtonIgnoredWhileUploading(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	startUploadForTest(t, g.drum)
	_ = waitForUploadResult(t, g.drum)
	if !g.drum.uploading {
		t.Fatalf("upload not active after starting upload")
	}

	g.drum.uploadBtn.OnClick()
	if !g.drum.uploading {
		t.Fatalf("upload cleared unexpectedly after second click")
	}
	for i := 0; i < 20; i++ {
		select {
		case <-g.drum.uploadCh:
			t.Fatalf("upload result queued while upload already active")
		default:
			runtime.Gosched()
		}
	}
}

func TestUploadButtonIgnoredWhileImporting(t *testing.T) {
	withDefaultAudio(t)
	tries := 0
	old := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) { tries++ }
	t.Cleanup(func() { selectJSONAsyncFn = old })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { closeImportForTest(t, g) })
	g.Layout(640, 480)
	g.drum.recalcButtons()

	startImportForTest(t, g.drum)
	if tries != 1 {
		t.Fatalf("import picker calls=%d want=1", tries)
	}

	g.drum.uploadBtn.OnClick()
	if g.drum.uploading {
		t.Fatalf("upload started while importing")
	}
	if g.drum.naming {
		t.Fatalf("naming started while importing")
	}
}
