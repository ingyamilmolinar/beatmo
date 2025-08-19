package ui

import (
	"reflect"
	"testing"
)

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

func TestSubdivisionVisibility(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	cam := &Camera{Scale: 1}
	groups := g.Lines(cam, 640, 480)
	var divs []int
	for _, gr := range groups {
		divs = append(divs, gr.Subdiv.Div)
	}
	if !reflect.DeepEqual(divs, []int{1, 4}) {
		t.Fatalf("divs=%v want [1 4]", divs)
	}

	cam.Scale = 2
	groups = g.Lines(cam, 640, 480)
	divs = divs[:0]
	for _, gr := range groups {
		divs = append(divs, gr.Subdiv.Div)
	}
	if !reflect.DeepEqual(divs, []int{1, 4, 2, 8}) {
		t.Fatalf("divs=%v want [1 4 2 8]", divs)
	}
}
