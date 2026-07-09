//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestSettingsOverlayHasCloseButton asserts the overlay exposes a close button
// (top-right of the panel) and that pressing it marks the overlay for removal.
func TestSettingsOverlayHasCloseButton(t *testing.T) {
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)

	cr := o.CloseButtonRect()
	if cr.Empty() {
		t.Fatal("close button rect is empty after Layout")
	}
	// Close button sits inside the panel, at the top-right.
	if !cr.In(o.rect) {
		t.Fatalf("close button %v not inside panel %v", cr, o.rect)
	}
	if cr.Min.X < (o.rect.Min.X+o.rect.Max.X)/2 {
		t.Fatalf("close button %v should be on the right half of panel %v", cr, o.rect)
	}

	area, ok := findHitAreaByTag(o.HitAreas(), "settings-close")
	if !ok {
		t.Fatal("no settings-close hit area published")
	}
	if o.ShouldClose() {
		t.Fatal("overlay should not be closed before the close button is pressed")
	}
	cx := (cr.Min.X + cr.Max.X) / 2
	cy := (cr.Min.Y + cr.Max.Y) / 2
	area.Handler.OnPress(cx, cy)
	area.Handler.OnRelease(cx, cy)
	if !o.ShouldClose() {
		t.Fatal("pressing the close button did not mark the overlay for removal")
	}
}

// TestSettingsOverlayLanguagePillsAreButtons asserts the language pills are real
// Buttons (so they get the shared keycap style + press/hover animation) and that
// the active locale renders as a toggled (latched) button — matching every other
// active control in the UI.
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

// TestSettingsLanguageButtonRealClickDispatch guards the FULL real-click path now
// that the pills are independent Buttons routed via buttonHitAdapter: a real
// press at the Español pill switches the locale through Game.Update().
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

// TestSettingsOverlayIsModal asserts the open settings overlay is a BLOCKING
// (modal) overlay so input outside the panel is suppressed — the gear used to
// open it non-modal, which let grid taps / camera pans fire behind the panel.
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

	// A press well outside the panel must not fall through to grid input
	// (modal filter restricts the HitIndex to the overlay's own areas).
	ov, ok := g.drum.portal().TopOverlay().(*SettingsOverlay)
	if !ok || ov == nil {
		t.Fatal("settings overlay is not the top portal overlay")
	}
	if !g.drum.portal().HasBlocking() {
		t.Fatal("settings portal entry is not blocking (Modal/Scrim both false)")
	}
}

// TestSettingsOverlayClosesViaCloseButtonDispatch drives the real input loop:
// clicking the close button removes the overlay from the portal.
func TestSettingsOverlayClosesViaCloseButtonDispatch(t *testing.T) {
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
	cr := ov.CloseButtonRect()
	cx := (cr.Min.X + cr.Max.X) / 2
	cy := (cr.Min.Y + cr.Max.Y) / 2

	holdTap := makeHoldTap(g, 1280, 720)
	holdTap(cx, cy, 4)
	advanceFrames(g, 2)

	if g.drum.portal().Has(settingsOverlayID) {
		t.Fatalf("clicking the close button at (%d,%d) in rect %v did not close the settings overlay", cx, cy, cr)
	}
}
