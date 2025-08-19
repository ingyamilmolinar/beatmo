package ui

import "testing"

func TestStepPixelsAlignment(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	scales := []float64{0.5, 0.75, 1.0, 1.25, 1.7}
	for _, s := range scales {
		step := g.StepPixels(s)
		sx := int((float64(g.Step) * s) + 0.5)
		if step != sx {
			t.Fatalf("step=%d want %d for scale %f", step, sx, s)
		}
	}
}
