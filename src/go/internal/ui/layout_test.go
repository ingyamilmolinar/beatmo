package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/tunkul/internal/log"
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
		inc := dv.bpmIncBtn.Rect()
		dec := dv.bpmDecBtn.Rect()
		if inc.Empty() || dec.Empty() {
			t.Fatalf("w=%d: bpm +/- rects empty", w)
		}
		if inc.Min.X != dec.Min.X || inc.Max.X != dec.Max.X {
			t.Fatalf("w=%d: bpm +/- not in same column: inc=%v dec=%v", w, inc, dec)
		}
		if !(inc.Min.Y < dec.Min.Y) {
			t.Fatalf("w=%d: bpm + not above - (inc=%v, dec=%v)", w, inc, dec)
		}
		// Entire pair should live within the top half of the transport widget
		trans := dv.widgetRects[WidgetTransport]
		topMaxY := trans.Min.Y + trans.Dy()/2
		if inc.Min.Y < trans.Min.Y || dec.Max.Y > topMaxY {
			t.Fatalf("w=%d: bpm +/- out of top bounds: inc=%v dec=%v top=[%d,%d]", w, inc, dec, trans.Min.Y, topMaxY)
		}

		// Verify Length +/- are vertically stacked in the same column
		linc := dv.lenIncBtn.Rect()
		ldec := dv.lenDecBtn.Rect()
		if linc.Empty() || ldec.Empty() {
			t.Fatalf("w=%d: len +/- rects empty", w)
		}
		if linc.Min.X != ldec.Min.X || linc.Max.X != ldec.Max.X {
			t.Fatalf("w=%d: len +/- not in same column: inc=%v dec=%v", w, linc, ldec)
		}
		if !(linc.Min.Y < ldec.Min.Y) {
			t.Fatalf("w=%d: len + not above - (inc=%v, dec=%v)", w, linc, ldec)
		}
		if linc.Min.Y < trans.Min.Y || ldec.Max.Y > topMaxY {
			t.Fatalf("w=%d: len +/- out of top bounds: inc=%v dec=%v top=[%d,%d]", w, linc, ldec, trans.Min.Y, topMaxY)
		}

		// Bottom row sanity (Upload at least)
		r := dv.uploadBtn.Rect()
		if r.Empty() {
			t.Fatalf("w=%d: upload rect empty", w)
		}
		tr := dv.uploadBtn.textRect()
		if !tr.In(r) {
			t.Fatalf("w=%d: upload text outside rect", w)
		}
	}
}

// Ensure commonly visible icon buttons render expected glyphs.
func TestTopIconButtonsHaveGlyphs(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	if dv.playBtn.Text != "▶" {
		t.Fatalf("play icon = %q want ▶", dv.playBtn.Text)
	}
	if dv.stopBtn.Text != "■" {
		t.Fatalf("stop icon = %q want ■", dv.stopBtn.Text)
	}
	dv.calcLayout()
	if len(dv.rowEditBtns) == 0 {
		t.Fatalf("no row edit buttons built")
	}
	if dv.rowEditBtns[0].Text != "✎" {
		t.Fatalf("pencil icon = %q want ✎", dv.rowEditBtns[0].Text)
	}
	if len(dv.rowSaveBtns) == 0 {
		t.Fatalf("no row save buttons built")
	}
	if dv.rowSaveBtns[0].Icon != "save" {
		t.Fatalf("save icon = %q want save", dv.rowSaveBtns[0].Icon)
	}
}

func TestDrumRowEditButtonLayout(t *testing.T) {
	assertDefaultParityState(t)
	logger := log.New(testLogOutput(), log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	lbl := dv.rowLabels[0].Rect()
	edit := dv.rowEditBtns[0].Rect()
	save := dv.rowSaveBtns[0].Rect()
	color := dv.rowColorBtns[0].Rect()
	slider := dv.rowVolSliders[0].Rect()
	if lbl.Max.X > edit.Min.X {
		t.Fatalf("edit button overlaps label")
	}
	if edit.Max.X > save.Min.X {
		t.Fatalf("save button overlaps edit")
	}
	if save.Max.X > color.Min.X {
		t.Fatalf("save button overlaps color")
	}
	if color.Max.X > slider.Min.X {
		t.Fatalf("color button overlaps slider")
	}
}

func intAbs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
