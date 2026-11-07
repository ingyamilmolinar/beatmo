package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// One pure method feeds both the test and the draw path.
func TestGridModeBanner_TextIsStyledAndLocalized(t *testing.T) {
	g := newTestGameForUndo(t)
	n := g.tryAddNode(3, 1, model.NodeTypeRegular)
	g.updateBeatInfos()

	// No banner when idle.
	if got := g.gridModeBannerText(); got != "" {
		t.Fatalf("idle banner = %q, want empty", got)
	}

	g.modes.Enter(GridModeMove, n)
	got := g.gridModeBannerText()
	if got == "" || got == "MOVING NODE (3,1) — TAP A POINT ON AN EDGE" {
		t.Fatalf("move banner should be localized title-case, got %q", got)
	}
	if !strings.Contains(got, "(3,1)") && !strings.Contains(got, "(3, 1)") {
		t.Errorf("move banner missing subject coords: %q", got)
	}
}

// Every armed mode, both locales: formatted (no stray verbs), with the subject's coordinates, not ALL-CAPS,
// different in EN and ES, and empty when idle.
func TestGridModeBannerText(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	n := g.tryAddNode(2, 4, model.NodeTypeRegular)
	g.updateBeatInfos()

	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		i18n.SetLocale(loc)
		for _, kind := range []GridModeKind{GridModeMove, GridModeCopy, GridModeConnect} {
			g.modes.Enter(kind, n)
			got := g.gridModeBannerText()
			g.modes.Cancel("test-reset")
			if got == "" {
				t.Fatalf("[%v] %v banner empty", loc, kind)
			}
			if strings.Contains(got, "%d") || strings.Contains(got, "%!") {
				t.Fatalf("[%v] %v banner has unformatted verb: %q", loc, kind, got)
			}
			// Move and connect embed the subject's grid coordinate.
			if kind != GridModeCopy && !strings.Contains(got, "(2,4)") {
				t.Fatalf("[%v] %v banner missing subject coords: %q", loc, kind, got)
			}
		}
		// No shouting caps: the legacy ALL-CAPS wording is retired.
		g.modes.Enter(GridModeMove, n)
		move := g.gridModeBannerText()
		g.modes.Cancel("test-reset")
		if strings.Contains(move, "MOVING NODE") {
			t.Fatalf("[%v] move banner still ALL-CAPS: %q", loc, move)
		}
	}

	// EN and ES actually differ (real translations, not copies).
	i18n.SetLocale(i18n.LocaleEN)
	g.modes.Enter(GridModeMove, n)
	en := g.gridModeBannerText()
	g.modes.Cancel("test-reset")

	i18n.SetLocale(i18n.LocaleES)
	g.modes.Enter(GridModeMove, n)
	es := g.gridModeBannerText()
	g.modes.Cancel("test-reset")

	if en == es {
		t.Fatalf("EN and ES move banners identical: %q", en)
	}

	// GridModeNone (idle) renders nothing.
	i18n.SetLocale(i18n.LocaleEN)
	if got := g.gridModeBannerText(); got != "" {
		t.Fatalf("none-mode banner = %q, want empty", got)
	}
}
