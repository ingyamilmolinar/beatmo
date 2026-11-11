package ui

import (
	"image"
	"testing"
)

func TestLayoutGroupBounds(t *testing.T) {
	g := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1, 1}, []float64{1})
	b := g.Bounds()
	if b.Dx() != 400 || b.Dy() != 200 {
		t.Fatalf("expected 400x200 bounds, got %v", b)
	}
}

func TestLayoutGroupCell(t *testing.T) {
	g := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1, 1}, []float64{1})
	c0 := g.Cell(0, 0)
	c1 := g.Cell(1, 0)
	if c0.Min.X != 0 || c0.Max.X != 200 {
		t.Fatalf("expected col0 [0,200], got %v", c0)
	}
	if c1.Min.X != 200 || c1.Max.X != 400 {
		t.Fatalf("expected col1 [200,400], got %v", c1)
	}
}

func TestLayoutGroupAddChildAndFind(t *testing.T) {
	root := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1}, []float64{1, 1})
	child := root.AddChild("top", 0, 0, []float64{1, 1}, []float64{1})
	grandchild := child.AddChild("left", 0, 0, []float64{1}, []float64{1})

	found := root.Find("top")
	if found != child {
		t.Fatal("Find('top') returned wrong group")
	}
	found = root.Find("left")
	if found != grandchild {
		t.Fatal("Find('left') returned wrong group")
	}
	found = root.Find("missing")
	if found != nil {
		t.Fatal("Find('missing') should return nil")
	}
}

func TestLayoutGroupSetBoundsCascade(t *testing.T) {
	root := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1}, []float64{1, 1})
	child := root.AddChild("top", 0, 0, []float64{1, 1}, []float64{1})

	// child's bounds should match root's cell(0,0) = top half
	b := child.Bounds()
	if b.Dy() != 100 {
		t.Fatalf("expected child height 100, got %d", b.Dy())
	}

	// Resize root — child should cascade.
	root.SetBounds(image.Rect(0, 0, 400, 400))
	b = child.Bounds()
	if b.Dy() != 200 {
		t.Fatalf("after resize, expected child height 200, got %d", b.Dy())
	}
}

func TestLayoutGroupSetVisibleHidesChildren(t *testing.T) {
	root := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1}, []float64{1})
	child := root.AddChild("c", 0, 0, []float64{1}, []float64{1})

	root.SetVisible(false)
	if !root.Bounds().Empty() {
		t.Fatal("hidden root should return empty bounds")
	}
	if !child.Bounds().Empty() {
		t.Fatal("child of hidden root should return empty bounds")
	}
	if !child.Cell(0, 0).Empty() {
		t.Fatal("Cell on hidden group should return empty rect")
	}

	root.SetVisible(true)
	if root.Bounds().Empty() {
		t.Fatal("visible root should return non-empty bounds")
	}
}

func TestLayoutGroupRelayout(t *testing.T) {
	root := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1}, []float64{1, 1})
	child := root.AddChild("top", 0, 0, []float64{1}, []float64{1})

	// Manually change root bounds then relayout child.
	root.SetBounds(image.Rect(10, 10, 410, 210))
	child.Relayout()
	b := child.Bounds()
	if b.Min.X != 10 || b.Min.Y != 10 {
		t.Fatalf("after relayout, expected child at (10,10), got %v", b.Min)
	}
}

func TestLayoutGroupMovingParentMovesChildren(t *testing.T) {
	root := NewLayoutGroup("root", image.Rect(0, 0, 400, 200), []float64{1, 1}, []float64{1})
	child := root.AddChild("left", 0, 0, []float64{1}, []float64{1})

	origChild := child.Bounds()
	if origChild.Min.X != 0 {
		t.Fatalf("expected child at x=0, got %d", origChild.Min.X)
	}

	// Move root to the right by changing its bounds.
	root.SetBounds(image.Rect(100, 0, 500, 200))
	newChild := child.Bounds()
	if newChild.Min.X != 100 {
		t.Fatalf("after move, expected child at x=100, got %d", newChild.Min.X)
	}
}
