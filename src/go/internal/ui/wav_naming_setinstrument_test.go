package ui

import "testing"

// Ensure SetInstrument safely handles an out-of-range selRow (e.g., after
// rows changed via import/upload flows) without panicking.
func TestSetInstrumentSelRowClamp(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	// Add a few rows to have a non-trivial length.
	for i := 0; i < 3; i++ {
		dv.AddRow()
	}
	// Force selRow beyond bounds.
	dv.selRow = len(dv.Rows)
	// Should not panic; should apply to last row instead.
	dv.SetInstrument("ASAP 808")
	if dv.selRow != len(dv.Rows)-1 {
		t.Fatalf("selRow not clamped: %d vs %d", dv.selRow, len(dv.Rows)-1)
	}
	if dv.Rows[dv.selRow].Instrument != "ASAP 808" {
		t.Fatalf("instrument not set on clamped row")
	}
}
