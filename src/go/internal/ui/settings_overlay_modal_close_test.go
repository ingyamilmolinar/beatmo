//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Framed by the shared MenuCard, so desktop publishes its close ×; mobile closes on a tap away.
func TestSettingsOverlayHasCardCloseButton(t *testing.T) {
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)
	_, ok := findHitAreaByTag(o.HitAreas(), "settings-close")
	if ok != desktopCloseVisible() {
		t.Errorf("settings-close hit area published=%v, want %v (desktopCloseVisible)", ok, desktopCloseVisible())
	}
}

// Real Buttons, so they get the shared keycap style and animation; the active locale is latched.
func TestSettingsOverlayLanguagePillsAreButtons(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)

	if o.enBtn == nil || o.esBtn == nil {
		t.Fatal("language pills are not backed by *Button instances")
	}
	// Each pill must be tree-routable via its own hit area (shared adapter).
	if _, ok := findHitAreaByTag(o.HitAreas(), "settings-lang-en"); !ok {
		t.Fatal("no settings-lang-en hit area published")
	}
	if _, ok := findHitAreaByTag(o.HitAreas(), "settings-lang-es"); !ok {
		t.Fatal("no settings-lang-es hit area published")
	}

	dst := ebiten.NewImage(1200, 800)

	i18n.SetLocale(i18n.LocaleEN)
	o.Draw(dst)
	if !o.enBtn.Toggled() || o.esBtn.Toggled() {
		t.Fatalf("with EN active: enBtn.Toggled=%v esBtn.Toggled=%v (want true,false)", o.enBtn.Toggled(), o.esBtn.Toggled())
	}

	i18n.SetLocale(i18n.LocaleES)
	o.Draw(dst)
	if o.enBtn.Toggled() || !o.esBtn.Toggled() {
		t.Fatalf("with ES active: enBtn.Toggled=%v esBtn.Toggled=%v (want false,true)", o.enBtn.Toggled(), o.esBtn.Toggled())
	}
}

// Through Game.Update, now that the pills are Buttons behind buttonHitAdapter.
func TestSettingsLanguageButtonRealClickDispatch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	i18n.SetLocale(i18n.LocaleEN)

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay after toggle")
	}
	_, esRect := ov.LanguagePillRects()
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

// Modal, so grid taps and camera pans cannot fire behind the panel.
func TestSettingsOverlayIsModal(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	if g.modalOverlayActive() {
		t.Fatal("no overlay open yet, but modalOverlayActive() is true")
	}

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)

	if !g.modalOverlayActive() {
		t.Fatal("settings overlay open but modalOverlayActive() is false — outside input not blocked")
	}

	// The modal filter limits the HitIndex to the overlay's own areas.
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay")
	}
	if !g.drum.portal().HasBlocking() {
		t.Fatal("settings portal entry is not blocking (Modal/Scrim both false)")
	}
}

func TestSettingsOverlayAnchorsUnderGear(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay after toggle")
	}

	gridH := g.split.GridH(g.winH)
	// It stays inside the grid pane: a tall panel is pushed up to fit, but its bottom never crosses the split.
	if ov.rect.Max.Y > gridH {
		t.Fatalf("settings panel %v spills past the grid pane (gridH=%d) into the drum-view panel", ov.rect, gridH)
	}
	if ov.rect.Min.X < 1280/2 {
		t.Fatalf("settings panel %v should hug the right side, not center", ov.rect)
	}
	if z := g.gridHelpReservedRect(); !z.Empty() && ov.rect.Overlaps(z) {
		t.Fatalf("settings panel %v overlaps gear zone %v", ov.rect, z)
	}
}

func TestSettingsOverlayClosesViaGearToggle(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	assertDefaultParityState(t)

	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	if !g.drum.portal().Has(settingsOverlayID) {
		t.Fatal("gear toggle should open the settings overlay")
	}

	// With no × button, tapping the gear again toggles it closed.
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	if g.drum.portal().Has(settingsOverlayID) {
		t.Fatal("second gear toggle must close the settings overlay")
	}
}
