package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// interceptDrawRect swaps the package drawRect var, forwarding to fn then the
// original, and returns a restore func.
func interceptDrawRect(fn func(*ebiten.Image, image.Rectangle, color.Color, bool)) func() {
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		fn(dst, r, c, filled)
		orig(dst, r, c, filled)
	}
	return func() { drawRect = orig }
}

// TestMenuActiveItemDrawsAzureStripe verifies an active menu item paints a
// left accent stripe in the azure family and a tinted fill; a resting item
// paints neither.
func TestMenuActiveItemDrawsAzureStripe(t *testing.T) {
	dst := ebiten.NewImage(200, 40)
	r := image.Rect(0, 0, 200, 30)

	var stripeSeen bool
	restore := interceptDrawRect(func(_ *ebiten.Image, rect image.Rectangle, _ color.Color, _ bool) {
		if rect.Min.X == r.Min.X && rect.Dx() <= accentStripeW() && rect.Dy() >= r.Dy()-2 {
			stripeSeen = true
		}
	})
	defer restore()

	drawMenuItemBackground(dst, r, menuItemActive)
	if !stripeSeen {
		t.Fatal("active menu item must draw a left azure accent stripe")
	}
}

// TestMenuItemRestIsFlat verifies a resting menu item paints nothing — rows sit
// directly on the menu's single panel surface (minimalist-flat styling, no
// per-row keycap pad/bevel).
func TestMenuItemRestIsFlat(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	var drew bool
	restore := interceptDrawRect(func(_ *ebiten.Image, _ image.Rectangle, _ color.Color, _ bool) { drew = true })
	defer restore()
	drawMenuItemBackgroundAccent(dst, image.Rect(0, 0, 40, 20), menuItemRest, colAccent)
	if drew {
		t.Fatal("rest menu item must be flat (no fill)")
	}
}

// TestMenuSeparatorDraws verifies the hairline group-divider helper draws a
// visible line at the row's vertical center.
func TestMenuSeparatorDraws(t *testing.T) {
	dst := ebiten.NewImage(40, 20)
	r := image.Rect(0, 0, 40, 20)
	wantY := r.Min.Y + r.Dy()/2
	var lineSeen bool
	restore := interceptDrawRect(func(_ *ebiten.Image, rect image.Rectangle, _ color.Color, _ bool) {
		if wantY >= rect.Min.Y && wantY < rect.Max.Y && rect.Dx() > 0 {
			lineSeen = true
		}
	})
	defer restore()
	drawMenuSeparator(dst, r)
	if !lineSeen {
		t.Fatal("separator must draw a visible line at the row's vertical center")
	}
}

// TestLongPressConnectHoverUsesAzure proves the Connect-hover text is in the
// azure single-accent family, not cyan (viz-only).
func TestLongPressConnectHoverUsesAzure(t *testing.T) {
	got := longPressConnectHoverColor()
	if got != color.Color(colAccent) && got != color.Color(colAccentBright) {
		t.Fatalf("Connect hover must use azure accent, got %v (cyan in chrome is a single-accent violation)", got)
	}
}

// Menu-row leading icons must be small (like the row context menu), not sized to
// the full row height, and the label must start to the right of the icon.
func TestMenuRowIconIsSmall(t *testing.T) {
	row := image.Rect(0, 0, 200, 44)
	ir := menuRowIconRect(row)
	if ir.Dx() != menuRowIconSize() || ir.Dy() != menuRowIconSize() {
		t.Fatalf("menu icon must be %dpx square, got %dx%d", menuRowIconSize(), ir.Dx(), ir.Dy())
	}
	if ir.Dy() >= row.Dy()-2 {
		t.Fatalf("menu icon must be small relative to the row (got %d in row %d)", ir.Dy(), row.Dy())
	}
	if menuRowLabelX(row) <= ir.Max.X {
		t.Fatalf("label x %d must be right of icon (icon max x %d)", menuRowLabelX(row), ir.Max.X)
	}
}

// menuTitleRole must sit between RoleCaption (old "File") and RolePanelTitle
// (old "Kick-1"): File reads a bit larger, instrument-name titles smaller, all
// the same unified size.
func TestMenuTitleRoleUnified(t *testing.T) {
	if StyledTextHeight(menuTitleRole) <= StyledTextHeight(RoleCaption) {
		t.Fatalf("menu title (%d) must be larger than caption (%d) — File a bit larger",
			StyledTextHeight(menuTitleRole), StyledTextHeight(RoleCaption))
	}
	if StyledTextHeight(menuTitleRole) >= StyledTextHeight(RolePanelTitle) {
		t.Fatalf("menu title (%d) must be smaller than panel-title (%d) — Kick-1 smaller",
			StyledTextHeight(menuTitleRole), StyledTextHeight(RolePanelTitle))
	}
}
