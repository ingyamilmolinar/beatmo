package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTransportSetBPMClamp(t *testing.T) {
	tr := NewTransport(200)
	tr.SetBPM(maxBPM + 500)
	if tr.BPM != maxBPM {
		t.Fatalf("BPM not clamped: %d", tr.BPM)
	}
	if tr.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on high bpm")
	}
	tr.bpmErrorAnim = 0
	tr.SetBPM(0)
	if tr.BPM != 1 {
		t.Fatalf("low BPM not clamped: %d", tr.BPM)
	}
	if tr.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on low bpm")
	}
}

func TestTransportBPMTextInput(t *testing.T) {
	tr := NewTransport(200)

	cx, cy := tr.bpmBox.Rect.Min.X+1, tr.bpmBox.Rect.Min.Y+1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	defer restore()

	tr.Update() // click to focus
	pressed = false

        chars = []rune{'5'}
        tr.Update()
        chars = []rune{'0'}
        tr.Update()
        chars = []rune{'0'}
        tr.Update()

        if tr.BPM != 120 {
                t.Fatalf("BPM changed before commit: %d", tr.BPM)
        }

        // click outside to commit
        pressed = true
        cx, cy = 0, 0
        tr.Update()

        if tr.BPM != 500 {
                t.Fatalf("expected BPM 500 got %d", tr.BPM)
        }
}

func TestTransportBPMTextInputNonNumeric(t *testing.T) {
        tr := NewTransport(200)

        cx, cy := tr.bpmBox.Rect.Min.X+1, tr.bpmBox.Rect.Min.Y+1
        pressed := true
        chars := []rune{}
        restore := SetInputForTest(
                func() (int, int) { return cx, cy },
                func(ebiten.MouseButton) bool { return pressed },
                func(ebiten.Key) bool { return false },
                func() []rune { c := chars; chars = nil; return c },
                func() (float64, float64) { return 0, 0 },
                func() (int, int) { return 200, 200 },
        )
        defer restore()

        tr.Update() // focus
        pressed = false

        chars = []rune{'a'}
        tr.Update()

        if tr.BPM != 120 {
                t.Fatalf("BPM changed before commit: %d", tr.BPM)
        }

        // click outside to commit
        pressed = true
        cx, cy = 0, 0
        tr.Update()

        if tr.BPM != 120 {
                t.Fatalf("expected BPM to remain 120 got %d", tr.BPM)
        }
        if tr.bpmErrorAnim == 0 {
                t.Fatalf("expected error animation for invalid input")
        }
}
