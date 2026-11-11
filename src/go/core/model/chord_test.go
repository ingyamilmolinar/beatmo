package model

import (
	"math"
	"reflect"
	"testing"
)

func TestChordQualityTableShape(t *testing.T) {
	if len(ChordQualities) != 20 {
		t.Fatalf("want 20 qualities, got %d", len(ChordQualities))
	}
	for _, p := range ChordPages {
		if n := len(ChordQualitiesOnPage(p)); n != 5 {
			t.Fatalf("page %s has %d qualities, want 5", p, n)
		}
	}
	seen := map[string]bool{}
	for _, q := range ChordQualities {
		if seen[q.ID] {
			t.Fatalf("duplicate quality id %s", q.ID)
		}
		seen[q.ID] = true
		if q.Intervals[0] != 0 {
			t.Fatalf("%s must start at the root", q.ID)
		}
		for i := 1; i < len(q.Intervals); i++ {
			if q.Intervals[i] <= q.Intervals[i-1] {
				t.Fatalf("%s intervals not ascending: %v", q.ID, q.Intervals)
			}
		}
	}
}

func layerSemis(ls []Layer) []float64 {
	out := make([]float64, len(ls))
	for i, l := range ls {
		out[i] = l.Semis
	}
	return out
}

func TestNodeLayersSingleVoiceForNonChordTypes(t *testing.T) {
	p := NodeParams{Chord: ChordSpec{Quality: "maj7"}}
	for _, ty := range []NodeType{NodeTypeRegular, NodeTypeSilent, NodeTypeMute, NodeTypeInvisible} {
		if NodeLayers(ty, p) != nil {
			t.Fatalf("type %d with a dormant spec must stay single-voice", ty)
		}
	}
	if got := layerSemis(NodeLayers(NodeTypeChord, NodeParams{})); !reflect.DeepEqual(got, []float64{0, 4, 7}) {
		t.Fatalf("chord with an empty spec is a major triad, got %v", got)
	}
}

func TestNodeLayersVoicings(t *testing.T) {
	cases := []struct {
		spec ChordSpec
		want []float64
	}{
		{ChordSpec{Quality: "maj"}, []float64{0, 4, 7}},
		{ChordSpec{Quality: "maj", Inversion: 1}, []float64{4, 7, 12}},
		{ChordSpec{Quality: "maj", Inversion: 2}, []float64{7, 12, 16}},
		{ChordSpec{Quality: "maj", Spread: ChordSpreadDrop2}, []float64{-8, 0, 7}},
		{ChordSpec{Quality: "maj", Spread: ChordSpreadWide}, []float64{-12, -8, 7}},
		{ChordSpec{Quality: "maj7", Inversion: 3}, []float64{11, 12, 16, 19}},
		{ChordSpec{Quality: "5", Spread: ChordSpreadDrop2}, []float64{-12, 7}},
		{ChordSpec{Quality: "5", Spread: ChordSpreadWide}, []float64{-12, 7}},
		{ChordSpec{Quality: "9"}, []float64{0, 4, 7, 10, 14}},
	}
	for _, c := range cases {
		got := layerSemis(NodeLayers(NodeTypeChord, NodeParams{Chord: c.spec}))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%+v: got %v want %v", c.spec, got, c.want)
		}
	}
	ls := NodeLayers(NodeTypeChord, NodeParams{Chord: ChordSpec{Quality: "maj7"}})
	for _, l := range ls {
		if math.Abs(l.Gain-0.5) > 1e-12 {
			t.Fatalf("a 4-voice chord's gain must be 1/sqrt(4), got %v", l.Gain)
		}
	}
}

func TestNormalizeChordClamps(t *testing.T) {
	got := NormalizeChord(ChordSpec{Quality: "maj", Inversion: 3, Spread: 9})
	if got.Inversion != 2 || got.Spread != ChordSpreadWide {
		t.Fatalf("triad clamps to inversion 2 / spread wide, got %+v", got)
	}
	if got := NormalizeChord(ChordSpec{Quality: "maj7", Inversion: -1, Spread: -1}); got.Inversion != 0 || got.Spread != 0 {
		t.Fatalf("negative values clamp to 0, got %+v", got)
	}
	if !NormalizeChord(ChordSpec{Quality: "nope", Inversion: 1}).IsZero() {
		t.Fatal("an unknown quality must normalize to the zero spec")
	}
}

func TestChordName(t *testing.T) {
	cases := []struct {
		midi int
		spec ChordSpec
		want string
	}{
		{60, ChordSpec{Quality: "maj"}, "C"},
		{61, ChordSpec{Quality: "m7"}, "C#m7"},
		{60, ChordSpec{Quality: "maj7", Inversion: 1}, "Cmaj7/E"},
		{60, ChordSpec{Quality: "maj", Inversion: 2}, "C/G"},
		{60, ChordSpec{Quality: "dim"}, "C°"},
		{60, ChordSpec{Quality: "m7b5"}, "Cø7"},
		{60, ChordSpec{Quality: "madd9"}, "Cm(add9)"},
		{60, ChordSpec{Quality: "maj", Spread: ChordSpreadWide}, "C"},
		{59, ChordSpec{Quality: "7", Inversion: 3}, "B7/A"},
	}
	for _, c := range cases {
		if got := ChordName(c.midi, c.spec); got != c.want {
			t.Errorf("ChordName(%d, %+v) = %q, want %q", c.midi, c.spec, got, c.want)
		}
	}
}

func TestNodeTypeNameRoundTrip(t *testing.T) {
	for _, ty := range []NodeType{NodeTypeRegular, NodeTypeSilent, NodeTypeMute, NodeTypeInvisible, NodeTypeChord} {
		got, err := ParseNodeType(ty.String())
		if err != nil || got != ty {
			t.Fatalf("type %d did not round-trip through %q", ty, ty.String())
		}
	}
	if NodeTypeChord.String() != "chord" {
		t.Fatalf("chord name = %q", NodeTypeChord.String())
	}
	if _, err := ParseNodeType("bogus"); err == nil {
		t.Fatal("unknown names must not parse")
	}
}

func TestNodeTypeFires(t *testing.T) {
	want := map[NodeType]bool{NodeTypeRegular: true, NodeTypeChord: true,
		NodeTypeSilent: false, NodeTypeMute: false, NodeTypeInvisible: false}
	for ty, w := range want {
		if ty.Fires() != w {
			t.Fatalf("type %d Fires() = %v, want %v", ty, ty.Fires(), w)
		}
	}
}

func TestSetNodeParamsCopiesChord(t *testing.T) {
	g := NewGraph(testLogger)
	id := g.AddNode(0, 0, NodeTypeChord)
	g.SetNodeParams(id, NodeParams{Volume: 1, Duration: 1, Chord: ChordSpec{Quality: "m7", Inversion: 1}})
	if c := g.Nodes[id].Params.Chord; c.Quality != "m7" || c.Inversion != 1 {
		t.Fatalf("SetNodeParams dropped the chord spec: %+v", c)
	}
}
