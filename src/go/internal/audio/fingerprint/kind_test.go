package fingerprint

import "testing"

func TestKindThresholds(t *testing.T) {
	tests := []struct {
		name            string
		kind            Kind
		drift, mismatch float64
	}{
		{"fraction", KindFraction, 0.05, 0.15},
		{"tiny fraction", KindTinyFraction, 0.005, 0.02},
		{"series deviation", KindSeriesDeviation, 0.15, 0.5},
		{"seconds", KindSeconds, 1.3, 2.0},
		{"hz", KindHz, 10.0, 40.0},
	}
	for _, tc := range tests {
		d, m := tc.kind.Thresholds()
		if d != tc.drift || m != tc.mismatch {
			t.Errorf("%s: got (%v,%v), want (%v,%v)", tc.name, d, m, tc.drift, tc.mismatch)
		}
	}
}

func TestVerdictOrdering(t *testing.T) {
	if !VerdictMismatch.WorseThan(VerdictDrift) || !VerdictDrift.WorseThan(VerdictOK) {
		t.Fatal("verdict severity ordering is wrong")
	}
	if VerdictOK.WorseThan(VerdictOK) {
		t.Fatal("a verdict is not worse than itself")
	}
}
