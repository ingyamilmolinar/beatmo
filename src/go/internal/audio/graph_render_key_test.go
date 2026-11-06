package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The graph-render cache key moves with every input the render reads — the
// document's values, the tempo, the pitch, the Sampler edit — and is stable
// otherwise (spec 2026-10-24 §4.3).
func TestGraphRenderKeyFollowsEveryInput(t *testing.T) {
	id := roster.Kick808
	d, ok := EnsureInstrumentDoc(id)
	if !ok {
		t.Fatal("no kick-808 document")
	}
	k := GraphRenderKey(id, d, 0)
	if GraphRenderKey(id, d, 0) != k {
		t.Fatal("the key is not stable for identical inputs")
	}
	d2, err := InstrumentDocWithFields(id, map[string]float64{"gen1_kick_pe_amt": 0.5})
	if err != nil {
		t.Fatal(err)
	}
	if GraphRenderKey(id, d2, 0) == k {
		t.Fatal("a changed document value kept the key")
	}
	longer := d.Clone()
	longer.Render.Seconds *= 2
	if GraphRenderKey(id, longer, 0) == k {
		t.Fatal("a changed note length kept the key")
	}
	if GraphRenderKey(id, d, 2) == k {
		t.Fatal("a changed pitch kept the key")
	}
	SetSampleEdit(id, SampleEdit{StartFrac: 0.1, EndFrac: 0.8, Reverse: true})
	t.Cleanup(func() { ClearSampleEdit(id) })
	if GraphRenderKey(id, d, 0) == k {
		t.Fatal("a Sampler edit kept the key")
	}
}

// GraphVoiceSec is the note's played length: positive, render.seconds.
func TestGraphVoiceSecIsTheNoteLength(t *testing.T) {
	d, ok := EnsureInstrumentDoc(roster.Kick808)
	if !ok {
		t.Fatal("no kick-808 document")
	}
	if s := GraphVoiceSec(d); s <= 0 {
		t.Fatalf("GraphVoiceSec = %v", s)
	}
}
