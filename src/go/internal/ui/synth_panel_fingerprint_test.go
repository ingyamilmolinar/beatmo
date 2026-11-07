package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Section and knob rects are re-derived every Layout, never sampled at construction: two games agree, and a repeat
// layout changes nothing.
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

	gA.drum.eqPanelZone.Layout(gA.drum.eqPanelZone.PanelRect())
	third := captureSynthFingerprint(gA)
	if !fingerprintsEqual(first, third) {
		t.Fatalf("re-layout produced different fingerprint\n  first:     %+v\n  re-layout: %+v", first, third)
	}
}

// The runtime-profile fork is active.
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
	for _, k := range g.drum.SynthTabControls() {
		out.knobRects = append(out.knobRects, k.Rect())
	}
	out.headerRect = g.drum.SynthTabHeader().rect
	// Category layer geometry: the drilled detail rect + the strip/cards.
	out.sectionRects = append(out.sectionRects, g.drum.instEditorMacroDetailR, g.drum.instEditorPipeRect)
	for _, c := range g.drum.instEditorMacroCards {
		out.sectionRects = append(out.sectionRects, c.r)
	}
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
	g.drum.Rows[0].Instrument = roster.Snare
	_ = audio.InstallInstrumentDocForTest(roster.Snare, roster.Snare)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	layoutSynthTab(t, g)
	return g
}
