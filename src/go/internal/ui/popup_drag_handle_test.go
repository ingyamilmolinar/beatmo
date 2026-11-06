package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestGripIconRegistered pins IconGrip into the icon dispatch switch. An icon
// const with no switch entry silently draws nothing (DrawIcon returns false).
func TestGripIconRegistered(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	if !DrawIcon(dst, IconGrip, r, colTextPrimary) {
		t.Fatalf("DrawIcon returned false for %q (not registered in drawIconByID switch)", IconGrip)
	}
}

// pane/reserved fixtures shared by the geometry tests: a 800×400 grid pane
// with a 120×40 button cluster welded to its top-right corner.
var (
	testDragPane     = image.Rect(0, 0, 800, 400)
	testDragReserved = image.Rect(680, 0, 800, 40)
)

// TestDragHandleAutoModeByDefault: an untouched handle is transparent — Place
// returns the caller's auto-anchored rect verbatim.
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

// TestDragHandleMovesByCursorDelta: the card follows the cursor 1:1 with no
// jump at press time — the grab offset keeps the grip under the finger.
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

// TestDragHandleTapDoesNotPin: a press+release inside the deadzone is a tap,
// not a drag — the card stays in auto mode and keeps following its node.
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

// TestDragHandleClampsInsidePane: dragged hard past every edge, the card stays
// fully inside the pane (the node card's GridTree layer is clipped to it).
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

// TestDragHandleGripEscapesReservedZone: the top-right button cluster sits at a
// higher z and swallows presses, so a card parked under it must still expose
// its grip — otherwise the user can never drag it back out.
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

// TestDragHandleReClampsOnGrowth: expanding a section grows the card; the
// pinned origin must slide just enough to keep it inside the pane.
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

// TestDragHandleResetRestoresAuto: closing the menu drops the pin, so the next
// open re-anchors beside its node.
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

// TestDragHandleHitRectMeetsTouchMin: the glyph stays small, but the mobile
// touch target must reach the 44px minimum.
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

// TestDragHandleGripSitsTopLeft: the affordance the user was promised is in
// the card's top-left corner.
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

// The three tests below exercise clampPopupOrigin directly (same package) so
// each asserts a concrete expected origin, not just "doesn't overlap" — a
// non-overlap-only assertion would pass even on an implementation that
// teleports the card to the pane origin. TestDragHandleGripEscapesReservedZone
// above never drives the push branch at all (a 260×200 card's clamped max
// origin X is 540, leaving the grip ~100px short of reserved.Min.X=680), so
// it is kept as a coarse end-to-end check but does not, by itself, pin the
// push/tie-break/fallback logic in clampPopupOrigin.

// TestClampPopupOriginPushEscapesReservedZone: a small enough card (100×100)
// leaves clamp headroom to actually land inside the reserved cluster, so the
// push branch must fire and produce a specific escaped origin — not just any
// non-overlapping one.
func TestClampPopupOriginPushEscapesReservedZone(t *testing.T) {
	size := image.Pt(100, 100)
	// Starting origin: the 16px grip (offset by popupGripInset=10) sits at
	// x=[700,716], y=[10,26] — squarely inside reserved's x=[680,800],
	// y=[0,40].
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

// TestClampPopupOriginPicksCheapestEscape: with two valid escape directions
// available (a horizontal push clearing the cluster's left edge, and a
// cheaper vertical push clearing its bottom edge), the resolver must pick the
// SMALLEST displacement. The horizontal (Left) delta is evaluated first in
// clampPopupOrigin's candidate list, so an implementation that returns the
// first valid candidate instead of the cheapest one fails this test.
func TestClampPopupOriginPicksCheapestEscape(t *testing.T) {
	size := image.Pt(100, 100)
	// Grip sits at x=[710,726], y=[30,46]: deeper inside reserved than the
	// push test above, so the Left escape (cost 46) is markedly more
	// expensive than the Bottom escape (cost 10).
	origin := image.Pt(700, 20)

	got := clampPopupOrigin(origin, size, testDragPane, testDragReserved)
	want := image.Pt(700, 30) // Bottom (cost 10), not Left (cost 46)
	if got != want {
		t.Fatalf("cheapest-escape tie-break: want %v got %v", want, got)
	}
}

// TestClampPopupOriginFallsBackWhenNoEscapeRoom: when reserved spans the
// entire pane (no push direction has ANY room to clear it, on either axis),
// clampPopupOrigin must fall back to the rule-1-only origin — still fully
// inside pane, no panic, no zero rect — rather than looping or returning
// something degenerate.
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
