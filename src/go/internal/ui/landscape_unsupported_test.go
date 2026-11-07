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

// Mobile landscape shows the rotate-to-portrait notice instead of a broken layout.
func TestLandscapeUnsupported_MobileLandscapeBlocked(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)

	g.Layout(800, 400)

	if !g.landscapeUnsupported() {
		t.Fatalf("mobile landscape (800x400) should be unsupported, got false")
	}
}

func TestLandscapeUnsupported_MobilePortraitOK(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)

	g.Layout(400, 800)

	if g.landscapeUnsupported() {
		t.Fatalf("mobile portrait (400x800) must be supported, got unsupported")
	}
}

func TestLandscapeUnsupported_DesktopLandscapeOK(t *testing.T) {
	setupMobileTest(t, false) // NOT a small screen
	g := newLandscapeTestGame(t)

	g.Layout(1280, 400)

	if g.landscapeUnsupported() {
		t.Fatalf("desktop landscape (1280x400) must be supported, got unsupported")
	}
}

func TestLandscapeUnsupported_DrawShowsNoticeSkipsPanes(t *testing.T) {
	setupMobileTest(t, true)
	g := newLandscapeTestGame(t)
	g.Layout(800, 400)

	noticeDrawn := false
	landscapeNoticeDrawHook = func() { noticeDrawn = true }
	t.Cleanup(func() { landscapeNoticeDrawHook = nil })

	// The normal grid-pane draw writes a real measurement; -1 left means it was skipped.
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

// The landscape detour must leave no corruption: both panes cover the full width again.
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

// A tap on empty space normally cancels connect mode, so connect mode surviving proves the tap never reached
// the hidden grid.
func TestLandscapeUnsupported_GridTapSuppressed(t *testing.T) {
	setupMobileTest(t, true) // also disables default-start
	g := newLandscapeTestGame(t)
	g.Layout(800, 400) // mobile landscape

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.updateBeatInfos()
	g.enterConnectMode(a)
	if !g.modes.Is(GridModeConnect) {
		t.Fatal("precondition: connect mode should be active")
	}

	emptyX, emptyY := screenPosForGridIJ(g, 100, 100)
	g.handleTapInGrid(emptyX, emptyY)

	if !g.modes.Is(GridModeConnect) {
		t.Fatal("in mobile landscape, grid tap must be suppressed (connect mode should remain active)")
	}
}

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
