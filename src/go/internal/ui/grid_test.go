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

func TestSnapFinestSubdivision(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	unit := g.Unit()
	gx, gy, i, j := g.Snap(0.6*unit, 1.2*unit)
	if i != 1 || j != 1 {
		t.Fatalf("snap coarse i=%d j=%d", i, j)
	}
	if math.Abs(gx-unit) > 1e-9 || math.Abs(gy-unit) > 1e-9 {
		t.Fatalf("coords (%f,%f) want (%f,%f)", gx, gy, unit, unit)
	}
	gx, _, i, _ = g.Snap(1.6*unit, 0)
	if i != 2 || math.Abs(gx-2*unit) > 1e-9 {
		t.Fatalf("snap fine i=%d gx=%f", i, gx)
	}
}

func TestRadiusScaling(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	r1 := g.NodeRadius(1)
	r2 := g.NodeRadius(2)
	if r1*2 >= g.Unit() {
		t.Fatalf("node radius overlaps unit: r=%f unit=%f", r1, g.Unit())
	}
	if r2*2 >= g.Unit() {
		t.Fatalf("node radius overlaps unit at scale2: r=%f unit=%f", r2, g.Unit())
	}
	if r1*1 >= r2*2 { // screen radius r*scale
		t.Fatalf("screen radius did not grow: r1=%f r2=%f", r1, r2)
	}
	sr1 := g.SignalRadius(1)
	sr2 := g.SignalRadius(2)
	if sr1*1 >= sr2*2 {
		t.Fatalf("signal screen radius did not grow: r1=%f r2=%f", sr1, sr2)
	}
	if g.NodeRadius(100)*100 > 16 || g.SignalRadius(100)*100 > 6 {
		t.Fatalf("radius clamp failed at extreme zoom")
	}
}

func TestLinesExtendBeyondView(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	// Pan the camera by a non-integer multiple of the step to simulate
	// arbitrary movement across the lattice.
	cam := &Camera{Scale: 1, OffsetX: 30, OffsetY: -45}
	minX, maxX, minY, maxY := visibleWorldRect(cam, 100, 100)
	groups := g.Lines(cam, 100, 100)
	// Subdivision Div=1 corresponds to beat lines; ensure they extend beyond
	// the computed visible rectangle on all sides so panning never reveals
	// empty space.
	for _, gr := range groups {
		if gr.Subdiv.Div != 1 {
			continue
		}
		left := math.MaxFloat64
		right := -math.MaxFloat64
		top := math.MaxFloat64
		bottom := -math.MaxFloat64
		for _, x := range gr.Xs {
			if x < left {
				left = x
			}
			if x > right {
				right = x
			}
		}
		for _, y := range gr.Ys {
			if y < top {
				top = y
			}
			if y > bottom {
				bottom = y
			}
		}
		if left > minX || right < maxX || top > minY || bottom < maxY {
			t.Fatalf("grid lines do not extend beyond view: left %f right %f top %f bottom %f", left, right, top, bottom)
		}
		return
	}
	t.Fatalf("no beat-level lines found")
}

func TestLineWidthConstantAcrossZoom(t *testing.T) {
	g := NewGrid(DefaultGridStep)
	scales := []float64{0.5, 1, 2, 4}
	for _, s := range scales {
		unitPx := g.UnitPixels(s)
		camScale := unitPx / g.Unit()
		world := g.Subs[0].Style.Width / camScale
		screen := world * camScale
		if math.Abs(screen-g.Subs[0].Style.Width) > 1e-9 {
			t.Fatalf("screen width=%f want=%f at scale %f", screen, g.Subs[0].Style.Width, s)
		}
	}
}
