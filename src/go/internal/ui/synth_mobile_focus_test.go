//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newMobileSynthTabGameForTest builds a *Game whose Synth tab is laid out under
// the MOBILE profile (browser runtime + forced small-screen + mobile EQ mode),
// so synthPreviewWidth returns 0 and the stacked mobile focus band should be
// reserved above the knob grid. Mirrors setupSynthFooterEnv
// (synth_panel_footer_hit_test.go), which is the proven mobile-synth-tab setup.
func newMobileSynthTabGameForTest(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	// Typical mobile viewport (short iPhone-class portrait). Under the OLD
	// surplus gate the focus band was EMPTY here (sections height ~185px <
	// the old minGridH=280 floor) — only ~1280px-tall panels got a band. The
	// always-on compact band must be non-empty even at this realistic size.
	g.Layout(390, 720)

	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Modular"
	g.drum.Rows[0].Instrument = "modular"
	audio.BindInstrumentToRecipe("modular", "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams("modular") })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update() // flush layout/hit index via tree
	return g
}

func TestMobileSynthTab_HasFocusBandAboveGrid(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatalf("mobile focus band is empty — focus graph unreachable on mobile (sections=%v, focusH=%d, isMobile=%v)",
			dv.synthSectionsRectForTest(), Profile().DensityValues().SynthFocusGraphH, Profile().IsMobile())
	}
	// Band must sit above the knob grid (sections) area.
	sections := dv.synthSectionsRectForTest()
	if band.Max.Y > sections.Min.Y+1 {
		t.Fatalf("mobile focus band (%v) must sit above the knob grid (%v)", band, sections)
	}
	// The shrunk knob grid must still have usable height — all knobs remain
	// reachable because the grid scrolls (selectSectionForKnobIdx /
	// ControlGrid.ScrollToIndex), but the visible grid must not be degenerate.
	if sections.Dy() <= 0 {
		t.Fatalf("knob grid collapsed to zero height after reserving the band (band=%v sections=%v)", band, sections)
	}
}

// TestMobileSynthTab_FocusGraphRespondsToKnobSelect drives the real draw path:
// on a typical mobile panel, after selecting a knob, drawSynthTab must paint ink
// INSIDE the mobile focus band — proving the focus graph actually renders on
// mobile (not just that a rect was reserved).
func TestMobileSynthTab_FocusGraphRespondsToKnobSelect(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatalf("mobile focus band empty — cannot render focus graph")
	}
	// Select a knob from the currently open section so the focus graph has a
	// concrete domain to draw.
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no open section with knobs on mobile")
	}
	dv.setSynthSelectedKnob(inst, sec.knobIdxs[0])

	contentR := dv.eqPanelZone.ContentRect()
	rects := collectFilledRects(t, func() {
		dv.drawSynthTab(ebiten.NewImage(g.winW, g.winH), contentR, inst)
	})
	// Re-read the band: drawSynthTab calls buildSynthTab (via layout) but in this
	// path the band was already laid out; read after draw to be safe.
	band = dv.synthMobileFocusRect()
	ink := 0
	for _, r := range rects {
		if r.Rect.Overlaps(band) {
			ink++
		}
	}
	if ink == 0 {
		t.Fatalf("no ink painted inside the mobile focus band %v — focus graph not rendering on mobile", band)
	}
}
