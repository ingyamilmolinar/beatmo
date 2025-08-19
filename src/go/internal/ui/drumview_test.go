package ui

import (
	"image"
	"image/color"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestNewDrumView(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	if drumView == nil {
		t.Fatal("NewDrumView returned nil")
	}
	if drumView.Length != 8 {
		t.Errorf("Expected initial drum view length to be 8, got %d", drumView.Length)
	}
	if len(drumView.Rows) != 1 {
		t.Fatalf("Expected 1 drum row, got %d", len(drumView.Rows))
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
	for i, step := range drumView.Rows[0].Steps {
		if step {
			t.Errorf("Expected step %d to be false (empty), got true", i)
		}
	}
}

func TestDrumViewLengthIncrease(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Simulate button press
	drumView.lenIncPressed = true
	drumView.Update()

	if drumView.Length != 9 {
		t.Errorf("Expected drum view length to increase to 9, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 9 {
		t.Errorf("Expected drum row steps length to be 9, got %d", len(drumView.Rows[0].Steps))
	}
}

// Ensure row labels and delete buttons sit beneath the control panel and align
// to the left of their corresponding step rows.
func TestDrumRowLayout(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 500, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowLabels) == 0 {
		t.Fatalf("expected at least one row label")
	}
	label := dv.rowLabels[0].Rect()
	if label.Min.Y < dv.uploadBtn.Rect().Max.Y {
		t.Fatalf("row label overlaps controls: label %v controls bottom %d", label, dv.uploadBtn.Rect().Max.Y)
	}
	stepStart := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	del := dv.rowDeleteBtns[0].Rect()
	if del.Min.X-label.Max.X < buttonPad {
		t.Fatalf("delete button lacks padding: %v vs %v", del, label)
	}
	if label.Max.X > stepStart {
		t.Fatalf("label encroaches into step area: %v >= %d", label, stepStart)
	}
	if dv.addRowBtn.Rect().Min.Y != label.Min.Y+dv.rowHeight() {
		t.Fatalf("add-row button not directly below row: %v", dv.addRowBtn.Rect())
	}
	for _, btn := range []*Button{dv.rowLabels[0], dv.rowDeleteBtns[0], dv.addRowBtn} {
		tr := btn.textRect()
		r := btn.Rect()
		if !tr.In(r) {
			t.Fatalf("text outside row button: %v not in %v", tr, r)
		}
		cx := (r.Min.X + r.Max.X) / 2
		ctx := (tr.Min.X + tr.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		cty := (tr.Min.Y + tr.Max.Y) / 2
		if intAbs(cx-ctx) > 1 || intAbs(cy-cty) > 1 {
			t.Fatalf("text not centered in row button: %v", r)
		}
	}
}

func TestDrumViewLengthDecrease(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Increase length first to ensure we can decrease
	drumView.lenIncPressed = true
	drumView.Update() // Length is now 9

	// Simulate button press
	drumView.lenDecPressed = true
	drumView.Update()

	if drumView.Length != 8 {
		t.Errorf("Expected drum view length to decrease to 8, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
}

func TestDrumViewVerticalScroll(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+2*24), nil, logger)
	for i := 0; i < 4; i++ {
		dv.AddRow()
	}
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, -1 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	if dv.rowOffset != 1 {
		t.Fatalf("rowOffset=%d", dv.rowOffset)
	}
	thumb := dv.scrollThumbRect()
	restore = SetInputForTest(
		func() (int, int) { return thumb.Min.X + 1, thumb.Min.Y + 1 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	restore = SetInputForTest(
		func() (int, int) { return thumb.Min.X + 1, dv.scrollBarRect().Max.Y - 1 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	if dv.rowOffset <= 1 {
		t.Fatalf("expected drag to scroll, rowOffset=%d", dv.rowOffset)
	}
}

func TestDrumViewWheelAdjustsLength(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)

	wheelVal := 1.0
	cursor := func() (int, int) { return dv.Bounds.Min.X + dv.labelW + 500, dv.Bounds.Min.Y + timelineHeight + 5 }
	restore := SetInputForTest(cursor,
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return 800, 600 },
	)
	dv.Update() // wheel up -> length++
	restore()
	if dv.Length != 9 {
		t.Fatalf("expected length 9 got %d", dv.Length)
	}
}

func TestDrumViewLengthMinMax(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Test min length (should not go below 1)
	drumView.Length = 1
	drumView.lenDecPressed = true
	drumView.Update()
	if drumView.Length != 1 {
		t.Errorf("Expected drum view length to stay at 1, got %d", drumView.Length)
	}

	// Test max length (should not go above 64)
	drumView.Length = 64
	drumView.lenIncPressed = true
	drumView.Update()
	if drumView.Length != 64 {
		t.Errorf("Expected drum view length to stay at 64, got %d", drumView.Length)
	}
}

func TestTimelineInfo(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.bpm = 120
	info := dv.timelineInfo(4)
	expected := "Beat 4.000/8.0 | 00:02.000/00:04.000 | View 00:00.000-00:04.000"
	if info != expected {
		t.Fatalf("expected %q got %q", expected, info)
	}
}

func TestTimelineInfoFractionalBeat(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.bpm = 120
	dv.timelineBeats = 32
	info := dv.timelineInfo(1.25)
	if !strings.HasPrefix(info, "Beat 1.250/32.0") {
		t.Fatalf("unexpected beat info: %q", info)
	}
	if strings.Count(info, "Beat") != 1 {
		t.Fatalf("duplicate beat counts in %q", info)
	}
}

func TestTimelineViewRect(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.SetBeatLength(16)
	dv.Offset = 4
	dv.recalcButtons()

	var got image.Rectangle
	orig := drawRect
	drewBorder := false
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineView {
				got = r
			}
		} else {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineViewHi {
				drewBorder = true
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(800, 200), nil, 0, nil, 0)

	totalBeats := dv.timelineBeats
	start := dv.timelineRect.Min.X + int(float64(dv.Offset)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
	width := int(float64(dv.Length) / float64(totalBeats) * float64(dv.timelineRect.Dx()))
	want := image.Rect(start, dv.timelineRect.Min.Y, start+width, dv.timelineRect.Max.Y)
	if got != want || !drewBorder {
		t.Fatalf("view rect/border mismatch: rect=%v border=%t want %v", got, drewBorder, want)
	}
}

func TestTimelineLayout(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	textY := dv.Bounds.Min.Y + 5
	if textY >= dv.timelineRect.Min.Y {
		t.Fatalf("info text overlaps timeline bar")
	}
	rowStart := dv.Bounds.Min.Y + timelineHeight
	if dv.timelineRect.Max.Y >= rowStart {
		t.Fatalf("timeline bar overlaps drum rows")
	}
}

func TestTimelineExpandsAndViewShrinks(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	img := ebiten.NewImage(800, 200)

	var rect image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineView {
				rect = r
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(img, nil, 0, nil, 0)
	baseWidth := rect.Dx()

	dv.Draw(img, nil, 0, nil, 20)
	expandedWidth := rect.Dx()

	if dv.timelineBeats != 28 {
		t.Fatalf("timelineBeats = %d want 28", dv.timelineBeats)
	}
	if expandedWidth >= baseWidth {
		t.Fatalf("view width did not shrink: base %d expanded %d", baseWidth, expandedWidth)
	}
}

func TestTimelineBeatMarkersDecimate(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.timelineBeats = 10000 // simulate long timeline

	var view image.Rectangle
	markers := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.RGBA); ok {
				if clr == colTimelineView {
					view = r
				} else if clr == colTimelineBeat && r.Min.Y == dv.timelineRect.Min.Y && r.Max.Y == dv.timelineRect.Max.Y {
					markers++
				}
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(400, 200), nil, 0, nil, 0)

	if view.Dx() < 1 {
		t.Fatalf("view width = %d want >=1", view.Dx())
	}
	if markers > dv.timelineRect.Dx()+1 {
		t.Fatalf("too many beat markers: %d > %d", markers, dv.timelineRect.Dx()+1)
	}
}

func TestTimelineScrubSeek(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 400, timelineHeight+2*24), nil, logger)
	dv.recalcButtons()
	dv.timelineBeats = 100

	mx := dv.timelineRect.Min.X + dv.timelineRect.Dx()/2
	my := dv.timelineRect.Min.Y + dv.timelineRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	want := (dv.timelineBeats - dv.Length) / 2
	if dv.Offset != want {
		t.Fatalf("offset=%d want %d", dv.Offset, want)
	}
}

func TestTimelineScrubLongTimeline(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 400, timelineHeight+2*24), nil, logger)
	dv.recalcButtons()
	dv.timelineBeats = 10000

	mx := dv.timelineRect.Max.X - 1
	my := dv.timelineRect.Min.Y + dv.timelineRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	total := dv.timelineBeats - dv.Length
	frac := float64(dv.timelineRect.Dx()-1) / float64(dv.timelineRect.Dx())
	want := int(frac * float64(total))
	if dv.Offset != want {
		t.Fatalf("offset=%d want %d", dv.Offset, want)
	}
	if dv.timelineBeats != 10000 {
		t.Fatalf("timelineBeats changed: %d", dv.timelineBeats)
	}
}

func TestDrumViewUpdatesGraphBeatLength(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	// Initial check
	if graph.BeatLength() != 8 {
		t.Errorf("Expected initial graph beat length to be 8, got %d", graph.BeatLength())
	}

	// Increase length and check graph
	drumView.lenIncPressed = true
	drumView.Update()
	if graph.BeatLength() != 9 {
		t.Errorf("Expected graph beat length to be 9 after increase, got %d", graph.BeatLength())
	}

	// Decrease length and check graph
	drumView.lenDecPressed = true
	drumView.Update()
	if graph.BeatLength() != 8 {
		t.Errorf("Expected graph beat length to be 8 after decrease, got %d", graph.BeatLength())
	}
}

func TestDrumViewLooping(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)

	// Create a looping graph: O > X > X > (loop start) X > X > (loop end)
	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node1 := graph.AddNode(1, 0, model.NodeTypeRegular)
	node2 := graph.AddNode(2, 0, model.NodeTypeRegular)
	node3 := graph.AddNode(3, 0, model.NodeTypeRegular)
	node4 := graph.AddNode(4, 0, model.NodeTypeRegular)

	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node2, node3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node3, node4}] = struct{}{}
	graph.Edges[[2]model.NodeID{node4, node2}] = struct{}{}

	drumView := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)
	drumView.Length = 10
	drumView.SetBeatLength(10)

	// Manually call updateBeatInfos to populate the drum view
	game := &Game{graph: graph, drum: drumView, logger: logger}
	game.updateBeatInfos()

	expectedSteps := []bool{true, true, true, true, true, true, true, true, true, true}
	t.Logf("Generated drum row: %v", drumView.Rows[0].Steps)
	if len(drumView.Rows[0].Steps) != len(expectedSteps) {
		t.Fatalf("Expected %d steps, but got %d", len(expectedSteps), len(drumView.Rows[0].Steps))
	}

	for i, step := range drumView.Rows[0].Steps {
		if step != expectedSteps[i] {
			t.Errorf("Step %d: expected %v, got %v", i, expectedSteps[i], step)
		}
	}
}

func TestDrumViewLoopHighlighting(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	logger.SetLevel(game_log.LevelDebug) // Enable debug logging for this test

	graph := model.NewGraph(logger)

	// Circuit: [O] > [] > [X] > [X]
	//                    ^     v
	//                   [X] < [X]
	// This translates to:
	// node0 (0,0) -> node_inv1 (1,0) -> node1 (2,0)
	// node1 (2,0) -> node_inv2 (2,1) -> node2 (2,2)
	// node2 (2,2) -> node_inv3 (1,2) -> node3 (0,2)
	// node3 (0,2) -> node_inv4 (0,1) -> node1 (2,0) (loop back to node1)

	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node_inv1 := graph.AddNode(1, 0, model.NodeTypeInvisible)
	node1 := graph.AddNode(2, 0, model.NodeTypeRegular)
	node_inv2 := graph.AddNode(2, 1, model.NodeTypeInvisible)
	node2 := graph.AddNode(2, 2, model.NodeTypeRegular)
	node_inv3 := graph.AddNode(1, 2, model.NodeTypeInvisible)
	node3 := graph.AddNode(0, 2, model.NodeTypeRegular)
	node_inv4 := graph.AddNode(0, 1, model.NodeTypeInvisible)

	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node_inv1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv1, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node_inv2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv2, node2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node2, node_inv3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv3, node3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node3, node_inv4}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv4, node1}] = struct{}{} // Loop back to node1

	drumView := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)
	drumView.Length = 10 // Set a reasonable length for the drum view
	drumView.SetBeatLength(drumView.Length)

	game := New(logger)
	game.graph = graph
	game.drum = drumView
	game.bpm = 120        // Set a BPM for consistent beat duration
	game.Layout(800, 720) // Set layout to initialize drum view bounds

	// Simulate starting playback
	game.playing = true
	game.updateBeatInfos() // Call updateBeatInfos after drum is set
	game.spawnPulseFrom(0)

	// Run for a few beats to test loop highlighting
	for i := 0; i < 20 && game.activePulse != nil; i++ {
		delete(game.highlightedBeats, makeBeatKey(0, game.activePulse.lastIdx))
		game.advancePulse(game.activePulse)
		t.Logf("Step %d: highlightedBeats: %v", i, game.highlightedBeats)
		if len(game.highlightedBeats) != 1 {
			t.Fatalf("step %d: expected one highlight got %v", i, game.highlightedBeats)
		}
	}
}

func TestDrumViewButtonsDrawn(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelInfo)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)

	count := 0
	orig := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		count++
	}
	defer func() { drawButton = orig }()

	dv.Draw(ebiten.NewImage(400, 200), map[int]int64{}, 0, nil, 0)
	if count != 16 {
		t.Fatalf("expected 16 buttons drawn, got %d", count)
	}
}

func TestDrumViewHighlightsMultipleRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.Length = 4
	dv.SetBeatLength(4)
	dv.AddRow()

	highlights := map[int]int64{
		makeBeatKey(0, 1): 1,
		makeBeatKey(1, 2): 1,
	}

	dst := ebiten.NewImage(800, 200)
	orig := drawRect
	var hits [][2]int
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && r.Min.Y >= dv.Bounds.Min.Y+timelineHeight {
			row := (r.Min.Y - (dv.Bounds.Min.Y + timelineHeight)) / dv.rowHeight()
			col := (r.Min.X - (dv.Bounds.Min.X + dv.labelW + dv.controlsW)) / dv.cell
			if color.RGBAModel.Convert(c).(color.RGBA) == colHighlight {
				hits = append(hits, [2]int{row, col})
			}
		}
		orig(dst, r, c, filled)
	}
	dv.Draw(dst, highlights, 0, make([]model.BeatInfo, dv.Length), 0)
	drawRect = orig

	want := map[[2]int]bool{{0, 1}: true, {1, 2}: true}
	if len(hits) != 2 || !want[hits[0]] || !want[hits[1]] {
		t.Fatalf("unexpected highlight cells %v", hits)
	}
}

func TestDrumViewAddAndDeleteRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dv.calcLayout()

	pressed := true
	cx, cy := dv.addRowBtn.Rect().Min.X+1, dv.addRowBtn.Rect().Min.Y+1
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if len(dv.Rows) != 2 {
		t.Fatalf("expected 2 rows got %d", len(dv.Rows))
	}

	// recalc layout and delete the second row directly
	dv.Update()
	dv.DeleteRow(1)
	if len(dv.Rows) != 1 {
		t.Fatalf("expected 1 row after deletion got %d", len(dv.Rows))
	}
}

func TestRenameOpensWithCursor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)

	cs := &countStyle{}
	dv.rowLabels[0].Style = cs

	calls := 0
	origCursor := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, col color.Color) { calls++ }
	defer func() { drawCursor = origCursor }()

	dv.rowEditBtns[0].OnClick()
	restore := SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	dv.Update()
	restore()

	dv.Draw(ebiten.NewImage(200, 200), map[int]int64{}, 0, nil, 0)

	if calls == 0 {
		t.Fatalf("cursor not drawn")
	}
	if cs.n != 0 {
		t.Fatalf("row label drawn while renaming")
	}
}

type countStyle struct{ n int }

func (c *countStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) { c.n++ }

func TestDeleteButtonDisabledWhenSingleRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), nil, logger)

	if dv.rowDeleteBtns[0].OnClick != nil {
		t.Fatalf("delete button should be disabled with single row")
	}
	dv.DeleteRow(0)
	if len(dv.Rows) != 1 {
		t.Fatalf("single row should not be deletable")
	}

	dv.AddRow()
	if dv.rowDeleteBtns[0].OnClick == nil || dv.rowDeleteBtns[1].OnClick == nil {
		t.Fatalf("delete buttons not enabled after adding row")
	}
	dv.DeleteRow(1)
	if len(dv.Rows) != 1 {
		t.Fatalf("row not deleted")
	}
	if dv.rowDeleteBtns[0].OnClick != nil {
		t.Fatalf("delete button should be disabled after deleting to one row")
	}
}

func TestDrumViewChangeInstrumentPerRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.instOptions = []string{"snare", "kick"}
	dv.AddRow()
	dv.Update()
	dv.selRow = 1
	dv.recalcButtons()

	before := dv.Rows[1].Instrument
	dv.CycleInstrument()
	if dv.Rows[1].Instrument == before {
		t.Fatalf("expected instrument to change")
	}
}

func TestDrumViewDeleteRowRecordsOrigin(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	dv.Rows[1].Origin = 42
	dv.DeleteRow(1)
	rows := dv.ConsumeDeletedRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(rows))
	}
	if rows[0].index != 1 || rows[0].origin != 42 {
		t.Fatalf("unexpected deleted row info: %+v", rows[0])
	}
}

func TestInstrumentMenuIncludesCustom(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	audio.ResetInstruments()
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
	audio.RegisterWAV("custom", "")
	dv.Update()

	dv.rowLabels[0].OnClick() // open menu
	var btn *Button
	for _, b := range dv.instMenuBtns {
		if b.Text == "Custom" {
			btn = b
		}
	}
	if btn == nil {
		t.Fatalf("custom instrument not listed")
	}
	btn.OnClick()
	if dv.Rows[0].Instrument != "custom" {
		t.Fatalf("expected custom instrument selected, got %s", dv.Rows[0].Instrument)
	}
	if len(dv.Rows) != 1 {
		t.Fatalf("unexpected row count %d", len(dv.Rows))
	}
}

func TestDrumViewConsumeAddedRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	rows := dv.ConsumeAddedRows()
	if len(rows) != 1 || rows[0] != 1 {
		t.Fatalf("expected added row index 1, got %v", rows)
	}
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("expected added rows cleared after consume")
	}
}

func TestDropdownBlocksUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+3*24), graph, logger)
	dv.Update()

	// open menu for first row which appears above the add-row button
	dv.rowLabels[0].OnClick()
	dv.Update()

	add := dv.addRowBtn.Rect()
	mx, my := add.Min.X+1, add.Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()

	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via dropdown click")
	}
}

func TestDropdownOutsideClickDoesNotTriggerUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+3*24), graph, logger)
	dv.Update()

	// open menu
	dv.rowLabels[0].OnClick()
	dv.Update()

	// click outside menu where the add-row button resides
	add := dv.addRowBtn.Rect()
	mx, my := add.Min.X+1, add.Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	dv.Update()
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via outside dropdown click")
	}
	pressed = false
	dv.Update()
	restore()
}

func TestRenameBoxBlocksUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+3*24), graph, logger)
	dv.Update()

	// open rename box for row 0
	pressed := true
	btn := dv.rowEditBtns[0]
	cx, cy := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	if dv.renameBox == nil {
		t.Fatalf("rename box not active")
	}

	rx := dv.renameBox.Rect.Min.X + 1
	ry := dv.renameBox.Rect.Min.Y + 1
	restore = SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()

	if dv.instMenuOpen {
		t.Fatalf("instrument menu opened via rename box click")
	}
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via rename box click")
	}
}

func TestDrumViewRenameInstrument(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), graph, logger)
	dv.Update()

	pressed := true
	btn := dv.rowEditBtns[0]
	cx, cy := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if dv.renameBox == nil {
		t.Fatalf("rename box not opened")
	}
	if dv.renameBox.Value() != dv.Rows[0].Name {
		t.Fatalf("rename box value %q", dv.renameBox.Value())
	}

	restore = SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return []rune("X") }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	dv.Update()
	restore()
	restore = SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(k ebiten.Key) bool { return k == ebiten.KeyEnter }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	dv.Update()
	restore()
	if dv.renameBox != nil {
		t.Fatalf("rename box still active")
	}
	if dv.Rows[0].Name != dv.rowLabels[0].Text || dv.Rows[0].Name != "SnareX" {
		t.Fatalf("unexpected name %q label %q", dv.Rows[0].Name, dv.rowLabels[0].Text)
	}
}

func TestDrumViewRenameBoxBounds(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), graph, logger)
	dv.Update()
	dv.rowEditBtns[0].OnClick()
	if dv.renameBox == nil {
		t.Fatalf("rename box not created")
	}
	expected := dv.renameBox.Rect
	minDim := expected.Dx()
	if expected.Dy() < minDim {
		minDim = expected.Dy()
	}
	inset := int(float64(minDim) * 0.1)
	want := image.Rect(expected.Min.X+inset, expected.Min.Y+inset, expected.Max.X-inset, expected.Max.Y-inset)
	var rects []image.Rectangle
	old := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, f, b color.Color, pressed bool) {
		rects = append(rects, r)
	}
	dv.Draw(ebiten.NewImage(300, 200), map[int]int64{}, 0, nil, 0)
	drawButton = old
	found := false
	for _, r := range rects {
		if r == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("rename box bounds %v not drawn", want)
	}
}

func TestRenameUpdatesInstrumentDropdown(t *testing.T) {
	audio.ResetInstruments()
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, logger)
	dv.renameRow = 0
	dv.renameBox = NewTextInput(image.Rect(0, 0, 80, 20), BPMBoxStyle)
	dv.renameBox.SetText("snare2")
	dv.renameBox.focused = true
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	if dv.Rows[0].Instrument != "snare2" {
		t.Fatalf("instrument=%s", dv.Rows[0].Instrument)
	}
	if !slices.Contains(audio.Instruments(), "snare2") {
		t.Fatalf("dropdown missing renamed instrument")
	}
}

func TestDrumViewOriginRequests(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	audio.ResetInstruments()
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
	dv.AddRow()
	dv.calcLayout()

	if len(dv.rowOriginBtns) < 2 {
		t.Fatalf("expected origin buttons for two rows, got %d", len(dv.rowOriginBtns))
	}
	dv.rowOriginBtns[0].OnClick()
	dv.rowOriginBtns[1].OnClick()
	rows := dv.ConsumeOriginRequests()
	if len(rows) != 2 || rows[0] != 0 || rows[1] != 1 {
		t.Fatalf("unexpected origin requests %v", rows)
	}
	if len(dv.ConsumeOriginRequests()) != 0 {
		t.Fatalf("origin requests not cleared")
	}
}

func TestDrumViewInstrumentColor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	audio.ResetInstruments()
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
	expected := instColor(dv.Rows[0].Instrument)
	if dv.Rows[0].Color != expected {
		t.Fatalf("expected initial color %v got %v", expected, dv.Rows[0].Color)
	}
	dv.CycleInstrument()
	expected = instColor(dv.Rows[0].Instrument)
	if dv.Rows[0].Color != expected {
		t.Fatalf("expected cycled color %v got %v", expected, dv.Rows[0].Color)
	}
}

func colorsEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestCustomInstrumentColorsRotate(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	audio.ResetInstruments()
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)

	audio.RegisterWAV("c1", "")
	dv.SetInstrument("c1")
	col1 := dv.Rows[0].Color

	dv.AddRow()
	dv.selRow = 1
	audio.RegisterWAV("c2", "")
	dv.SetInstrument("c2")
	col2 := dv.Rows[1].Color

	if colorsEqual(col1, col2) {
		t.Fatalf("expected different colors for custom instruments")
	}
	if colorsEqual(col1, colStep) || colorsEqual(col2, colStep) {
		t.Fatalf("unexpected fallback color used")
	}
}

func TestRowControlsSpanLeftPanel(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, testLogger)
	dv.calcLayout()
	got := dv.rowDeleteBtns[0].Rect()
	rightOf := dv.rowOriginBtns[0].Rect().Max.X
	if got.Min.X <= rightOf {
		t.Fatalf("delete button not rightmost: %v <= %d", got, rightOf)
	}
}

func TestInstrumentDropdownSelect(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	audio.ResetInstruments()
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), graph, logger)
	dv.calcLayout()
	dv.rowLabels[0].OnClick() // open menu
	if !dv.instMenuOpen {
		t.Fatalf("instrument menu not open")
	}
	if len(dv.instMenuBtns) < 2 {
		t.Fatalf("expected at least two instrument options")
	}
	btn := dv.instMenuBtns[1]
	bx, by := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return bx, by },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()
	if dv.Rows[0].Instrument != audio.Instruments()[1] {
		t.Fatalf("instrument not set via dropdown: %s vs %s", dv.Rows[0].Instrument, audio.Instruments()[1])
	}
	if dv.instMenuOpen {
		t.Fatalf("menu did not close after selection")
	}
}

func TestSelectingInstrumentDoesNotAddRow(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, testLogger)
	dv.calcLayout()
	startRows := len(dv.Rows)

	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu not opened")
	}
	if len(dv.instMenuBtns) == 0 {
		t.Fatalf("no instrument buttons")
	}
	b0 := dv.instMenuBtns[0]
	bx, by := b0.Rect().Min.X+1, b0.Rect().Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return bx, by },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()
	if len(dv.Rows) != startRows {
		t.Fatalf("rows=%d want %d", len(dv.Rows), startRows)
	}

	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu not reopened")
	}
	last := dv.instMenuBtns[len(dv.instMenuBtns)-1]
	bx, by = last.Rect().Min.X+1, last.Rect().Min.Y+1
	restore = SetInputForTest(
		func() (int, int) { return bx, by },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()
	if len(dv.Rows) != startRows {
		t.Fatalf("rows grew after change: %d", len(dv.Rows))
	}
}

func TestInstrumentDropdownFitsBounds(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, testLogger)
	dv.AddRow()
	dv.AddRow()
	dv.calcLayout()
	idx := len(dv.Rows) - 1
	dv.rowLabels[idx].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu not opened")
	}
	for _, btn := range dv.instMenuBtns {
		r := btn.Rect()
		if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > dv.Bounds.Max.Y {
			t.Fatalf("menu button out of bounds: %v vs %v", r, dv.Bounds)
		}
	}
	first := dv.instMenuBtns[0].Rect()
	base := dv.rowLabels[idx].Rect()
	if first.Max.Y > base.Min.Y {
		t.Fatalf("expected menu to open upward: %v vs %v", first, base)
	}
}

func TestDropdownHoverHighlight(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, testLogger)
	dv.calcLayout()
	dv.rowLabels[0].OnClick()
	if !dv.instMenuOpen {
		t.Fatalf("menu not open")
	}
	btn := dv.instMenuBtns[0]
	// capture normal draw colors
	img := ebiten.NewImage(10, 10)
	var normFill, normBorder color.Color
	orig := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		normFill, normBorder = fill, border
	}
	btn.Draw(img)
	if !colorsEqual(normBorder, colDropdownEdge) {
		t.Fatalf("unexpected border color: %#v", normBorder)
	}
	// simulate hover
	mx, my := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	btn.Handle(mx, my, false)
	var hovFill, hovBorder color.Color
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		hovFill, hovBorder = fill, border
	}
	btn.Draw(img)
	drawButton = orig
	if colorsEqual(normFill, hovFill) || colorsEqual(normBorder, hovBorder) {
		t.Fatalf("expected hover to change colors")
	}
}

func TestDrumViewLayoutStacksRows(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, testLogger)
	dv.AddRow()
	dv.calcLayout()
	if len(dv.rowLabels) != 2 {
		t.Fatalf("expected 2 row labels, got %d", len(dv.rowLabels))
	}
	if dv.rowLabels[1].Rect().Min.Y <= dv.rowLabels[0].Rect().Min.Y {
		t.Fatalf("row labels not stacked vertically: %v vs %v", dv.rowLabels[0].Rect(), dv.rowLabels[1].Rect())
	}
	if dv.addRowBtn.Rect().Min.Y <= dv.rowLabels[1].Rect().Min.Y {
		t.Fatalf("add button not below rows: %v vs %v", dv.addRowBtn.Rect(), dv.rowLabels[1].Rect())
	}
}

func TestDrumViewDrawHighlightsInvisibleCells(t *testing.T) {
	logger := game_log.New(os.Stdout, game_log.LevelDebug)
	graph := model.NewGraph(logger)

	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node1 := graph.AddNode(1, 0, model.NodeTypeInvisible)
	node2 := graph.AddNode(2, 0, model.NodeTypeRegular)
	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node2}] = struct{}{}

	dv := NewDrumView(image.Rect(0, 0, 300, timelineHeight+24), graph, logger)
	dv.Length = 3
	dv.SetBeatLength(3)

	game := &Game{graph: graph, drum: dv, logger: logger}
	game.updateBeatInfos()

	type call struct {
		c color.Color
		r image.Rectangle
	}
	calls := []call{}
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, call{c: c, r: r})
	}
	defer func() { drawRect = orig }()

	highlighted := map[int]int64{1: 1}
	dv.Draw(ebiten.NewImage(300, 50), highlighted, 0, game.beatInfos, 0)

	var highlightCount int
	for _, call := range calls {
		if clr, ok := call.c.(color.RGBA); ok && clr == colHighlight && call.r.Min.Y >= timelineHeight {
			highlightCount++
		}
	}
	if highlightCount != 1 {
		t.Fatalf("expected 1 highlight draw, got %d", highlightCount)
	}
}

func TestDrumViewSetBPMClamp(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(maxBPM + 10)
	if dv.bpm != maxBPM {
		t.Fatalf("expected BPM %d, got %d", maxBPM, dv.bpm)
	}
	if dv.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on high bpm")
	}
	dv.bpmErrorAnim = 0
	dv.SetBPM(0)
	if dv.bpm != 1 {
		t.Fatalf("expected BPM 1, got %d", dv.bpm)
	}
	if dv.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on low bpm")
	}
}

func TestDrumViewBPMTextInput(t *testing.T) {
	t.Skip("text input focus under refactor")
}

func TestVolumeSliderUpdatesRowVolume(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()
	r := dv.rowVolSliders[0].Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.Update()
	if dv.Rows[0].Volume < 0.49 || dv.Rows[0].Volume > 0.51 {
		t.Fatalf("expected volume ~0.5 got %f", dv.Rows[0].Volume)
	}
}

// Dragging a volume slider to its maximum and releasing over the delete button
// should not remove the row.
func TestVolumeDragReleaseDoesNotDeleteRow(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()
	sRect := dv.rowVolSliders[0].Rect()
	delRect := dv.rowDeleteBtns[0].Rect()

	mx, my := sRect.Min.X+1, sRect.Min.Y+sRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	dv.Update()           // press start
	mx = sRect.Max.X + 20 // drag beyond slider to max
	dv.Update()
	mx, my = delRect.Min.X+delRect.Dx()/2, delRect.Min.Y+delRect.Dy()/2
	dv.Update() // still dragging over delete button
	pressed = false
	dv.Update() // release over delete button

	if len(dv.Rows) != 1 {
		t.Fatalf("row deleted during slider drag")
	}
}

func TestMuteSoloButtons(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()

	// Click mute button on first row
	mRect := dv.rowMuteBtns[0].Rect()
	mx, my := mRect.Min.X+1, mRect.Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if !dv.Rows[0].Muted {
		t.Fatalf("expected row muted after clicking mute button")
	}

	// Click solo button on first row
	sRect := dv.rowSoloBtns[0].Rect()
	mx, my = sRect.Min.X+1, sRect.Min.Y+1
	pressed = true
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if !dv.Rows[0].Solo {
		t.Fatalf("expected row solo after clicking solo button")
	}
}

func TestMuteSoloInteractions(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 300, 100), g, testLogger)
	dv.AddRow()
	dv.calcLayout()

	mRect := dv.rowMuteBtns[0].Rect()
	mx, my := mRect.Min.X+1, mRect.Min.Y+1
	dv.rowMuteBtns[0].Handle(mx, my, true)
	dv.rowMuteBtns[0].Handle(mx, my, false)
	if !dv.Rows[0].Muted {
		t.Fatalf("expected row0 muted after single click")
	}
	if dv.Rows[0].Solo {
		t.Fatalf("row0 should not be solo when muted")
	}

	sRect := dv.rowSoloBtns[1].Rect()
	mx, my = sRect.Min.X+1, sRect.Min.Y+1
	dv.rowSoloBtns[1].Handle(mx, my, true)
	dv.rowSoloBtns[1].Handle(mx, my, false)
	if !dv.Rows[1].Solo {
		t.Fatalf("expected row1 solo after click")
	}
	if dv.Rows[1].Muted {
		t.Fatalf("solo row should not be muted")
	}
	if !dv.Rows[0].Muted {
		t.Fatalf("other rows should be muted when a solo is active")
	}
}

func TestTrackBeatCentersCurrent(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.SetLength(8)
	dv.follow = true

	dv.TrackBeat(1)
	if dv.Offset != 0 {
		t.Fatalf("expected offset 0 near start, got %d", dv.Offset)
	}

	dv.TrackBeat(6)
	if dv.Offset != 2 {
		t.Fatalf("offset=%d want 2", dv.Offset)
	}
	if !dv.OffsetChanged() {
		t.Fatalf("expected offset change after tracking")
	}
}
