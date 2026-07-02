package songrender

import "testing"

func TestLoadTemplate_BachToccata(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	if pt.BPM <= 0 || pt.Subdiv <= 0 {
		t.Errorf("bad header: bpm=%d subdiv=%d", pt.BPM, pt.Subdiv)
	}
	if len(pt.Instruments) == 0 || len(pt.Nodes) == 0 {
		t.Fatalf("empty: %d instruments, %d nodes", len(pt.Instruments), len(pt.Nodes))
	}
	// Organ is the Bach toccata instrument.
	found := false
	for _, in := range pt.Instruments {
		if in.ID == "organ" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an organ instrument in bach-toccata")
	}
}

func TestLoadTemplate_UnknownStem(t *testing.T) {
	if _, err := LoadTemplate("does-not-exist"); err == nil {
		t.Error("expected error for unknown stem")
	}
}
