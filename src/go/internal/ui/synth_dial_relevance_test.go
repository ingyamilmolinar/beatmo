//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The DSP decides which dials an instrument shows (spec 2026-10-25 §3.2): a
// dial whose full 0→100 sweep does not change the rendered note audibly is
// hidden. Under -tags test the renderer is the toy voice
// (graph_render_teststub.go), which reads the filter CUTOFF but never the
// RESONANCE — so on it Bright is audible and Edge is not.

// relevanceGame opens the Synth root on piano-felt (Bright and Edge both
// config-live) with a manually drained graph pipeline rendering the toy voice.
// edit, when non-nil, changes the sound BEFORE the tab first sees it, so no
// generated verdict knows it and the DSP must measure it at runtime.
func relevanceGame(t *testing.T, edit map[string]float64) (*Game, *synthGraphPipeline) {
	t.Helper()
	g := newViolinMacroGame(t)
	g.drum.Rows[0].Instrument = roster.PianoFelt
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.PianoFelt) })
	if edit != nil {
		audio.SetInstrumentParams(roster.PianoFelt, edit)
	}
	pipe := newSynthGraphPipelineForTest(audio.RenderDocForGraph)
	g.drum.graphPipe = pipe
	return openDialRoot(t, g), pipe
}

func rootHasDial(g *Game, card, dial string) bool {
	_, ok := g.drum.synthDialByID(card, dial)
	return ok
}

// settleRelevance runs every queued render and relevance probe, then lays the
// tab out again (what the next frames do in the app).
func settleRelevance(g *Game, pipe *synthGraphPipeline) {
	for i := 0; i < 3; i++ {
		pipe.drainForTest()
		g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
		idleFrame(g)
	}
}

func TestADialTheDSPHearsNothingFromIsHidden(t *testing.T) {
	g, pipe := relevanceGame(t, map[string]float64{"amp_decay": 0.777})
	if !rootHasDial(g, "brightness", "edge") || !rootHasDial(g, "brightness", "bright") {
		t.Fatal("precondition: until the DSP has measured the sound, every config-live dial shows (Bright and Edge)")
	}
	settleRelevance(g, pipe)
	if rootHasDial(g, "brightness", "edge") {
		t.Fatal("Edge still shows, although turning it 0→100 does not change the rendered note")
	}
	if !rootHasDial(g, "brightness", "bright") {
		t.Fatal("Bright was hidden, although turning it changes the rendered note")
	}
}

func TestDialRelevanceIsAnchoredWhileTheInstrumentIsEdited(t *testing.T) {
	g, pipe := relevanceGame(t, map[string]float64{"amp_decay": 0.777})
	settleRelevance(g, pipe)
	measured := pipe.relevanceMeasured()
	if measured != 1 {
		t.Fatalf("precondition: one sound measured, got %d", measured)
	}
	// Editing in the tab never re-hides or re-shows dials (no card jumps):
	// the verdict belongs to the instrument as the tab first saw it.
	inst := instrument.ID(dialInst(g))
	d, ok := g.drum.synthDialByID("brightness", "bright")
	if !ok {
		t.Fatal("no Bright dial")
	}
	g.drum.setSynthDialReading(inst, d, 3)
	settleRelevance(g, pipe)
	if got := pipe.relevanceMeasured(); got != measured {
		t.Fatalf("an edit in the tab measured the sound again (%d → %d)", measured, got)
	}
	if rootHasDial(g, "brightness", "edge") {
		t.Fatal("an edit in the tab brought a hidden dial back")
	}
	// Opening a circuit anchors anew: the edited sound is measured.
	snap := audio.CircuitSnapshot()
	t.Cleanup(func() { audio.RestoreCircuit(snap) })
	audio.SetCircuitInstrumentDocs(snap.Docs)
	g.drum.eqPanelZone.SetActiveChannel(channel.Of(roster.PianoFelt))
	settleRelevance(g, pipe)
	if got := pipe.relevanceMeasured(); got != measured+1 {
		t.Fatalf("a newly opened circuit did not re-measure its instrument's sound (%d → %d)", measured, got)
	}
}

// A shipped sound's verdict is in the generated table: it applies at the
// first frame, without rendering anything (the measurement runs only for
// sounds no table knows — the startup demo's edits, user and imported
// instruments).
func TestShippedSoundsTakeTheirVerdictFromTheTable(t *testing.T) {
	g := newViolinMacroGame(t)
	g.drum.Rows[0].Instrument = roster.Cowbell
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Cowbell) })
	pipe := newSynthGraphPipelineForTest(audio.RenderDocForGraph)
	g.drum.graphPipe = pipe
	openDialRoot(t, g)
	if pipe.relevancePending() {
		t.Fatal("a shipped sound queued a relevance measurement instead of reading the table")
	}
	if rootHasDial(g, "tone", "sizzle") {
		t.Fatal("the cowbell shows Sizzle: the table says its 0→100 sweep is inaudible there")
	}
	if !rootHasDial(g, "tone", "bright") {
		t.Fatal("the cowbell lost Bright, which the table keeps")
	}
}

// A sound's probes run one at a time even when the pool's worker and a
// synchronous drain (scene setup) both step the queue: every probe lands in
// the verdict exactly once and the queue never pops twice (a double pop
// panicked on the worker while holding the pipeline's lock and froze the app).
func TestRelevanceStepsFromTwoGoroutinesFinishEachSoundOnce(t *testing.T) {
	pipe := newSynthGraphPipelineForTest(func(id instrument.ID, d instrument.Doc, pitch float64) ([]float32, int, error) {
		out := make([]float32, 4800)
		for i := range out {
			out[i] = float32(0.5 * float64(i%100) / 100)
		}
		return out, 48000, nil
	})
	d, ok := audio.EnsureInstrumentDoc(roster.PianoFelt)
	if !ok {
		t.Fatal("no document")
	}
	var probes []dialProbe
	for i := 0; i < 12; i++ {
		probes = append(probes, dialProbe{key: "card/" + string(rune('a'+i)), view: viewLoudness})
	}
	for sig := uint64(1); sig <= 3; sig++ {
		pipe.measureRelevance(sig, roster.PianoFelt, d, probes)
	}
	done := make(chan struct{})
	for g := 0; g < 2; g++ {
		go func() {
			for pipe.relevancePending() {
				pipe.relevanceStep()
			}
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	if got := pipe.relevanceMeasured(); got != 3 {
		t.Fatalf("%d sounds measured, want 3", got)
	}
	for sig := uint64(1); sig <= 3; sig++ {
		if h, ok := pipe.relevanceVerdict(sig); !ok || len(h) != len(probes) {
			t.Fatalf("sound %d: verdict %v (%d of %d probes)", sig, ok, len(h), len(probes))
		}
	}
}
