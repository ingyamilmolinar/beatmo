//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Only controls laid out this frame.
func homeRowButtons(z *TransportZone) []struct {
	name string
	btn  *Button
} {
	all := []struct {
		name string
		btn  *Button
	}{
		{"play", z.playBtn},
		{"stop", z.stopBtn},
		{"record", z.recordBtn},
		{"undo", z.undoBtn},
		{"redo", z.redoBtn},
		{"bpmDec", z.bpmDecBtn},
		{"bpmInc", z.bpmIncBtn},
		{"subdiv", z.subdivBtn},
		{"track", z.trackBtn},
	}
	out := all[:0:0]
	for _, b := range all {
		if b.btn != nil && !b.btn.Rect().Empty() {
			out = append(out, b)
		}
	}
	return out
}

// Tree-dispatched buttons get hover on plain mouse-over, not only on press/drag/release; a real frame with the
// cursor over each must set `hovered`, and leaving clears it.
func TestHomeRowButtonsHoverOnPlainDesktopHover(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	z := g.drum.transportZone

	btns := homeRowButtons(z)
	if len(btns) == 0 {
		t.Fatal("no home-row buttons laid out")
	}

	oldCursor := cursorPosition
	oldMouse := isMouseButtonPressed
	inputForTestActive = true
	isMouseButtonPressed = func(b ebiten.MouseButton) bool { return false }
	t.Cleanup(func() {
		cursorPosition = oldCursor
		isMouseButtonPressed = oldMouse
		inputForTestActive = false
	})

	setCursor := func(pt image.Point) {
		cursorPosition = func() (int, int) { return pt.X, pt.Y }
	}

	for _, b := range btns {
		r := b.btn.Rect()
		center := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)

		// Cursor over the button → its own hovered flag must be set.
		setCursor(center)
		advanceFrames(g, 1)
		if !b.btn.hovered {
			t.Errorf("%s button: hovered=false while cursor at its center %v (no hover effect)", b.name, center)
		}

		// Cursor far away → hovered must clear.
		setCursor(image.Pt(2000, 2000))
		advanceFrames(g, 1)
		if b.btn.hovered {
			t.Errorf("%s button: hovered=true after cursor left to (2000,2000)", b.name)
		}
	}
}
