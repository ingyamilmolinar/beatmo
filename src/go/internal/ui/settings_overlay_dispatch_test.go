//go:build test

package ui

import (
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestSettingsLanguagePillRealClickDispatch guards the FULL real-click dispatch
// path for the Settings language pills: a real press at the Español pill's
// reported rect must flow through the tree's shared HitIndex (which the portal
// publishes into) → the overlay's hit handler → onPick → i18n.SetLocale.
//
// The earlier settings_overlay_test.go called hit.Handler.OnPress(...) directly
// and so never exercised real dispatch — it passed while the pills were
// unclickable in the app. Root cause: the portal flattens every overlay hit
// area's ZIndex to 300+stackPos, so a full-panel catch-all registered before
// the pills won the equal-z stable-sort tiebreak and consumed the press. This
// test reproduces that bug through Game.Update().
func TestSettingsLanguagePillRealClickDispatch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	i18n.SetLocale(i18n.LocaleEN)

	// Open the settings overlay the same way the gear / "/" shortcut does.
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay after toggle")
	}
	_, esRect := ov.LanguagePillRects()
	if esRect.Empty() {
		t.Fatalf("Español pill rect is empty: %v", esRect)
	}

	cx := (esRect.Min.X + esRect.Max.X) / 2
	cy := (esRect.Min.Y + esRect.Max.Y) / 2

	holdTap := makeHoldTap(g, 1280, 720)
	holdTap(cx, cy, 4)
	advanceFrames(g, 2)

	if got := i18n.ActiveLocale(); got != i18n.LocaleES {
		t.Fatalf("real click on the Español pill at (%d,%d) in rect %v did not switch locale: still %q",
			cx, cy, esRect, got)
	}
}

// TestSettingsEnglishPillRealClickDispatch is the mirror for the English pill:
// from a Spanish start, a real click on the English pill switches back to EN.
func TestSettingsEnglishPillRealClickDispatch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	i18n.SetLocale(i18n.LocaleES)

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay after toggle")
	}
	enRect, _ := ov.LanguagePillRects()
	if enRect.Empty() {
		t.Fatalf("English pill rect is empty: %v", enRect)
	}

	cx := (enRect.Min.X + enRect.Max.X) / 2
	cy := (enRect.Min.Y + enRect.Max.Y) / 2

	holdTap := makeHoldTap(g, 1280, 720)
	holdTap(cx, cy, 4)
	advanceFrames(g, 2)

	if got := i18n.ActiveLocale(); got != i18n.LocaleEN {
		t.Fatalf("real click on the English pill at (%d,%d) in rect %v did not switch locale: still %q",
			cx, cy, enRect, got)
	}
}
