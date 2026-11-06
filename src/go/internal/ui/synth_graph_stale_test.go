//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The graph never shows another instrument's sound, and its BEFORE ghost is
// never a stale render (review finding 2026-10-25): a picture that lands late
// belongs to the instrument and document it was asked for, or is not shown.

func manualGraphPipe(t *testing.T, g *Game) (*synthGraphPipeline, *fakeGraphRender) {
	t.Helper()
	f := &fakeGraphRender{}
	p := newSynthGraphPipelineForTest(f.render)
	g.drum.graphPipe = p
	return p, f
}

func drawGraphPane(g *Game) *viewPlot {
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	img := ebiten.NewImage(420, 170)
	dv.synp().drawSynthGraphPane(img, img.Bounds(), inst)
	return dv.synp().graphPlot
}

func TestGraphNeverShowsAnotherInstrumentsRender(t *testing.T) {
	g := newDialGame(t)
	pipe, _ := manualGraphPipe(t, g)
	dv := g.drum
	drawGraphPane(g)
	pipe.drainForTest()
	if vp := drawGraphPane(g); vp.empty != "" {
		t.Fatalf("precondition: the violin's render did not land (%q)", vp.empty)
	}
	dv.Rows[0].Instrument = roster.Kick808
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Kick808) })
	dv.eqPanelZone.SetActiveChannel(channel.Of(roster.Kick808))
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	idleFrame(g)
	vp := drawGraphPane(g) // the kick's render has not landed yet
	if vp.empty == "" {
		t.Fatal("after switching to the kick the graph still drew the violin's render")
	}
	pipe.drainForTest()
	if vp := drawGraphPane(g); vp.empty != "" {
		t.Fatalf("the kick's render landed but the graph shows %q", vp.empty)
	}
}

func TestBeforeIsTheSoundTheGestureStartedFrom(t *testing.T) {
	g := newDialGame(t)
	pipe, _ := manualGraphPipe(t, g)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	drawGraphPane(g)
	pipe.drainForTest()
	stale, _ := pipe.get(slotNow)
	// An edit lands in the document; its render has not landed yet.
	audio.SetInstrumentParamsLive(inst, map[string]float64{"pitch": 7})
	drawGraphPane(g)
	c := dv.instEditorMacroCards[1]
	dv.setSynthFocusDial(inst, c.id, c.dials[0].id) // the gesture starts now
	if b := dv.graphBefore; b.r == stale {
		t.Fatal("BEFORE captured a render of the document before the edit (stale)")
	}
	pipe.drainForTest()
	drawGraphPane(g)
	if b := dv.graphBefore; b.r == nil || b.r == stale {
		t.Fatalf("BEFORE must become the render of the document the gesture started from once it lands (got %v)", b.r)
	}
}

// While a dial turns, its two ends do not change (Turn lands exactly on
// them), so they render ONCE per focus: each detent renders only NOW (spec
// 2026-10-24 §4.3; review finding 2026-10-25: three renders per detent).
func TestEndGhostsDoNotReRenderWhileTheirDialTurns(t *testing.T) {
	g := newDialGame(t)
	pipe, f := manualGraphPipe(t, g)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[1]
	d, ok := dv.synthDialByID(c.id, c.dials[0].id)
	if !ok {
		t.Fatal("no dial")
	}
	dv.setSynthFocusDial(inst, c.id, c.dials[0].id)
	settle := func() {
		for i := 0; i < 3; i++ {
			drawGraphPane(g)
			pipe.drainForTest()
		}
	}
	settle()
	base := f.calls
	gs := newSynthDialGesture(inst, c.id, d)
	for i, target := range []int{20, 40, 60, 80} {
		dv.turnSynthDial(gs, target)
		settle()
		if got := f.calls - base; got > i+1 {
			t.Fatalf("after %d detents %d renders ran; want %d (NOW only — the ends did not change)", i+1, got, i+1)
		}
	}
	dv.endSynthDialGesture(gs, true)
}

// A language switch re-words the graph — the overview's title and readout too,
// whose plot key does not change with the locale (review finding 2026-10-25).
func TestGraphFollowsALanguageSwitch(t *testing.T) {
	g := newDialGame(t)
	pipe, _ := manualGraphPipe(t, g)
	drawGraphPane(g)
	pipe.drainForTest()
	en := drawGraphPane(g)
	if en.title != synthViewTitle(viewOverview) {
		t.Fatalf("precondition: overview title %q", en.title)
	}
	i18n.SetLocale(i18n.LocaleES)
	t.Cleanup(func() { i18n.SetLocale(i18n.LocaleEN) })
	g.drum.OnLocaleChanged()
	es := drawGraphPane(g)
	if es.title != i18n.T(i18n.KeyGraphViewOverview) || es.title == en.title {
		t.Fatalf("after switching to Spanish the overview still reads %q", es.title)
	}
}

// A render is identified by what it SOUNDS like (the sound signature, tempo,
// pitch, Sampler edit), never by the document's revision: returning to a
// sound the graph already rendered costs nothing, and a dial turned to one of
// its ends shows the end ghost's render instead of rendering it again.
func TestTheSameSoundIsRenderedOnce(t *testing.T) {
	g := newDialGame(t)
	pipe, f := manualGraphPipe(t, g)
	dv := g.drum
	inst := instrument.ID(dialInst(g))
	c := dv.instEditorMacroCards[1]
	d, ok := dv.synthDialByID(c.id, c.dials[0].id)
	if !ok {
		t.Fatal("no dial")
	}
	settle := func() {
		for i := 0; i < 3; i++ {
			drawGraphPane(g)
			pipe.drainForTest()
		}
	}
	dv.setSynthFocusDial(inst, c.id, c.dials[0].id)
	settle() // NOW + the two ends
	base := f.calls
	start := synthDialPercent(d.Reading(audio.MergedInstrumentParamsRO(inst)))
	gs := newSynthDialGesture(inst, c.id, d)
	dv.turnSynthDial(gs, 0) // lands exactly on the LO end
	settle()
	if got := f.calls - base; got != 0 {
		t.Fatalf("turning the dial to its LO end rendered %d more time(s): the end ghost already is that sound", got)
	}
	dv.turnSynthDial(gs, start) // back to the start values, bit for bit
	settle()
	if got := f.calls - base; got != 0 {
		t.Fatalf("returning to the starting sound rendered %d more time(s): it was rendered before", got)
	}
	dv.endSynthDialGesture(gs, false)
}
