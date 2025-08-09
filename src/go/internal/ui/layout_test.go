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
	drumView := NewDrumView(image.Rect(0, 0, TestWinW, 120), nil, logger)
	drumView.recalcButtons()

	buttons := []image.Rectangle{
		drumView.playBtn,
		drumView.stopBtn,
		drumView.bpmBox,
		drumView.lenDecBtn,
		drumView.lenIncBtn,
	}

	for i, btn := range buttons {
		if btn.Empty() {
			t.Errorf("Button %d is empty", i)
		}
		if i > 0 {
			if btn.Min.X < buttons[i-1].Max.X {
				t.Errorf("Button %d overlaps with the previous button", i)
			}
		}
	}
}
