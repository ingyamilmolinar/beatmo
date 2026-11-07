package songrender

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestBuildArrangement_BachToccataHasNotes(t *testing.T) {
	pt, err := LoadTemplate("bach-brandenburg-3")
	if err != nil {
		t.Fatal(err)
	}
	arr, err := BuildArrangement(pt, 4, 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(arr.Notes) == 0 {
		t.Fatal("no notes emitted from bach-brandenburg-3 over 4 bars")
	}
	// Every note references a real instrument id and lands within the timeline.
	valid := map[string]bool{}
	for _, in := range pt.Instruments {
		valid[string(in.ID)] = true
	}
	for _, n := range arr.Notes {
		if !valid[string(n.InstID)] {
			t.Errorf("note with unknown instrument %q", n.InstID)
		}
		if n.AtSample < 0 || n.AtSample >= arr.TotalSamples {
			t.Errorf("note at sample %d outside [0,%d)", n.AtSample, arr.TotalSamples)
		}
	}
}

func TestBuildArrangement_Deterministic(t *testing.T) {
	pt, _ := LoadTemplate("bach-brandenburg-3")
	a, _ := BuildArrangement(pt, 2, 44100)
	b, _ := BuildArrangement(pt, 2, 44100)
	if len(a.Notes) != len(b.Notes) {
		t.Fatalf("non-deterministic note count: %d vs %d", len(a.Notes), len(b.Notes))
	}
	for i := range a.Notes {
		if a.Notes[i] != b.Notes[i] {
			t.Fatalf("note %d differs between runs", i)
		}
	}
}

// Subdiv is units per BEAT: at 100 BPM the snare on beat two lands 60/100 s in.
func TestArrangementRunsAtTheTemplateTempo(t *testing.T) {
	pt, err := LoadTemplate("funky-drummer")
	if err != nil {
		t.Fatal(err)
	}
	const sr = 48000
	arr, err := BuildArrangement(pt, 1, sr)
	if err != nil {
		t.Fatal(err)
	}
	beat := 60.0 / float64(pt.BPM) * sr
	if got, want := arr.TotalSamples, int(4*beat+0.5); got < want-1 || got > want+1 {
		t.Errorf("one bar is %d samples, want %d (four beats at %d BPM)", got, want, pt.BPM)
	}
	var snare []int
	for _, n := range arr.Notes {
		if n.InstID == roster.SnareAcoustic {
			snare = append(snare, n.AtSample)
		}
	}
	if len(snare) == 0 || absInt(snare[0]-int(beat+0.5)) > 1 {
		t.Errorf("first backbeat at sample %v, want %d (beat two)", snare, int(beat+0.5))
	}
}

// (r-1)/(r+1) of a step late: 1/12 at 13:11. The downbeat beside it does not move.
func TestArrangementAppliesEachNodesGroove(t *testing.T) {
	pt, err := LoadTemplate("funky-drummer")
	if err != nil {
		t.Fatal(err)
	}
	const sr = 48000
	arr, err := BuildArrangement(pt, 1, sr)
	if err != nil {
		t.Fatal(err)
	}
	step := StepSeconds(pt.BPM, pt.Subdiv) * sr
	checked := 0
	for _, n := range arr.Notes {
		if n.InstID != roster.Hihat || n.AbsStep > 3 {
			continue
		}
		want := float64(n.AbsStep) * step
		if n.AbsStep%2 == 1 {
			want += math.Round(1000.0/12) / 1000 * step // the template stores the offset to 0.001
		}
		if math.Abs(float64(n.AtSample)-want) > 1 {
			t.Errorf("hat at step %d: sample %d, want %.0f", n.AbsStep, n.AtSample, want)
		}
		checked++
	}
	if checked != 4 {
		t.Errorf("checked %d hats in the first beat, want 4 sixteenths", checked)
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Pitch = node pitch + layer semis, volume × layer gain.
func TestArrangementChordEmitsOneNoteEventPerLayer(t *testing.T) {
	pt, err := LoadTemplate("funky-drummer")
	if err != nil {
		t.Fatal(err)
	}
	const sr = 48000
	base, err := BuildArrangement(pt, 1, sr)
	if err != nil {
		t.Fatal(err)
	}
	chordIdx := -1
	for i, n := range pt.Nodes {
		if n.Type == model.NodeTypeRegular {
			chordIdx = i
			break
		}
	}
	if chordIdx < 0 {
		t.Fatal("template has no regular node")
	}
	pt.Nodes[chordIdx].Type = model.NodeTypeChord
	pt.Nodes[chordIdx].Chord = &ChordSpec{Quality: "m7"}
	got, err := BuildArrangement(pt, 1, sr)
	if err != nil {
		t.Fatal(err)
	}
	type cell struct{ row, abs int }
	group := func(a Arrangement) map[cell][]NoteEvent {
		m := map[cell][]NoteEvent{}
		for _, n := range a.Notes {
			m[cell{n.Row, n.AbsStep}] = append(m[cell{n.Row, n.AbsStep}], n)
		}
		return m
	}
	b, c := group(base), group(got)
	chords := 0
	for k, ns := range c {
		ref := b[k]
		if len(ref) != 1 {
			t.Fatalf("baseline cell %v has %d notes", k, len(ref))
		}
		switch len(ns) {
		case 1:
			if ns[0] != ref[0] {
				t.Fatalf("cell %v changed: %+v vs %+v", k, ns[0], ref[0])
			}
		case 4:
			chords++
			for i, semis := range []float64{0, 3, 7, 10} {
				if ns[i].PitchSemis != ref[0].PitchSemis+semis || math.Abs(ns[i].Volume-ref[0].Volume*0.5) > 1e-12 || ns[i].AtSample != ref[0].AtSample {
					t.Fatalf("cell %v voice %d = %+v (base %+v)", k, i, ns[i], ref[0])
				}
			}
		default:
			t.Fatalf("cell %v has %d notes", k, len(ns))
		}
	}
	if chords == 0 || len(c) != len(b) {
		t.Fatalf("chord cells=%d, cells %d vs %d", chords, len(c), len(b))
	}
}
