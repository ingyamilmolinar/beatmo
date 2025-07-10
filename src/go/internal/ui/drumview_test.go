package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

func setupDV() *DrumView {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 100), g, testLogger)
	dv.Rows = []*DrumRow{{Name: "H", Steps: make([]bool, 4)}}
	dv.recalcButtons()
	return dv
}

func TestPlayStopButtons(t *testing.T) {
	dv := setupDV()
	mx := dv.playBtn.Min.X + 1
	my := dv.playBtn.Min.Y + 1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()
	if !dv.playing {
		t.Fatal("expected playing after clicking play")
	}

	mx = dv.stopBtn.Min.X + 1
	my = dv.stopBtn.Min.Y + 1
	dv.Update()
	if dv.playing {
		t.Fatal("expected stopped after clicking stop")
	}
}

func TestRowHeightFillsPane(t *testing.T) {
	dv := setupDV()
	dv.Update()
	want := dv.Bounds.Dy() / len(dv.Rows)
	if dv.rowHeight() != want {
		t.Fatalf("expected row height %d, got %d", want, dv.rowHeight())
	}
}

func TestRowHeightSplit(t *testing.T) {
	dv := setupDV()
	dv.Update()
	h := dv.rowHeight()
	expected := dv.Bounds.Dy() / len(dv.Rows)
	if h != expected {
		t.Fatalf("expected row height %d, got %d", expected, h)
	}
}

func TestDrawAfterInit(t *testing.T) {
	dv := setupDV()
	dv.Update()
	img := ebiten.NewImage(200, 100)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Draw panicked: %v", r)
		}
	}()
	dv.Draw(img)
}

func TestSetBoundsRebuilds(t *testing.T) {
	dv := setupDV()
	dv.Update()
	h1 := dv.rowHeight()
	dv.SetBounds(image.Rect(0, 0, 200, 150))
	dv.Update()
	h2 := dv.rowHeight()
	if h2 <= h1 {
		t.Fatalf("expected height to increase from %d to %d", h1, h2)
	}
}

func TestBackgroundWidthMatchesBounds(t *testing.T) {
	dv := setupDV()
	dv.Update()
	for idx, img := range dv.bgCache {
		if img.Bounds().Dx() != dv.Bounds.Dx() {
			t.Fatalf("row %d width=%d want %d", idx, img.Bounds().Dx(), dv.Bounds.Dx())
		}
	}
	// No longer have resizeSteps, so this part of the test is removed.
}

func TestRowHeightUnchangedAfterNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	h1 := g.drum.rowHeight()
	g.tryAddNode(0, 0)
	g.Update()
	if g.drum.rowHeight() != h1 {
		t.Fatalf("row height changed from %d to %d", h1, g.drum.rowHeight())
	}
}

func TestDrumViewSync_InitialState(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected empty beat row, got %v", g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_StartNodeOnly(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	_ = g.tryAddNode(0, 0) // This becomes the start node by default
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}

	// Change start node
	n1 := g.tryAddNode(1, 0)
	g.sel = n1
	g.start = n1
	g.graph.StartNodeID = n1.ID
	g.Update()

	expected = make([]bool, g.graph.BeatLength())
	expected[0] = true // Start node is at distance 0
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_ConnectedNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	n0 := g.tryAddNode(0, 0) // Start node
	n1 := g.tryAddNode(1, 0)
	n2 := g.tryAddNode(2, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	expected[2] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_DisconnectedNodes(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	_ = g.tryAddNode(0, 0) // Start node
	_ = g.tryAddNode(10, 10) // Disconnected node
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_GraphGrowth(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	n0 := g.tryAddNode(0, 0) // Start node
	n1 := g.tryAddNode(1, 0)
	g.addEdge(n0, n1)
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}

	n2 := g.tryAddNode(2, 0)
	g.addEdge(n1, n2)
	g.Update()

	expected = make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	expected[2] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_GraphShrinkage(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	n0 := g.tryAddNode(0, 0) // Start node
	n1 := g.tryAddNode(1, 0)
	n2 := g.tryAddNode(2, 0)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	expected[2] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}

	g.deleteNode(n2)
	g.Update()

	expected = make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func TestDrumViewSync_MultipleNodesSameI(t *testing.T) {
	g := New(testLogger)
	g.Layout(200, 120)
	n0 := g.tryAddNode(0, 0) // Start node
	n1a := g.tryAddNode(1, 0)
	n1b := g.tryAddNode(1, 1) // Another node at I=1
	g.addEdge(n0, n1a)
	g.addEdge(n0, n1b) // Connect both to start
	g.Update()

	expected := make([]bool, g.graph.BeatLength())
	expected[0] = true
	expected[1] = true
	if !compareBeatRows(g.drum.Rows[0].Steps, expected) {
		t.Fatalf("Expected beat row %v, got %v", expected, g.drum.Rows[0].Steps)
	}
}

func compareBeatRows(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}


