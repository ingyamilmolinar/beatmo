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

// A disabled stage is dimmed but interactive: its knobs can be pre-tweaked and the enable pill stays visible, so
// re-enabling is obvious.

// filter_enabled = 0.
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
	// The enable pill lives in the selected stage's detail header, and only that stage's knobs get rects.
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	g.drum.setSelectedSynthSection(roster.Snare, synthSectionFilter)
	layoutSynthTab(t, g)
	return g
}

// A bypassed, selected stage keeps its enable pill laid out; that values survive re-enabling is
// internal/audio's round-trip tests.
func TestSynthTab_DisabledStagePillStaysVisible(t *testing.T) {
	g := newDisabledFilterSnareGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if synthStageEnabled(roster.Snare, "filter_enabled") {
		t.Fatal("precondition: filter stage should be bypassed")
	}
	// Tappable, to re-enable.
	if g.drum.synthDetailEnablePillRect().Empty() {
		t.Fatal("FILTER enable pill rect is empty while the stage is bypassed (pill must stay visible)")
	}
}

// Painted over the drilled control grid when the stage is off (a surface-1 AlphaStrong rounded rect), not when
// on.
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
