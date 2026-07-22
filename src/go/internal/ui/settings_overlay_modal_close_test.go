//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestSettingsOverlayHasNoCloseButton: the settings overlay has NO close (×)
// button — it is toggled closed by the gear button (see
// TestSettingsOverlayClosesViaGearToggle and TestTopRightMenusHaveNoCloseButton).
func TestSettingsOverlayHasNoCloseButton(t *testing.T) {
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)
	o.Layout(screen, screen)
	if _, ok := findHitAreaByTag(o.HitAreas(), "settings-close"); ok {
		t.Error("settings overlay must not publish a close (×) hit area")
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

// TestSettingsOverlayAnchorsUnderGear asserts the settings panel drops from the
// grid-pane gear (top-right) instead of centering — the bottom-panel-pin bug.
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
	// Hugs the top-right (next to the gear) and is fully CONTAINED IN THE GRID
	// PANE — it must never spill past the split into the drum-view panel. A tall
	// panel is pushed up to fit, so its top may sit above the gear; the invariant
	// is that its bottom does not cross the split (and it is not centered).
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

// TestSettingsOverlayClosesViaCloseButtonDispatch drives the real input loop:
// clicking the close button removes the overlay from the portal.
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
