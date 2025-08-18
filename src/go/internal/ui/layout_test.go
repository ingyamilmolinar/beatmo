package ui

import (
	"image"
	"os"
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
	logger := log.New(os.Stdout, log.LevelInfo)
	game := New(logger)
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
	logger := log.New(os.Stdout, log.LevelInfo)
	widths := []int{320, 640, 1280}
	for _, w := range widths {
		dv := NewDrumView(image.Rect(0, 0, w, 200), nil, logger)
		dv.recalcButtons()

		topButtons := []*Button{dv.playBtn, dv.stopBtn, dv.bpmDecBtn, dv.bpmBox, dv.bpmIncBtn, dv.lenDecBtn, dv.lenIncBtn}
		prev := image.Rectangle{}
		for i, btn := range topButtons {
			r := btn.Rect()
			if r.Empty() {
				t.Fatalf("w=%d: top button %d empty", w, i)
			}
			if i > 0 && r.Min.X-prev.Max.X < buttonPad {
				t.Fatalf("w=%d: top button %d lacks padding", w, i)
			}
			tr := btn.textRect()
			if !tr.In(r) {
				t.Fatalf("w=%d: text outside top button %d", w, i)
			}
			cx := (r.Min.X + r.Max.X) / 2
			ctx := (tr.Min.X + tr.Max.X) / 2
			cy := (r.Min.Y + r.Max.Y) / 2
			cty := (tr.Min.Y + tr.Max.Y) / 2
			if intAbs(cx-ctx) > 1 || intAbs(cy-cty) > 1 {
				t.Fatalf("w=%d: text not centered in top button %d", w, i)
			}
			prev = r
		}

		bottomButtons := []*Button{dv.uploadBtn}
		prev = image.Rectangle{}
		for i, btn := range bottomButtons {
			r := btn.Rect()
			if r.Empty() {
				t.Fatalf("w=%d: bottom button %d empty", w, i)
			}
			if i > 0 && r.Min.X-prev.Max.X < buttonPad {
				t.Fatalf("w=%d: bottom button %d lacks padding", w, i)
			}
			tr := btn.textRect()
			if !tr.In(r) {
				t.Fatalf("w=%d: text outside bottom button %d", w, i)
			}
			cx := (r.Min.X + r.Max.X) / 2
			ctx := (tr.Min.X + tr.Max.X) / 2
			cy := (r.Min.Y + r.Max.Y) / 2
			cty := (tr.Min.Y + tr.Max.Y) / 2
			if intAbs(cx-ctx) > 1 || intAbs(cy-cty) > 1 {
				t.Fatalf("w=%d: text not centered in bottom button %d", w, i)
			}
			prev = r
		}
	}
}

func TestDrumRowEditButtonLayout(t *testing.T) {
	logger := log.New(os.Stdout, log.LevelInfo)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()
	lbl := dv.rowLabels[0].Rect()
	edit := dv.rowEditBtns[0].Rect()
	slider := dv.rowVolSliders[0].Rect()
	if lbl.Max.X > edit.Min.X {
		t.Fatalf("edit button overlaps label")
	}
	if edit.Max.X > slider.Min.X {
		t.Fatalf("edit button overlaps slider")
	}
}

func intAbs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}
