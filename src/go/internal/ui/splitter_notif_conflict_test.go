//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// pressOnceAt drives a single full Game.Update() frame with the left mouse
// button held at (x,y), exercising the real input pipeline: the legacy
// InputDispatcher (splitter, drum) AND the DrumViewTree (notif, etc.).
func pressOnceAt(t *testing.T, g *Game, x, y, winW, winH int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return winW, winH },
	)
	defer restore()
	g.Update()
}

// splitterPressPointOverNotif finds a press point that lies on the splitter's
// visible/grabbable handle pill AND on top of the notification strip — i.e. the
// region where the user sees the divider pill drawn over the notification bar.
// Returns ok=false when this layout doesn't put the pill over the notif.
func splitterPressPointOverNotif(g *Game) (image.Point, bool) {
	dv := g.drum
	s := g.split
	if dv == nil || s == nil || dv.notifRect.Empty() {
		return image.Point{}, false
	}
	pill := s.HandleRect().Inset(-SpaceSM)
	overlap := pill.Intersect(dv.notifRect)
	if overlap.Empty() {
		return image.Point{}, false
	}
	return image.Pt(
		(overlap.Min.X+overlap.Max.X)/2,
		(overlap.Min.Y+overlap.Max.Y)/2,
	), true
}

// TestSplitterPillPressDoesNotOpenNotifHistory pins the reported bug: on
// desktop the divider pill ("the one in between the drum view and the main
// grid") is drawn on top of the notification bar. Pressing the pill there must
// start a splitter drag — NOT open the notification-history popup.
//
// Root cause: the splitter is dispatched by the legacy InputDispatcher (z=150)
// while the notification button lives in the DrumViewTree/HitIndex (z=112).
// The two systems don't share a hit test, and the splitter's input-capture
// bounds were narrower than its visible pill, so a press on the lower half of
// the pill fell through to the tree and opened history.
func TestSplitterPillPressDoesNotOpenNotifHistory(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, false) // desktop
	defer restore()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)
	g.drum.recalcButtons()
	g.Update()
	g.Update()

	pt, ok := splitterPressPointOverNotif(g)
	if !ok {
		t.Skip("splitter pill does not overlap the notification strip in this layout")
	}
	if g.drum.IsNotifHistoryOpen() {
		t.Fatal("precondition: notif history should be closed before the press")
	}

	pressOnceAt(t, g, pt.X, pt.Y, 1280, 800)

	if g.drum.IsNotifHistoryOpen() {
		t.Errorf("pressing the splitter pill at %v opened the notification history — the divider pill and the notification button are competing for the same press", pt)
	}
	if !g.split.dragging {
		t.Errorf("pressing the splitter pill at %v did NOT start a splitter drag (split.dragging=false)", pt)
	}
}

// TestSplitterCapturesPillButNotOffPillBelowDivider proves the real invariant
// preserved by widening InputBounds to cover the pill: a press ON the visible
// handle pill (even below the divider line) captures the splitter, but a press
// below the divider that MISSES the pill is NOT captured — so drum-area
// controls beneath the divider are never stolen.
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

// TestTreeExternalCaptureBlocksNewPress pins the GENERIC half of the fix: the
// DrumViewTree must NOT dispatch a new press to its hit areas when an external
// handler (the legacy InputDispatcher — splitter, sidebar, …) already owns the
// press. This is the cross-system coordination that prevents ANY tree
// component from stealing a press a higher-priority legacy component captured —
// not just notif-vs-splitter.
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
