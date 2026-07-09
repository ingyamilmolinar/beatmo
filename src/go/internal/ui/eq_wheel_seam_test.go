//go:build test

package ui

import "testing"

// applyBandGainLive writes the source of truth, clamps to [-12,12], and fires
// the live callbacks (OnGainChange + OnApplyEQ) but NOT the commit callback.
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

// eqDBOpenAdapter.OnPress routes to OnOpenValueWheel when set, and falls back
// to the numeric editor when it is nil (preserving legacy behavior).
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
	if z.paramEditor != nil && z.paramEditor.Active() {
		t.Fatal("numeric editor must NOT open when wheel callback is set")
	}

	// Nil callback → falls back to numeric editor.
	z2 := NewEQPanelZone(EQCallbacks{})
	a2 := &eqDBOpenAdapter{z: z2, band: 1}
	a2.OnPress(0, 0)
	if z2.paramEditor == nil || !z2.paramEditor.Active() {
		t.Fatal("fallback: numeric editor must open when wheel callback is nil")
	}
}
