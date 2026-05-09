//go:build test

package ui

import (
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// newEQPanelZoneForTest constructs an EQPanelZone with empty callbacks for
// unit tests. The returned zone has flat (0 dB) band gains.
func newEQPanelZoneForTest(t *testing.T) *EQPanelZone {
	t.Helper()
	return NewEQPanelZone(EQCallbacks{})
}

func TestEQPanel_SampleCurveReturnsRequestedLength(t *testing.T) {
	z := newEQPanelZoneForTest(t)
	samples := z.SampleCurve(32)
	if len(samples) != 32 {
		t.Fatalf("SampleCurve(32) returned %d samples", len(samples))
	}
}

func TestEQPanel_SampleCurveTracksGains(t *testing.T) {
	z := newEQPanelZoneForTest(t)

	// Flat: all samples within 1 dB of zero.
	flat := z.SampleCurve(32)
	for i, s := range flat {
		if math.Abs(s) > 1.0 {
			t.Fatalf("flat sample[%d]=%.2f exceeds 1 dB tolerance", i, s)
		}
	}

	// Boost band 0 by 12 dB; expect at least one sample > 6 dB.
	z.SetBandGainDB(0, 12.0)
	boosted := z.SampleCurve(32)
	maxSample := math.Inf(-1)
	for _, s := range boosted {
		if s > maxSample {
			maxSample = s
		}
	}
	if maxSample < 6.0 {
		t.Fatalf("expected boosted curve max >6 dB, got %.2f", maxSample)
	}
}

func TestEQPeek_VisibleWhenCollapsed(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("expected mobileEQCollapsed=true on default mobile layout")
	}
	if g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect empty when EQ is collapsed; expected 24px peek strip")
	}
	if g.drum.eqPeekRect.Dy() != 24 {
		t.Fatalf("eqPeekRect height=%d; expected 24", g.drum.eqPeekRect.Dy())
	}
	if g.drum.eqPeekRect.Max.Y != g.drum.bottomActionBarRect.Min.Y {
		t.Fatalf("eqPeekRect should sit directly above bottom action bar: peek.Max.Y=%d barTop=%d",
			g.drum.eqPeekRect.Max.Y, g.drum.bottomActionBarRect.Min.Y)
	}
}

func TestEQPeek_HiddenWhenExpanded(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.drum.mobileEQCollapsed = false // force-expand for test
	g.drum.mobileEQMode = true
	g.Layout(360, 700)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect should be empty when EQ panel is expanded")
	}
}

func TestEQPeek_HiddenOnDesktop(t *testing.T) {
	setupMobileTest(t, false) // desktop
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 800)

	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect should be empty on desktop")
	}
}

// TestEQPeek_PolylineFollowsGains verifies the sparkline drawn into
// eqPeekRect actually responds to band gain changes — flat EQ should
// produce a tight Y-range polyline; boosting band 0 should widen it.
func TestEQPeek_PolylineFollowsGains(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if g.drum.eqPeekRect.Empty() {
		t.Fatalf("precondition: eqPeekRect empty")
	}

	// Capture two screens — flat baseline vs. boosted band 0.
	flatPx := pixelYSpread(t, g, g.drum.eqPeekRect)

	// Boost band 0 by +24 dB to widen the sparkline.
	if g.drum.eqPanelZone == nil {
		t.Fatalf("eqPanelZone nil — cannot mutate band gains")
	}
	g.drum.eqPanelZone.SetBandGainDB(0, 24.0)
	g.Layout(360, 700) // re-layout so any cached state refreshes
	bumpPx := pixelYSpread(t, g, g.drum.eqPeekRect)

	if bumpPx <= flatPx+2 {
		t.Fatalf("expected boosted EQ to widen sparkline Y-spread: flat=%d boosted=%d", flatPx, bumpPx)
	}
}

// TestEQPeek_TapExpandsPanel verifies that tapping inside the EQ peek
// strip toggles mobileEQCollapsed off, so the user can expand the EQ
// panel with a single touch instead of hunting for an overflow menu.
func TestEQPeek_TapExpandsPanel(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("precondition: expected mobileEQCollapsed=true on default mobile layout")
	}
	r := g.drum.eqPeekRect
	if r.Empty() {
		t.Fatalf("precondition: expected eqPeekRect non-empty")
	}

	// Tap dead center of peek strip. injectTouchTap drives a 2-frame
	// press+release cycle through updateTouchOverride; two g.Update()
	// calls advance both frames so the press is dispatched and released.
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	t.Cleanup(resetTouchOverride)
	injectTouchTap(mx, my)
	g.Update()
	g.Update()

	if g.drum.mobileEQCollapsed {
		t.Fatalf("expected EQ panel to expand on peek tap; mobileEQCollapsed still true")
	}
}

// TestEQPeek_PinchDoesNotExpandPanel verifies that during the
// multi-touch cooldown (set by globalTouchState after a pinch ends)
// a tap on the EQ peek strip is suppressed. Without this gate, a
// stray pinch-release fingertip could accidentally expand the EQ
// panel and disrupt the user's view (parallel to the scrub gate
// added in Task 3.2).
func TestEQPeek_PinchDoesNotExpandPanel(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("precondition: mobileEQCollapsed=true expected")
	}
	r := g.drum.eqPeekRect
	if r.Empty() {
		t.Fatalf("precondition: eqPeekRect non-empty expected")
	}

	// Pin the multi-touch cooldown.
	prev := globalTouchState.multiTouchCooldown
	globalTouchState.multiTouchCooldown = multiTouchCooldownFrames
	t.Cleanup(func() { globalTouchState.multiTouchCooldown = prev })

	adapter := &eqPeekHitAdapter{dv: g.drum}
	mid := image.Pt(r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)
	res := adapter.OnPress(mid.X, mid.Y)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("EQ panel should NOT expand during multi-touch cooldown; mobileEQCollapsed flipped to false")
	}
	if res == InputCaptured || res == InputConsumed {
		t.Fatalf("eqPeekHitAdapter.OnPress should ignore press during cooldown, got %v", res)
	}
}

// pixelYSpread renders the game and returns the vertical span (in px)
// of pixels in `r` that differ from the rect's surface background. Used
// to detect a sparkline polyline's vertical extent without coupling to
// any specific color.
func pixelYSpread(t *testing.T, g *Game, r image.Rectangle) int {
	t.Helper()
	screen := ebiten.NewImage(360, 700)
	g.Draw(screen)

	// Sample the surface background at a known-quiet point — the very
	// edge of the rect where the sparkline likely won't draw.
	bgR, bgG, bgB, _ := screen.At(r.Min.X+1, r.Min.Y+1).RGBA()

	minY, maxY := r.Max.Y, r.Min.Y
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pr, pg, pb, _ := screen.At(x, y).RGBA()
			if pr != bgR || pg != bgG || pb != bgB {
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
				break // one differing pixel per Y is enough
			}
		}
	}
	if maxY < minY {
		return 0
	}
	return maxY - minY
}
