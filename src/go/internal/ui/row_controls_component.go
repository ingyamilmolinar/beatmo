package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// RowControlsProps contains the external state passed to a row controls component.
type RowControlsProps struct {
	RowIndex    int
	Name        string
	Instrument  string
	Color       color.Color
	Volume      float64
	Muted       bool
	Solo        bool
	IsAvailable bool // Whether the instrument is available

	// Callbacks
	OnLabelClick   func(row int)
	OnEditClick    func(row int)
	OnSaveClick    func(row int)
	OnColorClick   func(row int)
	OnVolumeChange func(row int, vol float64)
	OnMuteToggle   func(row int)
	OnSoloToggle   func(row int)
	OnOriginClick  func(row int)
	OnDeleteClick  func(row int)

	// Whether delete is enabled (disabled when only one row exists)
	DeleteEnabled bool
}

// RowControlsState contains internal UI state for a row controls component.
type RowControlsState struct {
	sliderDragging bool
}

// RowControlsComponent manages the controls for a single drum row.
type RowControlsComponent struct {
	BaseComponent
	props RowControlsProps
	state RowControlsState

	// Child widgets
	labelBtn  *Button
	editBtn   *Button
	saveBtn   *Button
	colorBtn  *Button
	volSlider *Slider
	muteBtn   *Button
	soloBtn   *Button
	originBtn *Button
	deleteBtn *Button

	// Layout
	layoutDirty bool
}

// NewRowControlsComponent creates a new row controls component.
func NewRowControlsComponent(id string, rowIndex int) *RowControlsComponent {
	rc := &RowControlsComponent{
		BaseComponent: *NewBaseComponent(id),
		layoutDirty:   true,
	}
	rc.props.RowIndex = rowIndex
	rc.initWidgets()
	return rc
}

func (rc *RowControlsComponent) initWidgets() {
	idx := rc.props.RowIndex

	rc.labelBtn = NewButton("", InstButtonStyle, func() {
		if rc.props.OnLabelClick != nil {
			rc.props.OnLabelClick(rc.props.RowIndex)
		}
	})

	rc.editBtn = NewButton("", InstButtonStyle, func() {
		if rc.props.OnEditClick != nil {
			rc.props.OnEditClick(rc.props.RowIndex)
		}
	})
	rc.editBtn.Icon = "pencil"

	rc.saveBtn = NewButton("", InstButtonStyle, func() {
		if rc.props.OnSaveClick != nil {
			rc.props.OnSaveClick(rc.props.RowIndex)
		}
	})
	rc.saveBtn.Icon = "save"

	// Color swatch with dynamic color
	colorFn := func() color.Color {
		return rc.props.Color
	}
	rc.colorBtn = NewButton("", ColorSwatchStyle{Color: colorFn, Border: colButtonBorder}, func() {
		if rc.props.OnColorClick != nil {
			rc.props.OnColorClick(rc.props.RowIndex)
		}
	})

	rc.volSlider = NewSlider(1.0)

	rc.muteBtn = NewButton("M", InstButtonStyle, func() {
		if rc.props.OnMuteToggle != nil {
			rc.props.OnMuteToggle(rc.props.RowIndex)
		}
	})

	rc.soloBtn = NewButton("S", InstButtonStyle, func() {
		if rc.props.OnSoloToggle != nil {
			rc.props.OnSoloToggle(rc.props.RowIndex)
		}
	})

	rc.originBtn = NewButton("O", InstButtonStyle, func() {
		if rc.props.OnOriginClick != nil {
			rc.props.OnOriginClick(rc.props.RowIndex)
		}
	})

	rc.deleteBtn = NewButton("X", InstButtonStyle, func() {
		if rc.props.OnDeleteClick != nil && rc.props.DeleteEnabled {
			rc.props.OnDeleteClick(rc.props.RowIndex)
		}
	})
	rc.deleteBtn.ConsumeOnPress = true

	// Suppress "declared and not used" for idx (it's used in closure capture above)
	_ = idx
}

// SetProps updates the external props.
func (rc *RowControlsComponent) SetProps(p RowControlsProps) {
	// Update label button text
	if p.Name != rc.props.Name {
		rc.labelBtn.Text = p.Name
	}

	// Update label button style based on availability
	if p.IsAvailable != rc.props.IsAvailable {
		if p.IsAvailable {
			rc.labelBtn.Style = InstButtonStyle
		} else {
			rc.labelBtn.Style = MissingInstStyle
		}
	}

	// Update volume slider
	if p.Volume != rc.props.Volume {
		rc.volSlider.Value = p.Volume
	}

	// Update delete button style
	if p.DeleteEnabled != rc.props.DeleteEnabled {
		if p.DeleteEnabled {
			rc.deleteBtn.Style = InstButtonStyle
		} else {
			rc.deleteBtn.Style = DisabledButtonStyle
		}
	}

	rc.props = p
}

// Props returns the current props.
func (rc *RowControlsComponent) Props() RowControlsProps { return rc.props }

// Mount is called when the component is added to a parent.
func (rc *RowControlsComponent) Mount(ctx MountContext) {
	rc.BaseComponent.Mount(ctx)
	rc.layoutDirty = true
}

// SetBounds updates bounds and marks layout dirty.
func (rc *RowControlsComponent) SetBounds(r image.Rectangle) {
	if r != rc.bounds {
		rc.BaseComponent.SetBounds(r)
		rc.layoutDirty = true
	}
}

// HandleInput processes mouse input for row controls.
func (rc *RowControlsComponent) HandleInput(x, y int, pressed bool) InputResult {
	// Check if slider is capturing
	if rc.state.sliderDragging {
		if rc.volSlider.Handle(x, y, pressed) {
			if rc.props.OnVolumeChange != nil {
				rc.props.OnVolumeChange(rc.props.RowIndex, rc.volSlider.Value)
			}
			if !pressed {
				rc.state.sliderDragging = false
				rc.SetCapturing(false)
			}
			return InputCaptured
		}
		rc.state.sliderDragging = false
		rc.SetCapturing(false)
	}

	pt := image.Pt(x, y)
	if !pt.In(rc.bounds) {
		return InputIgnored
	}

	// Handle buttons
	buttons := []*Button{
		rc.labelBtn, rc.editBtn, rc.saveBtn, rc.colorBtn,
		rc.muteBtn, rc.soloBtn, rc.originBtn, rc.deleteBtn,
	}
	for _, btn := range buttons {
		if btn != nil && btn.Handle(x, y, pressed) {
			return InputConsumed
		}
	}

	// Handle volume slider
	if rc.volSlider.Handle(x, y, pressed) {
		if rc.volSlider.dragging {
			rc.state.sliderDragging = true
			rc.SetCapturing(true)
		}
		if rc.props.OnVolumeChange != nil {
			rc.props.OnVolumeChange(rc.props.RowIndex, rc.volSlider.Value)
		}
		return InputCaptured
	}

	return InputIgnored
}

// Draw renders the row controls.
func (rc *RowControlsComponent) Draw(dst *ebiten.Image) {
	if rc.layoutDirty {
		rc.recalcLayout()
		rc.layoutDirty = false
	}

	rc.labelBtn.Draw(dst)
	rc.editBtn.Draw(dst)
	rc.saveBtn.Draw(dst)
	rc.colorBtn.Draw(dst)
	rc.volSlider.Draw(dst)
	rc.muteBtn.Draw(dst)
	rc.soloBtn.Draw(dst)
	rc.originBtn.Draw(dst)
	rc.deleteBtn.Draw(dst)
}

func (rc *RowControlsComponent) recalcLayout() {
	bounds := rc.bounds
	if bounds.Empty() {
		return
	}

	// Grid: [label][edit][save][color][slider][M][S][O][X]
	// Note: edit and save share a column (split horizontally)
	g := NewGridLayout(bounds, []float64{6, 2, 2, 8, 2, 2, 2, 2}, []float64{1})

	rc.labelBtn.SetRect(insetRect(g.Cell(0, 0), buttonPad))

	// Edit and save buttons share column 1, split horizontally
	editCell := g.Cell(1, 0)
	editRect, saveRect := splitRectHoriz(editCell)
	splitPad := buttonPad
	if splitPad > 1 {
		splitPad--
	}
	rc.editBtn.SetRect(insetRectSafe(editRect, splitPad))
	rc.saveBtn.SetRect(insetRectSafe(saveRect, splitPad))

	rc.colorBtn.SetRect(insetRect(g.Cell(2, 0), buttonPad))
	rc.volSlider.SetRect(insetRect(g.Cell(3, 0), buttonPad))
	rc.muteBtn.SetRect(insetRect(g.Cell(4, 0), buttonPad))
	rc.soloBtn.SetRect(insetRect(g.Cell(5, 0), buttonPad))
	rc.originBtn.SetRect(insetRect(g.Cell(6, 0), buttonPad))
	rc.deleteBtn.SetRect(insetRect(g.Cell(7, 0), buttonPad))
}

// LabelButton returns the label button for external positioning/access.
func (rc *RowControlsComponent) LabelButton() *Button { return rc.labelBtn }

// ColorButton returns the color button for external positioning/access.
func (rc *RowControlsComponent) ColorButton() *Button { return rc.colorBtn }

// VolumeSlider returns the volume slider for external access.
func (rc *RowControlsComponent) VolumeSlider() *Slider { return rc.volSlider }

// MarkLayoutDirty forces a layout recalculation on the next Draw.
func (rc *RowControlsComponent) MarkLayoutDirty() {
	rc.layoutDirty = true
}
