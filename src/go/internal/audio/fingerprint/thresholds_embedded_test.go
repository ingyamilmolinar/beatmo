package fingerprint

import "testing"

// The frozen-baseline replay uses the Kind defaults and never reads the file, so nothing else catches a malformed
// entry (mismatch below drift, a typo'd ID, a row whose comparator ignores thresholds). The file is also hand-edited,
// so it gets source-level scrutiny.

func TestEmbeddedThresholdsAreWellFormed(t *testing.T) {
	f := LoadThresholdFile()
	if len(f) == 0 {
		t.Skip("no calibration checked in yet")
	}
	r := DefaultRegistry()
	for id, e := range f {
		d, ok := r.ByID(id)
		if !ok {
			t.Errorf("%s: no such descriptor — a renamed or deleted ID leaves a dead entry that silently calibrates nothing", id)
			continue
		}
		if e.Drift <= 0 {
			t.Errorf("%s: drift=%g — Descriptor.Thresholds reads a non-positive Drift as 'no override' "+
				"and falls back to the KIND's drift, which can exceed the calibrated mismatch and invert the two bands",
				id, e.Drift)
		}
		if e.Mismatch <= e.Drift {
			t.Errorf("%s: mismatch=%g must exceed drift=%g, or every flagged row is a MISMATCH and the drift band is unreachable",
				id, e.Mismatch, e.Drift)
		}
		switch e.Source {
		case "corpus", "probe", "hand":
		default:
			t.Errorf("%s: source=%q, want corpus|probe|hand", id, e.Source)
		}
		if e.Source == "corpus" && e.N < 5 {
			t.Errorf("%s: corpus entry with n=%d — below n=5 the p90 is the sample maximum, "+
				"i.e. one family's worst reading standing in for a threshold", id, e.N)
		}
		if e.Source != "corpus" && e.Note == "" {
			t.Errorf("%s: a %q entry overrides the corpus and must say why — "+
				"an unexplained pin is indistinguishable from a stale one", id, e.Source)
		}
		if d.Demoted {
			t.Errorf("%s: descriptor is Demoted and excluded from the trusted set — "+
				"calibrating thresholds nobody is allowed to believe buys nothing and "+
				"makes its documented hand values unreadable", id)
		}
	}
}

// Worth anything only if the live registry serves its numbers.
func TestEmbeddedThresholdsReachTheRegistry(t *testing.T) {
	f := LoadThresholdFile()
	if len(f) == 0 {
		t.Skip("no calibration checked in yet")
	}
	r := DefaultRegistry()
	checked := 0
	for id, e := range f {
		d, ok := r.ByID(id)
		if !ok {
			continue
		}
		drift, mismatch := d.Thresholds()
		if drift != e.Drift || mismatch != e.Mismatch {
			t.Errorf("%s: registry serves drift=%g mismatch=%g, file says %g/%g",
				id, drift, mismatch, e.Drift, e.Mismatch)
		}
		if got := r.CalibratedFrom(id); got != e.Source {
			t.Errorf("%s: CalibratedFrom=%q, file says %q", id, got, e.Source)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no file entry matched a registered descriptor — the gate has gone hollow")
	}
	t.Logf("%d calibrated descriptors verified against the live registry", checked)
}
