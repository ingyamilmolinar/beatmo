//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// setupMobileSynthTab builds a narrow mobile synth tab with a synth
// instrument bound and returns the game. The size is intentionally as
// narrow as a small portrait phone so the OLD cascade would have
// collapsed the Save/Preview actions into the overflow (⋯) chevron.
func setupMobileSynthTab(t *testing.T, w, h int) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// SetForceMobileProfile is a no-op under -tags test; force the small-screen
	// profile directly so the mobile two-row header layout actually runs.
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	g.Layout(w, h)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()
	return g
}

// TestSynthMobileActionsOnStickyBar — on mobile the Synth tab has NO in-panel
// banner/action row: identity lives in the channel pill and the actions relocate
// to the sticky bar as a [Preview][Save][⋯] cluster (Approach B). The overflow
// (⋯) button carries the IconOverflow kebab; the in-panel SynthTabButtons no
// longer contains the header actions.
func TestSynthMobileActionsOnStickyBar(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		t.Run(string(loc), func(t *testing.T) {
			i18n.SetLocale(loc)
			t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })

			g := setupMobileSynthTab(t, 360, 760)

			actions := g.drum.synthPanelMobileActions()
			if len(actions) != 3 {
				t.Fatalf("want 3 sticky-bar actions [Preview][Save][⋯], got %d", len(actions))
			}
			if actions[0].Text == "" || actions[1].Text == "" {
				t.Errorf("Preview/Save buttons must carry a label; got %q / %q", actions[0].Text, actions[1].Text)
			}
			overflow := actions[2]
			if overflow.Icon != string(IconOverflow) {
				t.Errorf("third action must be the ⋯ overflow (IconOverflow), got icon %q text %q", overflow.Icon, overflow.Text)
			}
			// The sticky bar laid them out — every action has a real rect.
			for i, b := range actions {
				if b.Rect().Empty() {
					t.Errorf("sticky-bar action %d has an empty rect (not laid out)", i)
				}
			}

			// The old in-panel header actions are gone on mobile.
			for _, b := range g.drum.SynthTabButtons() {
				if b == nil {
					continue
				}
				switch b.Text {
				case synthActionLabel("preview"), synthActionLabel("save"),
					synthActionLabel("save-as"), synthActionLabel("reset"):
					t.Errorf("action %q still laid out in the in-panel synth header on mobile", b.Text)
				}
			}
		})
	}
}

// TestSynthMobileOverflowMenuHasSaveAs — tapping the ⋯ overflow opens the
// shared action menu with Save As… and the portable Export/Import instrument
// actions (see instrumentActionMenuItems in action_menu_open.go).
func TestSynthMobileOverflowMenuHasSaveAs(t *testing.T) {
	g := setupMobileSynthTab(t, 360, 760)
	actions := g.drum.synthPanelMobileActions()
	if len(actions) != 3 || actions[2].OnClick == nil {
		t.Fatalf("overflow button missing its OnClick")
	}
	actions[2].OnClick() // open the menu

	m := g.drum.actionMenu
	if m == nil || !m.IsOpen() {
		t.Fatal("overflow tap did not open the action menu")
	}
	items := m.Buttons()
	// Save As · Export · Import, in that order (no Reset: Save As is the only save).
	wantLabels := []string{
		i18n.T(i18n.KeySaveAs),
		i18n.T(i18n.KeyMenuExport),
		i18n.T(i18n.KeyMenuImport),
	}
	if len(items) != len(wantLabels) {
		t.Fatalf("want %d menu items (Save As / Export / Import), got %d",
			len(wantLabels), len(items))
	}
	for i, want := range wantLabels {
		if items[i].Text != want {
			t.Errorf("item %d = %q, want %q", i, items[i].Text, want)
		}
	}
}
