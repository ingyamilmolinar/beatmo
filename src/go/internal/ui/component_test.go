package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestBaseComponent_Lifecycle(t *testing.T) {
	bc := NewBaseComponent("test-component")

	if bc.ID() != "test-component" {
		t.Errorf("ID() = %q, want %q", bc.ID(), "test-component")
	}

	if bc.IsMounted() {
		t.Error("IsMounted() should be false before Mount()")
	}

	bounds := image.Rect(10, 20, 100, 80)
	bc.Mount(MountContext{Bounds: bounds})

	if !bc.IsMounted() {
		t.Error("IsMounted() should be true after Mount()")
	}

	if bc.Bounds() != bounds {
		t.Errorf("Bounds() = %v, want %v", bc.Bounds(), bounds)
	}

	bc.Unmount()
	if bc.IsMounted() {
		t.Error("IsMounted() should be false after Unmount()")
	}
}

func TestBaseComponent_Children(t *testing.T) {
	parent := NewBaseComponent("parent")
	parent.Mount(MountContext{Bounds: image.Rect(0, 0, 200, 200)})

	child1 := NewBaseComponent("child1")
	child1.SetBounds(image.Rect(10, 10, 50, 50))
	child2 := NewBaseComponent("child2")
	child2.SetBounds(image.Rect(60, 10, 100, 50))

	parent.AddChild(child1)
	parent.AddChild(child2)

	if len(parent.Children()) != 2 {
		t.Errorf("Children() len = %d, want 2", len(parent.Children()))
	}

	if !child1.IsMounted() {
		t.Error("child1 should be mounted after AddChild")
	}
	if !child2.IsMounted() {
		t.Error("child2 should be mounted after AddChild")
	}

	// Test FindChild
	found := parent.FindChild("child1")
	if found == nil || found.ID() != "child1" {
		t.Error("FindChild(child1) failed")
	}

	// Test ChildAt
	childAt := parent.ChildAt(30, 30)
	if childAt == nil || childAt.ID() != "child1" {
		t.Error("ChildAt(30, 30) should return child1")
	}

	childAt = parent.ChildAt(80, 30)
	if childAt == nil || childAt.ID() != "child2" {
		t.Error("ChildAt(80, 30) should return child2")
	}

	// Test RemoveChild
	parent.RemoveChild("child1")
	if len(parent.Children()) != 1 {
		t.Errorf("Children() len after remove = %d, want 1", len(parent.Children()))
	}
	if child1.IsMounted() {
		t.Error("child1 should be unmounted after RemoveChild")
	}

	// Test ClearChildren
	parent.ClearChildren()
	if len(parent.Children()) != 0 {
		t.Errorf("Children() len after clear = %d, want 0", len(parent.Children()))
	}
	if child2.IsMounted() {
		t.Error("child2 should be unmounted after ClearChildren")
	}
}

func TestBaseComponent_HandleInput(t *testing.T) {
	parent := NewBaseComponent("parent")
	parent.SetBounds(image.Rect(0, 0, 200, 200))

	clickedID := ""
	child := &mockComponent{
		BaseComponent: *NewBaseComponent("child"),
		onInput: func(x, y int, pressed bool) InputResult {
			if pressed {
				clickedID = "child"
				return InputConsumed
			}
			return InputIgnored
		},
	}
	child.SetBounds(image.Rect(10, 10, 50, 50))

	parent.AddChild(child)
	parent.Mount(MountContext{Bounds: parent.Bounds()})

	// Click inside child
	result := parent.HandleInput(30, 30, true)
	if result != InputConsumed {
		t.Errorf("HandleInput inside child = %v, want InputConsumed", result)
	}
	if clickedID != "child" {
		t.Error("Child should have received click")
	}

	// Click outside child but inside parent
	clickedID = ""
	result = parent.HandleInput(100, 100, true)
	if result != InputIgnored {
		t.Errorf("HandleInput outside child = %v, want InputIgnored", result)
	}
}

func TestComponentHost(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 800, 600))

	root := NewBaseComponent("root")
	host.SetRoot(root)

	if host.Root() != root {
		t.Error("Root() should return the set root")
	}

	if !root.IsMounted() {
		t.Error("Root should be mounted after SetRoot")
	}

	// Test FindComponent
	child := NewBaseComponent("child")
	child.SetBounds(image.Rect(10, 10, 100, 100))
	root.AddChild(child)

	found := host.FindComponent("child")
	if found == nil || found.ID() != "child" {
		t.Error("FindComponent should find nested child")
	}

	// Test WalkComponents
	visited := []string{}
	host.WalkComponents(func(c Component) {
		visited = append(visited, c.ID())
	})
	if len(visited) != 2 {
		t.Errorf("WalkComponents visited %d, want 2", len(visited))
	}

	// Test SetRoot replaces and unmounts old root
	oldRoot := root
	newRoot := NewBaseComponent("new-root")
	host.SetRoot(newRoot)

	if oldRoot.IsMounted() {
		t.Error("Old root should be unmounted after SetRoot")
	}
	if !newRoot.IsMounted() {
		t.Error("New root should be mounted after SetRoot")
	}
}

func TestComponentHost_HandleInput_Capture(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 800, 600))

	captureCount := 0
	root := &mockComponent{
		BaseComponent: *NewBaseComponent("root"),
		onInput: func(x, y int, pressed bool) InputResult {
			captureCount++
			return InputCaptured
		},
		isCapturing: true,
	}
	root.SetBounds(image.Rect(0, 0, 800, 600))

	host.SetRoot(root)

	// First click captures
	result := host.HandleInput(100, 100, true)
	if result != true {
		t.Error("HandleInput should return true when captured")
	}

	// Subsequent input goes to captured component
	host.HandleInput(200, 200, true)
	if captureCount < 2 {
		t.Error("Captured component should receive all input")
	}

	// Release capture
	root.isCapturing = false
	host.HandleInput(100, 100, false)
	if host.Capturing() {
		t.Error("Host should not be capturing after component releases")
	}
}

// mockComponent is a test helper that wraps BaseComponent with custom handlers.
type mockComponent struct {
	BaseComponent
	onInput     func(x, y int, pressed bool) InputResult
	onDraw      func(dst *ebiten.Image)
	isCapturing bool
}

func (m *mockComponent) HandleInput(x, y int, pressed bool) InputResult {
	if m.onInput != nil {
		return m.onInput(x, y, pressed)
	}
	return m.BaseComponent.HandleInput(x, y, pressed)
}

func (m *mockComponent) Capturing() bool {
	return m.isCapturing
}

func (m *mockComponent) Draw(dst *ebiten.Image) {
	if m.onDraw != nil {
		m.onDraw(dst)
	}
}
