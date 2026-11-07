package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// An icon with no switch entry silently draws nothing.
func TestGripIconRegistered(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	if !DrawIcon(dst, IconGrip, r, colTextPrimary) {
		t.Fatalf("DrawIcon returned false for %q (not registered in drawIconByID switch)", IconGrip)
	}
}

// An 800×400 pane with a 120×40 cluster in its top-right corner.
var (
	testDragPane     = image.Rect(0, 0, 800, 400)
	testDragReserved = image.Rect(680, 0, 800, 40)
)

// Untouched, Place returns the auto-anchored rect verbatim.
func TestDragHandleAutoModeByDefault(t *testing.T) {
	var h PopupDragHandle
	auto := image.Rect(100, 50, 360, 250)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("auto mode should pass the rect through: want %v got %v", auto, got)
	}
	if h.Pinned() {
		t.Fatal("zero-value handle must not be pinned")
	}
}

// 1:1 with no jump at press time: the grab offset keeps the grip under the finger.
func TestDragHandleMovesByCursorDelta(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 50, 360, 250)
	size := image.Pt(panel.Dx(), panel.Dy())

	press := image.Pt(112, 62) // inside the grip, 12px in from the card corner
	h.Begin(press, panel)
	h.MoveTo(press.Add(image.Pt(60, 30)), testDragPane, testDragReserved, size)
	h.End()

	got := h.Place(panel, testDragPane, testDragReserved)
	want := panel.Add(image.Pt(60, 30))
	if got != want {
		t.Fatalf("pinned rect should track the cursor delta: want %v got %v", want, got)
	}
	if h.Dragging() {
		t.Fatal("End() must clear the dragging flag")
	}
}

// A tap within the deadzone leaves the card following its node.
func TestDragHandleTapDoesNotPin(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 50, 360, 250)
	size := image.Pt(panel.Dx(), panel.Dy())

	press := image.Pt(112, 62)
	h.Begin(press, panel)
	h.MoveTo(press.Add(image.Pt(1, 1)), testDragPane, testDragReserved, size)
	h.End()

	if h.Pinned() {
		t.Fatal("a 1px wobble is a tap, not a drag — must not pin")
	}
	auto := image.Rect(500, 200, 760, 400)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("after a tap the handle must still be transparent: want %v got %v", auto, got)
	}
}

// The node card's layer is clipped to the pane.
func TestDragHandleClampsInsidePane(t *testing.T) {
	size := image.Pt(260, 200)
	cases := []struct {
		name string
		to   image.Point
	}{
		{"far left/up", image.Pt(-5000, -5000)},
		{"far right/down", image.Pt(5000, 5000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var h PopupDragHandle
			panel := image.Rect(100, 150, 100+size.X, 150+size.Y)
			h.Begin(image.Pt(112, 162), panel)
			h.MoveTo(tc.to, testDragPane, testDragReserved, size)
			h.End()

			got := h.Place(panel, testDragPane, testDragReserved)
			if !got.In(testDragPane) {
				t.Fatalf("card %v escaped the pane %v", got, testDragPane)
			}
			if got.Dx() != size.X || got.Dy() != size.Y {
				t.Fatalf("clamping must not resize the card: want %v got %dx%d", size, got.Dx(), got.Dy())
			}
		})
	}
}

// The cluster is higher-z and swallows presses, so a card parked under it must still expose its grip.
func TestDragHandleGripEscapesReservedZone(t *testing.T) {
	var h PopupDragHandle
	size := image.Pt(260, 200)
	panel := image.Rect(100, 150, 100+size.X, 150+size.Y)

	h.Begin(image.Pt(112, 162), panel)
	// Aim the grip straight at the middle of the cluster.
	h.MoveTo(image.Pt(740, 20), testDragPane, testDragReserved, size)
	h.End()

	got := h.Place(panel, testDragPane, testDragReserved)
	if hit := h.HitRect(got); hit.Overlaps(testDragReserved) {
		t.Fatalf("grip hit rect %v must clear the reserved cluster %v (card %v)", hit, testDragReserved, got)
	}
	if !got.In(testDragPane) {
		t.Fatalf("escape adjustment pushed the card %v out of the pane %v", got, testDragPane)
	}
}

// A grown card slides just enough to stay inside the pane.
func TestDragHandleReClampsOnGrowth(t *testing.T) {
	var h PopupDragHandle
	small := image.Rect(100, 150, 360, 250) // 260×100
	h.Begin(image.Pt(112, 162), small)
	h.MoveTo(image.Pt(512, 562), testDragPane, testDragReserved, image.Pt(260, 100))
	h.End()

	grown := image.Rect(0, 0, 260, 300) // same card, 3× taller
	got := h.Place(grown, testDragPane, testDragReserved)
	if !got.In(testDragPane) {
		t.Fatalf("grown card %v must be re-clamped into the pane %v", got, testDragPane)
	}
	if got.Dy() != 300 {
		t.Fatalf("re-clamp must preserve the requested height: want 300 got %d", got.Dy())
	}
}

// Closing drops the pin, so the next open re-anchors.
func TestDragHandleResetRestoresAuto(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	h.Begin(image.Pt(112, 162), panel)
	h.MoveTo(image.Pt(300, 300), testDragPane, testDragReserved, image.Pt(260, 200))
	h.End()
	h.Reset()

	if h.Pinned() || h.Dragging() {
		t.Fatal("Reset must clear both pinned and dragging")
	}
	auto := image.Rect(500, 100, 760, 300)
	if got := h.Place(auto, testDragPane, testDragReserved); got != auto {
		t.Fatalf("after Reset the handle must be transparent: want %v got %v", auto, got)
	}
}

// A small glyph, a 44 px mobile target.
func TestDragHandleHitRectMeetsTouchMin(t *testing.T) {
	restore := SetDensityForTest(DensitySpacious)
	defer restore()

	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	grip := h.GripRect(panel)
	hit := h.HitRect(panel)

	if grip.Dx() != popupGripSize || grip.Dy() != popupGripSize {
		t.Fatalf("grip glyph should stay %dpx: got %dx%d", popupGripSize, grip.Dx(), grip.Dy())
	}
	min := TouchMinTarget()
	if hit.Dx() < min || hit.Dy() < min {
		t.Fatalf("hit rect %dx%d below touch min %d", hit.Dx(), hit.Dy(), min)
	}
	if !grip.In(hit) {
		t.Fatalf("hit rect %v must contain the glyph %v", hit, grip)
	}
}

func TestDragHandleGripSitsTopLeft(t *testing.T) {
	var h PopupDragHandle
	panel := image.Rect(100, 150, 360, 350)
	grip := h.GripRect(panel)
	if !grip.In(panel) {
		t.Fatalf("grip %v must sit inside the card %v", grip, panel)
	}
	if grip.Min.X > panel.Min.X+panel.Dx()/4 || grip.Min.Y > panel.Min.Y+panel.Dy()/4 {
		t.Fatalf("grip %v is not in the top-left quadrant of %v", grip, panel)
	}
}

// These assert exact origins: a non-overlap check would pass a card teleported to the pane origin, and the grip
// test above never reaches the push branch.

// Small enough to land inside the cluster, so the push must fire to a specific origin.
func TestClampPopupOriginPushEscapesReservedZone(t *testing.T) {
	size := image.Pt(100, 100)
	// The grip sits at x=[700,716], y=[10,26], inside the cluster's x=[680,800], y=[0,40].
	origin := image.Pt(690, 0)

	got := clampPopupOrigin(origin, size, testDragPane, testDragReserved)
	want := image.Pt(690, 30)
	if got != want {
		t.Fatalf("push escape: want %v got %v", want, got)
	}

	var h PopupDragHandle
	panel := image.Rect(got.X, got.Y, got.X+size.X, got.Y+size.Y)
	if hit := h.HitRect(panel); hit.Overlaps(testDragReserved) {
		t.Fatalf("grip hit rect %v still overlaps reserved %v after push", hit, testDragReserved)
	}
	if !panel.In(testDragPane) {
		t.Fatalf("pushed card %v escaped the pane %v", panel, testDragPane)
	}
}

// The smallest displacement wins; Left is tried first, so a first-valid resolver fails.
func TestClampPopupOriginPicksCheapestEscape(t *testing.T) {
	size := image.Pt(100, 100)
	// Left costs 46, Bottom 10.
	origin := image.Pt(700, 20)

	got := clampPopupOrigin(origin, size, testDragPane, testDragReserved)
	want := image.Pt(700, 30) // Bottom (cost 10), not Left (cost 46)
	if got != want {
		t.Fatalf("cheapest-escape tie-break: want %v got %v", want, got)
	}
}

// With no room on either axis it falls back to the in-pane origin: no loop, no panic, no zero rect.
func TestClampPopupOriginFallsBackWhenNoEscapeRoom(t *testing.T) {
	pane := image.Rect(0, 0, 120, 120)
	reserved := pane // spans all the escape room on both axes
	size := image.Pt(100, 100)

	got := clampPopupOrigin(image.Pt(5000, 5000), size, pane, reserved)
	want := image.Pt(20, 20) // rule-1 clamp only: pane.Max(120) - size(100)
	if got != want {
		t.Fatalf("no-escape-room fallback: want %v got %v", want, got)
	}
	panel := image.Rect(got.X, got.Y, got.X+size.X, got.Y+size.Y)
	if !panel.In(pane) {
		t.Fatalf("fallback card %v must still satisfy rule 1 (inside pane %v)", panel, pane)
	}
}
