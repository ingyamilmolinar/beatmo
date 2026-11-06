package fingerprint

import "testing"

func TestOrganicIDsAreRegistered(t *testing.T) {
	for _, id := range OrganicIDs() {
		if _, ok := DefaultRegistry().ByID(id); !ok {
			t.Errorf("organic id %q is not a registered descriptor", id)
		}
	}
}

func TestOrganicRangePlace(t *testing.T) {
	r := OrganicRange{"x": {P10: 1, P50: 2, P90: 3, N: 10}}
	if pos, where, ok := r.Place("x", 2.5); !ok || where != "inside" || pos != 0.25 {
		t.Fatalf("got %v %s %v", pos, where, ok)
	}
	if _, where, _ := r.Place("x", 0.5); where != "below" {
		t.Fatal("0.5 is below p10")
	}
	if _, where, _ := r.Place("x", 9); where != "above" {
		t.Fatal("9 is above p90")
	}
	if _, _, ok := r.Place("missing", 1); ok {
		t.Fatal("unknown id must not be ok")
	}
}

func TestComputeOrganicRangePercentiles(t *testing.T) {
	vals := map[string][]float64{"x": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
	r := ComputeOrganicRange(vals)
	e := r["x"]
	if e.N != 10 || e.P50 < 5 || e.P50 > 6 || e.P10 > 2 || e.P90 < 9 {
		t.Fatalf("unexpected entry %+v", e)
	}
}

func TestEmbeddedOrganicRangeParses(t *testing.T) {
	_ = LoadOrganicRange() // must not panic; may be empty before calibrate-organic has run
}

func TestOrganicRangePlace_LowNRefusesToClassify(t *testing.T) {
	r := OrganicRange{"x": {P10: 1, P50: 2, P90: 3, N: OrganicMinN - 1}}
	if _, _, ok := r.Place("x", 2.5); ok {
		t.Fatal("N below OrganicMinN must not be ok")
	}
	r["x"] = RangeEntry{P10: 1, P50: 2, P90: 3, N: OrganicMinN}
	if _, _, ok := r.Place("x", 2.5); !ok {
		t.Fatal("N == OrganicMinN must be ok")
	}
}
