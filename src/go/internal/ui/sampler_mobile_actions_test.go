//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// setupMobileSamplerTab builds a narrow mobile Sampler tab with a loaded
// buffer (so the edit knobs + action buttons lay out) and returns the game.
func setupMobileSamplerTab(t *testing.T, w, h int) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// SetForceMobileProfile is a no-op under -tags test; force the small-screen
	// profile directly (the canonical test path) so the mobile layout runs.
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	g.Layout(w, h)
	g.drum.SetMobileEQMode(true)
	g.drum.sampler.captureFromSynth("kick") // give the editor a buffer
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()
	if !Profile().IsMobile() {
		t.Fatal("setup did not enter mobile profile")
	}
	return g
}

// TestSamplerMobileActionsAlwaysVisibleNoTruncation — to match the Synth
// tab, the Sampler tab's mobile button rows must show EVERY button as a
// directly-tappable, untruncated control: the Rev/Norm/Fade toggles AND the
// Preview/Save/Save As/Reset actions. Previously all seven were crammed into
// one clamped row, truncating the tail (Save / Save As / Reset). Verified in
// every supported language.
func TestSamplerMobileActionsAlwaysVisibleNoTruncation(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		t.Run(string(loc), func(t *testing.T) {
			i18n.SetLocale(loc)
			t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })

			g := setupMobileSamplerTab(t, 360, 760)
			dv := g.drum

			tags := []string{
				"sampler-reverse", "sampler-normalize", "sampler-fade",
				"sampler-preview", "sampler-save", "sampler-save-as", "sampler-reset",
			}
			for _, tag := range tags {
				b := dv.samplerButtonByTag(tag)
				if b == nil {
					t.Errorf("%s: button missing", tag)
					continue
				}
				r := b.Rect()
				if r.Empty() {
					t.Errorf("%s: empty rect (not visible)", tag)
					continue
				}
				if r.Dx() < TextWidth(b.Text) {
					t.Errorf("%s: width %d < label %q width %d (truncated)", tag, r.Dx(), b.Text, TextWidth(b.Text))
				}
			}
		})
	}
}
