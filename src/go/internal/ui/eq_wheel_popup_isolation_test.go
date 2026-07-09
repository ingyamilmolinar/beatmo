//go:build test

package ui

import "testing"

// openEQWheel sets up the EQ tab with the wheel popup open and returns a grid
// point outside the popup (a "tap on the scrim over the grid"). Mirrors
// openMobileSynthWheel in synth_wheel_popup_isolation_test.go, using
// setupEQTabForTest (eq_wheel_popup_test.go) as the canonical EQ-tab bring-up
// sequence.
func openEQWheel(t *testing.T) (g *Game, gx, gy int) {
	t.Helper()
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	g.drum.openEQKnobWheelPopup(0)
	if g.drum.eqWheelPopup == nil || !g.drum.eqWheelPopup.IsOpen() {
		t.Fatal("setup did not open the EQ wheel popup")
	}
	gx, gy = 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Fatalf("test point (%d,%d) is not in the grid pane", gx, gy)
	}
	if pr := g.drum.eqWheelPopup.Rect(); pr.Min.Y <= gy {
		t.Fatalf("test point y=%d overlaps popup rect %v", gy, pr)
	}
	return g, gx, gy
}

// While the EQ wheel is open it is a blocking modal overlay.
func TestEQWheelPopup_IsModalBlocking(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, _, _ := openEQWheel(t)
	if !g.drum.PortalHasBlocking() {
		t.Fatal("EQ wheel popup must register a blocking (modal+scrim) portal")
	}
	if !g.modalOverlayActive() {
		t.Fatal("modalOverlayActive() must be true while the EQ wheel is open")
	}
}

// A tap in the grid pane while the wheel is open must NOT create a node.
func TestEQWheelPopup_BlocksGridTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openEQWheel(t)
	before := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) != before {
		t.Fatalf("grid tap leaked through scrim: nodes %d -> %d", before, len(g.nodes))
	}
}

// TestEQWheelPopup_AppliesToMasterChannel: a wheel step on the master channel
// (default eqActiveChannel == "main") must flow through OnGainChange ->
// OnApplyEQ -> dv.applyEQ() -> dv.applyMasterEQ(), landing in dv.eqApplied —
// the same observation seam TestEQBandMuteUnmutePreservesGain and
// TestEQPanelMuteVisualState use (eq_per_instrument_test.go).
func TestEQWheelPopup_AppliesToMasterChannel(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	if ch := dv.activeEQChannel(); ch != "main" {
		t.Fatalf("precondition: activeEQChannel = %q, want main", ch)
	}

	dv.openEQKnobWheelPopup(0)
	b := dv.eqWheelPopup.binding

	// Drive one wheel step: move the transient knob to a known dB value and
	// fire the same OnChange the wheel fires per notch (per Task 2's test,
	// HandleWheel's notcher needs many events to move — drive Value+OnChange
	// directly instead).
	const wantDB = 4.5
	b.Knob.Value = (wantDB - eqDBMin) / eqDBSpan
	b.OnChange()

	if got := dv.eqPanelZone.bandGainsDB[0]; got != wantDB {
		t.Fatalf("wheel OnChange did not write bandGainsDB[0]: got %v, want %v", got, wantDB)
	}
	if len(dv.eqApplied) == 0 {
		t.Fatal("wheel OnChange did not flow through to dv.eqApplied (OnApplyEQ -> applyEQ -> applyMasterEQ)")
	}
	if got := dv.eqApplied[0].GainDB; got != wantDB {
		t.Fatalf("dv.eqApplied[0].GainDB = %v, want %v — wheel edit did not reach the master EQ audio-apply path", got, wantDB)
	}
}

// TestEQWheelPopup_AppliesToRowChannel: the same wheel wiring, but with a row
// (per-instrument) channel selected, must flow through applyRowEQ instead of
// applyMasterEQ, landing in that row's EQGainsDB.
func TestEQWheelPopup_AppliesToRowChannel(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	setupEQTabForTest(g, 1280, 720)
	dv := g.drum

	if len(dv.Rows) == 0 {
		t.Fatal("precondition: expected at least one row")
	}
	rowInst := dv.Rows[0].Instrument
	dv.setEQActiveChannel(rowInst)
	if ch := dv.activeEQChannel(); ch != rowInst {
		t.Fatalf("precondition: activeEQChannel = %q, want %q", ch, rowInst)
	}
	if len(dv.Rows[0].EQGainsDB) != len(eqBandDefs) {
		dv.Rows[0].EQGainsDB = make([]float64, len(eqBandDefs))
	}
	if len(dv.eqBandGainsDB()) != len(eqBandDefs) {
		t.Fatalf("dv.eqBandGainsDB() len = %d, want %d — row-channel gains not backing the zone", len(dv.eqBandGainsDB()), len(eqBandDefs))
	}

	dv.openEQKnobWheelPopup(0)
	b := dv.eqWheelPopup.binding

	const wantDB = -7.0
	b.Knob.Value = (wantDB - eqDBMin) / eqDBSpan
	b.OnChange()

	if got := dv.Rows[0].EQGainsDB[0]; got != wantDB {
		t.Fatalf("wheel OnChange did not reach row EQGainsDB[0]: got %v, want %v", got, wantDB)
	}
}
