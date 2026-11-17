package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// synth_disabled_stage_interactivity_test.go pins the spec's confirmed UX
// choice for disabled stages (design doc § UI unification): "dimmed but
// interactive" — a user may pre-tweak a bypassed stage's knobs, the value
// persists and takes effect when the stage is re-enabled, and the enable pill
// stays visible (bright) so re-enabling is obvious. Previously this behavior
// existed only as code + comment (drawSynthSection); these tests make it a
// contract.

// newDisabledFilterSnareGame builds a full game on the Synth tab with the
// snare instrument's FILTER stage bypassed (filter_enabled = 0).
func newDisabledFilterSnareGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })

	audio.SetInstrumentParam(roster.Snare, "filter_enabled", 0)
	// Open the Synth tab at production height and select the FILTER stage so its
	// detail pane (knobs + enable pill + bypass scrim) is laid out. The
	// per-card pill is gone in the chip-strip redesign — the enable pill lives
	// in the detail header of the SELECTED stage, and only the selected stage's
	// knobs get non-empty rects.
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	g.drum.setSelectedSynthSection(roster.Snare, synthSectionFilter)
	layoutSynthTab(t, g)
	return g
}

// TestSynthTab_DisabledStagePillStaysVisible asserts the enable pill of a
// BYPASSED, selected stage stays laid out (visible) so re-enabling is obvious.
// The old sibling that pre-tweaked a bypassed stage's knob via an in-place
// rotary drag was retired with the ParamCard + value-wheel-popup redesign
// (value edits now flow through the popup, not a dial drag); the
// value-survives-re-enable contract is covered by the merge/import round-trip
// tests in internal/audio.
func TestSynthTab_DisabledStagePillStaysVisible(t *testing.T) {
	g := newDisabledFilterSnareGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if synthStageEnabled(roster.Snare, "filter_enabled") {
		t.Fatal("precondition: filter stage should be bypassed")
	}
	// The pill survives the bypass — it must stay tappable to re-enable.
	// FILTER is the selected stage, so its pill lives in the detail header.
	if g.drum.synthDetailEnablePillRect().Empty() {
		t.Fatal("FILTER enable pill rect is empty while the stage is bypassed (pill must stay visible)")
	}
}

// TestSynthTab_DisabledStageScrimDrawn asserts the bypass scrim is painted
// over the SELECTED (FILTER) stage's detail-pane knob area (below the detail
// header band) when the stage is off, and NOT painted when the stage is on.
// In the category layer the scrim covers the drilled view's control grid:
// drawSynthMacroLayer emits a WithAlpha(Surface1, AlphaStrong) filled rounded
// rect spanning the grid's content rect.
func TestSynthTab_DisabledStageScrimDrawn(t *testing.T) {
	g := newDisabledFilterSnareGame(t)

	scrimColor := WithAlpha(TokenSurface1(), AlphaStrong)
	countScrims := func() int {
		// The scrim covers the drilled view's control grid (below the header row).
		body := g.drum.instEditorMacroGrid.ContentRect()
		n := 0
		orig := drawRoundedRect
		drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
			if filled && c == scrimColor && r.In(body) && r.Dx() > body.Dx()/2 {
				n++
			}
			orig(dst, r, c, radius, filled)
		}
		defer func() { drawRoundedRect = orig }()
		scratch := ebiten.NewImage(1280, 720)
		g.drum.eqPanelZone.Draw(scratch)
		return n
	}

	if got := countScrims(); got == 0 {
		t.Error("no bypass scrim drawn over the disabled FILTER stage's detail knob area")
	}

	// Re-enable → the scrim must disappear.
	audio.SetInstrumentParam(roster.Snare, "filter_enabled", 1)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if got := countScrims(); got != 0 {
		t.Errorf("bypass scrim still drawn (%d) after re-enabling the FILTER stage", got)
	}
}
