//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// newLandscapeTestGame builds a Game for the landscape-unsupported tests.
func newLandscapeTestGame(t *testing.T) *Game {
	t.Helper()
	g := New(log.New(testLogOutput(), log.LevelInfo))
	t.Cleanup(g.CloseForTest)
	return g
}

// TestLandscapeUnsupported_MobileLandscapeBlocked verifies that on a mobile
// (small-screen) profile, a landscape framebuffer (width > height) is reported
// as unsupported so the UI can show the rotate-to-portrait notice instead of
// the broken layout.
func TestLandscapeUnsupported_MobileLandscapeBlocked(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)

	g.Layout(800, 400)

	if !g.landscapeUnsupported() {
		t.Fatalf("mobile landscape (800x400) should be unsupported, got false")
	}
}

// TestLandscapeUnsupported_MobilePortraitOK verifies portrait mobile is always
// supported (height >= width).
func TestLandscapeUnsupported_MobilePortraitOK(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)

	g.Layout(400, 800)

	if g.landscapeUnsupported() {
		t.Fatalf("mobile portrait (400x800) must be supported, got unsupported")
	}
}

// TestLandscapeUnsupported_DesktopLandscapeOK verifies a wide desktop landscape
// is NOT blocked — only the mobile profile disables landscape.
func TestLandscapeUnsupported_DesktopLandscapeOK(t *testing.T) {
	setupMobileTest(t, false) // NOT a small screen
	g := newLandscapeTestGame(t)

	g.Layout(1280, 400)

	if g.landscapeUnsupported() {
		t.Fatalf("desktop landscape (1280x400) must be supported, got unsupported")
	}
}

// TestLandscapeUnsupported_DrawShowsNoticeSkipsPanes verifies that when the
// mobile landscape layout is active, Draw renders the unsupported notice and
// skips the normal grid/drum panes entirely.
func TestLandscapeUnsupported_DrawShowsNoticeSkipsPanes(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)
	g.Layout(800, 400)

	noticeDrawn := false
	landscapeNoticeDrawHook = func() { noticeDrawn = true }
	t.Cleanup(func() { landscapeNoticeDrawHook = nil })

	// Sentinel: the normal grid-pane draw path overwrites lastDrawGridMS with a
	// real (>=0) measurement. If it stays -1, the normal panes were skipped.
	g.lastDrawGridMS = -1

	img := ebiten.NewImage(800, 400)
	g.Draw(img)

	if !noticeDrawn {
		t.Fatalf("landscape Draw should render the unsupported notice, but it did not")
	}
	if g.lastDrawGridMS != -1 {
		t.Fatalf("landscape Draw should skip the normal grid pane, but lastDrawGridMS changed to %v", g.lastDrawGridMS)
	}
}

// TestLandscapeUnsupported_PortraitDrawsNormally verifies the notice path is NOT
// taken in portrait — the normal panes still draw.
func TestLandscapeUnsupported_PortraitDrawsNormally(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)
	g.Layout(400, 800)

	noticeDrawn := false
	landscapeNoticeDrawHook = func() { noticeDrawn = true }
	t.Cleanup(func() { landscapeNoticeDrawHook = nil })

	img := ebiten.NewImage(400, 800)
	g.Draw(img)

	if noticeDrawn {
		t.Fatalf("portrait Draw must not render the landscape notice")
	}
}

// TestLandscapeUnsupported_RotationRoundTripRestores is the seamless-transition
// guard: portrait -> landscape -> portrait must end fully supported again with
// both panes covering the full width (no corruption from the landscape detour).
func TestLandscapeUnsupported_RotationRoundTripRestores(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)

	g.Layout(390, 844) // portrait
	if g.landscapeUnsupported() {
		t.Fatal("portrait should be supported")
	}

	g.Layout(844, 390) // landscape
	if !g.landscapeUnsupported() {
		t.Fatal("landscape should be unsupported")
	}

	w, h := 390, 844
	g.Layout(w, h) // back to portrait
	if g.landscapeUnsupported() {
		t.Fatal("after rotating back to portrait, layout must be supported again")
	}

	grid := g.split.GridRect(w, h)
	drum := g.split.DrumRect(w, h)
	if grid.Dx() != w || drum.Dx() != w {
		t.Fatalf("after L->P, panes must cover full width %d: grid=%d drum=%d", w, grid.Dx(), drum.Dx())
	}
	if grid.Max.Y != drum.Min.Y {
		t.Fatalf("after L->P, panes must be gapless: grid.Max.Y=%d drum.Min.Y=%d", grid.Max.Y, drum.Min.Y)
	}
}

// TestLandscapeUnsupported_GridTapSuppressed verifies that in mobile landscape
// the grid tap handler is disabled, so a tap can't reach the (invisible) grid
// beneath the notice. Uses connect-mode: a tap on empty space normally cancels
// connect mode; in landscape it must be ignored, leaving connect mode active.
func TestLandscapeUnsupported_GridTapSuppressed(t *testing.T) {
	setupMobileTest(t, true) // also disables default-start
	g := newLandscapeTestGame(t)
	g.Layout(800, 400)       // mobile landscape

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.enterConnectMode(a)
	if !g.connectMode {
		t.Fatal("precondition: connect mode should be active")
	}

	emptyX, emptyY := screenPosForGridIJ(g, 100, 100)
	g.handleTapInGrid(emptyX, emptyY)

	if !g.connectMode {
		t.Fatal("in mobile landscape, grid tap must be suppressed (connect mode should remain active)")
	}
}

// TestLandscapeNoticeLines_Localized verifies the notice text resolves to
// non-empty strings in every supported locale.
func TestLandscapeNoticeLines_Localized(t *testing.T) {
	prev := i18n.ActiveLocale()
	t.Cleanup(func() { i18n.SetLocale(prev) })

	for _, loc := range i18n.AllLocales() {
		i18n.SetLocale(loc)
		lines := landscapeNoticeLines()
		if len(lines) == 0 {
			t.Fatalf("locale %s: landscapeNoticeLines() returned no lines", loc)
		}
		for i, ln := range lines {
			if ln == "" {
				t.Fatalf("locale %s: landscapeNoticeLines()[%d] is empty", loc, i)
			}
		}
	}
}
