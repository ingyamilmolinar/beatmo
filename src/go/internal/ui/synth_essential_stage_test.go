package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// synth_essential_stage_test.go pins the fix for "a beginner can silence an
// instrument by disabling the one stage that IS its sound". For a sole-source
// generator stage (the OSC stage on a pure-oscillator voice such as the bare
// modular synth or FM Bass), the Synth-tab enable pill must be GREYED OUT and
// non-interactive: it stays visible (so the pipeline order still reads) but a
// tap can never turn the sound off. Processor stages (filter, env, …) stay
// fully interactive because their off-state is shaping, not silence.
//
// The classification lives in audio.StageDisableSilences (unit-tested in
// internal/audio); these tests pin the UI wiring on top of it.

// TestSynthTab_EssentialOscPillDoesNotDisable is the bug reproduction: on the
// bare modular synth the OSC stage is the whole voice, so tapping its enable
// pill must NOT disable it (pre-fix, the tap set osc_enabled=0 → total silence).
func TestSynthTab_EssentialOscPillDoesNotDisable(t *testing.T) {
	g := setupModularSynthGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	// Precondition: the OSC stage is the modular synth's sole sound source.
	if !audio.StageDisableSilences(roster.Modular, "osc_enabled") {
		t.Fatal("precondition: modular OSC must be the sole sound source (essential)")
	}
	if !synthStageEnabled(roster.Modular, "osc_enabled") {
		t.Fatal("precondition: OSC must start enabled")
	}

	selectStageProdHeight(t, g, roster.Modular, synthSectionOsc)

	// The pill stays visible (the stage still shows in the pipeline).
	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("essential OSC stage must still render its (greyed) enable pill")
	}

	// Tapping the pill must be a no-op: the sole generator can't be silenced.
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2
	g.drum.handleSynthTabInput(cx, cy, true, roster.Modular)
	g.drum.handleSynthTabInput(cx, cy, false, roster.Modular)
	if !synthStageEnabled(roster.Modular, "osc_enabled") {
		t.Error("tapping the greyed OSC pill disabled the sole sound source (must be non-interactive)")
	}

	// A non-interactive pill publishes no toggle hit area.
	for _, a := range g.drum.synthTabHitAreas() {
		if a.Tag == "synth-toggle-osc_enabled" {
			t.Error("essential OSC pill must not publish a synth-toggle hit area")
		}
	}
}

// TestSynthTab_ProcessorPillStillDisables is the control: a shaper stage
// (FILTER) is NOT the sound source, so its pill stays interactive and a tap
// bypasses it as before.
func TestSynthTab_ProcessorPillStillDisables(t *testing.T) {
	g := setupModularSynthGame(t)
	restore := SwapSynthAuditionFnForTest(func(instrument.ID) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	if audio.StageDisableSilences(roster.Modular, "filter_enabled") {
		t.Fatal("precondition: FILTER is a shaper, never the sole sound source")
	}
	if !synthStageEnabled(roster.Modular, "filter_enabled") {
		t.Fatal("precondition: FILTER must start enabled")
	}

	selectStageProdHeight(t, g, roster.Modular, synthSectionFilter)

	pill := g.drum.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatal("FILTER enable pill rect is empty")
	}

	// Its toggle hit area is published (interactive).
	seen := false
	for _, a := range g.drum.synthTabHitAreas() {
		if a.Tag == "synth-toggle-filter_enabled" {
			seen = true
		}
	}
	if !seen {
		t.Error("interactive FILTER pill must publish its synth-toggle hit area")
	}

	// Tapping bypasses the filter.
	cx, cy := (pill.Min.X+pill.Max.X)/2, (pill.Min.Y+pill.Max.Y)/2
	if !g.drum.handleSynthTabInput(cx, cy, true, roster.Modular) {
		t.Fatal("tap on FILTER pill was not handled")
	}
	if synthStageEnabled(roster.Modular, "filter_enabled") {
		t.Error("tapping the FILTER pill did not bypass the stage")
	}
}

// TestSynthTab_EssentialOscPillDrawnGreyed asserts the OSC pill is painted with
// the muted "locked" track tint (never the live green track) while a processor
// stage's pill keeps the live green track.
func TestSynthTab_EssentialOscPillDrawnGreyed(t *testing.T) {
	g := setupModularSynthGame(t)

	mutedTrack := WithAlpha(colPlayGreen, AlphaSubtle)

	// countTrack draws the panel and counts filled rounded rects inside the
	// selected stage's pill that match wantCol.
	countTrack := func(sec synthSectionID, wantCol color.Color) int {
		selectStageProdHeight(t, g, roster.Modular, sec)
		pill := g.drum.synthDetailEnablePillRect()
		if pill.Empty() {
			t.Fatalf("pill rect empty for section %v", sec)
		}
		n := 0
		orig := drawRoundedRect
		drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
			if filled && c == wantCol && r.In(pill) {
				n++
			}
			orig(dst, r, c, radius, filled)
		}
		defer func() { drawRoundedRect = orig }()
		scratch := ebiten.NewImage(1280, 720)
		g.drum.eqPanelZone.Draw(scratch)
		return n
	}

	if got := countTrack(synthSectionOsc, mutedTrack); got == 0 {
		t.Error("essential OSC pill was not drawn with the muted (locked) track tint")
	}
	if got := countTrack(synthSectionFilter, mutedTrack); got != 0 {
		t.Errorf("interactive FILTER pill drew the muted (locked) track tint %d times (want live green)", got)
	}
	// Sanity: the live green track appears on the interactive FILTER pill.
	if got := countTrack(synthSectionFilter, colPlayGreen); got == 0 {
		t.Error("interactive FILTER pill did not draw the live green track")
	}
}
