//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

// Words until the user asks for Details; one toggle switches to numbers, remembered across sessions.

type fakePanelSink struct {
	doc   userprefs.AudioPanelStateDoc
	saves int
}

func (f *fakePanelSink) LoadAudioPanelState() (userprefs.AudioPanelStateDoc, error) {
	return f.doc, nil
}
func (f *fakePanelSink) SaveAudioPanelState(d userprefs.AudioPanelStateDoc) error {
	f.doc = d
	f.saves++
	return nil
}

func withPanelSink(t *testing.T, f *fakePanelSink) {
	t.Helper()
	SetAudioPanelStateSink(f)
	t.Cleanup(func() { SetAudioPanelStateSink(nil) })
}

func pressDetails(t *testing.T, g *Game) {
	t.Helper()
	inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
	for _, h := range g.drum.synp().synthMacroHitAreas(inst) {
		if h.Tag == "synth-graph-details" {
			r := h.Rect
			c := r.Min.Add(r.Size().Div(2))
			h.Handler.OnPress(c.X, c.Y)
			h.Handler.OnRelease(c.X, c.Y)
			return
		}
	}
	t.Fatal("the graph has no Details toggle")
}

func TestDetailsToggleShowsTheNumbersAndIsRemembered(t *testing.T) {
	sink := &fakePanelSink{}
	withPanelSink(t, sink)
	g := newDialGame(t)
	pipe, _ := manualGraphPipe(t, g)
	drawGraphPane(g)
	pipe.drainForTest()
	vp := drawGraphPane(g)
	if vp.numbers || vp.shownReadout() != vp.summary {
		t.Fatal("the graph must open on its WORDS readout")
	}
	pressDetails(t, g)
	vp = drawGraphPane(g)
	if !vp.numbers || vp.shownReadout() != vp.readout {
		t.Fatal("Details on: the graph must show its NUMBERS readout")
	}
	if !sink.doc.SynthGraphDetails || sink.saves != 1 {
		t.Fatalf("Details on was not remembered (saved %d times, doc %+v)", sink.saves, sink.doc)
	}
	// A new session starts where the last one left off.
	g2 := newDialGame(t)
	pipe2, _ := manualGraphPipe(t, g2)
	drawGraphPane(g2)
	pipe2.drainForTest()
	if vp := drawGraphPane(g2); !vp.numbers {
		t.Fatal("a new session forgot the Details choice")
	}
	pressDetails(t, g2)
	if vp := drawGraphPane(g2); vp.numbers || sink.doc.SynthGraphDetails {
		t.Fatal("Details off did not return to words, or was not remembered")
	}
}
