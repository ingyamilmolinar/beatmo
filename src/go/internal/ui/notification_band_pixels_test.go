//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawNotifAndCollect renders the game once and returns the filled draw
// rects emitted, so tests can assert the notification area's severity
// stripe color without depending on SubImage pixel propagation (which
// the ebitenstub does not mirror to the parent screen).
func drawNotifAndCollect(t *testing.T, g *Game) []drawnRect {
	t.Helper()
	screen := ebiten.NewImage(1280, 720)
	return collectFilledRects(t, func() { g.Draw(screen) })
}

func TestNotifArea_ErrorStripeIsRed(t *testing.T) {
	g := laidOutDesktopGame(t)
	dv := g.drum
	dv.notifyError("disaster struck while loading")
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}
	if dv.notifRect.Empty() {
		t.Fatal("precondition: notifRect empty")
	}
	rects := drawNotifAndCollect(t, g)
	if n := rectsWithColorInside(rects, dv.notifRect, colError); n == 0 {
		t.Fatal("error notification should draw a red (colError) severity stripe inside the notif area, got 0")
	}
}

func TestNotifArea_InfoStripeNotRed(t *testing.T) {
	g := laidOutDesktopGame(t)
	dv := g.drum
	dv.notifyInfo("everything is fine")
	for i := 0; i < 2; i++ {
		_ = g.Update()
	}
	if dv.notifRect.Empty() {
		t.Fatal("precondition: notifRect empty")
	}
	rects := drawNotifAndCollect(t, g)
	if n := rectsWithColorInside(rects, dv.notifRect, colError); n != 0 {
		t.Fatalf("info notification must NOT draw a red severity stripe, got %d colError rects", n)
	}
}

func TestNotifArea_IdleNoStripe(t *testing.T) {
	// With no session notification, the band shows the recessed slot only —
	// no severity stripe of either color.
	g := laidOutDesktopGame(t)
	dv := g.drum
	if dv.notifRect.Empty() {
		t.Fatal("precondition: notifRect empty")
	}
	rects := drawNotifAndCollect(t, g)
	if n := rectsWithColorInside(rects, dv.notifRect, colError); n != 0 {
		t.Fatalf("idle band must not draw an error stripe, got %d", n)
	}
}
