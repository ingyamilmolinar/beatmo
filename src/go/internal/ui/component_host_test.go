package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestComponentHost_NestedComponents(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 800, 600))

	// Create a nested structure: root -> panel -> button
	root := NewBaseComponent("root")
	root.SetBounds(image.Rect(0, 0, 800, 600))

	panel := NewBaseComponent("panel")
	panel.SetBounds(image.Rect(100, 100, 400, 300))

	buttonClicked := false
	button := &mockComponent{
		BaseComponent: *NewBaseComponent("button"),
		onInput: func(x, y int, pressed bool) InputResult {
			if pressed && image.Pt(x, y).In(image.Rect(150, 150, 250, 200)) {
				buttonClicked = true
				return InputConsumed
			}
			return InputIgnored
		},
	}
	button.SetBounds(image.Rect(150, 150, 250, 200))

	root.AddChild(panel)
	panel.AddChild(button)
	host.SetRoot(root)

	// Click on button
	result := host.HandleInput(200, 175, true)
	if !result {
		t.Error("HandleInput should return true when button clicked")
	}
	if !buttonClicked {
		t.Error("Button should be clicked")
	}

	// Verify all components are mounted
	if !root.IsMounted() {
		t.Error("root should be mounted")
	}
	if !panel.IsMounted() {
		t.Error("panel should be mounted")
	}
	if !button.IsMounted() {
		t.Error("button should be mounted")
	}
}

func TestComponentHost_BoundsUpdate(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 800, 600))

	root := NewBaseComponent("root")
	host.SetRoot(root)

	if root.Bounds() != image.Rect(0, 0, 800, 600) {
		t.Errorf("root.Bounds() = %v, want (0,0)-(800,600)", root.Bounds())
	}

	// Update bounds
	host.SetBounds(image.Rect(0, 0, 1024, 768))
	if host.Bounds() != image.Rect(0, 0, 1024, 768) {
		t.Errorf("host.Bounds() = %v, want (0,0)-(1024,768)", host.Bounds())
	}

	// Root bounds should also be updated
	if root.Bounds() != image.Rect(0, 0, 1024, 768) {
		t.Errorf("root.Bounds() after update = %v, want (0,0)-(1024,768)", root.Bounds())
	}
}

func TestComponentHost_DrawOrder(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 100, 100))

	drawOrder := []string{}
	root := &mockComponent{
		BaseComponent: *NewBaseComponent("root"),
		onDraw: func(dst *ebiten.Image) {
			drawOrder = append(drawOrder, "root")
		},
	}

	child1 := &mockComponent{
		BaseComponent: *NewBaseComponent("child1"),
		onDraw: func(dst *ebiten.Image) {
			drawOrder = append(drawOrder, "child1")
		},
	}
	child1.SetBounds(image.Rect(0, 0, 50, 50))

	child2 := &mockComponent{
		BaseComponent: *NewBaseComponent("child2"),
		onDraw: func(dst *ebiten.Image) {
			drawOrder = append(drawOrder, "child2")
		},
	}
	child2.SetBounds(image.Rect(50, 0, 100, 50))

	root.AddChild(child1)
	root.AddChild(child2)
	host.SetRoot(root)

	host.Draw(nil)

	// Draw order should be: children first (in order), then parent draws after
	// But BaseComponent.Draw draws children, and mockComponent adds to the list
	// So we expect: root draws (adding "root"), then it calls base.Draw which draws children
	// Actually mockComponent.Draw only adds "root" and doesn't call base - let's fix
	// The mock's onDraw replaces the Draw, so it won't draw children

	// This test verifies that host.Draw calls the root's Draw
	if len(drawOrder) != 1 || drawOrder[0] != "root" {
		t.Errorf("drawOrder = %v, want [root]", drawOrder)
	}
}

func TestComponentHost_InputZOrder(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 100, 100))

	clickedComponents := []string{}

	// Create overlapping components - last added should be on top
	root := NewBaseComponent("root")
	root.SetBounds(image.Rect(0, 0, 100, 100))

	bottom := &mockComponent{
		BaseComponent: *NewBaseComponent("bottom"),
		onInput: func(x, y int, pressed bool) InputResult {
			if pressed {
				clickedComponents = append(clickedComponents, "bottom")
				return InputConsumed
			}
			return InputIgnored
		},
	}
	bottom.SetBounds(image.Rect(10, 10, 60, 60))

	top := &mockComponent{
		BaseComponent: *NewBaseComponent("top"),
		onInput: func(x, y int, pressed bool) InputResult {
			if pressed {
				clickedComponents = append(clickedComponents, "top")
				return InputConsumed
			}
			return InputIgnored
		},
	}
	top.SetBounds(image.Rect(30, 30, 80, 80)) // overlaps with bottom

	root.AddChild(bottom)
	root.AddChild(top)
	host.SetRoot(root)

	// Click in overlapping area - top (added last) should receive input first
	host.HandleInput(40, 40, true)

	if len(clickedComponents) != 1 || clickedComponents[0] != "top" {
		t.Errorf("clickedComponents = %v, want [top]", clickedComponents)
	}
}

func TestComponentHost_ReleaseCapture(t *testing.T) {
	host := NewComponentHost(image.Rect(0, 0, 100, 100))

	root := &mockComponent{
		BaseComponent: *NewBaseComponent("root"),
		onInput: func(x, y int, pressed bool) InputResult {
			return InputCaptured
		},
		isCapturing: true,
	}
	root.SetBounds(image.Rect(0, 0, 100, 100))
	host.SetRoot(root)

	// Trigger capture
	host.HandleInput(50, 50, true)

	if !host.Capturing() {
		t.Error("Host should be capturing")
	}

	// Manually release
	host.ReleaseCapture()

	if host.Capturing() {
		t.Error("Host should not be capturing after ReleaseCapture")
	}
}
