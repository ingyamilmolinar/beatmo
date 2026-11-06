//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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
	g.drum.sampler.captureFromSynth(roster.Kick) // give the editor a buffer
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()
	if !Profile().IsMobile() {
		t.Fatal("setup did not enter mobile profile")
	}
	return g
}

// TestSamplerMobileTogglesInPanelActionsOnStickyBar — on mobile the Sampler tab
// keeps ONLY the Rev/Norm/Fade transforms row in-panel; the Preview/Save/Save
// As/Reset actions relocate to the sticky bar (Approach B), so the in-panel
// action buttons have empty rects. Verified in every supported language.
func TestSamplerMobileTogglesInPanelActionsOnStickyBar(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		t.Run(string(loc), func(t *testing.T) {
			i18n.SetLocale(loc)
			t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })

			g := setupMobileSamplerTab(t, 360, 760)
			dv := g.drum

			// Transforms row stays in-panel: visible + untruncated.
			for _, tag := range []string{"sampler-reverse", "sampler-normalize", "sampler-fade"} {
				b := dv.samplerButtonByTag(tag)
				if b == nil {
					t.Errorf("%s: button missing", tag)
					continue
				}
				if b.Rect().Empty() {
					t.Errorf("%s: empty rect (transforms row must stay in-panel)", tag)
					continue
				}
				if b.Rect().Dx() < TextWidth(b.Text) {
					t.Errorf("%s: width %d < label %q width %d (truncated)", tag, b.Rect().Dx(), b.Text, TextWidth(b.Text))
				}
			}

			// Action buttons relocated off-panel: their in-panel rects are empty.
			for _, tag := range []string{"sampler-preview", "sampler-save-as"} {
				if b := dv.samplerButtonByTag(tag); b != nil && !b.Rect().Empty() {
					t.Errorf("%s: still laid out in-panel on mobile (should be on the sticky bar)", tag)
				}
			}

			// The sticky-bar cluster is [Preview][Save][⋯].
			actions := dv.samplerPanelMobileActions()
			if len(actions) != 3 {
				t.Fatalf("want 3 sticky-bar actions, got %d", len(actions))
			}
			if actions[2].Icon != string(IconOverflow) {
				t.Errorf("third action must be the ⋯ overflow, got icon %q", actions[2].Icon)
			}
		})
	}
}

// TestSamplerMobileOverflowMenuHasSaveAs — the Sampler ⋯ overflow opens the
// shared instrument action menu: Save As…, portable Export/Import (the
// *.beatmo-inst.json feature) — same item set as the Synth tab
// (instrumentActionMenuItems).
func TestSamplerMobileOverflowMenuHasSaveAs(t *testing.T) {
	g := setupMobileSamplerTab(t, 360, 760)
	actions := g.drum.samplerPanelMobileActions()
	if len(actions) != 3 || actions[2].OnClick == nil {
		t.Fatalf("overflow button missing its OnClick")
	}
	actions[2].OnClick()

	m := g.drum.actionMenu
	if m == nil || !m.IsOpen() {
		t.Fatal("overflow tap did not open the action menu")
	}
	items := m.Buttons()
	want := []string{
		i18n.T(i18n.KeySaveAs),
		i18n.T(i18n.KeyMenuExport),
		i18n.T(i18n.KeyMenuImport),
	}
	if len(items) != len(want) {
		t.Fatalf("want %d menu items (Save As / Export / Import), got %d", len(want), len(items))
	}
	for i, w := range want {
		if items[i].Text != w {
			t.Errorf("item[%d] = %q, want %q", i, items[i].Text, w)
		}
	}
}
