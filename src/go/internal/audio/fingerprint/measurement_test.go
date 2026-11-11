package fingerprint

import (
	"strings"
	"testing"
)

func TestScalarMeasurement(t *testing.T) {
	m := Scalar(0.42)
	if !m.OK || m.Form != FormScalar || m.Scalar != 0.42 {
		t.Fatalf("got %+v", m)
	}
}

func TestUnavailableCarriesReason(t *testing.T) {
	m := Unavailable("no sustain plateau")
	if m.OK {
		t.Fatal("Unavailable must not be OK")
	}
	if m.NA != "no sustain plateau" {
		t.Fatalf("reason lost: %q", m.NA)
	}
	if m.Scalar != 0 || m.Values != nil {
		t.Fatal("an unavailable measurement must not carry a misleading value")
	}
}

func TestVectorKeepsLabels(t *testing.T) {
	m := Vector([]float64{1, 0.5}, []string{"H1", "H2"})
	if m.Form != FormVector || len(m.Values) != 2 || m.Labels[1] != "H2" {
		t.Fatalf("got %+v", m)
	}
	if !m.OK {
		t.Fatal("Vector must be OK")
	}
}

func TestTrackKeepsHop(t *testing.T) {
	m := Track([]float64{1, 2, 3}, 0.01)
	if m.Form != FormTrack || m.HopSec != 0.01 || len(m.Values) != 3 {
		t.Fatalf("got %+v", m)
	}
}

// TestHeadline_NeverOffersComponentZeroAsTheReading pins the display rule for
// non-scalar measurements. The three concrete regressions it guards are named
// in Headline's doc: band.t60's -1 sentinel printed as "-1.000 s",
// mode.ratios' constant "0 cents" anchor gap, and decay.per_mode's lowest
// mode standing in for a mean.
func TestHeadline_NeverOffersComponentZeroAsTheReading(t *testing.T) {
	scalarD := Descriptor{Scope: ScopeWhole}
	if txt, ok := scalarD.Headline(Scalar(1.5)); ok {
		t.Errorf("a scalar needs no headline override, got %q", txt)
	}
	if txt, ok := scalarD.Headline(Unavailable("nope")); ok {
		t.Errorf("an unavailable measurement needs no headline, got %q", txt)
	}

	// A COMPOUND vector deliberately puts its headline first (ring T60 carries
	// flags behind it), so Primary() is honest there and must be left alone.
	compound := Descriptor{Scope: ScopeWhole, Compound: true}
	if txt, ok := compound.Headline(Vector([]float64{2.5, 1, -30}, nil)); ok {
		t.Errorf("a compound measurement's Values[0] IS the reading, got %q", txt)
	}

	// Band vector: the count reflects the measurable bands, and the -1
	// sentinel is NOT counted as one.
	band := Descriptor{Scope: ScopeBand}
	txt, ok := band.Headline(Vector([]float64{bandT60Unfit, 2.0, 3.5, bandT60Unfit}, nil))
	if !ok {
		t.Fatal("a band vector must get a headline override")
	}
	if txt != "(vector: 2/4 bands measurable)" {
		t.Errorf("band headline = %q", txt)
	}
	if strings.Contains(txt, "-1") {
		t.Errorf("the unfittable sentinel leaked into the display: %q", txt)
	}

	// All-peer non-band vector (mode.ratios, decay.per_mode).
	peer := Descriptor{Scope: ScopeWhole}
	if txt, ok := peer.Headline(Vector([]float64{0, 700, 1200}, nil)); !ok || txt != "(vector: 3 components)" {
		t.Errorf("peer vector headline = %q (ok=%v)", txt, ok)
	}

	// Tracks say how long they are, never what their first frame was.
	if txt, ok := peer.Headline(Track([]float64{1, 2, 3, 4}, 0.01)); !ok || txt != "(track: 4 frames)" {
		t.Errorf("track headline = %q (ok=%v)", txt, ok)
	}
}

// TestHeadline_LiveVectorRowsAreCovered checks the rule against the REAL
// registry rows that motivated it, so a Scope or Compound change on any of
// them re-opens the display hole rather than passing silently.
func TestHeadline_LiveVectorRowsAreCovered(t *testing.T) {
	r := DefaultRegistry()
	for _, id := range []string{IDBandT60, IDModeRatios, IDDecayPerMode} {
		d, ok := r.ByID(id)
		if !ok {
			t.Fatalf("%s not registered", id)
		}
		if d.Compound {
			t.Errorf("%s is marked Compound, which exempts it from the headline rule "+
				"and puts component 0 back in the report column", id)
		}
		if _, ok := d.Headline(Vector([]float64{-1, 2, 3}, nil)); !ok {
			t.Errorf("%s: vector measurement got no headline override", id)
		}
	}
}
