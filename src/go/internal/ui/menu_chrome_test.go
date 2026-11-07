package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func interceptDrawRect(fn func(*ebiten.Image, image.Rectangle, color.Color, bool)) func() {
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		fn(dst, r, c, filled)
		orig(dst, r, c, filled)
	}
	return func() { drawRect = orig }
}

// A left azure stripe and a tinted fill when active; neither at rest.
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

// Rows sit directly on the menu's one panel surface: no per-row pad or bevel.
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

// At the row's vertical centre.
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

// Azure, the single accent family; cyan is for visualizers only.
func TestLongPressConnectHoverUsesAzure(t *testing.T) {
	got := longPressConnectHoverColor()
	if got != color.Color(colAccent) && got != color.Color(colAccentBright) {
		t.Fatalf("Connect hover must use azure accent, got %v (cyan in chrome is a single-accent violation)", got)
	}
}

// Small like the row context menu's, not row-height, with the label right of it.
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

// Between RoleCaption and RolePanelTitle: one size for file and instrument-name titles alike.
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
