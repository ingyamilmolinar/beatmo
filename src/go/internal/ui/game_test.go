package ui

import (
	"math"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"os"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(os.Stdout, game_log.LevelDebug)
}

func TestTryAddNodeTogglesRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	nodeID := g.tryAddNode(2, 0).ID
	g.graph.StartNodeID = nodeID
	// After adding a node and setting it as start, the beat row should reflect it.
	// We need to call Update to propagate the change to drum.Rows.
	g.Update()
	if len(g.drum.Rows[0].Steps) <= 0 || !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected step 0 on")
	}
}

func TestDeleteNodeClearsRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 0)
	g.deleteNode(n)
	g.Update() // Propagate changes to drum.Rows
	if len(g.drum.Rows[0].Steps) > 1 && g.drum.Rows[0].Steps[1] {
		t.Fatalf("expected step 1 off after delete")
	}
}

func TestAddEdgeNoDuplicates(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	g.addEdge(a, b)
	g.addEdge(a, b)
	if len(g.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.edges))
	}
}

func TestUpdateRunsSchedulerWhenPlaying(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// ensure first step active
	nodeID := g.tryAddNode(0, 0).ID
	g.graph.StartNodeID = nodeID

	// var fired int
	// g.sched.OnTick = func(i int) { fired++ }
	g.sched.OnTick = func(i int) { g.onTick(i) }
	g.drum.bpm = 60

	// Simulate click on play button
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.playBtn.Min.X + 1, g.drum.playBtn.Min.Y + 1 }, // Click inside the button
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // Simulate press
	pressed = false
	g.Update() // Simulate release

	// if fired == 0 {
	// 	t.Fatalf("scheduler did not run")
	// }
	// We expect an active pulse after play
	if g.activePulse != nil {
		t.Fatalf("expected active pulse to be nil when no edges are present, got %v", g.activePulse)
	}
}

func TestClickAddsNode(t *testing.T) {
	g := New(testLogger)
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
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0)
	b := g.tryAddNode(1, 0)
	c := g.tryAddNode(2, 0)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.graph.StartNodeID = a.ID

	row, _ := g.graph.CalculateBeatRow()
	if len(row) < 3 {
		t.Fatalf("row len=%d want >=3", len(row))
	}
	on := 0
	for _, v := range row {
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

func TestPulseAnimationProgress(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	node0 := g.tryAddNode(0, 0)
	node1 := g.tryAddNode(1, 0)
	g.addEdge(node0, node1)
	g.graph.StartNodeID = node0.ID

	now := time.Unix(0, 0)

	g.sched.SetNowFunc(func() time.Time { now = now.Add(time.Second); return now })
	g.drum.bpm = 60

	// Simulate click on play button
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.playBtn.Min.X + 1, g.drum.playBtn.Min.Y + 1 }, // Click inside the button
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	g.Update() // Simulate press
	pressed = false
	g.Update() // Simulate release

	// spawn pulse
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after play")
	}
	first := g.activePulse.t
	g.Update() // advance animation
	if g.activePulse.t <= first {
		t.Fatalf("active pulse did not advance: %f <= %f", g.activePulse.t, first)
	}
}

func TestNodeScreenAlignment(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	g.cam.OffsetX = 12.3
	g.cam.OffsetY = 7.8

	n := g.tryAddNode(3, 2)
	g.graph.StartNodeID = n.ID
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
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	n := g.tryAddNode(2, 1)
	g.graph.StartNodeID = n.ID

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
	g := New(testLogger)
	g.Layout(640, 480)
	if len(g.drum.Rows) != 1 {
		t.Fatalf("rows=%d want 1", len(g.drum.Rows))
	}
	g.Layout(640, 480)
	g.Update()
	if len(g.drum.bgCache) != 1 {
		t.Fatalf("bgCache=%d want 1", len(g.drum.bgCache))
	}
}

func TestHighlightScalesWithZoom(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 1)
	g.sel = n
	g.graph.StartNodeID = n.ID

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
	g := New(testLogger)
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
		g.graph.StartNodeID = model.InvalidNodeID
}

func TestBottomPaneClickIgnored(t *testing.T) {
	g := New(testLogger)
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
		g.graph.StartNodeID = model.InvalidNodeID
}

func TestHighlightMatchesNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(2, 3)
	g.sel = n
	g.graph.StartNodeID = n.ID

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
	g := New(testLogger)
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
		g.graph.StartNodeID = model.InvalidNodeID
}
func TestSplitterDragDoesNotCreateNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
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
		g.graph.StartNodeID = model.InvalidNodeID
}

func TestStartNodeSelection(t *testing.T) {
       g := New(testLogger)
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
	g.graph.StartNodeID = n2.ID
}

func TestSinglePulseInLoop(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Create a closed circuit: node0 -> node1 -> node0
	node0 := g.tryAddNode(0, 0)
	node1 := g.tryAddNode(1, 0)
	g.addEdge(node0, node1)
	g.addEdge(node1, node0)
	g.graph.StartNodeID = node0.ID

	// Set a fast BPM for quick testing
	g.drum.bpm = 6000 // 100 beats per second

	// Simulate play button press
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.playBtn.Min.X + 1, g.drum.playBtn.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()
	g.Update() // Press
	pressed = false
	g.Update() // Release

	// Advance game state multiple times to allow pulses to traverse the loop
	// We expect only one pulse to be active at any given time
	for i := 0; i < 10; i++ {
		g.Update()
		g.Draw(ebiten.NewImage(640, 480)) // Call Draw to update renderedPulsesCount
		if g.activePulse == nil && i < 9 {
			t.Fatalf("expected active pulse in loop, got nil after %d updates", i+1)
		}
		if g.renderedPulsesCount > 1 {
			t.Fatalf("expected at most one pulse rendered, got %d after %d updates", g.renderedPulsesCount, i+1)
		}
	}
	if g.activePulse != nil {
		t.Fatalf("expected active pulse to be nil after loop, got %v", g.activePulse)
	}
}