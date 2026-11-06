package ui

import "testing"

func TestSynthArmedTokensPresent(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		v := Profile().DensityValues()
		if v.SynthArmedCardH <= 0 || v.SynthWheelLaneW <= 0 || v.SynthWheelRowH <= 0 {
			t.Fatalf("density %v: armed tokens must be positive, got card=%d lane=%d row=%d", d, v.SynthArmedCardH, v.SynthWheelLaneW, v.SynthWheelRowH)
		}
		if v.SynthWheelVisibleRows%2 == 0 {
			t.Fatalf("density %v: visible rows must be odd (center + N each side), got %d", d, v.SynthWheelVisibleRows)
		}
		restore()
	}
}
