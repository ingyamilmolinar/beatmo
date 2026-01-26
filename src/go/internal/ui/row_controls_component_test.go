package ui

import (
	"image"
	"image/color"
	"testing"
)

func TestRowControlsComponent_Creation(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)

	if rc.ID() != "row-0" {
		t.Errorf("ID() = %q, want %q", rc.ID(), "row-0")
	}

	if rc.props.RowIndex != 0 {
		t.Errorf("RowIndex = %d, want 0", rc.props.RowIndex)
	}

	// Verify all widgets are initialized
	if rc.labelBtn == nil {
		t.Error("labelBtn should be initialized")
	}
	if rc.volSlider == nil {
		t.Error("volSlider should be initialized")
	}
	if rc.muteBtn == nil {
		t.Error("muteBtn should be initialized")
	}
	if rc.deleteBtn == nil {
		t.Error("deleteBtn should be initialized")
	}
}

func TestRowControlsComponent_SetProps(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)

	testColor := color.RGBA{255, 0, 0, 255}
	props := RowControlsProps{
		RowIndex:      0,
		Name:          "Kick",
		Instrument:    "kick",
		Color:         testColor,
		Volume:        0.75,
		Muted:         true,
		Solo:          false,
		IsAvailable:   true,
		DeleteEnabled: true,
	}
	rc.SetProps(props)

	got := rc.Props()
	if got.Name != "Kick" {
		t.Errorf("Props().Name = %q, want %q", got.Name, "Kick")
	}
	if got.Volume != 0.75 {
		t.Errorf("Props().Volume = %f, want 0.75", got.Volume)
	}
	if !got.Muted {
		t.Error("Props().Muted should be true")
	}

	// Label button text should be updated
	if rc.labelBtn.Text != "Kick" {
		t.Errorf("labelBtn.Text = %q, want %q", rc.labelBtn.Text, "Kick")
	}

	// Volume slider should be updated
	if rc.volSlider.Value != 0.75 {
		t.Errorf("volSlider.Value = %f, want 0.75", rc.volSlider.Value)
	}
}

func TestRowControlsComponent_Callbacks(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)
	rc.SetBounds(image.Rect(0, 0, 400, 40))
	rc.Mount(MountContext{Bounds: rc.Bounds()})

	labelClicked := false
	muteClicked := false
	deleteClicked := false

	rc.SetProps(RowControlsProps{
		RowIndex:      0,
		DeleteEnabled: true,
		OnLabelClick: func(row int) {
			labelClicked = true
			if row != 0 {
				t.Errorf("OnLabelClick row = %d, want 0", row)
			}
		},
		OnMuteToggle: func(row int) {
			muteClicked = true
		},
		OnDeleteClick: func(row int) {
			deleteClicked = true
		},
	})

	// Force layout
	rc.recalcLayout()

	// Simulate label click
	rc.labelBtn.OnClick()
	if !labelClicked {
		t.Error("OnLabelClick callback should be called")
	}

	// Simulate mute click
	rc.muteBtn.OnClick()
	if !muteClicked {
		t.Error("OnMuteToggle callback should be called")
	}

	// Simulate delete click
	rc.deleteBtn.OnClick()
	if !deleteClicked {
		t.Error("OnDeleteClick callback should be called")
	}
}

func TestRowControlsComponent_DeleteDisabled(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)

	deleteClicked := false
	rc.SetProps(RowControlsProps{
		RowIndex:      0,
		DeleteEnabled: false,
		OnDeleteClick: func(row int) {
			deleteClicked = true
		},
	})

	// Delete should not be called when disabled
	rc.deleteBtn.OnClick()
	if deleteClicked {
		t.Error("OnDeleteClick should not be called when DeleteEnabled is false")
	}
}

func TestRowControlsComponent_Layout(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)
	bounds := image.Rect(0, 0, 400, 40)
	rc.SetBounds(bounds)
	rc.Mount(MountContext{Bounds: bounds})

	// Force layout calculation
	rc.recalcLayout()

	// Verify buttons have non-empty rects
	if rc.labelBtn.Rect().Empty() {
		t.Error("labelBtn should have non-empty rect after layout")
	}
	if rc.muteBtn.Rect().Empty() {
		t.Error("muteBtn should have non-empty rect after layout")
	}
	if rc.deleteBtn.Rect().Empty() {
		t.Error("deleteBtn should have non-empty rect after layout")
	}

	// Verify buttons are within bounds
	if !rc.labelBtn.Rect().In(bounds) {
		t.Error("labelBtn should be within bounds")
	}
	if !rc.deleteBtn.Rect().In(bounds) {
		t.Error("deleteBtn should be within bounds")
	}
}

func TestRowControlsComponent_StyleUpdates(t *testing.T) {
	rc := NewRowControlsComponent("row-0", 0)

	// Start with instrument available
	rc.SetProps(RowControlsProps{
		RowIndex:    0,
		IsAvailable: true,
	})

	// labelBtn should use InstButtonStyle
	if rc.labelBtn.Style != InstButtonStyle {
		t.Error("labelBtn should use InstButtonStyle when available")
	}

	// Make instrument unavailable
	rc.SetProps(RowControlsProps{
		RowIndex:    0,
		IsAvailable: false,
	})

	// labelBtn should use MissingInstStyle
	if rc.labelBtn.Style != MissingInstStyle {
		t.Error("labelBtn should use MissingInstStyle when unavailable")
	}
}
