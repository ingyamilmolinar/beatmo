package songrender

import "testing"

func TestBuildArrangement_BachToccataHasNotes(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	arr, err := BuildArrangement(pt, 4, 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(arr.Notes) == 0 {
		t.Fatal("no notes emitted from bach-toccata over 4 bars")
	}
	// Every note references a real instrument id and lands within the timeline.
	valid := map[string]bool{}
	for _, in := range pt.Instruments {
		valid[in.ID] = true
	}
	for _, n := range arr.Notes {
		if !valid[n.InstID] {
			t.Errorf("note with unknown instrument %q", n.InstID)
		}
		if n.AtSample < 0 || n.AtSample >= arr.TotalSamples {
			t.Errorf("note at sample %d outside [0,%d)", n.AtSample, arr.TotalSamples)
		}
	}
}

func TestBuildArrangement_Deterministic(t *testing.T) {
	pt, _ := LoadTemplate("bach-toccata")
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
