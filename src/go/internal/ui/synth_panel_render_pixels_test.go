package ui

import (
	"image"
	"image/color"
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The expected rects receive a paint call during one Draw; pixel content is not checked (stub fonts make it
// brittle), and the knob arcs use vector strokes outside this interceptor.
func TestSynthPanelDrawIntercepts(t *testing.T) {
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

	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	rects := installDrawRectInterceptor(t)

	scratch := ebiten.NewImage(640, 480)
	g.drum.eqPanelZone.Draw(scratch)

	rectsCaptured := rects.snapshot()

	header := g.drum.SynthTabHeader()
	// At 640×480 the header collapses to zero height.
	if !header.rect.Empty() && header.rect.Dy() > 0 {
		if !anyRectInside(rectsCaptured, header.rect) {
			t.Errorf("no draw call landed inside the header rect %v (got %d total rects)", header.rect, len(rectsCaptured))
		}
	}

	for _, s := range g.drum.SynthTabSections() {
		if s.Rect().Empty() {
			continue
		}
		if !anyRectInside(rectsCaptured, s.Rect()) {
			t.Errorf("section %q rect %v received no draw calls", s.Label(), s.Rect())
		}
	}

	for _, btn := range g.drum.SynthTabButtons() {
		if btn.Rect().Empty() {
			continue
		}
		if !anyRectInside(rectsCaptured, btn.Rect()) {
			t.Errorf("reset button rect %v received no draw calls", btn.Rect())
		}
	}
}

// A stage card always paints its background, never blank.
func TestSynthPanelStageSectionRendersCard(t *testing.T) {
	g := setupModularSynthGame(t)
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	g.drum.setSynthStageOpen(inst, synthSectionOsc)
	// At 640×480 the drilled grid is a few px tall.
	expandSynthPanelForTest(t, g)
	order := g.drum.instEditorDrilledOrder
	if len(order) == 0 {
		t.Fatal("drilled OSC stage laid out no controls")
	}
	card := g.drum.instEditorControls[order[0]].Rect()
	if card.Empty() {
		t.Fatal("OSC stage's first ParamCard has an empty rect")
	}

	// The cards render through an origin-relative sprite; RENDER_SAFE draws them at absolute positions.
	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })
	rects := installDrawRectInterceptor(t)
	scratch := ebiten.NewImage(1280, 720)
	g.drum.eqPanelZone.Draw(scratch)
	captured := rects.snapshot()

	// At least one filled rect inside the card.
	foundCard := false
	for _, r := range captured {
		if r.In(card) && r.Dx() > 8 && r.Dy() > 8 {
			foundCard = true
			break
		}
	}
	if !foundCard {
		t.Fatalf("no filled rect inside the OSC card %v (%d rects captured)", card, len(captured))
	}
}

// A sample instrument paints its no-synth banner and lays out no section cards.
func TestSynthPanelNoSynthBannerRenders(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Kick"
	g.drum.Rows[0].Instrument = "kick-wav"
	audio.RemoveInstrumentDocForTest("kick-wav")
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())

	if !g.drum.SynthTabNoSynth() {
		t.Fatal("test invariant broken: kick-wav must enter the no-synth state")
	}
	if n := len(g.drum.SynthTabSections()); n != 0 {
		t.Fatalf("no-synth state must lay out zero section cards (got %d) — otherwise pulse borders could still blink", n)
	}

	rects := installDrawRectInterceptor(t)
	scratch := ebiten.NewImage(640, 480)
	g.drum.eqPanelZone.Draw(scratch)
	captured := rects.snapshot()

	banner := g.drum.SynthTabBannerRect()
	if banner.Empty() {
		t.Fatal("banner rect is empty")
	}
	foundBanner := false
	for _, r := range captured {
		if r.In(banner) && r.Dx() > 8 && r.Dy() > 8 {
			foundBanner = true
			break
		}
	}
	if !foundBanner {
		t.Errorf("no-synth banner rect %v received no card fill", banner)
	}
}

// ---- helpers ----

type drawRectInterceptor struct {
	mu    sync.Mutex
	calls []image.Rectangle
}

func (i *drawRectInterceptor) snapshot() []image.Rectangle {
	i.mu.Lock()
	defer i.mu.Unlock()
	out := make([]image.Rectangle, len(i.calls))
	copy(out, i.calls)
	return out
}

// Pass-through recorders, restored on Cleanup.
func installDrawRectInterceptor(t *testing.T) *drawRectInterceptor {
	t.Helper()
	intercept := &drawRectInterceptor{}
	origRect := drawRect
	origRound := drawRoundedRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		intercept.mu.Lock()
		intercept.calls = append(intercept.calls, r)
		intercept.mu.Unlock()
		origRect(dst, r, c, filled)
	}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		intercept.mu.Lock()
		intercept.calls = append(intercept.calls, r)
		intercept.mu.Unlock()
		origRound(dst, r, c, radius, filled)
	}
	t.Cleanup(func() {
		drawRect = origRect
		drawRoundedRect = origRound
	})
	return intercept
}

func anyRectInside(rects []image.Rectangle, container image.Rectangle) bool {
	for _, r := range rects {
		if r.Overlaps(container) {
			return true
		}
	}
	return false
}
