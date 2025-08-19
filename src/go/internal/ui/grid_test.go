package ui

import (
	"math"
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
	if !reflect.DeepEqual(divs, []int{1, 2, 4, 8}) {
		t.Fatalf("divs=%v want [1 2 4 8]", divs)
	}
}

func TestLinesNoOverlap(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	cam := &Camera{Scale: 2}
	groups := g.Lines(cam, 200, 200)
	seenX := map[int]struct{}{}
	seenY := map[int]struct{}{}
	for _, gr := range groups {
		for _, x := range gr.Xs {
			k := int(math.Round(x * 1000))
			if _, ok := seenX[k]; ok {
				t.Fatalf("duplicate x %v for div %d", x, gr.Subdiv.Div)
			}
			seenX[k] = struct{}{}
		}
		for _, y := range gr.Ys {
			k := int(math.Round(y * 1000))
			if _, ok := seenY[k]; ok {
				t.Fatalf("duplicate y %v for div %d", y, gr.Subdiv.Div)
			}
			seenY[k] = struct{}{}
		}
	}
}
