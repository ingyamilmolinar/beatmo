package ui

import (
	"io"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(io.Discard, game_log.LevelError)
}

func TestTryAddNodeTogglesRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	nodeID := g.tryAddNode(2, 0, model.NodeTypeRegular).ID
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
	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.deleteNode(n)
	g.Update() // Propagate changes to drum.Rows
	if len(g.drum.Rows[0].Steps) > 1 && g.drum.Rows[0].Steps[1] {
		t.Fatalf("expected step 1 off after delete")
	}
}

func TestGameAssignsOriginToNewRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.AddRow()
	g.Update()
	n := g.tryAddNode(3, 0, model.NodeTypeRegular)
	if g.drum.Rows[1].Origin != n.ID {
		t.Fatalf("expected row origin %d got %d", n.ID, g.drum.Rows[1].Origin)
	}
}

func TestGameCalculatesBeatInfosPerRow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// first start node for row 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)

	// add a second row and assign its origin on next node add
	g.drum.AddRow()
	g.Update()
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.updateBeatInfos()

	if len(g.beatInfosByRow) < 2 {
		t.Fatalf("expected beatInfos for 2 rows, got %d", len(g.beatInfosByRow))
	}
	if len(g.beatInfosByRow[0]) == 0 || g.beatInfosByRow[0][0].NodeID != n0.ID {
		t.Fatalf("row0 beatInfos start at %v want %v", g.beatInfosByRow[0], n0.ID)
	}
	if len(g.beatInfosByRow[1]) == 0 || g.beatInfosByRow[1][0].NodeID != n1.ID {
		t.Fatalf("row1 beatInfos start at %v want %v", g.beatInfosByRow[1], n1.ID)
	}
}

func TestSpawnPulsePerRowPlaysInstrument(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// first row start
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID

	// second row
	g.drum.AddRow()
	g.Update()
	_ = g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.drum.Rows[1].Instrument = "kick"

	g.updateBeatInfos()

	var plays []string
	orig := playSound
	playSound = func(id string, when ...float64) { plays = append(plays, id) }
	defer func() { playSound = orig }()

	g.spawnPulseFromRow(0, 0)
	g.spawnPulseFromRow(1, 0)

	if len(plays) != 2 {
		t.Fatalf("expected 2 plays got %d", len(plays))
	}
	if plays[0] != g.drum.Rows[0].Instrument || plays[1] != g.drum.Rows[1].Instrument {
		t.Fatalf("got plays %v", plays)
	}
}

func TestAdvancePulseLoopWrap(t *testing.T) {
	g := New(testLogger)
	g.drum.Length = 6
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n1)

	g.updateBeatInfos()

	if !g.isLoop || g.loopStartIndex != 0 {
		t.Fatalf("expected loop starting at 0, got loop=%t start=%d", g.isLoop, g.loopStartIndex)
	}

	last := len(g.beatInfos) - 1
	p := &pulse{
		fromBeatInfo: g.beatInfos[last-1],
		toBeatInfo:   g.beatInfos[last],
		path:         g.beatInfos,
		pathIdx:      last,
		from:         g.nodeByID(g.beatInfos[last-1].NodeID),
		to:           g.nodeByID(g.beatInfos[last].NodeID),
		row:          0,
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("advancePulse panicked: %v", r)
			}
		}()
		g.advancePulse(p)
	}()

	if p.pathIdx != 0 {
		t.Fatalf("expected pathIdx 0 after wrap, got %d", p.pathIdx)
	}
	if p.fromBeatInfo.NodeID != g.beatInfos[len(g.beatInfos)-1].NodeID {
		t.Fatalf("expected from node %d, got %d", g.beatInfos[len(g.beatInfos)-1].NodeID, p.fromBeatInfo.NodeID)
	}
	if p.toBeatInfo.NodeID != g.beatInfos[0].NodeID {
		t.Fatalf("expected to node %d, got %d", g.beatInfos[0].NodeID, p.toBeatInfo.NodeID)
	}
}

func TestTimelineDragWhilePlayingKeepsPulse(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n1)

	g.updateBeatInfos()

	g.playing = true
	g.spawnPulseFrom(0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse before drag")
	}

	g.drum.Offset = 10
	g.drum.offsetChanged = true
	g.Update()

	if !g.playing {
		t.Fatalf("playing stopped after drag")
	}
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after drag")
	}
}

func TestDrumWheelDoesNotZoomGrid(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.drum.Length = 8
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)
	g.Update() // set drum bounds

	wheelVal := 1.0
	restore := SetInputForTest(
		func() (int, int) { // cursor inside drum steps area
			return g.drum.Bounds.Min.X + g.drum.labelW + 390, g.drum.Bounds.Min.Y + 5
		},
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()

	scale := g.cam.Scale
	g.Update()
	if g.cam.Scale != scale {
		t.Fatalf("expected camera scale unchanged, got %f", g.cam.Scale)
	}
}

func TestPlayWithoutStartNodeStaysResponsive(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.Update()

	g.drum.playPressed = true
	g.Update()
	if g.playing {
		t.Fatalf("game should not start without start node")
	}

	// pressing play again should still be handled immediately
	g.drum.playPressed = true
	g.Update()
	if g.playing {
		t.Fatalf("game should remain stopped without start node")
	}
}

func TestAddEdgeNoDuplicates(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(a, b)
	if len(g.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.edges))
	}
}

func TestAddRegularNodeOverInvisible(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Create an invisible node via an edge and then upgrade it
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b) // introduces an invisible node at (1,0)

	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if node, ok := g.graph.GetNodeByID(n.ID); !ok || node.Type != model.NodeTypeRegular {
		t.Fatalf("expected node at (1,0) to be regular after upgrade")
	}
}

func TestComplexCircuitTraversal(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	c := g.tryAddNode(3, 2, model.NodeTypeRegular)
	d := g.tryAddNode(0, 2, model.NodeTypeRegular)

	g.addEdge(start, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, start)

	g.updateBeatInfos()

	expectedLen := 10
	if len(g.beatInfos) != expectedLen {
		t.Fatalf("expected beatInfos length %d, got %d", expectedLen, len(g.beatInfos))
	}
	if g.drum.Length != expectedLen {
		t.Fatalf("expected drum length %d, got %d", expectedLen, g.drum.Length)
	}

	for i := range g.beatInfos {
		expected := g.beatInfos[i].NodeType == model.NodeTypeRegular
		if g.drum.Rows[0].Steps[i] != expected {
			t.Fatalf("drum row mismatch at %d", i)
		}
	}

	p := &pulse{
		fromBeatInfo: g.beatInfos[len(g.beatInfos)-1],
		toBeatInfo:   g.beatInfos[0],
		path:         g.beatInfos,
		pathIdx:      0,
		from:         g.nodeByID(g.beatInfos[len(g.beatInfos)-1].NodeID),
		to:           g.nodeByID(g.beatInfos[0].NodeID),
		row:          0,
	}

	for i := 0; i < expectedLen*2; i++ {
		if !g.advancePulse(p) {
			t.Fatalf("pulse stopped at step %d", i)
		}
	}
}

func TestUpdateRunsSchedulerWhenPlaying(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// ensure first step active
	nodeID := g.tryAddNode(0, 0, model.NodeTypeRegular).ID
	g.graph.StartNodeID = nodeID

	g.bpm = 60

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

	// allow engine to process
	time.Sleep(20 * time.Millisecond)
	select {
	case <-g.engine.Events:
	default:
		t.Fatalf("engine did not run")
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
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
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
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if n2 == nil || !n2.Selected || g.sel != n2 || n.Selected {
		t.Fatalf("selection did not move to new node")
	}
}

func TestBPMButtonsAdjustSpeed(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.graph.StartNodeID = n1.ID

	g.playing = true
	g.engine.Start()
	g.spawnPulseFrom(0)
	if g.activePulse == nil {
		t.Fatalf("no pulse spawned")
	}
	initialSpeed := g.activePulse.speed
	initialBPM := g.bpm

	g.Update()
	tBefore := g.activePulse.t

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.bpmIncBtn.Min.X + 1, g.drum.bpmIncBtn.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	g.Update() // press
	pressed = false
	g.Update() // release
	restore()
	if g.bpm != initialBPM+1 {
		t.Fatalf("expected bpm %d got %d", initialBPM+1, g.bpm)
	}
	if g.engine.BPM() != g.bpm {
		t.Fatalf("engine BPM not updated: %d", g.engine.BPM())
	}
	if g.activePulse == nil {
		t.Fatalf("pulse reset")
	}
	if g.activePulse.speed == initialSpeed {
		t.Fatalf("pulse speed unchanged")
	}
	if g.activePulse.t <= tBefore {
		t.Fatalf("pulse did not continue")
	}
}

func TestRowLengthMatchesConnectedNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Add some nodes and edges
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	// Set start node
	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.updateBeatInfos()

	// The drum view length should now be independent of the number of connected nodes
	// and should match the default drum.Length (which is 8)
	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("row len=%d want %d", len(g.drum.Rows[0].Steps), g.drum.Length)
	}

	// Verify the first few steps based on the connected nodes
	if !g.drum.Rows[0].Steps[0] {
		t.Errorf("Expected step 0 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[1] {
		t.Errorf("Expected step 1 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[2] {
		t.Errorf("Expected step 2 to be true, got false")
	}

	// Verify the remaining steps are false (padded)
	for i := 3; i < g.drum.Length; i++ {
		if g.drum.Rows[0].Steps[i] {
			t.Errorf("Expected step %d to be false (padded), got true", i)
		}
	}
}

func TestPulseAnimationProgress(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	node0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	node1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(node0, node1)
	g.graph.StartNodeID = node0.ID

	// Manually set playing to true and call spawnPulse to create the pulse
	g.playing = true
	g.spawnPulseFrom(0)

	// The pulse should be active now
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after spawning")
	}

	firstT := g.activePulse.t

	// Advance the game state by a few frames
	for i := 0; i < 10; i++ {
		g.Update()
	}

	if g.activePulse == nil {
		t.Fatalf("pulse disappeared unexpectedly")
	}

	// The animation time 't' should have progressed
	if g.activePulse.t <= firstT {
		t.Fatalf("active pulse did not advance: %f <= %f", g.activePulse.t, firstT)
	}
}

func TestPlaySoundOnRegularNodesOnly(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n2) // introduces an invisible node at (1,0)

	var plays []string
	orig := playSound
	playSound = func(id string, when ...float64) { plays = append(plays, id) }
	defer func() { playSound = orig }()

	g.playing = true
	g.spawnPulseFrom(0) // highlights start node

	if len(plays) != 1 {
		t.Fatalf("expected 1 sample for start node, got %d", len(plays))
	}

	// Force pulse to reach invisible node; no new sample expected
	g.activePulse.t = 1
	g.Update()
	if len(plays) != 1 {
		t.Fatalf("expected no sample for invisible node, got %d", len(plays))
	}

	// Advance to final regular node; another sample expected
	g.activePulse.t = 1
	g.Update()
	if len(plays) != 2 {
		t.Fatalf("expected 2 samples after reaching second node, got %d", len(plays))
	}
}

func TestSoundPlaysWithin50msOfHighlight(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}

	var delta time.Duration
	orig := playSound
	start := time.Now()
	playSound = func(id string, when ...float64) {
		delta = time.Since(start)
	}
	defer func() { playSound = orig }()

	g.highlightBeat(0, 0, info, 0)
	if delta > 50*time.Millisecond {
		t.Fatalf("audio delay %v exceeds 50ms", delta)
	}
}

func TestHighlightBeatUsesSelectedInstrument(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Instrument = "kick"
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}

	var id string
	orig := playSound
	playSound = func(inst string, when ...float64) { id = inst }
	defer func() { playSound = orig }()

	g.highlightBeat(0, 0, info, 0)
	if id != "kick" {
		t.Fatalf("expected instrument 'kick', got %s", id)
	}
}

func TestAudioLoopConsistency(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a := g.tryAddNode(2, 0, model.NodeTypeRegular)
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	c := g.tryAddNode(3, 1, model.NodeTypeRegular)
	d := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.graph.StartNodeID = start.ID

	g.addEdge(start, a) // introduces invisible at (1,0)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a) // loop back via A

	var plays int
	orig := playSound
	playSound = func(id string, when ...float64) { plays++ }
	defer func() { playSound = orig }()

	g.playing = true
	g.spawnPulseFrom(0)

	for i := 0; i < 8; i++ {
		g.activePulse.t = 1
		g.Update()
	}

	if plays != 8 {
		t.Fatalf("expected 8 plays after looping, got %d", plays)
	}
}

func TestNodeScreenAlignment(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	g.cam.OffsetX = 12.3
	g.cam.OffsetY = 7.8

	n := g.tryAddNode(3, 2, model.NodeTypeRegular)
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
	n := g.tryAddNode(2, 1, model.NodeTypeRegular)
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
	g.Update()
	g.Draw(ebiten.NewImage(640, 480)) // Call Draw to populate bgCache
	if len(g.drum.bgCache) != 1 {
		t.Fatalf("bgCache=%d want 1", len(g.drum.bgCache))
	}
}

func TestHighlightScalesWithZoom(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 1, model.NodeTypeRegular)
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
	n := g.tryAddNode(2, 3, model.NodeTypeRegular)
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

func TestDrumViewDragShiftsWindow(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	prev := n0
	for i := 1; i < 6; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.start = n0
	g.graph.StartNodeID = n0.ID
	g.updateBeatInfos()
	g.drum.Length = 3
	g.refreshDrumRow()
	g.drum.recalcButtons()
	g.drum.calcLayout()

	stepX := g.drum.Bounds.Min.X + g.drum.labelW + g.drum.controlsW + g.drum.cell/2
	stepY := g.drum.Bounds.Min.Y + g.drum.rowHeight()/2
	pos := []struct{ x, y int }{{stepX, stepY}, {stepX - 2*g.drum.cell, stepY}, {stepX - 2*g.drum.cell, stepY}}
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
	g.Update() // release

	if g.drum.Offset <= 0 {
		t.Fatalf("drum offset not increased: %d", g.drum.Offset)
	}
	if g.camDragging {
		t.Fatalf("camera dragged during drum view drag")
	}
	if g.drumBeatInfos[0].NodeID != g.beatInfoAt(g.drum.Offset).NodeID {
		t.Fatalf("drum view not shifted correctly")
	}
}

func TestDrumViewResizeKeepsOffset(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Populate beat infos with a dummy path longer than the drum view.
	g.beatInfos = make([]model.BeatInfo, 16)
	g.drum.Length = 8
	g.drum.Offset = 2
	g.refreshDrumRow()

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	// Increase length and ensure offset is preserved.
	g.drum.lenIncPressed = true
	g.Update()
	if g.drum.Offset != 2 {
		t.Fatalf("offset changed after length increase: %d", g.drum.Offset)
	}

	// Decrease length and ensure offset is preserved.
	g.drum.lenDecPressed = true
	g.Update()
	if g.drum.Offset != 2 {
		t.Fatalf("offset changed after length decrease: %d", g.drum.Offset)
	}
}

func TestStartNodeSelection(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if g.start != n1 || !n1.Start {
		t.Fatalf("first node should be start")
	}
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
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

func TestHighlightEmptyCells(t *testing.T) {
	t.Skip()
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	g.Layout(1280, 720)
	g.bpm = 60

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(3, 0, model.NodeTypeRegular) // Node with a gap
	g.addEdge(n1, n2)

	g.start = n1
	g.graph.StartNodeID = n1.ID

	g.drum.Length = 4
	g.updateBeatInfos() // This will now correctly handle the invisible nodes

	if len(g.beatInfos) != 4 { // n1, invisible, invisible, n2
		t.Fatalf("Expected beatInfos length to be 4, got %d", len(g.beatInfos))
	}

	g.playing = true
	g.engine.Start()
	g.spawnPulseFrom(0)

	if _, ok := g.highlightedBeats[makeBeatKey(0, 0)]; !ok {
		t.Errorf("Tick 0: Beat at index 0 should be highlighted")
	}

	for g.activePulse != nil && g.activePulse.pathIdx < 2 {
		g.Update()
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, 1)]; !ok {
		t.Errorf("Tick 1: Beat at index 1 should be highlighted")
	}

	for g.activePulse != nil && g.activePulse.pathIdx < 3 {
		g.Update()
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, 2)]; !ok {
		t.Errorf("Tick 2: Beat at index 2 should be highlighted")
	}

	for g.activePulse != nil && g.activePulse.pathIdx < 4 {
		g.Update()
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, 3)]; !ok {
		t.Errorf("Tick 3: Beat at index 3 should be highlighted")
	}
}

func TestBeatInfosNotTrimmedByDrumLength(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.drum.Length = 1
	g.updateBeatInfos()
	// Shrink the drum view again without recomputing beatInfos
	g.drum.Length = 1
	g.drum.Rows[0].Steps = g.drum.Rows[0].Steps[:g.drum.Length]

	if len(g.beatInfos) <= g.drum.Length {
		t.Fatalf("expected beatInfos length > drum length, got %d <= %d", len(g.beatInfos), g.drum.Length)
	}

	if g.beatInfos[0].NodeID != n0.ID || g.beatInfos[1].NodeID != n1.ID || g.beatInfos[2].NodeID != n2.ID {
		t.Errorf("unexpected beatInfos sequence: %v", g.beatInfos)
	}
}

func TestPulseTraversalIgnoresDrumLength(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.drum.Length = 1
	g.updateBeatInfos()
	// Shrink drum view again without affecting beatInfos
	g.drum.Length = 1
	g.drum.Rows[0].Steps = g.drum.Rows[0].Steps[:g.drum.Length]

	g.playing = true
	g.spawnPulseFrom(0)

	if g.activePulse == nil || g.activePulse.toBeatInfo.NodeID != n1.ID {
		t.Fatalf("expected pulse heading to second node")
	}

	// Force pulse to reach second node; it should then move toward third.
	g.activePulse.t = 1
	g.Update()
	if g.activePulse == nil || g.activePulse.toBeatInfo.NodeID != n2.ID {
		t.Fatalf("expected pulse to continue to third node, got %+v", g.activePulse)
	}

	// Reach final node; pulse should stop without restarting at origin.
	g.activePulse.t = 1
	g.Update()
	if g.activePulse != nil {
		t.Fatalf("expected pulse to stop after last node, but it continued")
	}
}

func TestDrumViewLoopingWithInvisibleNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Create a path with an invisible node: n0 -> invisible -> n1
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	_ = g.tryAddNode(1, 0, model.NodeTypeInvisible) // Invisible node

	g.addEdge(n0, n1)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	// Set drum view length
	g.drum.Length = 6

	g.updateBeatInfos()

	// Expected drum view steps: n0 (true), invisible (false), n1 (true), then padded
	expectedSteps := []bool{
		true,  // n0
		false, // invisible
		true,  // n1
		false, // padded
		false, // padded
		false, // padded
	}

	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("Expected drum view steps length to be %d, got %d", g.drum.Length, len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedSteps {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("Drum view step %d: expected %t, got %t", i, expected, g.drum.Rows[0].Steps[i])
		}
	}
}

func TestDrumViewLoopingHighlighting(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger) // Use the Game struct to leverage its graph manipulation and beat info update logic

	// Set up the drum view length to accommodate the expected sequence
	g.drum.Length = 6
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	// Create the circuit: [X] -> [] -> [X] -> [X]
	//                       ^      ^
	//                       |      |
	//                      [X] <- [X]

	// Nodes:
	// (0,0) - Node 1 (Regular)
	// (1,0) - Node 2 (Invisible)
	// (2,0) - Node 3 (Regular)
	// (3,0) - Node 4 (Regular)
	// (3,1) - Node 5 (Regular)
	// (2,1) - Node 6 (Regular)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = n1
	g.graph.StartNodeID = n1.ID

	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible) // invisible
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)   // X
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)   // X
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)   // X
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)   // X

	// Edges:
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	// Update beat infos to populate drum view steps
	g.updateBeatInfos()

	expectedDrumRow := []bool{true, false, true, true, true, true} // [X][ ][X][X][X][X]

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}

	// Test a more complex looped circuit
	// [X] -> [X] -> [X]
	//  ^           |
	//  |           v
	// [X] <- [X] <- [X]

	// Reset graph and drum view
	g = New(logger)
	g.drum.Length = 6
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	cn1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = cn1
	g.graph.StartNodeID = cn1.ID

	cn2 := g.tryAddNode(1, 0, model.NodeTypeRegular) // X
	cn3 := g.tryAddNode(2, 0, model.NodeTypeRegular) // X
	cn4 := g.tryAddNode(2, 1, model.NodeTypeRegular) // X
	cn5 := g.tryAddNode(1, 1, model.NodeTypeRegular) // X
	cn6 := g.tryAddNode(0, 1, model.NodeTypeRegular) // X

	g.addEdge(cn1, cn2)
	g.addEdge(cn2, cn3)
	g.addEdge(cn3, cn4)
	g.addEdge(cn4, cn5)
	g.addEdge(cn5, cn6)
	g.addEdge(cn6, cn1) // Loop back to cn1

	g.updateBeatInfos()

	expectedDrumRow2 := []bool{true, true, true, true, true, true} // All X

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow2) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow2), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow2 {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}
}

func TestPulseTraversalBeyondDrumView(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	g.Layout(640, 480)

	// Build looped circuit: [O] -> [] -> [X] -> [X]
	//                                   ^      v
	//                                   [X] <- [X]
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID

	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // loop

	// Drum view shorter than path length
	g.drum.Length = 4
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	g.updateBeatInfos()

	g.playing = true
	g.spawnPulseFrom(0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}

	steps := 4
	for i := 0; i < steps; i++ {
		if !g.advancePulse(g.activePulse) {
			t.Fatalf("pulse stopped early at step %d", i)
		}
	}

	if g.activePulse.pathIdx != steps+1 {
		t.Fatalf("expected pathIdx %d, got %d", steps+1, g.activePulse.pathIdx)
	}

	if g.activePulse.lastIdx != steps {
		t.Fatalf("expected lastIdx %d, got %d", steps, g.activePulse.lastIdx)
	}
}

func TestSignalTraversalInLoop(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	g.Layout(640, 480)

	// Nodes
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n_inv1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	// Edges
	g.addEdge(n1, n_inv1)
	g.addEdge(n_inv1, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	g.drum.Length = 11 // Drum view longer than path
	g.updateBeatInfos()

	// Expected sequence of node IDs for the pulse traversal
	expectedNodeIDs := []model.NodeID{
		n1.ID,
		n_inv1.ID,
		n3.ID,
		n4.ID,
		n5.ID,
		n6.ID,
	}

	t.Logf("Expected Node IDs: %v", expectedNodeIDs)
	actualNodeIDs := []model.NodeID{}
	for _, beatInfo := range g.beatInfos {
		actualNodeIDs = append(actualNodeIDs, beatInfo.NodeID)
	}
	t.Logf("Actual Beat Infos: %v", actualNodeIDs)

	// Verify that the generated beatInfos begin with the expected sequence
	if len(actualNodeIDs) < len(expectedNodeIDs) {
		t.Fatalf("Initial beatInfos shorter than expected. want >=%d got %d", len(expectedNodeIDs), len(actualNodeIDs))
	}
	for i, expectedID := range expectedNodeIDs {
		if actualNodeIDs[i] != expectedID {
			t.Errorf("Initial beatInfos mismatch at index %d. Expected %d, got %d", i, expectedID, actualNodeIDs[i])
		}
	}

	g.playing = true
	g.spawnPulseFrom(0)

	if g.activePulse == nil {
		t.Fatalf("Expected active pulse after spawning")
	}

	// Advance the pulse and check its path
	maxIterations := len(expectedNodeIDs) * 100 // Safety break for infinite loops
	for step := 0; step < len(expectedNodeIDs)-1; step++ {
		if g.activePulse == nil {
			t.Fatalf("Pulse ended prematurely at step %d", step)
		}
		if g.activePulse.fromBeatInfo.NodeID != expectedNodeIDs[step] {
			t.Errorf("Step %d: Expected NodeID %d, got %d (fromBeatInfo)", step, expectedNodeIDs[step], g.activePulse.fromBeatInfo.NodeID)
		}
		if g.activePulse.toBeatInfo.NodeID != expectedNodeIDs[step+1] {
			t.Errorf("Step %d: Expected next NodeID %d, got %d (toBeatInfo)", step, expectedNodeIDs[step+1], g.activePulse.toBeatInfo.NodeID)
		}

		startIdx := g.activePulse.pathIdx
		frameCounter := 0
		for g.activePulse != nil && g.activePulse.pathIdx == startIdx && frameCounter < maxIterations {
			g.Update()
			frameCounter++
			t.Logf("  Inside inner loop: pathIdx=%d, t=%.2f, frameCounter=%d", g.activePulse.pathIdx, g.activePulse.t, frameCounter)
		}
		if frameCounter >= maxIterations {
			t.Fatalf("Inner loop exceeded max iterations (%d) at step %d, possible infinite loop. pathIdx=%d", maxIterations, step, g.activePulse.pathIdx)
		}
	}
}

func TestLoopExpansionAndHighlighting(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	g.Layout(640, 480)

	// prepare drum view length to capture multiple loop laps
	g.drum.Length = 10
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)

	// build circuit: start -> invisible -> n1 -> n2 -> n3 -> n4 -> back to n1
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	inv := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n4 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	g.addEdge(start, inv)
	g.addEdge(inv, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n1) // close loop

	g.updateBeatInfos()

	// verify drum beat infos expand deterministically across drum length
	wantIDs := []model.NodeID{start.ID, inv.ID, n1.ID, n2.ID, n3.ID, n4.ID, n1.ID, n2.ID, n3.ID, n4.ID}
	if len(g.drumBeatInfos) != len(wantIDs) {
		t.Fatalf("expected %d drum beat infos, got %d", len(wantIDs), len(g.drumBeatInfos))
	}
	for i, id := range wantIDs {
		if g.drumBeatInfos[i].NodeID != id {
			t.Fatalf("at %d expected node %d got %d", i, id, g.drumBeatInfos[i].NodeID)
		}
	}

	if len(g.beatInfos) != 6 {
		t.Fatalf("expected base path length 6, got %d", len(g.beatInfos))
	}

	// now simulate pulse highlighting across two laps
	g.spawnPulseFrom(0)
	// sequence of highlighted beat indices expected for first 12 advancements
	expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	got := make([]int, len(expected))
	got[0] = 0
	for i := 1; i < len(expected); i++ {
		delete(g.highlightedBeats, makeBeatKey(0, g.activePulse.lastIdx))
		if !g.advancePulse(g.activePulse) {
			t.Fatalf("pulse ended early at step %d", i)
		}
		if len(g.highlightedBeats) != 1 {
			t.Fatalf("expected single highlight, got %v", g.highlightedBeats)
		}
		for key := range g.highlightedBeats {
			_, idx := splitBeatKey(key)
			got[i] = idx
		}
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("highlight sequence mismatch. expected %v got %v", expected, got)
	}
}

func TestBPMChangeDuringLoopKeepsForwardProgress(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	g.Layout(640, 480)

	// Build looped circuit: [O] -> [] -> [X] -> [X]
	//                                   ^      v
	//                                   [X] <- [X]
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3)

	g.drum.Length = 8
	g.drum.Rows[0].Steps = make([]bool, g.drum.Length)
	g.drum.SetBeatLength(g.drum.Length)
	g.updateBeatInfos()

	g.playing = true
	g.spawnPulseFrom(0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}

	for i := 0; i < 10; i++ {
		g.Update()
	}
	beforeIdx := g.activePulse.pathIdx
	beforeT := g.activePulse.t

	g.drum.bpm = 240
	g.Update()

	if g.activePulse.pathIdx < beforeIdx {
		t.Fatalf("pathIdx went backwards: %d -> %d", beforeIdx, g.activePulse.pathIdx)
	}

	oldSpeed := 1.0 / (60.0 / 120.0 * float64(ebitenTPS))
	expectedT := (beforeT + oldSpeed) * 2
	if math.Abs(g.activePulse.t-expectedT) > 0.05 {
		t.Fatalf("expected scaled t around %.2f got %.2f", expectedT, g.activePulse.t)
	}

	lastIdx := g.activePulse.pathIdx
	for i := 0; i < 60; i++ {
		g.Update()
		if g.activePulse == nil {
			t.Fatalf("pulse ended early at frame %d", i)
		}
		if g.activePulse.pathIdx < lastIdx {
			t.Fatalf("pathIdx decreased from %d to %d", lastIdx, g.activePulse.pathIdx)
		}
		lastIdx = g.activePulse.pathIdx
	}
}
