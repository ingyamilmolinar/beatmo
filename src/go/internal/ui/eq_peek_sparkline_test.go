//go:build test

package ui

import (
	"math"
	"testing"
)

// newEQPanelZoneForTest constructs an EQPanelZone with empty callbacks for
// unit tests. The returned zone has flat (0 dB) band gains.
func newEQPanelZoneForTest(t *testing.T) *EQPanelZone {
	t.Helper()
	return NewEQPanelZone(EQCallbacks{})
}

func TestEQPanel_SampleCurveReturnsRequestedLength(t *testing.T) {
	z := newEQPanelZoneForTest(t)
	samples := z.SampleCurve(32)
	if len(samples) != 32 {
		t.Fatalf("SampleCurve(32) returned %d samples", len(samples))
	}
}

func TestEQPanel_SampleCurveTracksGains(t *testing.T) {
	z := newEQPanelZoneForTest(t)

	// Flat: all samples within 1 dB of zero.
	flat := z.SampleCurve(32)
	for i, s := range flat {
		if math.Abs(s) > 1.0 {
			t.Fatalf("flat sample[%d]=%.2f exceeds 1 dB tolerance", i, s)
		}
	}

	// Boost band 0 by 12 dB; expect at least one sample > 6 dB.
	z.SetBandGainDB(0, 12.0)
	boosted := z.SampleCurve(32)
	maxSample := math.Inf(-1)
	for _, s := range boosted {
		if s > maxSample {
			maxSample = s
		}
	}
	if maxSample < 6.0 {
		t.Fatalf("expected boosted curve max >6 dB, got %.2f", maxSample)
	}
}
