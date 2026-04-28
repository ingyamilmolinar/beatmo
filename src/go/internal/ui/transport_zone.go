package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
)

// audio import used indirectly via callbacks (GetMainVolume/SetMainVolume).

// TransportCallbacks contains callbacks for the TransportZone to communicate
// with the DrumView and audio engine. Zones don't reference Game or each other.
type TransportCallbacks struct {
	OnPlayToggle   func()            // play/pause pressed
	OnStop         func()            // stop pressed
	OnBPMChange    func(bpm int)     // BPM committed (from text or +/-)
	OnFollowChange func(follow bool) // track toggle
	OnUploadClick  func()            // delegates to DrumView's upload goroutine
	OnImportClick  func()            // delegates to DrumView's import picker
	OnExportClick  func()            // delegates to DrumView's export
	OnViewCycle    func()            // mobile view mode toggle
	OnRecordToggle func()            // record button pressed
	IsPlaying      func() bool       // read current playback state
	IsRecording    func() bool       // read current recording state
	GetMainVolume  func() float64    // read master volume
	SetMainVolume  func(v float64)   // set master volume
	OnNotifyError  func(msg string)  // display error notification

	// Overlay callbacks: delegate to DrumView's overlay mechanisms.
	OnSubdivClick    func() // delegates to DrumView's SubdivMenuComponent
	OnOverflowOpen   func() // delegates to DrumView's overflow menu
	OnMasterVolClick func() // delegates to DrumView's master vol popup
	MasterVolPopup   *SliderPopup // forwarded to transportVolIconHitAdapter for drag-through
}

// TransportZone implements the Zone interface for the transport controls.
// It owns the play/stop buttons, BPM controls, upload/import/export buttons,
// master volume slider, and view mode / overflow buttons.
type TransportZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  TransportCallbacks
	portal     *OverlayPortal // set by tree wiring

	// Transport buttons
	playBtn        *Button
	stopBtn        *Button
	recordBtn      *Button
	bpmDecBtn      *Button
	bpmBox         *TextInput
	bpmIncBtn      *Button
	subdivBtn      *Button
	trackBtn       *Button
	uploadBtn      *Button
	importBtn      *Button
	exportBtn      *Button
	eqToggleMobile *Button
	viewSwitchBtn  *Button
	overflowBtn    *Button

	// Master volume
	mainVolSlider   *Slider
	mainVolGroup    *SliderGroup
	mainVolIconRect image.Rectangle
	mainVolRect     image.Rectangle

	// Group visual container rects (drawn behind button groups)
	bpmGroupRect       image.Rectangle // BPM box + inc/dec
	transportGroupRect image.Rectangle // play + stop + record
	fileOpsGroupRect   image.Rectangle // upload + import + export

	// Animation state (decayed each frame)
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64

	// BPM state
	bpm      int
	bpmPrev  int
	bpmDelta int

	// Follow/track state
	follow bool

	// Transport pressed flags (one-frame pulses consumed by DrumView)
	playPressed   bool
	stopPressed   bool
	recordPressed bool

	// Playing flag (set by DrumView via SetPlaying)
	isPlaying   bool
	isRecording bool

	// Record button animation
	recordAnim  float64
	recordPulse float64

	// Persistent layout group for desktop transport.
	transportGroup *LayoutGroup

	// Toolbar cache
	toolbarCache     *ebiten.Image
	toolbarCacheHash uint64
	toolbarCacheRect image.Rectangle

	// Hit areas cache (rebuilt on Layout)
	hitAreas []HitArea

	// inputBlocked returns true when a popup/overlay is open and the BPM box
	// should be force-blurred to prevent stale focus. Set by DrumView wiring.
	inputBlocked func() bool
}

// NewTransportZone creates a TransportZone with the provided callbacks.
// Buttons are created internally; DrumView reads them via aliases.
func NewTransportZone(cb TransportCallbacks) *TransportZone {
	z := &TransportZone{
		needLayout: true,
		callbacks:  cb,
		bpm:        120,
		// follow defaults to true: timeline tracks the playhead until the
		// user explicitly opts out. Single source of truth for the follow
		// state on both desktop and mobile.
		follow: true,
	}
	z.initButtons()
	z.initSliders()
	return z
}

func (z *TransportZone) initButtons() {
	p := Profile()

	z.playBtn = NewButton("", p.PlayBtnStyle, func() {
		z.playPressed = true
		z.playAnim = 1
		if z.callbacks.OnPlayToggle != nil {
			z.callbacks.OnPlayToggle()
		}
	})
	z.playBtn.Icon = string(IconPlay)
	z.playBtn.IconColor = p.PlayIconColor

	z.stopBtn = NewButton("", p.StopBtnStyle, func() {
		z.stopPressed = true
		z.stopAnim = 1
		if z.callbacks.OnStop != nil {
			z.callbacks.OnStop()
		}
	})
	z.stopBtn.Icon = string(IconStop)
	z.stopBtn.IconColor = p.StopIconColor

	z.recordBtn = NewButton("", p.StopBtnStyle, func() {
		z.recordPressed = true
		z.recordAnim = 1
		if z.callbacks.OnRecordToggle != nil {
			z.callbacks.OnRecordToggle()
		}
	})
	z.recordBtn.Icon = string(IconRecord)
	z.recordBtn.IconColor = colRecordIdle

	z.bpmDecBtn = NewButton("", p.BPMDecBtnStyle, func() {
		z.bpmDelta--
		z.bpmDecAnim = 1
	})
	z.bpmDecBtn.Repeat = true
	z.bpmDecBtn.Icon = string(IconChevronDown)
	z.bpmDecBtn.IconColor = p.BPMIconColor

	z.bpmBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
	z.bpmBox.MaxLen = 4
	z.bpmBox.SetText("120")
	z.bpmBox.InputMode = "numeric"
	z.bpmBox.MobileInputID = "bpm"
	z.bpmBox.OnFocusGained = func() { softKeyboardShow("numeric") }
	z.bpmBox.OnFocusLost = func() { softKeyboardHide() }

	z.bpmIncBtn = NewButton("", p.BPMIncBtnStyle, func() {
		z.bpmDelta++
		z.bpmIncAnim = 1
	})
	z.bpmIncBtn.Repeat = true
	z.bpmIncBtn.Icon = string(IconChevronUp)
	z.bpmIncBtn.IconColor = p.BPMIconColor

	z.subdivBtn = NewButton("\u00f732", p.SubdivBtnStyle, func() {
		if z.callbacks.OnSubdivClick != nil {
			z.callbacks.OnSubdivClick()
		}
	})

	z.trackBtn = NewButton("", p.TrackBtnStyle, func() {
		z.SetFollow(!z.follow)
	})
	// Icon + IconColor are authoritative-set by syncTrackBtnVisual below;
	// no need to seed them here. The chrome (Style) stays as TrackBtnStyle
	// (= TransportMiscStyle on both profiles after the layout-profile fix)
	// for both states, mirroring how SetPlaying treats the play button.
	z.syncTrackBtnVisual()

	// Phase 4 PR3 migration: file-ops buttons + mobile EQ toggle render via
	// the generated `button-secondary` spec (Spec(ComponentButtonSecondary))
	// — byte-equivalent to UploadBtnStyle / InstButtonStyle per
	// TestComponentSpecsDrift, so this is a code-path swap, not a
	// behavior change.
	// IconColor for these three buttons flows from the generated
	// ComponentButtonSecondary spec (DESIGN.md: iconColor: {colors.on-surface-muted}).
	// SetSpec seeds b.IconColor at construction time; no explicit assignment
	// here.
	z.uploadBtn = NewSpecButton("", ComponentButtonSecondary, func() {
		z.uploadAnim = 1
		if z.callbacks.OnUploadClick != nil {
			z.callbacks.OnUploadClick()
		}
	})
	z.uploadBtn.Icon = string(IconUpload)

	z.importBtn = NewSpecButton("", ComponentButtonSecondary, func() {
		if z.callbacks.OnImportClick != nil {
			z.callbacks.OnImportClick()
		}
	})
	z.importBtn.Icon = string(IconImport)

	z.exportBtn = NewSpecButton("", ComponentButtonSecondary, func() {
		if z.callbacks.OnExportClick != nil {
			z.callbacks.OnExportClick()
		}
	})
	z.exportBtn.Icon = string(IconExport)

	// Mobile EQ toggle button (legacy, hidden — replaced by viewSwitchBtn).
	z.eqToggleMobile = NewSpecButton("EQ", ComponentButtonSecondary, func() {
		if z.callbacks.OnViewCycle != nil {
			z.callbacks.OnViewCycle()
		}
	})

	z.viewSwitchBtn = NewButton("", p.ViewSwitchStyle, func() {
		if z.callbacks.OnViewCycle != nil {
			z.callbacks.OnViewCycle()
		}
	})
	z.viewSwitchBtn.Icon = string(IconAudio)
	z.viewSwitchBtn.IconColor = colTextSecondary

	z.overflowBtn = NewButton("", p.OverflowStyle, func() {
		if z.callbacks.OnOverflowOpen != nil {
			z.callbacks.OnOverflowOpen()
		}
	})
	z.overflowBtn.Icon = string(IconOverflow)
	z.overflowBtn.IconColor = colTextSecondary
}

func (z *TransportZone) initSliders() {
	vol := 0.5
	if z.callbacks.GetMainVolume != nil {
		vol = z.callbacks.GetMainVolume()
	}
	if vol < 0 {
		vol = 0
	}
	if vol > 1 {
		vol = 1
	}
	z.mainVolSlider = NewSlider(vol)
	z.mainVolGroup = NewSliderGroup([]*Slider{z.mainVolSlider}, func(_ int, val float64) {
		if z.callbacks.SetMainVolume != nil {
			z.callbacks.SetMainVolume(val)
		}
	})
}

// --- Zone interface ---

func (z *TransportZone) ID() string { return "transport" }

func (z *TransportZone) NeedsLayout() bool { return z.needLayout }

func (z *TransportZone) Invalidate() { z.needLayout = true }

func (z *TransportZone) Layout(rect image.Rectangle) {
	z.rect = rect
	z.needLayout = false
	z.layoutButtons(rect)
	z.rebuildHitAreas()
}

func (z *TransportZone) Update() {
	z.decayAnims()
	// Apply accumulated BPM delta from +/- buttons.
	if z.bpmDelta != 0 {
		z.SetBPM(z.bpm + z.bpmDelta)
		z.bpmDelta = 0
	}

	// ─── BPM text input handling ───
	// The TransportZone is the single authority for BPM box focus/blur/commit.
	if z.bpmBox == nil {
		return
	}

	// Mobile native input for BPM box — poll result before normal handling.
	mobileBPMActive := Profile().IsMobile() && mobileInputActive("bpm")
	if mobileBPMActive {
		if val, committed, ok := mobileInputPollResult("bpm"); ok {
			if committed && val != "" {
				if v, ok := parseBPM(val); ok {
					z.SetBPM(v)
				} else {
					z.bpmErrorAnim = 1
					if z.callbacks.OnNotifyError != nil {
						z.callbacks.OnNotifyError("Invalid BPM")
					}
				}
			}
			z.bpmBox.SetText(strconv.Itoa(z.bpm))
			z.bpmBox.focused = false
		}
		return
	}

	// When input is blocked (popup/overlay open or tree suppressing),
	// force-blur the BPM box to prevent stale focus.
	blocked := z.inputBlocked != nil && z.inputBlocked()
	if blocked {
		if z.bpmBox.Focused() {
			z.forceBlurBPM()
		}
		return
	}

	// Normal BPM text input handling.
	prevFocus := z.bpmBox.Focused()
	z.bpmBox.Update()

	// Enter key: commit immediately and blur.
	if z.bpmBox.Focused() && isKeyPressed(ebiten.KeyEnter) {
		z.commitBPMText()
		// commitBPMText already blurred the box; skip the blur handler below.
		return
	}

	// Focus gained: save previous BPM, clear text for entry.
	if !prevFocus && z.bpmBox.Focused() {
		z.bpmPrev = z.bpm
		z.bpmBox.SetText("")
	}

	// Focus lost (blur): commit the value.
	if prevFocus && !z.bpmBox.Focused() {
		z.commitBPMText()
	}
}

func (z *TransportZone) HitAreas() []HitArea {
	return z.hitAreas
}

func (z *TransportZone) Draw(screen *ebiten.Image) {
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}
	z.renderToolbarControls(screen)
}

func (z *TransportZone) HandleKey(key ebiten.Key) InputResult {
	if z.bpmBox == nil || !z.bpmBox.Focused() {
		return InputIgnored
	}
	if key == ebiten.KeyEnter {
		z.commitBPMText()
		return InputConsumed
	}
	if key == ebiten.KeyEscape {
		// Revert to previous BPM on Escape.
		prev := z.bpmPrev
		if prev < 1 {
			prev = z.bpm
		}
		z.SetBPM(prev)
		z.bpmBox.SetText(strconv.Itoa(z.bpm))
		z.bpmBox.focused = false
		return InputConsumed
	}
	return InputIgnored
}

func (z *TransportZone) HandleChars(chars []rune) InputResult {
	if z.bpmBox == nil || !z.bpmBox.Focused() {
		return InputIgnored
	}
	// Characters are handled by bpmBox.Update() in TransportZone.Update().
	// This handler exists for the tree's keyboard routing framework.
	return InputIgnored
}

// --- Public accessors (for DrumView migration bridge) ---

// SetPlaying updates the play button icon to reflect playback state.
func (z *TransportZone) SetPlaying(p bool) {
	z.isPlaying = p
	prof := Profile()
	// DESIGN.md §0/§5: icon-only button — never raw Unicode in Text.
	z.playBtn.Text = ""
	if p {
		z.playBtn.Icon = string(IconPause)
		// Playing state uses the accent so the eye knows the transport is live.
		z.playBtn.IconColor = colAccent
	} else {
		z.playBtn.Icon = string(IconPlay)
		z.playBtn.IconColor = prof.PlayIconColor
	}
}

// SetBPM updates the BPM value and text box. Clamps to [1, maxBPM].
func (z *TransportZone) SetBPM(b int) {
	if b < 1 {
		z.bpm = 1
		z.bpmErrorAnim = 1
		if z.callbacks.OnBPMChange != nil {
			z.callbacks.OnBPMChange(z.bpm)
		}
		return
	}
	if b > maxBPM {
		z.bpm = maxBPM
		z.bpmErrorAnim = 1
		if z.callbacks.OnBPMChange != nil {
			z.callbacks.OnBPMChange(z.bpm)
		}
		return
	}
	z.bpm = b
	if z.bpmBox != nil && !z.bpmBox.Focused() {
		z.bpmBox.SetText(strconv.Itoa(z.bpm))
	}
	if z.callbacks.OnBPMChange != nil {
		z.callbacks.OnBPMChange(z.bpm)
	}
}

// BPM returns the current BPM.
func (z *TransportZone) BPM() int { return z.bpm }

// PlayPressed returns and clears the one-frame play pressed flag.
func (z *TransportZone) PlayPressed() bool {
	if z.playPressed {
		z.playPressed = false
		return true
	}
	return false
}

// StopPressed returns and clears the one-frame stop pressed flag.
func (z *TransportZone) StopPressed() bool {
	if z.stopPressed {
		z.stopPressed = false
		return true
	}
	return false
}

// RecordPressed returns and clears the one-frame record pressed flag.
func (z *TransportZone) RecordPressed() bool {
	if z.recordPressed {
		z.recordPressed = false
		return true
	}
	return false
}

// SetRecording updates the recording visual state.
func (z *TransportZone) SetRecording(rec bool) {
	z.isRecording = rec
}

// IsRecording returns the current recording visual state.
func (z *TransportZone) IsRecording() bool {
	return z.isRecording
}

// SetFollow sets the follow/track state and updates the button visual.
func (z *TransportZone) SetFollow(f bool) {
	z.follow = f
	z.syncTrackBtnVisual()
	if z.callbacks.OnFollowChange != nil {
		z.callbacks.OnFollowChange(f)
	}
}

// FollowPlayback returns whether auto-scroll is enabled.
func (z *TransportZone) FollowPlayback() bool { return z.follow }

// BPMErrorAnim returns the current BPM error animation value.
func (z *TransportZone) BPMErrorAnim() float64 { return z.bpmErrorAnim }

// MainVolSlider returns the master volume slider.
func (z *TransportZone) MainVolSlider() *Slider { return z.mainVolSlider }

// MainVolGroup returns the master volume slider group.
func (z *TransportZone) MainVolGroup() *SliderGroup { return z.mainVolGroup }

// MainVolIconRect returns the master volume icon rect.
func (z *TransportZone) MainVolIconRect() image.Rectangle { return z.mainVolIconRect }

// SetPortal sets the portal reference for opening overlays.
func (z *TransportZone) SetPortal(p *OverlayPortal) { z.portal = p }

// --- Layout ---

func (z *TransportZone) layoutButtons(topBounds image.Rectangle) {
	spec := ActiveTopBarSpec()
	pad := spec.Padding
	if pad > topBounds.Dy()/4 {
		pad = topBounds.Dy() / 4
	}

	if Profile().IsMobile() {
		z.layoutMobile(topBounds, pad, spec)
	} else {
		z.layoutDesktop(topBounds, pad, spec)
	}
	ensureGapBtns(z.playBtn, z.stopBtn, spec.ControlGap)
}

func (z *TransportZone) layoutMobile(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	outerGrid := NewGridLayout(topBounds, []float64{1}, []float64{1, 1})
	row0Grid := outerGrid.SubGrid(0, 0,
		[]float64{1.0, 1.0, 2.0, 1.0, 1.0}, []float64{1})
	row1Grid := outerGrid.SubGrid(0, 1,
		[]float64{1.0, 1.0, 1.0}, []float64{1})
	row0Bounds := outerGrid.Cell(0, 0)

	z.playBtn.SetRect(safeInsetTransport(row0Grid.Cell(0, 0), pad))
	z.stopBtn.SetRect(safeInsetTransport(row0Grid.Cell(1, 0), pad))
	z.bpmBox.Rect = safeInsetTransport(row0Grid.Cell(2, 0), pad)
	bpmCol := safeInsetTransport(row0Grid.Cell(3, 0), pad)
	stackVerticalTransport(z.bpmIncBtn, z.bpmDecBtn, bpmCol, row0Bounds)

	// Compute BPM group container rect (covers BPM box + inc/dec arrows).
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)

	// Compute transport group container rect (covers play + stop on mobile).
	z.transportGroupRect = computeGroupRect([]image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(),
	}, spec.GroupOutlinePad)

	z.subdivBtn.SetRect(safeInsetTransport(row0Grid.Cell(4, 0), pad))

	// Mobile: volume icon opens popup; no inline slider.
	z.mainVolIconRect = safeInsetTransport(row1Grid.Cell(0, 0), pad)
	if z.mainVolSlider != nil {
		z.mainVolSlider.SetRect(image.Rectangle{})
		z.mainVolRect = image.Rectangle{}
	}
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.SetRect(safeInsetTransport(row1Grid.Cell(1, 0), pad))
	}
	if z.overflowBtn != nil {
		z.overflowBtn.SetRect(safeInsetTransport(row1Grid.Cell(2, 0), pad))
	}
	// Hide desktop-only buttons on mobile.
	z.trackBtn.SetRect(image.Rectangle{})
	z.recordBtn.SetRect(image.Rectangle{}) // TODO: add to mobile layout
	z.uploadBtn.SetRect(image.Rectangle{})
	z.importBtn.SetRect(image.Rectangle{})
	z.exportBtn.SetRect(image.Rectangle{})
	z.fileOpsGroupRect = image.Rectangle{} // no file-ops on mobile
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.SetRect(image.Rectangle{})
	}
}

func (z *TransportZone) layoutDesktop(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	// Single-row transport: Play | Stop | Record | BPM | ± | Subdiv | spacer | Vol | Upload | Import | Export
	rowWeights := spec.ColumnWeights
	if z.transportGroup == nil || len(z.transportGroup.grid.colWeights) != len(rowWeights) {
		z.transportGroup = NewLayoutGroup("transport", topBounds, rowWeights, []float64{1})
	}
	z.transportGroup.SetBounds(topBounds)

	minBtn := spec.BtnMinSize
	playRect := safeInsetTransport(z.transportGroup.Cell(0, 0), pad)
	playRect = enforceMinSize(playRect, minBtn, minBtn)
	z.playBtn.SetRect(playRect)

	stopRect := safeInsetTransport(z.transportGroup.Cell(1, 0), pad)
	stopRect = enforceMinSize(stopRect, minBtn, minBtn)
	z.stopBtn.SetRect(stopRect)

	recordRect := safeInsetTransport(z.transportGroup.Cell(2, 0), pad)
	recordRect = enforceMinSize(recordRect, minBtn, minBtn)
	z.recordBtn.SetRect(recordRect)

	z.bpmBox.Rect = safeInsetTransport(z.transportGroup.Cell(3, 0), pad)
	bpmCol := safeInsetTransport(z.transportGroup.Cell(4, 0), pad)
	stackVerticalTransport(z.bpmIncBtn, z.bpmDecBtn, bpmCol, topBounds)

	// Compute BPM group container rect (covers BPM box + inc/dec arrows).
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)

	// Compute transport group container rect (covers play + stop + record).
	z.transportGroupRect = computeGroupRect([]image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
	}, spec.GroupOutlinePad)

	z.subdivBtn.SetRect(safeInsetTransport(z.transportGroup.Cell(5, 0), pad))

	// col 6 is flexible spacer (empty)

	// Desktop: icon-only volume (popup on click, matching mobile pattern).
	volCell := safeInsetTransport(z.transportGroup.Cell(7, 0), pad)
	z.mainVolIconRect = volCell
	if z.mainVolSlider != nil {
		z.mainVolSlider.SetRect(image.Rectangle{})
		z.mainVolRect = image.Rectangle{}
	}

	z.uploadBtn.SetRect(safeInsetTransport(z.transportGroup.Cell(8, 0), pad))
	z.importBtn.SetRect(safeInsetTransport(z.transportGroup.Cell(9, 0), pad))
	z.exportBtn.SetRect(safeInsetTransport(z.transportGroup.Cell(10, 0), pad))

	// Compute file-ops group container rect (covers upload + import + export).
	z.fileOpsGroupRect = computeGroupRect([]image.Rectangle{
		z.uploadBtn.Rect(), z.importBtn.Rect(), z.exportBtn.Rect(),
	}, spec.GroupOutlinePad)

	// Track button positioned in timeline area, not toolbar.
	z.trackBtn.SetRect(image.Rectangle{})
	// Hide mobile-only buttons on desktop.
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.SetRect(image.Rectangle{})
	}
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.SetRect(image.Rectangle{})
	}
	if z.overflowBtn != nil {
		z.overflowBtn.SetRect(image.Rectangle{})
	}
}

// --- Hit area construction ---

func (z *TransportZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]

	const zIdx = 110 // TransportZone z-index

	// Simple button hit areas.
	simpleButtons := []struct {
		btn   *Button
		tag   string
		touch bool
	}{
		{z.playBtn, "transport-play", true},
		{z.stopBtn, "transport-stop", true},
		{z.recordBtn, "transport-record", true},
		{z.subdivBtn, "transport-subdiv", true},
		{z.trackBtn, "transport-track", true},
		{z.uploadBtn, "transport-upload", true},
		{z.importBtn, "transport-import", true},
		{z.exportBtn, "transport-export", true},
		{z.eqToggleMobile, "transport-eq-toggle", false},
		{z.viewSwitchBtn, "transport-view-switch", true},
		{z.overflowBtn, "transport-overflow", true},
	}
	for _, sb := range simpleButtons {
		if sb.btn == nil {
			continue
		}
		r := sb.btn.Rect()
		if r.Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:     r,
			ZIndex:   zIdx,
			Handler:  &buttonHitAdapter{btn: sb.btn},
			Tag:      sb.tag,
			Touch:    sb.touch,
			ClipRect: z.rect,
		})
	}

	// BPM +/- buttons use repeatButtonHitAdapter for hold-to-repeat.
	for _, rb := range []struct {
		btn *Button
		tag string
	}{
		{z.bpmDecBtn, "transport-bpm-dec"},
		{z.bpmIncBtn, "transport-bpm-inc"},
	} {
		if rb.btn == nil {
			continue
		}
		r := rb.btn.Rect()
		if r.Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:     r,
			ZIndex:   zIdx,
			Handler:  &repeatButtonHitAdapter{btn: rb.btn},
			Tag:      rb.tag,
			Touch:    true,
			ClipRect: z.rect,
		})
	}

	// BPM text box — on press, focuses the text input and returns InputIgnored
	// so that the legacy TextInput.Update() path can handle subsequent
	// text editing. The higher z-index ensures this area takes priority over
	// touch-expanded BPM +/- button rects.
	if z.bpmBox != nil && !z.bpmBox.Rect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.bpmBox.Rect,
			ZIndex:  zIdx + 1,
			Handler: &textInputHitAdapter{ti: z.bpmBox},
			Tag:     "transport-bpm-box",
		})
	}

	// Master volume slider group — z+1 so sliders get first dispatch over
	// adjacent buttons at the same z. Fall-through ensures clicks outside
	// the slider track still reach underlying buttons.
	if z.mainVolGroup != nil && z.mainVolSlider != nil && !z.mainVolSlider.Rect().Empty() {
		groupBounds := z.mainVolGroup.InputBounds()
		if !groupBounds.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:     groupBounds,
				ZIndex:   zIdx + 1,
				Handler:  &sliderGroupHitAdapter{group: z.mainVolGroup},
				Tag:      "transport-vol-slider",
				Touch:    true,
				ClipRect: z.rect,
			})
		}
	}

	// Master volume icon (mobile: opens popup).
	if Profile().DrawMasterVolIcon && !z.mainVolIconRect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:   z.mainVolIconRect,
			ZIndex: zIdx,
			Handler: &transportVolIconHitAdapter{
				onClick: z.callbacks.OnMasterVolClick,
				popup:   z.callbacks.MasterVolPopup,
			},
			Tag:      "transport-vol-icon",
			Touch:    true,
			ClipRect: z.rect,
		})
	}
}

// --- Hit handler adapters ---

// repeatButtonHitAdapter wraps a Repeat-enabled Button as a HitHandler.
// Delegates to Button.Handle which has proper repeat timing (initial delay,
// acceleration) instead of firing every frame.
type repeatButtonHitAdapter struct {
	btn *Button
}

func (h *repeatButtonHitAdapter) OnPress(x, y int) InputResult {
	h.btn.Handle(x, y, true)
	return InputCaptured
}

func (h *repeatButtonHitAdapter) OnDrag(x, y int) {
	h.btn.Handle(x, y, true)
}

func (h *repeatButtonHitAdapter) OnRelease(x, y int) {
	h.btn.Handle(x, y, false)
}

func (h *repeatButtonHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// transportVolIconHitAdapter handles the mobile master volume icon click.
// OnPress opens the popup and returns InputCaptured so that subsequent
// drag/release events are routed here, enabling tap-and-slide in one gesture.
type transportVolIconHitAdapter struct {
	onClick func()
	popup   *SliderPopup
}

func (h *transportVolIconHitAdapter) OnPress(x, y int) InputResult {
	if h.onClick != nil {
		h.onClick()
	}
	return InputCaptured
}

func (h *transportVolIconHitAdapter) OnDrag(x, y int) {
	if h.popup != nil {
		h.popup.HandleInput(x, y, true)
	}
}

func (h *transportVolIconHitAdapter) OnRelease(x, y int) {
	if h.popup != nil {
		h.popup.HandleInput(x, y, false)
	}
}
func (h *transportVolIconHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Animation decay ---

func (z *TransportZone) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	decay(&z.playAnim)
	decay(&z.stopAnim)
	decay(&z.recordAnim)
	decay(&z.bpmDecAnim)
	decay(&z.bpmIncAnim)
	decay(&z.uploadAnim)
	decay(&z.bpmErrorAnim)

	// Record button pulsing animation when recording.
	if z.isRecording {
		z.recordPulse += 0.05
		alpha := 0.5 + 0.5*math.Sin(z.recordPulse*2)
		z.recordBtn.IconColor = WithAlpha(genColorRecordActive, uint8(255*alpha))
	} else {
		z.recordPulse = 0
		z.recordBtn.IconColor = colRecordIdle
	}
}

// SetInputBlocked sets the callback used to check if BPM input should be blocked.
func (z *TransportZone) SetInputBlocked(fn func() bool) {
	z.inputBlocked = fn
}

// BPMPrev returns the saved BPM value from before text editing began.
func (z *TransportZone) BPMPrev() int { return z.bpmPrev }

// SetBPMPrev sets the saved BPM value (used for test compatibility during migration).
func (z *TransportZone) SetBPMPrev(v int) { z.bpmPrev = v }

// BPMDelta returns the current accumulated BPM delta.
func (z *TransportZone) BPMDelta() int { return z.bpmDelta }

// SetBPMDelta sets the accumulated BPM delta (used for test compatibility during migration).
func (z *TransportZone) SetBPMDelta(v int) { z.bpmDelta = v }

// --- BPM text commit ---

func (z *TransportZone) commitBPMText() {
	txt := z.bpmBox.Value()
	if txt == "" {
		prev := z.bpmPrev
		if prev < 1 {
			prev = z.bpm
		}
		z.SetBPM(prev)
	} else if v, ok := parseBPM(txt); ok {
		z.SetBPM(v)
	} else {
		z.bpmErrorAnim = 1
		if z.callbacks.OnNotifyError != nil {
			z.callbacks.OnNotifyError("Invalid BPM")
		}
		prev := z.bpmPrev
		if prev < 1 {
			prev = z.bpm
		}
		z.SetBPM(prev)
	}
	z.bpmBox.SetText(strconv.Itoa(z.bpm))
	z.bpmBox.focused = false
}

// forceBlurBPM commits the current BPM box value and blurs the box.
// Used when an overlay or popup steals focus.
func (z *TransportZone) forceBlurBPM() {
	z.commitBPMText()
}

// --- Track button visual ---

// syncTrackBtnVisual mirrors the play/stop visual model: chrome (Style) stays
// fixed across states; the icon glyph and icon color carry the active/inactive
// signal. Active = bright cyan (colFollowActive, the same accent play uses
// while playing); inactive = the platform-neutral BPM/secondary-control tint.
func (z *TransportZone) syncTrackBtnVisual() {
	if z.trackBtn == nil {
		return
	}
	prof := Profile()
	syncToggleVisual(
		z.trackBtn, z.follow,
		prof.TrackBtnStyle, prof.TrackBtnStyle, // same chrome both states
		IconTrack, IconTrackOff,
		colFollowActive, prof.BPMIconColor,
	)
}

// --- Toolbar caching ---

func (z *TransportZone) toolbarStateHash() uint64 {
	h := uint64(17)
	mix := func(v uint64) { h = h*31 + v }
	boolBit := func(b bool) uint64 {
		if b {
			return 1
		}
		return 0
	}
	// Layout bounds.
	mix(uint64(z.rect.Min.X))
	mix(uint64(z.rect.Min.Y))
	mix(uint64(z.rect.Dx()))
	mix(uint64(z.rect.Dy()))
	// Transport state.
	mix(boolBit(z.isPlaying))
	mix(boolBit(z.isRecording))
	mix(uint64(z.bpm))
	mix(boolBit(z.follow))
	// BPM text.
	for _, r := range z.bpmBox.Text {
		mix(uint64(r))
	}
	mix(boolBit(z.bpmBox.Focused()))
	// Subdiv text.
	for _, r := range z.subdivBtn.Text {
		mix(uint64(r))
	}
	// Volume slider position.
	if z.mainVolSlider != nil {
		mix(math.Float64bits(z.mainVolSlider.Value))
	}
	// Button hover/press state.
	mix(boolBit(z.playBtn.hovered))
	mix(boolBit(z.playBtn.pressed))
	mix(boolBit(z.stopBtn.hovered))
	mix(boolBit(z.stopBtn.pressed))
	mix(boolBit(z.bpmDecBtn.hovered))
	mix(boolBit(z.bpmDecBtn.pressed))
	mix(boolBit(z.bpmIncBtn.hovered))
	mix(boolBit(z.bpmIncBtn.pressed))
	mix(boolBit(z.subdivBtn.hovered))
	mix(boolBit(z.subdivBtn.pressed))
	mix(boolBit(z.trackBtn.hovered))
	mix(boolBit(z.trackBtn.pressed))
	mix(boolBit(z.uploadBtn.hovered))
	mix(boolBit(z.uploadBtn.pressed))
	mix(boolBit(z.importBtn.hovered))
	mix(boolBit(z.importBtn.pressed))
	mix(boolBit(z.exportBtn.hovered))
	mix(boolBit(z.exportBtn.pressed))
	// Track button text changes with follow state.
	for _, r := range z.trackBtn.Text {
		mix(uint64(r))
	}
	// Play button icon changes with play state.
	if z.playBtn.Icon != "" {
		for _, r := range z.playBtn.Icon {
			mix(uint64(r))
		}
	}
	if z.eqToggleMobile != nil {
		for _, r := range z.eqToggleMobile.Text {
			mix(uint64(r))
		}
		mix(boolBit(z.eqToggleMobile.hovered))
		mix(boolBit(z.eqToggleMobile.pressed))
	}
	if z.overflowBtn != nil {
		for _, r := range z.overflowBtn.Text {
			mix(uint64(r))
		}
		mix(boolBit(z.overflowBtn.hovered))
		mix(boolBit(z.overflowBtn.pressed))
	}
	if z.viewSwitchBtn != nil {
		for _, r := range z.viewSwitchBtn.Icon {
			mix(uint64(r))
		}
		mix(boolBit(z.viewSwitchBtn.hovered))
		mix(boolBit(z.viewSwitchBtn.pressed))
	}
	// Group container rects.
	mix(uint64(z.bpmGroupRect.Min.X))
	mix(uint64(z.bpmGroupRect.Min.Y))
	mix(uint64(z.bpmGroupRect.Max.X))
	mix(uint64(z.bpmGroupRect.Max.Y))
	mix(uint64(z.transportGroupRect.Min.X))
	mix(uint64(z.transportGroupRect.Min.Y))
	mix(uint64(z.transportGroupRect.Max.X))
	mix(uint64(z.transportGroupRect.Max.Y))
	mix(uint64(z.fileOpsGroupRect.Min.X))
	mix(uint64(z.fileOpsGroupRect.Min.Y))
	mix(uint64(z.fileOpsGroupRect.Max.X))
	mix(uint64(z.fileOpsGroupRect.Max.Y))
	// Animation state (non-zero = visual change).
	mix(math.Float64bits(z.bpmErrorAnim))
	return h
}

func (z *TransportZone) toolbarBounds() image.Rectangle {
	first := true
	var minX, minY, maxX, maxY int
	expand := func(r image.Rectangle) {
		if r.Empty() {
			return
		}
		if first {
			minX, minY, maxX, maxY = r.Min.X, r.Min.Y, r.Max.X, r.Max.Y
			first = false
		} else {
			if r.Min.X < minX {
				minX = r.Min.X
			}
			if r.Min.Y < minY {
				minY = r.Min.Y
			}
			if r.Max.X > maxX {
				maxX = r.Max.X
			}
			if r.Max.Y > maxY {
				maxY = r.Max.Y
			}
		}
	}
	expand(z.playBtn.Rect())
	expand(z.stopBtn.Rect())
	expand(z.bpmDecBtn.Rect())
	expand(z.bpmBox.Rect)
	expand(z.bpmIncBtn.Rect())
	expand(z.subdivBtn.Rect())
	expand(z.uploadBtn.Rect())
	expand(z.importBtn.Rect())
	expand(z.exportBtn.Rect())
	if z.eqToggleMobile != nil {
		expand(z.eqToggleMobile.Rect())
	}
	if z.overflowBtn != nil {
		expand(z.overflowBtn.Rect())
	}
	if z.viewSwitchBtn != nil {
		expand(z.viewSwitchBtn.Rect())
	}
	if z.mainVolSlider != nil {
		expand(z.mainVolSlider.Rect())
	}
	expand(z.mainVolIconRect)
	expand(z.bpmGroupRect)
	expand(z.transportGroupRect)
	expand(z.fileOpsGroupRect)
	if first {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func (z *TransportZone) renderToolbarControls(dst *ebiten.Image) {
	hash := z.toolbarStateHash()
	rect := z.toolbarBounds()
	if rect.Empty() {
		z.renderToolbarDirect(dst)
		return
	}

	// Cache hit.
	if z.toolbarCache != nil && z.toolbarCacheHash == hash && z.toolbarCacheRect == rect {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
		dst.DrawImage(z.toolbarCache, &op)
		return
	}

	// Cache miss: rebuild.
	w, h := rect.Dx(), rect.Dy()
	if z.toolbarCache == nil || z.toolbarCache.Bounds().Dx() != w || z.toolbarCache.Bounds().Dy() != h {
		z.toolbarCache = ebiten.NewImage(w, h)
	} else {
		z.toolbarCache.Clear()
	}

	z.renderToolbarToCache(z.toolbarCache, rect.Min.X, rect.Min.Y)
	z.toolbarCacheHash = hash
	z.toolbarCacheRect = rect

	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	dst.DrawImage(z.toolbarCache, &op)
}

func (z *TransportZone) renderToolbarToCache(cache *ebiten.Image, offsetX, offsetY int) {
	// Draw group container backgrounds (behind buttons).
	z.drawTransportGroupOffset(cache, offsetX, offsetY)
	z.drawBPMGroupOffset(cache, offsetX, offsetY)
	z.drawFileOpsGroupOffset(cache, offsetX, offsetY)

	drawBtnOff(cache, z.playBtn, offsetX, offsetY)
	drawBtnOff(cache, z.stopBtn, offsetX, offsetY)
	drawBtnOff(cache, z.recordBtn, offsetX, offsetY)
	drawBtnOff(cache, z.bpmDecBtn, offsetX, offsetY)
	drawTIOffset(cache, z.bpmBox, offsetX, offsetY)
	if z.bpmErrorAnim > 0 {
		r := z.bpmBox.Rect.Sub(image.Pt(offsetX, offsetY))
		drawRect(cache, r, fadeColor(colError, z.bpmErrorAnim), false)
	}
	drawBtnOff(cache, z.bpmIncBtn, offsetX, offsetY)
	drawBtnOff(cache, z.subdivBtn, offsetX, offsetY)
	drawBtnOff(cache, z.trackBtn, offsetX, offsetY)
	drawBtnOff(cache, z.uploadBtn, offsetX, offsetY)
	drawBtnOff(cache, z.importBtn, offsetX, offsetY)
	drawBtnOff(cache, z.exportBtn, offsetX, offsetY)
	if z.eqToggleMobile != nil {
		drawBtnOff(cache, z.eqToggleMobile, offsetX, offsetY)
	}
	if z.overflowBtn != nil {
		drawBtnOff(cache, z.overflowBtn, offsetX, offsetY)
	}
	if z.viewSwitchBtn != nil {
		drawBtnOff(cache, z.viewSwitchBtn, offsetX, offsetY)
	}
	if z.mainVolSlider != nil {
		// Always draw the volume icon on desktop; on mobile it is controlled by DrawMasterVolIcon.
		if !Profile().IsMobile() || Profile().DrawMasterVolIcon {
			z.drawMasterVolIconOffset(cache, offsetX, offsetY)
		}
		if !z.mainVolSlider.Rect().Empty() {
			drawSliderOff(cache, z.mainVolSlider, offsetX, offsetY)
		}
	}

	// Draw mobile second-row labels below icons.
	if Profile().IsMobile() {
		z.drawMobileRowLabelsOffset(cache, offsetX, offsetY)
	}

	z.drawToolbarSeparators(cache, offsetX, offsetY)
}

func (z *TransportZone) renderToolbarDirect(dst *ebiten.Image) {
	// Draw group container backgrounds.
	z.drawTransportGroupOffset(dst, 0, 0)
	z.drawBPMGroupOffset(dst, 0, 0)
	z.drawFileOpsGroupOffset(dst, 0, 0)

	z.playBtn.Draw(dst)
	z.stopBtn.Draw(dst)
	z.recordBtn.Draw(dst)
	z.bpmDecBtn.Draw(dst)
	z.bpmBox.Draw(dst)
	if z.bpmErrorAnim > 0 {
		drawRect(dst, z.bpmBox.Rect, fadeColor(colError, z.bpmErrorAnim), false)
	}
	z.bpmIncBtn.Draw(dst)
	z.subdivBtn.Draw(dst)
	z.trackBtn.Draw(dst)
	z.uploadBtn.Draw(dst)
	z.importBtn.Draw(dst)
	z.exportBtn.Draw(dst)
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.Draw(dst)
	}
	if z.overflowBtn != nil {
		z.overflowBtn.Draw(dst)
	}
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.Draw(dst)
	}
	if z.mainVolSlider != nil {
		if !Profile().IsMobile() || Profile().DrawMasterVolIcon {
			z.drawMasterVolIconOffset(dst, 0, 0)
		}
		if !z.mainVolSlider.Rect().Empty() {
			z.mainVolSlider.Draw(dst)
		}
	}

	// Draw mobile second-row labels.
	if Profile().IsMobile() {
		z.drawMobileRowLabelsOffset(dst, 0, 0)
	}
}

func (z *TransportZone) drawToolbarSeparators(cache *ebiten.Image, offsetX, offsetY int) {
	if !Profile().DrawToolbarSep {
		return
	}
	sepCol := WithAlpha(genColorBorder, genAlphaTransportSeparator)
	var pairs [][2]image.Rectangle
	pairs = [][2]image.Rectangle{
		{z.stopBtn.Rect(), z.bpmBox.Rect},
		{z.bpmIncBtn.Rect(), z.subdivBtn.Rect()},
	}
	if z.viewSwitchBtn != nil {
		pairs = append(pairs, [2]image.Rectangle{z.subdivBtn.Rect(), z.viewSwitchBtn.Rect()})
	}
	for _, p := range pairs {
		left, right := p[0], p[1]
		if left.Empty() || right.Empty() {
			continue
		}
		sepX := (left.Max.X + right.Min.X) / 2
		top := left.Min.Y + left.Dy()/4
		bot := left.Max.Y - left.Dy()/4
		if bot <= top {
			continue
		}
		drawRect(cache, image.Rect(sepX-offsetX, top-offsetY, sepX-offsetX+1, bot-offsetY), sepCol, true)
	}
}

// drawMasterVolIconOffset renders the speaker glyph plus a small volume-level
// tick mark. The glyph itself lives in the unified icon system; this function
// only picks the right variant (on/off) and overlays the live volume bar.
func (z *TransportZone) drawMasterVolIconOffset(cache *ebiten.Image, offsetX, offsetY int) {
	r := z.mainVolIconRect
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	vol := 0.0
	if z.mainVolSlider != nil {
		vol = z.mainVolSlider.Value
	}

	iconCol := colVolumeIconOn
	glyph := IconSpeaker
	if vol <= 0 {
		iconCol = colVolumeIconOff
		glyph = IconSpeakerOff
	}

	// Fit the glyph inside a square centered in r so the speaker proportions
	// match the other toolbar icons regardless of cell aspect.
	side := minI(r.Dx(), r.Dy())
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	box := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side)
	DrawIcon(cache, glyph, box, iconCol)
}

// drawBPMGroupOffset draws the BPM group visual container: a colSurface1
// rounded rect with colBorderSubtle border, plus a "BPM" caption label.
func (z *TransportZone) drawBPMGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.bpmGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colSurface1, RadiusMD, true)
	drawRoundedRect(dst, r, colBorderSubtle, RadiusMD, false)

	// Draw "BPM" caption label to the left of the BPM value box.
	captionScale := FontSizeCaption / FontSizeBody
	label := "BPM"
	labelW := int(float64(TextWidth(label)) * captionScale)
	labelH := int(float64(TextHeight()) * captionScale)
	// Position: vertically centered in the group rect, horizontally to the
	// left of the BPM text box (inside the group container padding).
	bpmBoxR := z.bpmBox.Rect.Sub(image.Pt(offsetX, offsetY))
	lx := bpmBoxR.Min.X - labelW - 4
	if lx < r.Min.X+2 {
		lx = r.Min.X + 2
	}
	ly := r.Min.Y + (r.Dy()-labelH)/2
	DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
}

// drawTransportGroupOffset draws the transport group pill container
// (play+stop+record) as a rounded rect with subtle fill and border.
func (z *TransportZone) drawTransportGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.transportGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	radius := RadiusMD
	if Profile().IsMobile() {
		radius = RadiusMD + 2 // 10px for mobile
	}
	drawRoundedRect(dst, r, colTransportGroupBG, radius, true)
	drawRoundedRect(dst, r, colTransportGroupBorder, radius, false)
}

// drawFileOpsGroupOffset draws the file-ops group pill container
// (upload+import+export) as a rounded rect with subtle fill and border.
func (z *TransportZone) drawFileOpsGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.fileOpsGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colTransportGroupBG, RadiusMD, true)
	drawRoundedRect(dst, r, colTransportGroupBorder, RadiusMD, false)
}

// drawMobileRowLabelsOffset draws "Vol", "View", "Menu" labels below the
// mobile second-row icons (volume, view switch, overflow).
func (z *TransportZone) drawMobileRowLabelsOffset(dst *ebiten.Image, offsetX, offsetY int) {
	captionScale := FontSizeCaption / FontSizeBody
	type iconLabel struct {
		rect  image.Rectangle
		label string
	}
	labels := []iconLabel{
		{z.mainVolIconRect, "Vol"},
	}
	if z.viewSwitchBtn != nil {
		labels = append(labels, iconLabel{z.viewSwitchBtn.Rect(), "View"})
	}
	if z.overflowBtn != nil {
		labels = append(labels, iconLabel{z.overflowBtn.Rect(), "Menu"})
	}
	for _, il := range labels {
		r := il.rect
		if r.Empty() {
			continue
		}
		r = r.Sub(image.Pt(offsetX, offsetY))
		labelW := int(float64(TextWidth(il.label)) * captionScale)
		lx := r.Min.X + (r.Dx()-labelW)/2
		// Place just below the button rect, offset by 1px.
		ly := r.Max.Y - int(float64(TextHeight())*captionScale) - 1
		DrawTextColorAtScale(dst, il.label, lx, ly, colTextSecondary, captionScale)
	}
}

// --- Offset drawing helpers (package-level to avoid DrumView dependency) ---

func drawBtnOff(cache *ebiten.Image, btn *Button, offsetX, offsetY int) {
	origRect := btn.Rect()
	btn.SetRect(origRect.Sub(image.Pt(offsetX, offsetY)))
	btn.Draw(cache)
	btn.SetRect(origRect)
}

func drawTIOffset(cache *ebiten.Image, ti *TextInput, offsetX, offsetY int) {
	origRect := ti.Rect
	ti.Rect = origRect.Sub(image.Pt(offsetX, offsetY))
	ti.Draw(cache)
	ti.Rect = origRect
}

func drawSliderOff(cache *ebiten.Image, slider *Slider, offsetX, offsetY int) {
	origRect := slider.Rect()
	slider.SetRect(origRect.Sub(image.Pt(offsetX, offsetY)))
	slider.Draw(cache)
	slider.SetRect(origRect)
}

// --- Layout helpers (package-level) ---

// safeInsetTransport insets a rect by pad, clamped to keep minimum usable size.
func safeInsetTransport(r image.Rectangle, pad int) image.Rectangle {
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

// stackVerticalTransport stacks two buttons vertically in a column rect.
func stackVerticalTransport(top, bot *Button, col image.Rectangle, bounds image.Rectangle) {
	split := col.Dy() / 2
	if split < 8 {
		split = col.Dy() / 2
	}
	top.SetRect(col)
	tr := top.Rect()
	tr.Max.Y = tr.Min.Y + split
	top.SetRect(tr)
	bot.SetRect(col)
	br := bot.Rect()
	br.Min.Y = tr.Max.Y
	if br.Max.Y > bounds.Max.Y {
		br.Max.Y = bounds.Max.Y
	}
	if tr.Max.Y > bounds.Max.Y {
		tr.Max.Y = bounds.Max.Y
	}
	if br.Min.Y > br.Max.Y {
		br.Min.Y = br.Max.Y
	}
	bot.SetRect(br)
	clampBtnTransport(top, bounds)
	clampBtnTransport(bot, bounds)
}

func clampBtnTransport(btn *Button, bounds image.Rectangle) {
	r := btn.Rect()
	if r.Min.Y < bounds.Min.Y {
		r.Min.Y = bounds.Min.Y
	}
	if r.Max.Y > bounds.Max.Y {
		r.Max.Y = bounds.Max.Y
	}
	btn.SetRect(r)
}

// enforceMinSize expands a rect to at least minW x minH, centered within the
// original bounds if possible.
func enforceMinSize(r image.Rectangle, minW, minH int) image.Rectangle {
	if r.Dx() < minW {
		cx := (r.Min.X + r.Max.X) / 2
		r.Min.X = cx - minW/2
		r.Max.X = r.Min.X + minW
	}
	if r.Dy() < minH {
		cy := (r.Min.Y + r.Max.Y) / 2
		r.Min.Y = cy - minH/2
		r.Max.Y = r.Min.Y + minH
	}
	return r
}

// computeGroupRect returns the bounding rect enclosing all provided rects,
// expanded by pad pixels. Empty rects are ignored.
func computeGroupRect(rects []image.Rectangle, pad int) image.Rectangle {
	first := true
	var minX, minY, maxX, maxY int
	for _, r := range rects {
		if r.Empty() {
			continue
		}
		if first {
			minX, minY, maxX, maxY = r.Min.X, r.Min.Y, r.Max.X, r.Max.Y
			first = false
		} else {
			if r.Min.X < minX {
				minX = r.Min.X
			}
			if r.Min.Y < minY {
				minY = r.Min.Y
			}
			if r.Max.X > maxX {
				maxX = r.Max.X
			}
			if r.Max.Y > maxY {
				maxY = r.Max.Y
			}
		}
	}
	if first {
		return image.Rectangle{}
	}
	return image.Rect(minX-pad, minY-pad, maxX+pad, maxY+pad)
}

// computeBPMGroupRect returns the bounding rect enclosing the BPM box and
// inc/dec buttons, expanded by `pad` pixels for the visual container.
// `pad` is the active TopBarSpec.GroupOutlinePad.
func computeBPMGroupRect(bpmBox, incBtn, decBtn image.Rectangle, pad int) image.Rectangle {
	return computeGroupRect([]image.Rectangle{bpmBox, incBtn, decBtn}, pad)
}

// ensureGapBtns ensures at least `gap` pixels between two adjacent buttons.
// `gap` is the active TopBarSpec.ControlGap.
func ensureGapBtns(left, right *Button, gap int) {
	if left == nil || right == nil {
		return
	}
	lr, rr := left.Rect(), right.Rect()
	if lr.Max.X >= rr.Min.X {
		dx := lr.Max.X - rr.Min.X + gap
		rr.Min.X += dx
		rr.Max.X += dx
		right.SetRect(rr)
	}
}

// textInputHitAdapter returns InputIgnored so the legacy TextInput.Update()
// path can handle focus/blur normally. Its higher z-index ensures it is
// checked first; InputIgnored falls through to lower-z handlers via the
// tree's fall-through loop, which is correct (no side effects to block).
type textInputHitAdapter struct {
	ti *TextInput
}

func (h *textInputHitAdapter) OnPress(x, y int) InputResult        { return InputIgnored }
func (h *textInputHitAdapter) OnDrag(x, y int)                     {}
func (h *textInputHitAdapter) OnRelease(x, y int)                  {}
func (h *textInputHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// Verify interface at compile time.
var _ Zone = (*TransportZone)(nil)
