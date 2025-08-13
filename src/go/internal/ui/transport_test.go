package ui

import "testing"

func TestTransportSetBPMClamp(t *testing.T) {
	tr := NewTransport(200)
	tr.SetBPM(maxBPM + 500)
	if tr.BPM != maxBPM {
		t.Fatalf("BPM not clamped: %d", tr.BPM)
	}
	if tr.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on high bpm")
	}
	tr.bpmErrorAnim = 0
	tr.SetBPM(0)
	if tr.BPM != 1 {
		t.Fatalf("low BPM not clamped: %d", tr.BPM)
	}
	if tr.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on low bpm")
	}
}
