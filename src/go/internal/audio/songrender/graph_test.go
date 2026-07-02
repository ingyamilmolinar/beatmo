package songrender

import "testing"

func TestBuildGraph_BachToccata(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	rp, err := BuildGraph(pt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Paths) != len(pt.Instruments) {
		t.Fatalf("got %d row paths, want %d (one per instrument)", len(rp.Paths), len(pt.Instruments))
	}
	if len(rp.Nodes) == 0 {
		t.Error("no nodes in graph snapshot")
	}
	// At least one instrument row must have a non-empty path.
	nonEmpty := 0
	for _, p := range rp.Paths {
		if len(p) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		t.Error("all row paths empty — graph wiring or origin lookup is broken")
	}
}
