//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestIconPaletteDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconPalette, image.Rect(0, 0, 24, 24), color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("IconPalette not registered in drawIconByID")
	}
}

func TestThemeButtonPlacementAndReservedZone(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		if mobile {
			setupMobileTest(t, true) // forces the mobile screen class; call BEFORE New()
		}
		g := newGameWithStart(t)
		if mobile {
			g.Layout(390, 844)
		} else {
			g.Layout(1280, 720)
		}
		advanceFrames(g, 2)
		th, logs := g.gridThemeButtonRect(), g.gridLogsButtonRect()
		if th.Empty() || logs.Empty() {
			t.Fatalf("mobile=%v: empty rects theme=%v logs=%v", mobile, th, logs)
		}
		if th.Dx() != logs.Dx() || th.Dy() != logs.Dy() || th.Min.Y != logs.Min.Y {
			t.Fatalf("mobile=%v: theme %v must match logs %v size/row", mobile, th, logs)
		}
		if th.Max.X+SpaceSM != logs.Min.X {
			t.Fatalf("mobile=%v: theme must sit one SpaceSM left of logs: %v vs %v", mobile, th, logs)
		}
		if th.Min.X < g.split.GridRect(g.winW, g.winH).Min.X {
			t.Fatalf("mobile=%v: cluster overflows the pane: %v", mobile, th)
		}
		cx, cy := (th.Min.X+th.Max.X)/2, (th.Min.Y+th.Max.Y)/2
		if !g.inTopRightButtonZone(cx, cy) || !g.inTopRightButtonZone(th.Max.X+SpaceSM/2, cy) {
			t.Fatalf("mobile=%v: theme button / gap not in the protected zone", mobile)
		}
		if g.gridThemeBtn.Icon != string(IconPalette) {
			t.Fatalf("theme icon=%q want %q", g.gridThemeBtn.Icon, IconPalette)
		}
	}
}

func TestThemeButtonRealClickTogglesOverlay(t *testing.T) {
	g := newGameWithStart(t)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	r := g.gridThemeButtonRect()
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	holdTap := makeHoldTap(g, 1280, 720)
	holdTap(cx, cy, 3)
	advanceFrames(g, 2)
	p := g.drum.portal()
	if !p.Has(themeOverlayID) {
		t.Fatal("theme overlay did not open on click")
	}
	if p.Has(settingsOverlayID) {
		t.Fatal("opening the theme overlay must close Settings (mutual exclusion)")
	}
	holdTap(cx, cy, 3)
	advanceFrames(g, 2)
	if p.Has(themeOverlayID) {
		t.Fatal("second click must close the theme overlay")
	}
}
