package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

const (
	TestWinW = 1280
	TestWinH = 720
)

// TODO: Add a font file to the project and load it here.
// var testFont font.Face

func TestMainLayout(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	game := New(logger)
	t.Cleanup(game.CloseForTest)
	game.Layout(TestWinW, TestWinH)

	if game.split.Y <= 0 || game.split.Y >= TestWinH {
		t.Errorf("Splitter position is out of bounds: %d", game.split.Y)
	}

	drumBounds := game.drum.Bounds
	if drumBounds.Min.Y != game.split.Y {
		t.Errorf("Drum view should start at the splitter's Y position. Got %d, want %d", drumBounds.Min.Y, game.split.Y)
	}

	if drumBounds.Max.Y != TestWinH {
		t.Errorf("Drum view should end at the bottom of the window. Got %d, want %d", drumBounds.Max.Y, TestWinH)
	}
}

func TestDrumViewButtonLayout(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	widths := []int{320, 640, 1280}
	for _, w := range widths {
		dv := NewDrumView(image.Rect(0, 0, w, 200), nil, logger)
		dv.recalcButtons()

		// Verify BPM +/- are vertically stacked in the same column
		inc := dv.bpmIncBtn().Rect()
		dec := dv.bpmDecBtn().Rect()
		if inc.Empty() || dec.Empty() {
			t.Fatalf("w=%d: bpm +/- rects empty", w)
		}
		if inc.Min.X != dec.Min.X || inc.Max.X != dec.Max.X {
			t.Fatalf("w=%d: bpm +/- not in same column: inc=%v dec=%v", w, inc, dec)
		}
		if inc.Min.Y >= dec.Min.Y {
			t.Fatalf("w=%d: bpm + not above - (inc=%v, dec=%v)", w, inc, dec)
		}
		// Entire pair should live within the transport widget (single row)
		trans := dv.widgetRects[WidgetTransport]
		if inc.Min.Y < trans.Min.Y || dec.Max.Y > trans.Max.Y {
			t.Fatalf("w=%d: bpm +/- out of transport bounds: inc=%v dec=%v trans=%v", w, inc, dec, trans)
		}

		// Verify Length +/- are vertically stacked in the timeline area
		linc := dv.lenIncBtn.Rect()
		ldec := dv.lenDecBtn.Rect()
		if linc.Empty() || ldec.Empty() {
			t.Fatalf("w=%d: len +/- rects empty", w)
		}
		if linc.Min.X != ldec.Min.X || linc.Max.X != ldec.Max.X {
			t.Fatalf("w=%d: len +/- not in same column: inc=%v dec=%v", w, linc, ldec)
		}
		if linc.Min.Y >= ldec.Min.Y {
			t.Fatalf("w=%d: len + not above - (inc=%v, dec=%v)", w, linc, ldec)
		}
		// Len buttons should be in the timeline widget area, not the transport.
		tl := dv.widgetRects[WidgetTimeline]
		if !tl.Empty() {
			if linc.Max.X > tl.Max.X || ldec.Max.X > tl.Max.X {
				t.Fatalf("w=%d: len buttons extend beyond timeline widget: inc=%v dec=%v tl=%v", w, linc, ldec, tl)
			}
		}

		// Upload button is now icon-only in the single row
		r := dv.uploadBtn().Rect()
		if r.Empty() {
			t.Fatalf("w=%d: upload rect empty", w)
		}
		if dv.uploadBtn().Icon != "upload" {
			t.Fatalf("w=%d: upload icon = %q want upload", w, dv.uploadBtn().Icon)
		}
	}
}

// Ensure commonly visible icon buttons render expected glyphs.
func TestTopIconButtonsHaveGlyphs(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	// Transport buttons are icon-only (no text).
	if dv.playBtn().Text != "" {
		t.Fatalf("play text = %q want empty", dv.playBtn().Text)
	}
	if dv.playBtn().Icon != "play" {
		t.Fatalf("play icon = %q want play", dv.playBtn().Icon)
	}
	if dv.stopBtn().Text != "" {
		t.Fatalf("stop text = %q want empty", dv.stopBtn().Text)
	}
	if dv.stopBtn().Icon != "stop" {
		t.Fatalf("stop icon = %q want stop", dv.stopBtn().Icon)
	}
	dv.calcLayout()
	if len(dv.rowEditBtns()) == 0 {
		t.Fatalf("no row edit buttons built")
	}
	if dv.rowEditBtns()[0].Text != "" {
		t.Fatalf("edit text = %q want empty", dv.rowEditBtns()[0].Text)
	}
	if dv.rowEditBtns()[0].Icon != "pencil" {
		t.Fatalf("edit icon = %q want pencil", dv.rowEditBtns()[0].Icon)
	}
	if len(dv.rowSaveBtns()) == 0 {
		t.Fatalf("no row save buttons built")
	}
	if dv.rowSaveBtns()[0].Icon != "save" {
		t.Fatalf("save icon = %q want save", dv.rowSaveBtns()[0].Icon)
	}
}

func TestDrumRowEditButtonLayout(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	// On desktop, edit/save/color buttons are hidden (empty rects) — they
	// live in the overflow menu. Verify they are indeed empty and that the
	// visible controls (label, volume slider) have valid rects.
	for _, name := range []string{"edit", "save", "color"} {
		var r image.Rectangle
		switch name {
		case "edit":
			r = dv.rowEditBtns()[0].Rect()
		case "save":
			r = dv.rowSaveBtns()[0].Rect()
		case "color":
			r = dv.rowColorBtns()[0].Rect()
		}
		if !r.Empty() {
			t.Fatalf("expected %s button rect empty on desktop, got %v", name, r)
		}
	}
	lbl := dv.rowLabels()[0].Rect()
	slider := dv.rowVolSliders()[0].Rect()
	if lbl.Empty() {
		t.Fatalf("label rect should not be empty")
	}
	if slider.Empty() {
		t.Fatalf("volume slider rect should not be empty")
	}
	if lbl.Max.X > slider.Min.X {
		t.Fatalf("label overlaps slider: label=%v slider=%v", lbl, slider)
	}
}

func TestLenButtonsHiddenInAudioViewMode(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), nil, logger)

	// In audio view mode, len +/- should be hidden.
	dv.currentViewMode = viewModeEQ
	dv.recalcButtons()
	dv.calcLayout()
	if !dv.lenIncBtn.Rect().Empty() {
		t.Fatalf("lenIncBtn should be empty in viewModeEQ, got %v", dv.lenIncBtn.Rect())
	}
	if !dv.lenDecBtn.Rect().Empty() {
		t.Fatalf("lenDecBtn should be empty in viewModeEQ, got %v", dv.lenDecBtn.Rect())
	}

	// Back to rows mode, buttons should be visible.
	dv.currentViewMode = viewModeRows
	dv.recalcButtons()
	dv.calcLayout()
	if dv.lenIncBtn.Rect().Empty() {
		t.Fatalf("lenIncBtn should not be empty in viewModeRows")
	}
	if dv.lenDecBtn.Rect().Empty() {
		t.Fatalf("lenDecBtn should not be empty in viewModeRows")
	}
}

func TestMaxCellCapIncreased(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 1280, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	tlW := dv.timelineRect.Dx()
	if tlW <= 0 {
		t.Fatalf("timeline width is %d, expected positive", tlW)
	}
	// Old cap was tlW/4. New min cell width of 2 allows tlW/2.
	oldCap := tlW / 4
	clamped := dv.clampLength(99999)
	if clamped <= oldCap {
		t.Fatalf("clampLength should exceed old cap %d, got %d", oldCap, clamped)
	}
}

func intAbs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// TestDesktopDrumPanelCappedAt50Percent verifies that on desktop, the drum
// panel (including header + rows + EQ) never exceeds 50% of the window height,
// even when many instrument rows would push it past that limit.
func TestDesktopDrumPanelCappedAt50Percent(t *testing.T) {
	assertDefaultParityState(t)

	forceAutoSize = true
	t.Cleanup(func() { forceAutoSize = false })
	SetDefaultStartForTest(false)
	t.Cleanup(func() { SetDefaultStartForTest(true) })

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)

	// Add enough rows so that the content-based drum height would exceed 50%.
	// With rowHeight=28, header=72, eqPanel=190:
	//   want = 72 + (10+1)*28 + 190 = 570  → 79% of 720px
	for i := 0; i < 9; i++ {
		g.drum.AddRow()
	}

	const winW, winH = 1280, 720
	g.Layout(winW, winH)

	drumH := winH - g.split.Y
	maxAllowed := winH / 2 // 50%
	if drumH > maxAllowed {
		t.Fatalf("desktop drum panel height %d exceeds 50%% cap (%d) with %d rows; split.Y=%d",
			drumH, maxAllowed, len(g.drum.Rows), g.split.Y)
	}
}
