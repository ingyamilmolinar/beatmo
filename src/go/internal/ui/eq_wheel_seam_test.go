//go:build test

package ui

import "testing"

// Clamped to [-12, 12]; the live callbacks fire, the commit does not.
func TestApplyBandGainLive_WritesClampsAndFiresLive(t *testing.T) {
	var gotBand int
	var gotDB float64
	gainCalls, applyCalls, commitCalls := 0, 0, 0
	z := NewEQPanelZone(EQCallbacks{
		OnGainChange:   func(band int, db float64) { gainCalls++; gotBand = band; gotDB = db },
		OnApplyEQ:      func() { applyCalls++ },
		OnEQBandCommit: func() { commitCalls++ },
	})

	z.applyBandGainLive(2, 5.5)
	if z.bandGainsDB[2] != 5.5 {
		t.Fatalf("bandGainsDB[2] = %v, want 5.5", z.bandGainsDB[2])
	}
	if gainCalls != 1 || gotBand != 2 || gotDB != 5.5 {
		t.Fatalf("OnGainChange calls=%d band=%d db=%v, want 1/2/5.5", gainCalls, gotBand, gotDB)
	}
	if applyCalls != 1 {
		t.Fatalf("OnApplyEQ calls=%d, want 1", applyCalls)
	}
	if commitCalls != 0 {
		t.Fatalf("OnEQBandCommit calls=%d, want 0 (commit is release-only)", commitCalls)
	}

	z.applyBandGainLive(2, 99) // out of range
	if z.bandGainsDB[2] != 12 {
		t.Fatalf("clamp high: bandGainsDB[2] = %v, want 12", z.bandGainsDB[2])
	}
	z.applyBandGainLive(2, -99)
	if z.bandGainsDB[2] != -12 {
		t.Fatalf("clamp low: bandGainsDB[2] = %v, want -12", z.bandGainsDB[2])
	}
}

// The only dB entry besides the curve handles.
func TestEQDBOpenAdapter_RoutesToWheelCallback(t *testing.T) {
	wheelBand := -1
	z := NewEQPanelZone(EQCallbacks{
		OnOpenValueWheel: func(band int) { wheelBand = band },
	})
	a := &eqDBOpenAdapter{z: z, band: 4}
	if res := a.OnPress(0, 0); res != InputConsumed {
		t.Fatalf("OnPress result = %v, want InputConsumed", res)
	}
	if wheelBand != 4 {
		t.Fatalf("OnOpenValueWheel band = %d, want 4", wheelBand)
	}

	// Nothing opens, but the press is consumed so it cannot reach a sibling.
	z2 := NewEQPanelZone(EQCallbacks{})
	a2 := &eqDBOpenAdapter{z: z2, band: 1}
	if res := a2.OnPress(0, 0); res != InputConsumed {
		t.Fatalf("nil-callback OnPress = %v, want InputConsumed", res)
	}
}
