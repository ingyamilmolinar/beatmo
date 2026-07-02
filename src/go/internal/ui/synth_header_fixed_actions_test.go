//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
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
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update()
	return g
}

// TestSynthHeaderActionsAlwaysVisibleNoOverflow — the four synth header
// actions (Preview / Save / Save As / Reset) must ALWAYS be laid out as
// visible buttons on mobile, never collapsed behind an overflow (⋯)
// chevron, and never truncated — in every supported language.
func TestSynthHeaderActionsAlwaysVisibleNoOverflow(t *testing.T) {
	for _, loc := range []i18n.Locale{i18n.LocaleEN, i18n.LocaleES} {
		t.Run(string(loc), func(t *testing.T) {
			i18n.SetLocale(loc)
			t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })

			g := setupMobileSynthTab(t, 360, 760)

			want := map[string]bool{
				synthPreviewButtonTag: false,
				synthSaveButtonTag:    false,
				synthSaveAsButtonTag:  false,
				synthResetButtonTag:   false,
			}
			for _, b := range g.drum.SynthTabButtons() {
				if b == nil {
					continue
				}
				if b.Icon == string(IconOverflow) {
					t.Fatalf("found an overflow (⋯) button on the synth header — the cascade must be gone")
				}
				if _, ok := want[b.Text]; ok {
					if b.Rect().Empty() {
						t.Errorf("action %q has an empty rect (not visible)", synthHeaderButtonLabel(b.Text))
						continue
					}
					want[b.Text] = true
					if label := synthHeaderButtonLabel(b.Text); TextWidth(label) > b.Rect().Dx() {
						t.Errorf("action %q label width %d exceeds button width %d (truncated)",
							label, TextWidth(label), b.Rect().Dx())
					}
				}
			}
			for tag, seen := range want {
				if !seen {
					t.Errorf("action %q is not laid out as a visible button on the mobile synth header", tag)
				}
			}
		})
	}
}
