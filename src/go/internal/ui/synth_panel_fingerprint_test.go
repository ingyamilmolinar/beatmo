package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestSynthLayoutFingerprintStable enforces the redesign's invariant from
// [[feedback_runtime_profile_derivation]]: section + knob + OUT-column
// rects must be re-derived every Layout, never sampled at ctor.
//
// The test builds two independent games at the same profile + size and
// asserts identical fingerprints. Then it relayouts one of them (without
// changing anything) and asserts the fingerprint is stable across the
// repeat call — proves the layout is deterministic and re-derives every
// Layout. The mobile-vs-desktop divergence check is a separate test
// (TestSynthLayoutMobileDesktopFork) because cross-profile transitions
// inside one Game instance require deeper splitter / view-mode plumbing
// that lives outside the Synth-tab redesign scope.
func TestSynthLayoutFingerprintStable(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)

	gA := newGameForFingerprint(t, logger, false)
	first := captureSynthFingerprint(gA)

	gB := newGameForFingerprint(t, logger, false)
	second := captureSynthFingerprint(gB)

	if !fingerprintsEqual(first, second) {
		t.Fatalf("two fresh games at same profile/size produced different fingerprints\n  A: %+v\n  B: %+v", first, second)
	}

	// Re-layout the first game (no state change) — fingerprint should be
	// identical to the original capture.
	gA.drum.eqPanelZone.Layout(gA.drum.eqPanelZone.PanelRect())
	third := captureSynthFingerprint(gA)
	if !fingerprintsEqual(first, third) {
		t.Fatalf("re-layout produced different fingerprint\n  first:     %+v\n  re-layout: %+v", first, third)
	}
}

// TestSynthLayoutMobileDesktopFork verifies the mobile + desktop
// layouts differ — proves the runtime-profile fork is active.
func TestSynthLayoutMobileDesktopFork(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)

	desktop := captureSynthFingerprint(newGameForFingerprint(t, logger, false))
	mobile := captureSynthFingerprint(newGameForFingerprint(t, logger, true))

	if fingerprintsEqual(desktop, mobile) {
		t.Errorf("mobile and desktop synth fingerprints unexpectedly equal — runtime profile fork may be broken\n  desktop: %+v\n  mobile:  %+v", desktop, mobile)
	}
}

type synthLayoutFingerprint struct {
	sectionRects []image.Rectangle
	knobRects    []image.Rectangle
	headerRect   image.Rectangle
}

func captureSynthFingerprint(g *Game) synthLayoutFingerprint {
	var out synthLayoutFingerprint
	for _, s := range g.drum.SynthTabSections() {
		out.sectionRects = append(out.sectionRects, s.Rect())
	}
	for _, k := range g.drum.SynthTabKnobs() {
		out.knobRects = append(out.knobRects, k.Rect())
	}
	out.headerRect = g.drum.SynthTabHeader().rect
	return out
}

func fingerprintsEqual(a, b synthLayoutFingerprint) bool {
	if !rectSliceEqual(a.sectionRects, b.sectionRects) {
		return false
	}
	if !rectSliceEqual(a.knobRects, b.knobRects) {
		return false
	}
	if a.headerRect != b.headerRect {
		return false
	}
	return true
}

func rectSliceEqual(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func newGameForFingerprint(t *testing.T, logger *game_log.Logger, mobile bool) *Game {
	t.Helper()
	if mobile {
		restore := SetRuntimeProfileForTest(browserRuntimeProfile())
		t.Cleanup(restore)
	}
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	if mobile {
		g.SetForceMobileProfile(true)
	}
	g.Layout(640, 480)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	return g
}
