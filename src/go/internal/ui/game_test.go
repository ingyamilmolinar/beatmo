package ui

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestTryAddNodeTogglesRow(t *testing.T) {
	g := New()
	g.tryAddNode(2, 0)
	if len(g.graph.Row) <= 2 || !g.graph.Row[2] {
		t.Fatalf("expected step 2 on")
	}
}

func TestDeleteNodeClearsRow(t *testing.T) {
	g := New()
	n := g.tryAddNode(1, 0)
	g.deleteNode(n)
	if len(g.graph.Row) > 1 && g.graph.Row[1] {
		t.Fatalf("expected step 1 off after delete")
	}
}

func TestAddEdgeNoDuplicates(t *testing.T) {
	g := New()
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	g.addEdge(a, b)
	g.addEdge(a, b)
	if len(g.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.edges))
	}
}

func TestSpawnPulseFrom(t *testing.T) {
	g := New()
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	g.addEdge(a, b)
	g.spawnPulseFrom(a, 1)
	if len(g.pulses) != 1 {
		t.Fatalf("expected 1 pulse, got %d", len(g.pulses))
	}
}

func TestOnBeatUsesRoot(t *testing.T) {
	g := New()
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(2, 0)
	g.addEdge(a, b)
	g.onBeat(0)
	if len(g.pulses) != 1 {
		t.Fatalf("expected pulse from root on beat, got %d", len(g.pulses))
	}
}

func TestUpdateRunsSchedulerWhenPlaying(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	// ensure first step active
	g.graph.ToggleStep(0)

	var fired int
	g.sched.OnBeat = func(int) { fired++ }
	g.drum.playing = true
	g.drum.bpm = 60

	g.Update()
	if fired == 0 {
		t.Fatalf("scheduler did not run")
	}
}

func TestClickAddsNode(t *testing.T) {
	g := New()
	g.Layout(640, 480)

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return 10, topOffset + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	if g.nodeAt(0, 0) != nil {
		t.Fatalf("node created before release")
	}
	pressed = false
	g.Update() // release
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatalf("expected node created at (0,0) after release")
	}
	if !n.Selected || g.sel != n {
		t.Fatalf("new node should be selected")
	}
	pressed = true
	// click another position
	restore2 := SetInputForTest(
		func() (int, int) { return StepPixels(g.cam.Scale) + 10, topOffset + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore2()
	g.Update() // press second node
	if g.nodeAt(1, 0) != nil {
		t.Fatalf("node created before release at second position")
	}
	pressed = false
	g.Update() // release second node
	n2 := g.nodeAt(1, 0)
	if n2 == nil || !n2.Selected || g.sel != n2 || n.Selected {
		t.Fatalf("selection did not move to new node")
	}
}

func TestRowLengthMatchesConnectedNodes(t *testing.T) {
	g := New()
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	c := g.tryAddNode(2, 0)
	g.addEdge(a, b)
	g.addEdge(b, c)

	if len(g.graph.Row) < 3 {
		t.Fatalf("row len=%d want >=3", len(g.graph.Row))
	}
	on := 0
	for _, v := range g.graph.Row {
		if v {
			on++
		}
	}
	if on != 3 {
		t.Fatalf("active steps=%d want 3", on)
	}
	if len(g.nodes) != 3 || len(g.edges) != 2 {
		t.Fatalf("nodes=%d edges=%d", len(g.nodes), len(g.edges))
	}
}

func TestPlayStopPulseSequence(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	g.graph.ToggleStep(0)
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	g.addEdge(a, b)

	var beats int
	g.sched.OnBeat = func(i int) { beats++; g.onBeat(i) }

	g.drum.playing = true
	g.drum.bpm = 60
	g.Update()
	if beats == 0 || len(g.pulses) == 0 {
		t.Fatalf("expected pulse after play")
	}
	prev := len(g.pulses)

	g.drum.playing = false
	g.Update()
	if len(g.pulses) != prev {
		t.Fatalf("pulses changed after stop")
	}
}

func TestPulseAnimationProgress(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	g.graph.ToggleStep(0)
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	g.addEdge(a, b)

	now := time.Unix(0, 0)

	g.sched.SetNowFunc(func() time.Time { now = now.Add(time.Second); return now })
	g.drum.playing = true
	g.drum.bpm = 60

	g.Update() // spawn pulse
	if len(g.pulses) == 0 {
		t.Fatalf("expected pulse after play")
	}
	first := g.pulses[0].t
	g.Update() // advance animation
	if g.pulses[0].t <= first {
		t.Fatalf("pulse did not advance: %f <= %f", g.pulses[0].t, first)
	}
}

func TestNodeScreenAlignment(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	g.cam.OffsetX = 12.3
	g.cam.OffsetY = 7.8

	n := g.tryAddNode(3, 2)
	step := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	sx := offX + float64(step*n.I)
	sy := offY + float64(step*n.J)

	expX := sx
	expY := sy

	if sx != expX || sy != expY {
		t.Fatalf("screen (%v,%v) want (%v,%v)", sx, sy, expX, expY)
	}
}

func TestDragMaintainsAlignment(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	n := g.tryAddNode(2, 1)

	pos := []struct{ x, y int }{{100, topOffset + 100}, {120, topOffset + 110}}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	step := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	nodeX := offX + float64(step*n.I)
	nodeY := offY + float64(step*n.J)

	xs, ys := GridLines(g.cam, g.winW, g.split.Y)
	foundX, foundY := false, false
	for _, x := range xs {
		if math.Abs(x-nodeX) < 1e-3 {
			foundX = true
			break
		}
	}
	for _, y := range ys {
		if math.Abs(y-nodeY) < 1e-3 {
			foundY = true
			break
		}
	}
	if !foundX || !foundY {
		t.Fatalf("node not aligned with grid after drag")
	}
}
func TestInitialDrumRows(t *testing.T) {
	g := New()
	if len(g.drum.Rows) != 2 {
		t.Fatalf("rows=%d want 2", len(g.drum.Rows))
	}
	g.Layout(640, 480)
	g.Update()
	if len(g.drum.bgCache) != 2 {
		t.Fatalf("bgCache=%d want 2", len(g.drum.bgCache))
	}
}

func TestHighlightScalesWithZoom(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	n := g.tryAddNode(1, 1)
	g.sel = n

	g.cam.Scale = 1.0
	a1, _, a2, _ := g.nodeScreenRect(n)
	w1 := a2 - a1

	g.cam.Scale = 2.0
	b1, _, b2, _ := g.nodeScreenRect(n)
	w2 := b2 - b1

	if math.Abs(w2-2*w1) > 1e-3 {
		t.Fatalf("highlight width did not scale: w1=%f w2=%f", w1, w2)
	}
}

func TestDragPanDoesNotCreateNode(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	pos := []struct{ x, y int }{
		{10, topOffset + 10},
		{30, topOffset + 20},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	if g.nodeAt(0, 0) != nil {
		t.Fatalf("node created after drag")
	}
}

func TestBottomPaneClickIgnored(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	g.cam.OffsetY = 100

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return 10, g.split.Y + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press in bottom pane
	pressed = false
	g.Update() // release

	if len(g.nodes) != 0 {
		t.Fatalf("node created from bottom pane click")
	}
}

func TestHighlightMatchesNode(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	n := g.tryAddNode(2, 3)
	g.sel = n
	g.cam.Scale = 1.5
	g.cam.OffsetX = 12
	g.cam.OffsetY = 8

	step := StepPixels(g.cam.Scale)
	camScale := float64(step) / float64(GridStep)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	worldX := float64(n.I * GridStep)
	worldY := float64(n.J * GridStep)
	screenX := worldX*camScale + offX
	screenY := worldY*camScale + offY + float64(topOffset)
	half := float64(NodeSpriteSize) * camScale / 2

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	if math.Abs(x1-(screenX-half)) > 1e-3 || math.Abs(x2-(screenX+half)) > 1e-3 ||
		math.Abs(y1-(screenY-half)) > 1e-3 || math.Abs(y2-(screenY+half)) > 1e-3 {
		t.Fatalf("highlight mismatch: (%f,%f,%f,%f) want (%f,%f,%f,%f)",
			x1, y1, x2, y2,
			screenX-half, screenY-half, screenX+half, screenY+half)
	}
}

func TestSplitterDragPersists(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	startY := g.split.Y
	pos := []struct{ x, y int }{
		{10, startY},
		{10, startY + 50},
		{10, startY + 50},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update()         // release
	g.Layout(640, 480) // layout called again as in game loop
	g.Update()
	if g.split.Y != startY+50 {
		t.Fatalf("splitter Y=%d want %d", g.split.Y, startY+50)
	}
}
func TestSplitterDragDoesNotCreateNode(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	startY := g.split.Y
	pos := []struct{ x, y int }{
		{10, startY},      // press on divider
		{10, startY + 40}, // drag
		{10, startY + 40}, // release
		{10, startY + 40}, // idle
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update() // release while over divider
	idx = 3
	g.Update() // after release
	g.Layout(640, 480)
	g.Update()

	if len(g.nodes) != 0 {
		t.Fatalf("unexpected node created during splitter drag")
	}
}

func TestStartNodeSelection(t *testing.T) {
	g := New()
	g.Layout(640, 480)
	n1 := g.tryAddNode(0, 0)
	if g.start != n1 || !n1.Start {
		t.Fatalf("first node should be start")
	}
	n2 := g.tryAddNode(1, 0)
	g.sel = n2
	restore := SetInputForTest(
		func() (int, int) { return 0, topOffset + 10 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyS },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()
	g.Update()
	if g.start != n2 || !n2.Start || n1.Start {
		t.Fatalf("start node not updated")
	}
}

func TestPatternLengthButtonsModifyGraph(t *testing.T) {
	g := New()
	g.Layout(640, 480)

	g.drum.resizeSteps(+1)

	initial := len(g.graph.Row)
	g.Update()
	afterPlus := len(g.graph.Row)
	t.Logf("after plus: %d", afterPlus)
	if afterPlus <= initial {
		t.Fatalf("expected graph length to increase")
	}
	g.drum.resizeSteps(-1)
	g.Update()
	afterMinus := len(g.graph.Row)
	t.Logf("after minus: %d", afterMinus)
	if afterMinus >= afterPlus {
		t.Fatalf("expected graph length to decrease after minus")
	}
}
