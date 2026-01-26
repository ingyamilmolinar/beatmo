package ui

import (
	"image"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

// TransportProps contains the external state passed to the transport component.
type TransportProps struct {
	BPM       int
	IsPlaying bool
	Follow    bool
	Subdiv    int
	Length    int

	// Callbacks
	OnPlay         func()
	OnStop         func()
	OnBPMChange    func(delta int) // delta is +1 or -1 for increment/decrement
	OnBPMCommit    func(bpm int)   // Called when BPM box value is committed
	OnSubdivToggle func()          // Opens/closes subdiv dropdown
	OnLengthChange func(delta int) // delta is +1 or -1 for increment/decrement
	OnFollowToggle func()
	OnUpload       func()
	OnImport       func()
	OnExport       func()
	OnVolumeChange func(vol float64)

	// Current main volume (0-1)
	MainVolume float64
}

// TransportState contains the internal UI state owned by the transport component.
type TransportState struct {
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64

	bpmPrev  int // previous BPM before editing
	bpmDelta int // accumulated BPM adjustments from +/- buttons
}

// TransportComponent manages the transport controls (play, stop, BPM, etc.).
type TransportComponent struct {
	BaseComponent
	props TransportProps
	state TransportState

	// Child widgets
	playBtn       *Button
	stopBtn       *Button
	bpmDecBtn     *Button
	bpmBox        *TextInput
	bpmIncBtn     *Button
	subdivBtn     *Button
	lenDecBtn     *Button
	lenIncBtn     *Button
	trackBtn      *Button
	uploadBtn     *Button
	importBtn     *Button
	exportBtn     *Button
	mainVolSlider *Slider
	mainVolRect   image.Rectangle

	// Layout cache
	layoutDirty bool
}

// NewTransportComponent creates a new transport component.
func NewTransportComponent(id string) *TransportComponent {
	t := &TransportComponent{
		BaseComponent: *NewBaseComponent(id),
		layoutDirty:   true,
	}
	t.initWidgets()
	return t
}

func (t *TransportComponent) initWidgets() {
	t.playBtn = NewButton("", PlayButtonStyle, func() {
		t.state.playAnim = 1
		if t.props.OnPlay != nil {
			t.props.OnPlay()
		}
	})
	t.playBtn.Icon = "play"

	t.stopBtn = NewButton("", StopButtonStyle, func() {
		t.state.stopAnim = 1
		if t.props.OnStop != nil {
			t.props.OnStop()
		}
	})
	t.stopBtn.Icon = "stop"

	t.bpmDecBtn = NewButton("-", BPMDecStyle, func() {
		t.state.bpmDecAnim = 1
		t.state.bpmDelta--
		if t.props.OnBPMChange != nil {
			t.props.OnBPMChange(-1)
		}
	})
	t.bpmDecBtn.Repeat = true

	t.bpmBox = NewTextInput(image.Rectangle{}, BPMBoxStyle)
	t.bpmBox.MaxLen = 4
	t.bpmBox.SetText("120")

	t.bpmIncBtn = NewButton("+", BPMIncStyle, func() {
		t.state.bpmIncAnim = 1
		t.state.bpmDelta++
		if t.props.OnBPMChange != nil {
			t.props.OnBPMChange(1)
		}
	})
	t.bpmIncBtn.Repeat = true

	t.subdivBtn = NewButton("32", InstButtonStyle, func() {
		if t.props.OnSubdivToggle != nil {
			t.props.OnSubdivToggle()
		}
	})

	t.lenDecBtn = NewButton("-", LenDecStyle, func() {
		t.state.lenDecAnim = 1
		if t.props.OnLengthChange != nil {
			t.props.OnLengthChange(-1)
		}
	})
	t.lenDecBtn.Repeat = true

	t.lenIncBtn = NewButton("+", LenIncStyle, func() {
		t.state.lenIncAnim = 1
		if t.props.OnLengthChange != nil {
			t.props.OnLengthChange(1)
		}
	})
	t.lenIncBtn.Repeat = true

	t.trackBtn = NewButton("Track", InstButtonStyle, func() {
		if t.props.OnFollowToggle != nil {
			t.props.OnFollowToggle()
		}
	})

	t.uploadBtn = NewButton("Upload", UploadBtnStyle, func() {
		t.state.uploadAnim = 1
		if t.props.OnUpload != nil {
			t.props.OnUpload()
		}
	})

	t.importBtn = NewButton("Import", UploadBtnStyle, func() {
		if t.props.OnImport != nil {
			t.props.OnImport()
		}
	})

	t.exportBtn = NewButton("Export", UploadBtnStyle, func() {
		if t.props.OnExport != nil {
			t.props.OnExport()
		}
	})

	t.mainVolSlider = NewSlider(1.0)
}

// SetProps updates the external props and marks layout as needing update.
func (t *TransportComponent) SetProps(p TransportProps) {
	if p.BPM != t.props.BPM {
		t.bpmBox.SetText(strconv.Itoa(p.BPM))
	}
	if p.MainVolume != t.props.MainVolume {
		t.mainVolSlider.Value = p.MainVolume
	}
	t.props = p
}

// Props returns the current props.
func (t *TransportComponent) Props() TransportProps { return t.props }

// State returns the current internal state.
func (t *TransportComponent) State() TransportState { return t.state }

// Mount is called when the component is added to a parent.
func (t *TransportComponent) Mount(ctx MountContext) {
	t.BaseComponent.Mount(ctx)
	t.layoutDirty = true
}

// SetBounds updates bounds and marks layout dirty.
func (t *TransportComponent) SetBounds(r image.Rectangle) {
	if r != t.bounds {
		t.BaseComponent.SetBounds(r)
		t.layoutDirty = true
	}
}

// HandleInput processes mouse input for transport controls.
func (t *TransportComponent) HandleInput(x, y int, pressed bool) InputResult {
	// Check if we're capturing (e.g., slider drag)
	if t.mainVolSlider.dragging {
		if t.mainVolSlider.Handle(x, y, pressed) {
			if t.props.OnVolumeChange != nil {
				t.props.OnVolumeChange(t.mainVolSlider.Value)
			}
			if !pressed {
				t.SetCapturing(false)
			}
			return InputCaptured
		}
		t.SetCapturing(false)
	}

	pt := image.Pt(x, y)
	if !pt.In(t.bounds) {
		return InputIgnored
	}

	// Handle BPM box
	if t.bpmBox.Update() {
		return InputConsumed
	}

	// Handle buttons
	buttons := []*Button{
		t.playBtn, t.stopBtn, t.bpmDecBtn, t.bpmIncBtn,
		t.subdivBtn, t.lenDecBtn, t.lenIncBtn, t.trackBtn,
		t.uploadBtn, t.importBtn, t.exportBtn,
	}
	for _, btn := range buttons {
		if btn != nil && btn.Handle(x, y, pressed) {
			return InputConsumed
		}
	}

	// Handle volume slider
	if t.mainVolSlider != nil && t.mainVolSlider.Handle(x, y, pressed) {
		if t.mainVolSlider.dragging {
			t.SetCapturing(true)
		}
		if t.props.OnVolumeChange != nil {
			t.props.OnVolumeChange(t.mainVolSlider.Value)
		}
		return InputCaptured
	}

	return InputIgnored
}

// Draw renders the transport controls.
func (t *TransportComponent) Draw(dst *ebiten.Image) {
	if t.layoutDirty {
		t.recalcLayout()
		t.layoutDirty = false
	}

	t.decayAnims()

	t.playBtn.Draw(dst)
	t.stopBtn.Draw(dst)
	t.bpmDecBtn.Draw(dst)
	t.bpmBox.Draw(dst)
	if t.state.bpmErrorAnim > 0 {
		drawRect(dst, t.bpmBox.Rect, fadeColor(colError, t.state.bpmErrorAnim), false)
	}
	t.bpmIncBtn.Draw(dst)
	t.subdivBtn.Draw(dst)
	t.lenDecBtn.Draw(dst)
	t.lenIncBtn.Draw(dst)
	t.trackBtn.Draw(dst)
	t.uploadBtn.Draw(dst)
	t.importBtn.Draw(dst)
	t.exportBtn.Draw(dst)
	if t.mainVolSlider != nil {
		t.mainVolSlider.Draw(dst)
	}
}

func (t *TransportComponent) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	decay(&t.state.playAnim)
	decay(&t.state.stopAnim)
	decay(&t.state.bpmDecAnim)
	decay(&t.state.bpmIncAnim)
	decay(&t.state.lenDecAnim)
	decay(&t.state.lenIncAnim)
	decay(&t.state.uploadAnim)
	decay(&t.state.bpmErrorAnim)
}

func (t *TransportComponent) recalcLayout() {
	bounds := t.bounds
	if bounds.Empty() {
		return
	}

	pad := buttonPad + 2
	totalH := bounds.Dy()
	topRowH := totalH / 2
	botRowH := totalH - topRowH

	// Derive padding from available height
	if pad > topRowH/4 {
		pad = topRowH / 4
	}

	safeInset := func(r image.Rectangle, pad int) image.Rectangle {
		if r.Empty() {
			return r
		}
		minDim := r.Dx()
		if r.Dy() < minDim {
			minDim = r.Dy()
		}
		maxPad := (minDim - 2) / 2
		if maxPad < 0 {
			maxPad = 0
		}
		minW := 48
		minH := debugCharH + 2
		maxPadW := (r.Dx() - minW) / 2
		if maxPadW < 0 {
			maxPadW = 0
		}
		maxPadH := (r.Dy() - minH) / 2
		if maxPadH < 0 {
			maxPadH = 0
		}
		if maxPadW < maxPad {
			maxPad = maxPadW
		}
		if maxPadH < maxPad {
			maxPad = maxPadH
		}
		if pad > maxPad {
			pad = maxPad
		}
		return insetRect(r, pad)
	}

	// Top row: play | stop | bpm box | bpm(+/- stacked) | subdiv | len(+/- stacked) | track | slider
	controlLeft := bounds.Min.X + 12
	if controlLeft >= bounds.Max.X {
		controlLeft = bounds.Min.X
	}
	topEnd := bounds.Min.Y + topRowH
	if topEnd > bounds.Max.Y {
		topEnd = bounds.Max.Y
	}
	topBounds := image.Rect(controlLeft, bounds.Min.Y, bounds.Max.X, topEnd)
	topGrid := NewGridLayout(topBounds, []float64{1, 1, 2, 1, 1, 1, 1, 1, 3}, []float64{1})

	t.playBtn.SetRect(safeInset(topGrid.Cell(0, 0), pad))
	t.stopBtn.SetRect(safeInset(topGrid.Cell(1, 0), pad))
	t.bpmBox.Rect = safeInset(topGrid.Cell(2, 0), pad)

	// BPM +/- stacked vertically
	bpmCol := safeInset(topGrid.Cell(3, 0), pad)
	split := bpmCol.Dy() / 2
	if split < 8 {
		split = bpmCol.Dy() / 2
	}
	incR := bpmCol
	incR.Max.Y = incR.Min.Y + split
	t.bpmIncBtn.SetRect(incR)

	decR := bpmCol
	decR.Min.Y = incR.Max.Y
	if decR.Max.Y > topBounds.Max.Y {
		decR.Max.Y = topBounds.Max.Y
	}
	if decR.Min.Y > decR.Max.Y {
		decR.Min.Y = decR.Max.Y
	}
	t.bpmDecBtn.SetRect(decR)

	t.subdivBtn.SetRect(safeInset(topGrid.Cell(4, 0), pad))

	// Length +/- stacked vertically
	lenCol := safeInset(topGrid.Cell(5, 0), pad)
	lenSplit := lenCol.Dy() / 2
	if lenSplit < 8 {
		lenSplit = lenCol.Dy() / 2
	}
	li := lenCol
	li.Max.Y = li.Min.Y + lenSplit
	t.lenIncBtn.SetRect(li)

	ld := lenCol
	ld.Min.Y = li.Max.Y
	if ld.Max.Y > topBounds.Max.Y {
		ld.Max.Y = topBounds.Max.Y
	}
	if ld.Min.Y > ld.Max.Y {
		ld.Min.Y = ld.Max.Y
	}
	t.lenDecBtn.SetRect(ld)

	t.trackBtn.SetRect(safeInset(topGrid.Cell(6, 0), pad))

	// Ensure gaps between adjacent buttons
	ensureGap := func(left, right *Button) {
		if left == nil || right == nil {
			return
		}
		lr, rr := left.Rect(), right.Rect()
		if lr.Max.X >= rr.Min.X {
			dx := lr.Max.X - rr.Min.X + 2
			rr.Min.X += dx
			rr.Max.X += dx
			right.SetRect(rr)
		}
	}
	ensureGap(t.playBtn, t.stopBtn)

	// Clamp stacked buttons
	clampBtn := func(btn *Button, b image.Rectangle) {
		r := btn.Rect()
		if r.Min.Y < b.Min.Y {
			r.Min.Y = b.Min.Y
		}
		if r.Max.Y > b.Max.Y {
			r.Max.Y = b.Max.Y
		}
		btn.SetRect(r)
	}
	clampBtn(t.bpmIncBtn, topBounds)
	clampBtn(t.bpmDecBtn, topBounds)
	clampBtn(t.lenIncBtn, topBounds)
	clampBtn(t.lenDecBtn, topBounds)

	// Volume slider
	t.mainVolRect = safeInset(topGrid.Cell(8, 0), pad)
	t.mainVolSlider.SetRect(t.mainVolRect)

	// Bottom row: upload | import | export
	fileTop := topBounds.Max.Y
	fileBottom := bounds.Max.Y
	if fileBottom-fileTop < botRowH {
		fileBottom = fileTop + botRowH
	}
	botBounds := image.Rect(controlLeft, fileTop, bounds.Max.X, fileBottom)
	botGrid := NewGridLayout(botBounds, []float64{1, 1, 1}, []float64{1})

	t.uploadBtn.SetRect(safeInset(botGrid.Cell(0, 0), pad))
	t.importBtn.SetRect(safeInset(botGrid.Cell(1, 0), pad))
	t.exportBtn.SetRect(safeInset(botGrid.Cell(2, 0), pad))
}

// SetBPMText sets the BPM text box value directly.
func (t *TransportComponent) SetBPMText(s string) {
	t.bpmBox.SetText(s)
}

// BPMText returns the current BPM text box value.
func (t *TransportComponent) BPMText() string {
	return t.bpmBox.Value()
}

// SetSubdivText sets the subdivision button text.
func (t *TransportComponent) SetSubdivText(s string) {
	t.subdivBtn.Text = s
}

// ShowBPMError triggers the BPM error animation.
func (t *TransportComponent) ShowBPMError() {
	t.state.bpmErrorAnim = 1
}

// MarkLayoutDirty forces a layout recalculation on the next Draw.
func (t *TransportComponent) MarkLayoutDirty() {
	t.layoutDirty = true
}
