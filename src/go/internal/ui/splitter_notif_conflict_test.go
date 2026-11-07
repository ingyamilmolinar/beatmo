//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A press on the pill below the line captures; one below the line that misses it does not, so drum controls
// beneath the divider are never stolen.
func TestSplitterCapturesPillButNotOffPillBelowDivider(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800
	s.totalH = 600

	pill := s.HandleRect().Inset(-SpaceSM)
	belowY := pill.Max.Y - 1 // below the divider, still on the pill
	if belowY <= s.Y {
		t.Skip("pill does not extend below the divider in this build")
	}

	// On the pill, below the divider → captures.
	cx := (pill.Min.X + pill.Max.X) / 2
	if got := s.HandleInput(cx, belowY, true); got != InputCaptured || !s.dragging {
		t.Errorf("press on the pill below the divider should capture: got %v dragging=%v", got, s.dragging)
	}
	s.dragging = false
	s.Y = 300

	// Off the pill (far left), same Y below the divider → NOT captured.
	offX := 10
	if image.Pt(offX, belowY).In(pill) {
		t.Skip("chosen off-pill x is unexpectedly inside the pill")
	}
	if got := s.HandleInput(offX, belowY, true); got == InputCaptured || s.dragging {
		t.Errorf("press below the divider but OFF the pill must NOT capture (would steal drum controls): got %v dragging=%v", got, s.dragging)
	}
}

// A press the legacy InputDispatcher (splitter, sidebar…) already owns must not also reach the tree's hit areas.
func TestTreeExternalCaptureBlocksNewPress(t *testing.T) {
	var mx, my int
	var pressed bool
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree := NewDrumViewTree()
	external := true
	tree.SetExternalCapture(func() bool { return external })

	handler := &testHitHandler{pressResult: InputConsumed}
	z := newTestZone("test")
	z.hitAreas = []HitArea{
		{Rect: image.Rect(10, 10, 100, 100), ZIndex: 100, Handler: handler, Tag: "btn"},
	}
	tree.RegisterZone(z, 100)
	tree.SetZoneRect("test", image.Rect(0, 0, 400, 300))
	tree.Update() // layout frame

	// Press inside the hit area while an external handler holds capture.
	mx, my = 50, 50
	pressed = true
	tree.Update()
	if handler.pressCount != 0 {
		t.Errorf("handler received press while external capture held (got %d) — tree must defer to the legacy dispatcher", handler.pressCount)
	}

	// Release, drop external capture, press again — now it should dispatch.
	pressed = false
	tree.Update()
	external = false
	mx, my = 50, 50
	pressed = true
	tree.Update()
	if handler.pressCount != 1 {
		t.Errorf("handler should receive press once external capture is released, got %d", handler.pressCount)
	}
}
