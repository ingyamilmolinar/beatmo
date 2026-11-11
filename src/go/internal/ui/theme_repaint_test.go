//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// collectAllFills records every filled drawRect AND drawRoundedRect call
// made while fn runs (button keycaps are rounded rects).
func collectAllFills(t *testing.T, fn func()) []drawnRect {
	t.Helper()
	var rects []drawnRect
	origRect, origRound := drawRect, drawRoundedRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rects = append(rects, drawnRect{Rect: r, Color: color.RGBAModel.Convert(c).(color.RGBA)})
		}
		origRect(dst, r, c, filled)
	}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if filled {
			rects = append(rects, drawnRect{Rect: r, Color: color.RGBAModel.Convert(c).(color.RGBA)})
		}
		origRound(dst, r, c, radius, filled)
	}
	defer func() { drawRect, drawRoundedRect = origRect, origRound }()
	fn()
	return rects
}

// TestThemeApplyRepaintsEverySurface proves a theme change reaches the
// screen on the very next frame through every cache: the OLD accent and
// OLD background must vanish from the frame's fills and the DERIVED ones
// must appear (transport buttons, grid tile, drum cells, panels).
func TestThemeApplyRepaintsEverySurface(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 3)

	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })

	screen := ebiten.NewImage(1280, 720)
	before := collectAllFills(t, func() { g.Draw(screen) })
	oldPrimary, oldBG := genColorDefaultByName["primary"], genColorDefaultByName["background"]
	if rectsWithColorInside(before, screen.Bounds(), oldPrimary) == 0 || rectsWithColorInside(before, screen.Bounds(), oldBG) == 0 {
		t.Fatal("precondition: default accent/background not painted")
	}

	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "background": "electric-blue-500"}})
	t.Cleanup(restore)
	advanceFrames(g, 2)
	after := collectAllFills(t, func() { g.Draw(screen) })

	newPrimary, newBG := genColorPrimary, genColorBackground
	if newPrimary == oldPrimary || newBG == oldBG {
		t.Fatal("precondition: theme did not change the tokens")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), newPrimary); n == 0 {
		t.Error("derived accent never painted after ApplyTheme (spec/alias rebuild or a cache missed)")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), newBG); n == 0 {
		t.Error("derived background never painted after ApplyTheme")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), oldPrimary); n != 0 {
		t.Errorf("stale default accent still painted %d times after ApplyTheme — a cache was not invalidated", n)
	}
	if n := rectsWithColorInside(after, screen.Bounds(), oldBG); n != 0 {
		t.Errorf("stale default background still painted %d times after ApplyTheme", n)
	}
}
