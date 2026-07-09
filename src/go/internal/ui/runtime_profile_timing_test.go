package ui

import "testing"

func TestRuntimeProfileTimingBounds(t *testing.T) {
	d := desktopRuntimeProfile()
	if !d.Timing.Enabled {
		t.Error("desktop timing gate should be enabled")
	}
	if !d.Timing.FatalDefault {
		t.Error("desktop timing fatal default should be true")
	}
	if d.Timing.EmitDrift.AbsMax <= 0 {
		t.Error("desktop emit-drift AbsMax must be set")
	}
	b := browserRuntimeProfile()
	if b.Timing.FatalDefault {
		t.Error("browser timing fatal default must be false (log-only)")
	}
	// Browser bounds must be >= desktop (looser).
	if b.Timing.EmitDrift.AbsMax < d.Timing.EmitDrift.AbsMax {
		t.Error("browser emit-drift AbsMax should be looser than desktop")
	}
}
