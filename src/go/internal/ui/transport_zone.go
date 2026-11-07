package ui

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// audio import used indirectly via callbacks (GetMainVolume/SetMainVolume).

// Zones never reference Game or each other.
type TransportCallbacks struct {
	OnPlayToggle     func()                             // play/pause pressed
	OnStop           func()                             // stop pressed
	OnBPMChange      func(bpm int)                      // BPM committed (from text or +/-)
	OnBPMLive        func(bpm int)                      // BPM applied live by the value wheel (no announce/undo; the wheel commits once on close)
	OnOpenBPMWheel   func(anchor image.Rectangle)       // tap on the BPM readout: open the value wheel over anchor
	OnFollowChange   func(follow bool)                  // track toggle
	OnUploadClick    func()                             // delegates to DrumView's upload goroutine
	OnImportClick    func()                             // delegates to DrumView's import picker
	OnExportClick    func()                             // delegates to DrumView's export
	OnViewCycle      func()                             // mobile view mode toggle
	OnRecordToggle   func()                             // record button pressed
	OnUndo           func()                             // undo button pressed
	OnRedo           func()                             // redo button pressed
	CanUndo          func() bool                        // whether the undo stack is non-empty (drives dim)
	CanRedo          func() bool                        // whether the redo stack is non-empty (drives dim)
	IsPlaying        func() bool                        // read current playback state
	IsRecording      func() bool                        // read current recording state
	GetMainVolume    func() float64                     // read master volume
	SetMainVolume    func(v float64)                    // set master volume (per-frame, live)
	OnMainVolCommit  func()                             // fires once at master-vol drag release (emit + undo)
	OnNotifyErrorKey func(key i18n.Key, args ...string) // display a localized error notification (optional Tf args)

	// Overlay callbacks: delegate to DrumView's overlay mechanisms.
	OnSubdivClick    func()       // delegates to DrumView's SubdivMenuComponent
	OnMasterVolClick func()       // delegates to DrumView's master vol popup
	MasterVolPopup   *SliderPopup // forwarded to transportVolIconHitAdapter for drag-through

	// nil = always ready (bare zones in tests).
	PlaybackReady func() bool
}

type TransportZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  TransportCallbacks
	portal     *OverlayPortal // set by tree wiring

	// Transport buttons
	playBtn        *Button
	stopBtn        *Button
	recordBtn      *Button
	undoBtn        *Button
	redoBtn        *Button
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

	// Master volume
	mainVolSlider   *Slider
	mainVolGroup    *SliderGroup
	mainVolIconRect image.Rectangle
	mainVolRect     image.Rectangle

	// Group visual container rects (drawn behind button groups)
	bpmGroupRect       image.Rectangle // BPM box + inc/dec
	transportGroupRect image.Rectangle // play + stop + record
	fileOpsGroupRect   image.Rectangle // upload + import + export
	// Mobile only: there the toolbar icons are bodiless and the container IS the body. Desktop controls
	// carry their own bodies, and a container only stuck out a 2 px frame.
	groupChrome bool

	// Animation state (decayed each frame)
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64

	// BPM state
	bpm           int
	bpmDelta      int
	blockedAtTick bool // input-blocked snapshot taken at Tick (see Update)

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

	// A size change blurs the BPM box, so no stale focus ring survives a resize or rotation.
	prevLayoutRect image.Rectangle

	// While blocked, a BPM edit is cancelled and a tap on the readout opens nothing.
	inputBlocked func() bool

	// true: DrumView hosts the vol/view buttons in its bottom bar, so clear them here. false (the bar
	// collapsed on a short viewport): keep the two-row layout so they stay reachable. Set before each Layout.
	useBottomBar bool

	// Set by DrumView after it positions the bar.
	barRect image.Rectangle
}

// DrumView reads the buttons through aliases.
func NewTransportZone(cb TransportCallbacks) *TransportZone {
	z := &TransportZone{
		needLayout: true,
		callbacks:  cb,
		bpm:        120,
		// The single source of the follow state on every platform.
		follow: true,
	}
	z.initButtons()
	z.initSliders()
	return z
}

func (z *TransportZone) initButtons() {
	// refreshProfileStyles is the one authority for Style and IconColor, so a viewport switch retints live.
	z.playBtn = NewButton("", nil, func() {
		z.playPressed = true
		z.playAnim = 1
		hapticTransportTap()
		if z.callbacks.OnPlayToggle != nil {
			z.callbacks.OnPlayToggle()
		}
	})
	z.playBtn.Icon = string(IconPlay)

	z.stopBtn = NewButton("", nil, func() {
		z.stopPressed = true
		z.stopAnim = 1
		hapticTransportTap()
		if z.callbacks.OnStop != nil {
			z.callbacks.OnStop()
		}
	})
	z.stopBtn.Icon = string(IconStop)

	z.recordBtn = NewButton("", nil, func() {
		z.recordPressed = true
		z.recordAnim = 1
		hapticTransportRecord()
		if z.callbacks.OnRecordToggle != nil {
			z.callbacks.OnRecordToggle()
		}
	})
	z.recordBtn.Icon = string(IconRecord)
	z.recordBtn.IconColor = colRecordIdle

	// Icons, not text: the narrow cell clipped "Undo" to "...", which read as a broken overflow button.
	z.undoBtn = NewSpecButton("", ComponentButtonSecondary, func() {
		hapticTransportTap()
		if z.callbacks.OnUndo != nil {
			z.callbacks.OnUndo()
		}
	})
	z.undoBtn.Icon = string(IconUndo)
	z.redoBtn = NewSpecButton("", ComponentButtonSecondary, func() {
		hapticTransportTap()
		if z.callbacks.OnRedo != nil {
			z.callbacks.OnRedo()
		}
	})
	z.redoBtn.Icon = string(IconRedo)

	z.bpmDecBtn = NewButton("", nil, func() {
		z.bpmDelta--
		z.bpmDecAnim = 1
	})
	z.bpmDecBtn.Repeat = true
	z.bpmDecBtn.Icon = string(IconChevronDown)

	// Display only: a tap opens the BPM wheel.
	z.bpmBox = newReadoutBox(BPMBoxStyle)
	z.bpmBox.TextColor = colTextSecondary
	z.bpmBox.SetText("120")

	z.bpmIncBtn = NewButton("", nil, func() {
		z.bpmDelta++
		z.bpmIncAnim = 1
	})
	z.bpmIncBtn.Repeat = true
	z.bpmIncBtn.Icon = string(IconChevronUp)

	z.subdivBtn = NewButton("\u00f732", nil, func() {
		if z.callbacks.OnSubdivClick != nil {
			z.callbacks.OnSubdivClick()
		}
	})
	// Disabled and latched states still override in Button.Draw.
	z.subdivBtn.TextColor = colTextSecondary

	z.trackBtn = NewButton("", nil, func() {
		z.SetFollow(!z.follow)
	})
	// syncTrackBtnVisual sets the style, icon and color.
	z.syncTrackBtnVisual()

	// The generated button-secondary spec seeds IconColor; nothing to assign here.
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

	z.viewSwitchBtn = NewButton("", nil, func() {
		if z.callbacks.OnViewCycle != nil {
			z.callbacks.OnViewCycle()
		}
	})
	z.viewSwitchBtn.Icon = string(IconAudio)
	z.viewSwitchBtn.IconColor = colTextSecondary

	// Before Layout so the buttons start valid; Layout re-runs it so a viewport switch retints live.
	z.refreshProfileStyles()
}

// Idempotent and O(buttons): safe every frame.
func (z *TransportZone) refreshProfileStyles() {
	p := Profile()
	if z.playBtn != nil {
		z.playBtn.Style = p.PlayBtnStyle
		z.playBtn.IconColor = p.PlayIconColor
	}
	if z.stopBtn != nil {
		z.stopBtn.Style = p.StopBtnStyle
		z.stopBtn.IconColor = p.StopIconColor
	}
	if z.recordBtn != nil {
		z.recordBtn.Style = p.StopBtnStyle
		// recordBtn.IconColor (idle/armed) is managed elsewhere.
	}
	if z.bpmDecBtn != nil {
		z.bpmDecBtn.Style = p.BPMDecBtnStyle
		z.bpmDecBtn.IconColor = p.BPMIconColor
	}
	if z.bpmIncBtn != nil {
		z.bpmIncBtn.Style = p.BPMIncBtnStyle
		z.bpmIncBtn.IconColor = p.BPMIconColor
	}
	if z.subdivBtn != nil {
		z.subdivBtn.Style = p.SubdivBtnStyle
	}
	if z.trackBtn != nil {
		z.trackBtn.Style = p.TrackBtnStyle
	}
	// On mobile the view switch recedes onto surface-1, ancillary to the primary row.
	if p.IsMobile() {
		recede := ButtonStyleFromSpec(ComponentButtonSecondary)
		recede.Fill = colSurface1
		if z.viewSwitchBtn != nil {
			z.viewSwitchBtn.Style = recede
		}
	} else {
		if z.viewSwitchBtn != nil {
			z.viewSwitchBtn.Style = p.ViewSwitchStyle
		}
	}
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
	if !z.prevLayoutRect.Empty() && rect.Size() != z.prevLayoutRect.Size() {
		z.BlurInputs()
	}
	z.prevLayoutRect = rect
	z.rect = rect
	z.needLayout = false
	// So a viewport switch between frames is reflected in this pass.
	z.refreshProfileStyles()
	z.layoutButtons(rect)
	z.rebuildHitAreas()
}

// Runs on a size change so no stale focus ring outlives the transition.
func (z *TransportZone) BlurInputs() {
	if z.bpmBox != nil {
		z.bpmBox.focused = false
	}
}

func (z *TransportZone) Update() {
	z.decayAnims()
	z.refreshDesktopHover()
	z.syncUndoRedoVisual()
	if z.playBtn != nil {
		z.playBtn.Disabled = z.callbacks.PlaybackReady != nil && !z.callbacks.PlaybackReady()
	}
	// Apply accumulated BPM delta from +/- buttons.
	if z.bpmDelta != 0 {
		z.SetBPM(z.bpm + z.bpmDelta)
		z.bpmDelta = 0
	}

	// The single authority for BPM box focus, blur and commit.
	if z.bpmBox == nil {
		return
	}
	// Latched at Tick, before dispatch: a foreign dropdown that dismisses itself this frame would
	// otherwise read as unblocked by the time the readout's OnPress runs.
	blocked := z.inputBlocked != nil && z.inputBlocked()
	z.blockedAtTick = blocked

	_ = blocked // gates bpmOpenAdapter (the BPM wheel opens from the readout)
}

// The dispatcher only fires on press/drag/release, so desktop hover needs this per-frame pass.
// The toolbar cache hashes hovered, so a change re-renders the cap.
func (z *TransportZone) refreshDesktopHover() {
	if Profile().IsMobile() {
		return
	}
	mx, my := cursorPosition()
	for _, b := range []*Button{
		z.playBtn, z.stopBtn, z.recordBtn, z.undoBtn, z.redoBtn,
		z.bpmDecBtn, z.bpmIncBtn, z.subdivBtn, z.trackBtn,
		z.uploadBtn, z.importBtn, z.exportBtn, z.viewSwitchBtn,
	} {
		if b != nil {
			b.UpdateHover(mx, my)
		}
	}
}

func (z *TransportZone) HitAreas() []HitArea {
	return z.hitAreas
}

// The bar holds only the view switcher, so the toolbar buttons keep the toolbar rect as their clip.
func (z *TransportZone) SetBarRect(bar image.Rectangle) {
	z.barRect = bar
}

// Empty on desktop and before Layout.
func (z *TransportZone) BarRect() image.Rectangle { return z.barRect }

func (z *TransportZone) Draw(screen *ebiten.Image) {
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}
	// At DRAW time: Update ticks before this frame's undo step is committed, so syncing there lags a frame.
	z.syncUndoRedoVisual()
	z.renderToolbarControls(screen)
}

// A no-op: the readout is never focused, so no BPM keys come here. Kept for the Zone interface.
func (z *TransportZone) HandleKey(key ebiten.Key) InputResult {
	return InputIgnored
}

// A no-op, like HandleKey.
func (z *TransportZone) HandleChars(chars []rune) InputResult {
	return InputIgnored
}

// --- Public accessors (for DrumView migration bridge) ---

// On mobile the stopped state uses the saturated primary style so it dominates; playing recedes.
func (z *TransportZone) SetPlaying(p bool) {
	z.isPlaying = p
	// Subdivision changes are rejected while playing, so the selector reads unavailable.
	if z.subdivBtn != nil {
		z.subdivBtn.Disabled = p
	}
	prof := Profile()
	// A running transport is the play key's active state.
	z.playBtn.SetToggled(p)
	// DESIGN.md §0/§5: icon-only button — never raw Unicode in Text.
	z.playBtn.Text = ""
	if p {
		z.playBtn.Icon = string(IconPause)
		// Playing state uses the accent so the eye knows the transport is live.
		z.playBtn.IconColor = colAccent
		if prof.IsMobile() {
			z.playBtn.Style = prof.PlayBtnStyle
		}
	} else {
		z.playBtn.Icon = string(IconPlay)
		if prof.IsMobile() {
			z.playBtn.Style = PrimaryActionStyle
			z.playBtn.IconColor = colTextPrimary
		} else {
			z.playBtn.IconColor = prof.PlayIconColor
		}
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

// Not announced: the wheel commits once on close, so a scroll is one log line and one undo step.
func (z *TransportZone) SetBPMLive(b int) {
	if b < 1 {
		b = 1
	} else if b > maxBPM {
		b = maxBPM
	}
	z.bpm = b
	if z.bpmBox != nil && !z.bpmBox.Focused() {
		z.bpmBox.SetText(strconv.Itoa(z.bpm))
	}
	if z.callbacks.OnBPMLive != nil {
		z.callbacks.OnBPMLive(z.bpm)
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

// Call before Layout.
func (z *TransportZone) SetUseBottomBar(use bool) {
	if z.useBottomBar != use {
		z.useBottomBar = use
		z.needLayout = true
	}
}

// --- Layout ---

func (z *TransportZone) layoutButtons(topBounds image.Rectangle) {
	spec := ActiveTopBarSpec()
	pad := spec.Padding
	if pad > topBounds.Dy()/4 {
		pad = topBounds.Dy() / 4
	}

	z.groupChrome = Profile().IsMobile()
	if z.groupChrome {
		z.layoutMobile(topBounds, pad, spec)
	} else {
		z.layoutDesktop(topBounds, pad, spec)
	}
	ensureGapBtns(z.playBtn, z.stopBtn, spec.ControlGap)
}

func (z *TransportZone) layoutMobile(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	// One row when every cell still gets TouchMinTarget, otherwise two rows. The binary view-switch
	// is suppressed on mobile; the segmented control replaces it.
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.SetRect(image.Rectangle{})
	}

	row0Bounds := topBounds
	useTwoRow := topBounds.Dy() >= 2*TouchMinTarget()
	var row1Bounds image.Rectangle
	if useTwoRow {
		outerGrid := NewGridLayout(topBounds, []float64{1}, []float64{1, 1})
		row0Bounds = outerGrid.Cell(0, 0)
		row1Bounds = outerGrid.Cell(0, 1)
	}

	if useTwoRow {
		row1Grid := NewGridLayout(row1Bounds, []float64{1.0}, []float64{1})
		z.mainVolIconRect = safeInsetTransport(row1Grid.Cell(0, 0), pad)
		if z.mainVolSlider != nil {
			z.mainVolSlider.SetRect(image.Rectangle{})
			z.mainVolRect = image.Rectangle{}
		}
	}

	// No Undo/Redo on the mobile row: at <= 480 px they would shrink Record below the touch minimum.
	// Mobile buttons are not forced square: they fill the cell height and rely on ExpandHitArea.
	weights := []float64{1.0, 1.0, 1.0, 3.0, 1.0}
	if !useTwoRow {
		weights = []float64{1.0, 1.0, 1.0, 3.0, 1.0, 1.0}
	}
	row0Grid := NewGridLayout(row0Bounds, weights, []float64{1})

	z.playBtn.SetRect(safeInsetTransport(row0Grid.Cell(0, 0), pad))
	z.stopBtn.SetRect(safeInsetTransport(row0Grid.Cell(1, 0), pad))
	// Same cell treatment as play and stop: tone, not a shrunk geometry, keeps it from inviting an
	// accidental record, and shrinking it broke cluster alignment.
	z.recordBtn.SetRect(safeInsetTransport(row0Grid.Cell(2, 0), pad))
	bpmStepperBounds := row0Grid.Cell(3, 0)
	stepperGrid := NewGridLayout(bpmStepperBounds, []float64{1.0, 2.0, 1.0}, []float64{1})
	z.bpmDecBtn.SetRect(safeInsetTransport(stepperGrid.Cell(0, 0), pad))
	z.bpmBox.Rect = safeInsetTransport(stepperGrid.Cell(1, 0), pad)
	z.bpmIncBtn.SetRect(safeInsetTransport(stepperGrid.Cell(2, 0), pad))

	// Compute BPM group container rect (covers BPM box + inc/dec arrows).
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)
	// Clamped, or the pill outline spills into the subdiv column.
	if z.bpmGroupRect.Min.X < bpmStepperBounds.Min.X {
		z.bpmGroupRect.Min.X = bpmStepperBounds.Min.X
	}
	if z.bpmGroupRect.Max.X > bpmStepperBounds.Max.X {
		z.bpmGroupRect.Max.X = bpmStepperBounds.Max.X
	}

	// Compute transport group container rect (covers play + stop + record on mobile).
	z.transportGroupRect = computeGroupRect([]image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
	}, spec.GroupOutlinePad)

	z.subdivBtn.SetRect(safeInsetTransport(row0Grid.Cell(4, 0), pad))

	if !useTwoRow {
		z.mainVolIconRect = safeInsetTransport(row0Grid.Cell(5, 0), pad)
		if z.mainVolSlider != nil {
			z.mainVolSlider.SetRect(image.Rectangle{})
			z.mainVolRect = image.Rectangle{}
		}
	}

	// Zeroed on mobile (no room at the touch minimum); the keyboard still has them.
	if z.undoBtn != nil {
		z.undoBtn.SetRect(image.Rectangle{})
	}
	if z.redoBtn != nil {
		z.redoBtn.SetRect(image.Rectangle{})
	}

	// Hide desktop-only buttons on mobile.
	z.trackBtn.SetRect(image.Rectangle{})
	z.uploadBtn.SetRect(image.Rectangle{})
	z.importBtn.SetRect(image.Rectangle{})
	z.exportBtn.SetRect(image.Rectangle{})
	z.fileOpsGroupRect = image.Rectangle{} // no file-ops on mobile
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.SetRect(image.Rectangle{})
	}
}

// Fits 4 digits; the run's only wide exception.
const transportBPMBoxUnits = 2.0

// Narrower than a square, the second exception.
const transportBPMStepUnits = 0.9

func (z *TransportZone) layoutDesktop(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	// One left-aligned run of equal squares with uniform gaps (the BPM box and stepper are the only
	// exceptions), in four groups separated by spacing.cluster: transport | tempo | level | history.
	const nGaps = 8 // 9 controls in the run → 8 inter-control gaps (Nav + Overflow removed)
	const nClusterGaps = 3
	totalUnits := 7*1.0 + transportBPMBoxUnits + transportBPMStepUnits
	runBounds := topBounds
	_, baseGap, _, _ := transportRunMetrics(topBounds, pad, spec, totalUnits, nGaps)
	clusterExtra := 0
	if SpaceCluster > baseGap {
		clusterExtra = SpaceCluster - baseGap
	}
	runBounds.Max.X -= nClusterGaps * clusterExtra
	sq, gap, top, bot := transportRunMetrics(runBounds, pad, spec, totalUnits, nGaps)

	x := topBounds.Min.X
	square := func(b *Button) {
		if b == nil {
			return
		}
		b.SetRect(image.Rect(x, top, x+sq, bot))
		x += sq + gap
	}
	groupBreak := func() { x += clusterExtra }

	square(z.playBtn)
	square(z.stopBtn)
	square(z.recordBtn)
	groupBreak()

	bpmW := int(transportBPMBoxUnits * float64(sq))
	z.bpmBox.Rect = image.Rect(x, top, x+bpmW, bot)
	x += bpmW + gap

	stepW := int(transportBPMStepUnits * float64(sq))
	if stepW < 1 {
		stepW = 1
	}
	stackVerticalTransport(z.bpmIncBtn, z.bpmDecBtn, image.Rect(x, top, x+stepW, bot), topBounds)
	x += stepW + gap

	square(z.subdivBtn)
	groupBreak()

	// Desktop: icon-only volume (popup on click) occupies a square slot.
	z.mainVolIconRect = image.Rect(x, top, x+sq, bot)
	x += sq + gap
	if z.mainVolSlider != nil {
		z.mainVolSlider.SetRect(image.Rectangle{})
		z.mainVolRect = image.Rectangle{}
	}
	groupBreak()

	square(z.undoBtn)
	square(z.redoBtn)

	// Group container pills derive from the final button rects.
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)
	z.transportGroupRect = computeGroupRect([]image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
	}, spec.GroupOutlinePad)
	// File-ops live behind the overflow menu; no inline group outline.
	z.fileOpsGroupRect = image.Rectangle{}

	// Desktop keeps the file ops in the overflow menu, so their rects stay empty.
	z.uploadBtn.SetRect(image.Rectangle{})
	z.importBtn.SetRect(image.Rectangle{})
	z.exportBtn.SetRect(image.Rectangle{})
	// Track button positioned in timeline area, not toolbar.
	z.trackBtn.SetRect(image.Rectangle{})
	// Hide mobile-only buttons on desktop.
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.SetRect(image.Rectangle{})
	}
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.SetRect(image.Rectangle{})
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
		{z.undoBtn, "transport-undo", true},
		{z.redoBtn, "transport-redo", true},
		{z.subdivBtn, "transport-subdiv", true},
		{z.trackBtn, "transport-track", true},
		{z.uploadBtn, "transport-upload", true},
		{z.importBtn, "transport-import", true},
		{z.exportBtn, "transport-export", true},
		{z.eqToggleMobile, "transport-eq-toggle", false},
		{z.viewSwitchBtn, "transport-view-switch", true},
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

	// repeatButtonHitAdapter: its OnDrag re-invokes HandleInputResult every held frame, which is what
	// drives the repeat acceleration (buttonHitAdapter's OnDrag is a no-op).
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

	// Higher z, so it beats the touch-expanded +/- rects.
	if z.bpmBox != nil && !z.bpmBox.Rect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:   z.bpmBox.Rect,
			ZIndex: zIdx + 1,
			// Display only: a tap opens the shared editor.
			Handler: &bpmOpenAdapter{z: z},
			Tag:     "transport-bpm-box",
		})
	}

	// z+1 so sliders win; clicks off the track still reach the buttons.
	if z.mainVolGroup != nil && z.mainVolSlider != nil && !z.mainVolSlider.Rect().Empty() {
		groupBounds := z.mainVolGroup.InputBounds()
		if !groupBounds.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:   groupBounds,
				ZIndex: zIdx + 1,
				Handler: &sliderGroupHitAdapter{group: z.mainVolGroup, onRelease: func() {
					if z.callbacks.OnMainVolCommit != nil {
						z.callbacks.OnMainVolCommit()
					}
				}},
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

// Through Button.Handle, which has proper repeat timing instead of firing every frame.
type repeatButtonHitAdapter struct {
	btn *Button
}

func (h *repeatButtonHitAdapter) OnPress(x, y int) InputResult {
	h.btn.HandleInputResult(x, y, true)
	return InputCaptured
}

func (h *repeatButtonHitAdapter) OnDrag(x, y int) {
	h.btn.HandleInputResult(x, y, true)
}

func (h *repeatButtonHitAdapter) OnRelease(x, y int) {
	h.btn.HandleInputResult(x, y, false)
}

func (h *repeatButtonHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// InputCaptured, so tap-and-slide works in one gesture.
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

	// A breathing tint from the animation token, phased by the monotonic frame counter.
	if z.isRecording {
		z.recordPulse++
		a := SinPulseAlpha(int64(z.recordPulse), genAnimButtonTogglePulse)
		z.recordBtn.IconColor = WithAlpha(colRecordActive, a)
	} else {
		z.recordPulse = 0
		z.recordBtn.IconColor = colRecordIdle
	}
}

// SetInputBlocked sets the callback used to check if BPM input should be blocked.
func (z *TransportZone) SetInputBlocked(fn func() bool) {
	z.inputBlocked = fn
}

// BPMDelta returns the current accumulated BPM delta.
func (z *TransportZone) BPMDelta() int { return z.bpmDelta }

// SetBPMDelta sets the accumulated BPM delta (used for test compatibility during migration).
func (z *TransportZone) SetBPMDelta(v int) { z.bpmDelta = v }

// bpmOpenAdapter opens the BPM value wheel on press.
type bpmOpenAdapter struct{ z *TransportZone }

func (a *bpmOpenAdapter) OnPress(x, y int) InputResult {
	// Read the Tick snapshot: a foreign dropdown can be dismissed earlier this frame. Still consume, so
	// the press cannot leak to an expanded sibling.
	if a.z.blockedAtTick || (a.z.inputBlocked != nil && a.z.inputBlocked()) {
		return InputConsumed
	}
	if a.z.callbacks.OnOpenBPMWheel != nil && a.z.bpmBox != nil {
		a.z.callbacks.OnOpenBPMWheel(a.z.bpmBox.Rect)
	}
	return InputConsumed
}
func (a *bpmOpenAdapter) OnDrag(x, y int)                     {}
func (a *bpmOpenAdapter) OnRelease(x, y int)                  {}
func (a *bpmOpenAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Track button visual ---

// Chrome stays fixed; the glyph and its color carry the state (accent when locked, muted when free).
func (z *TransportZone) syncTrackBtnVisual() {
	if z.trackBtn == nil {
		return
	}
	// Active via SetToggled, not pressed: ButtonStyle draws pressed as the press dim, which made a
	// latched lock a near-black square.
	z.trackBtn.Style = Profile().TrackBtnStyle
	z.trackBtn.SetToggled(z.follow)
	if z.follow {
		z.trackBtn.Icon = string(IconTrack)
		z.trackBtn.IconColor = colAccent
	} else {
		z.trackBtn.Icon = string(IconTrackOff)
		z.trackBtn.IconColor = colTextSecondary
	}
}

// The icon color carries the state while the chrome stays fixed.
func (z *TransportZone) syncUndoRedoVisual() {
	dim := func(btn *Button, can func() bool) {
		if btn == nil {
			return
		}
		enabled := can == nil || can()
		col := colTextSecondary
		if !enabled {
			col = colTextDisabled
		}
		// TextColor kept in sync in case a label is ever set.
		btn.IconColor = col
		btn.TextColor = col
	}
	dim(z.undoBtn, z.callbacks.CanUndo)
	dim(z.redoBtn, z.callbacks.CanRedo)
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
	// The cache bakes every key's colors, so a theme change must miss.
	mix(uint64(ThemeGeneration()))
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
	if z.recordBtn != nil {
		mix(boolBit(z.recordBtn.hovered))
		mix(boolBit(z.recordBtn.pressed))
	}
	mix(boolBit(z.bpmDecBtn.hovered))
	mix(boolBit(z.bpmDecBtn.pressed))
	mix(boolBit(z.bpmIncBtn.hovered))
	mix(boolBit(z.bpmIncBtn.pressed))
	mix(boolBit(z.subdivBtn.hovered))
	mix(boolBit(z.subdivBtn.pressed))
	mix(boolBit(z.trackBtn.hovered))
	mix(boolBit(z.trackBtn.pressed))
	mix(boolBit(z.trackBtn.toggled))
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
	// The readiness gate flips Disabled independently of every other hashed field, so without this the
	// stale enabled-looking sprite hides a really disabled button.
	mix(boolBit(z.playBtn.Disabled))
	if z.eqToggleMobile != nil {
		for _, r := range z.eqToggleMobile.Text {
			mix(uint64(r))
		}
		mix(boolBit(z.eqToggleMobile.hovered))
		mix(boolBit(z.eqToggleMobile.pressed))
	}
	if z.viewSwitchBtn != nil {
		for _, r := range z.viewSwitchBtn.Icon {
			mix(uint64(r))
		}
		mix(boolBit(z.viewSwitchBtn.hovered))
		mix(boolBit(z.viewSwitchBtn.pressed))
	}
	// CanUndo/CanRedo drive the tint, so the cache must invalidate when a stack changes.
	if z.undoBtn != nil {
		mix(boolBit(z.undoBtn.hovered))
		mix(boolBit(z.undoBtn.pressed))
		mix(boolBit(z.callbacks.CanUndo != nil && z.callbacks.CanUndo()))
	}
	if z.redoBtn != nil {
		mix(boolBit(z.redoBtn.hovered))
		mix(boolBit(z.redoBtn.pressed))
		mix(boolBit(z.callbacks.CanRedo != nil && z.callbacks.CanRedo()))
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
	if z.undoBtn != nil {
		expand(z.undoBtn.Rect())
	}
	if z.redoBtn != nil {
		expand(z.redoBtn.Rect())
	}
	if z.eqToggleMobile != nil {
		expand(z.eqToggleMobile.Rect())
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
		releaseImage(z.toolbarCache)
		z.toolbarCache = newTrackedImage("transportZone.toolbarCache", w, h)
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
	// Drawn first, so all chrome renders on top.
	if Profile().IsMobile() {
		bounds := cache.Bounds()
		drawRect(cache, bounds, colTransportSurface, true)
	}
	// Mobile only (groupChrome).
	if z.groupChrome {
		z.drawTransportGroupOffset(cache, offsetX, offsetY)
		z.drawBPMGroupOffset(cache, offsetX, offsetY)
		z.drawFileOpsGroupOffset(cache, offsetX, offsetY)
	}

	z.drawPlayAccentOffset(cache, offsetX, offsetY)
	drawBtnOff(cache, z.playBtn, offsetX, offsetY)
	drawBtnOff(cache, z.stopBtn, offsetX, offsetY)
	drawBtnOff(cache, z.recordBtn, offsetX, offsetY)
	z.drawRecordArmedRingOffset(cache, offsetX, offsetY)
	z.drawRecordIndicatorOffset(cache, offsetX, offsetY)
	drawBtnOff(cache, z.bpmDecBtn, offsetX, offsetY)
	drawTIOffset(cache, z.bpmBox, offsetX, offsetY)
	if z.bpmErrorAnim > 0 {
		r := z.bpmBox.Rect.Sub(image.Pt(offsetX, offsetY))
		drawRect(cache, r, fadeColor(colError, z.bpmErrorAnim), false)
	}
	drawBtnOff(cache, z.bpmIncBtn, offsetX, offsetY)
	z.drawSubdivPillOffset(cache, offsetX, offsetY)
	drawBtnOff(cache, z.subdivBtn, offsetX, offsetY)
	drawBtnOff(cache, z.trackBtn, offsetX, offsetY)
	drawBtnOff(cache, z.uploadBtn, offsetX, offsetY)
	drawBtnOff(cache, z.importBtn, offsetX, offsetY)
	drawBtnOff(cache, z.exportBtn, offsetX, offsetY)
	if z.undoBtn != nil {
		drawBtnOff(cache, z.undoBtn, offsetX, offsetY)
	}
	if z.redoBtn != nil {
		drawBtnOff(cache, z.redoBtn, offsetX, offsetY)
	}
	if z.eqToggleMobile != nil {
		drawBtnOff(cache, z.eqToggleMobile, offsetX, offsetY)
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

	z.drawToolbarSeparators(cache, offsetX, offsetY)
}

func (z *TransportZone) renderToolbarDirect(dst *ebiten.Image) {
	// Mobile surface hierarchy: see renderToolbarToCache for rationale.
	if Profile().IsMobile() && !z.rect.Empty() {
		drawRect(dst, z.rect, colTransportSurface, true)
	}
	// Draw group container backgrounds — mobile only, see groupChrome.
	if z.groupChrome {
		z.drawTransportGroupOffset(dst, 0, 0)
		z.drawBPMGroupOffset(dst, 0, 0)
		z.drawFileOpsGroupOffset(dst, 0, 0)
	}

	z.drawPlayAccentOffset(dst, 0, 0)
	z.playBtn.Draw(dst)
	z.stopBtn.Draw(dst)
	z.recordBtn.Draw(dst)
	z.drawRecordArmedRingOffset(dst, 0, 0)
	z.drawRecordIndicatorOffset(dst, 0, 0)
	z.bpmDecBtn.Draw(dst)
	z.bpmBox.Draw(dst)
	if z.bpmErrorAnim > 0 {
		drawRect(dst, z.bpmBox.Rect, fadeColor(colError, z.bpmErrorAnim), false)
	}
	// The shared BPM editor is drawn over the composited toolbar, never inside the cache.
	z.bpmIncBtn.Draw(dst)
	z.drawSubdivPillOffset(dst, 0, 0)
	z.subdivBtn.Draw(dst)
	z.trackBtn.Draw(dst)
	z.uploadBtn.Draw(dst)
	z.importBtn.Draw(dst)
	z.exportBtn.Draw(dst)
	if z.undoBtn != nil {
		z.undoBtn.Draw(dst)
	}
	if z.redoBtn != nil {
		z.redoBtn.Draw(dst)
	}
	if z.eqToggleMobile != nil {
		z.eqToggleMobile.Draw(dst)
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
}

func (z *TransportZone) drawToolbarSeparators(cache *ebiten.Image, offsetX, offsetY int) {
	if !Profile().DrawToolbarSep {
		return
	}
	sepCol := WithAlpha(genColorBorder, genAlphaTransportSeparator)

	// So the two rows read as deliberate grouping.
	if Profile().IsMobile() {
		row0 := z.playBtn.Rect()
		row1 := z.mainVolIconRect
		if z.viewSwitchBtn != nil && row1.Empty() {
			row1 = z.viewSwitchBtn.Rect()
		}
		if !row0.Empty() && !row1.Empty() && row0.Max.Y < row1.Min.Y {
			y := (row0.Max.Y + row1.Min.Y) / 2
			drawRect(cache, image.Rect(z.rect.Min.X-offsetX, y-offsetY,
				z.rect.Max.X-offsetX, y-offsetY+1), sepCol, true)
		}
	}

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

// The same component as the per-row volume; only the wiring differs.
func (z *TransportZone) drawMasterVolIconOffset(cache *ebiten.Image, offsetX, offsetY int) {
	r := z.mainVolIconRect
	if r.Empty() {
		return
	}
	vol := 0.0
	if z.mainVolSlider != nil {
		vol = z.mainVolSlider.Value
	}
	// nil tint keeps master identical to a row speaker on the same path.
	drawVolumeButton(cache, r.Sub(image.Pt(offsetX, offsetY)), vol, false, nil)
}

// surface-2, one step above the toolbar, so the pill stands out.
func (z *TransportZone) drawBPMGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.bpmGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colSurface2, RadiusContainer, true)

	// Draw "BPM" caption label to the left of the BPM value box.
	captionScale := FontSizeCaption / FontSizeBody
	label := "BPM"
	labelW := int(float64(TextWidth(label)) * captionScale)
	labelH := int(float64(TextHeight()) * captionScale)
	bpmBoxR := z.bpmBox.Rect.Sub(image.Pt(offsetX, offsetY))
	lx := bpmBoxR.Min.X - labelW - 4
	if lx < r.Min.X+2 {
		lx = r.Min.X + 2
	}
	ly := r.Min.Y + (r.Dy()-labelH)/2
	DrawTextColorAtScale(dst, label, lx, ly, colTextSecondary, captionScale)
}

// A no-op kept for its call sites; SetPlaying swaps the style instead.
func (z *TransportZone) drawPlayAccentOffset(dst *ebiten.Image, offsetX, offsetY int) {
}

// After the button so the ring sits on its border; on both platforms.
func (z *TransportZone) drawRecordArmedRingOffset(dst *ebiten.Image, offsetX, offsetY int) {
	if !z.isRecording {
		return
	}
	r := z.recordBtn.Rect()
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	const ringInset = -2
	outer := image.Rect(r.Min.X+ringInset, r.Min.Y+ringInset, r.Max.X-ringInset, r.Max.Y-ringInset)
	drawRoundedRect(dst, outer, genColorDestructive, RadiusControl+(-ringInset), false)
	inner := image.Rect(outer.Min.X+1, outer.Min.Y+1, outer.Max.X-1, outer.Max.Y-1)
	drawRoundedRect(dst, inner, genColorDestructive, RadiusControl+(-ringInset)-1, false)
}

// Gated on isRecording, which is in the cache hash, so recording differs pixel-wise the moment it
// starts (the breathing tint can freeze between hash changes).
func (z *TransportZone) drawRecordIndicatorOffset(dst *ebiten.Image, offsetX, offsetY int) {
	if !z.isRecording {
		return
	}
	rb := z.recordBtn.Rect()
	if rb.Empty() {
		return
	}
	const label = "REC"
	captionScale := FontSizeCaption / FontSizeBody
	labelW := int(float64(TextWidth(label)) * captionScale)
	labelH := int(float64(TextHeight()) * captionScale)
	// Center horizontally on the record button.
	lx := rb.Min.X + (rb.Dx()-labelW)/2
	// Sit the caption just below the button, clamped to the toolbar rect.
	ly := rb.Max.Y - labelH
	if !z.rect.Empty() {
		if ly+labelH > z.rect.Max.Y {
			ly = z.rect.Max.Y - labelH
		}
		if ly < z.rect.Min.Y {
			ly = z.rect.Min.Y
		}
		if lx < z.rect.Min.X {
			lx = z.rect.Min.X
		}
	}
	DrawTextColorAtScale(dst, label, lx-offsetX, ly-offsetY, colRecordActive, captionScale)
}

// Before the button, whose transparent mobile style lets the pill show through.
func (z *TransportZone) drawSubdivPillOffset(dst *ebiten.Image, offsetX, offsetY int) {
	if !Profile().IsMobile() {
		return
	}
	r := z.subdivBtn.Rect()
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colSurface2, RadiusControl, true)
}

// Translucent, so it reads on either toolbar surface; identical on both platforms.
func (z *TransportZone) drawTransportGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.transportGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colTransportGroupBG, RadiusContainer, true)
}

func (z *TransportZone) drawFileOpsGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.fileOpsGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colTransportGroupBG, RadiusContainer, true)
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

// Squares match the bar's inner height, shrinking ONLY when the run would overflow; totalUnits counts
// the run in square sides. The gap is at least 2×GroupOutlinePad so group pills never overlap.
func transportRunMetrics(topBounds image.Rectangle, pad int, spec TopBarSpec, totalUnits float64, nGaps int) (sq, gap, top, bot int) {
	innerTop := topBounds.Min.Y + pad
	innerBot := topBounds.Max.Y - pad
	if innerBot <= innerTop {
		innerTop, innerBot = topBounds.Min.Y, topBounds.Max.Y
	}
	maxSq := innerBot - innerTop

	gap = spec.ControlGap
	if g2 := 2 * spec.GroupOutlinePad; gap < g2 {
		gap = g2
	}

	usable := float64(topBounds.Dx() - nGaps*gap)
	if usable < 1 {
		usable = 1
	}
	if totalUnits < 1 {
		totalUnits = 1
	}
	fit := int(usable / totalUnits)
	sq = maxSq
	if fit < sq {
		sq = fit
	}
	if sq < 1 {
		sq = 1
	}

	// Every control shares one row height even when the squares shrank.
	cy := (innerTop + innerBot) / 2
	top = cy - sq/2
	bot = top + sq
	return sq, gap, top, bot
}

// safeInsetTransport insets a rect by pad, clamped to keep minimum usable size.
func safeInsetTransport(r image.Rectangle, pad int) image.Rectangle {
	if r.Empty() {
		return r
	}
	// Each axis INDEPENDENTLY: grid cells differ by a pixel in width, and a shared budget then changed
	// the vertical inset too, so siblings rendered at different heights.
	padX := clampInsetAxis(pad, r.Dx(), genGeomTransportMinBtnW)
	padY := clampInsetAxis(pad, r.Dy(), debugCharH+2)
	return image.Rect(r.Min.X+padX, r.Min.Y+padY, r.Max.X-padX, r.Max.Y-padY)
}

// Keeps at least minExtent of content and never below 2 px.
func clampInsetAxis(pad, extent, minExtent int) int {
	maxPad := (extent - minExtent) / 2
	if hard := (extent - 2) / 2; hard < maxPad {
		maxPad = hard
	}
	if maxPad < 0 {
		maxPad = 0
	}
	if pad > maxPad {
		return maxPad
	}
	return pad
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

// A small horizontal gap so cells never touch at narrow widths, never taken if it would collapse
// the button.
func insetTransportCell(cell image.Rectangle, pad, gap int) image.Rectangle {
	r := safeInsetTransport(cell, pad)
	g := gap / 2
	if g < 1 {
		g = 1
	}
	if r.Dx() > 2*g+6 {
		r = image.Rect(r.Min.X+g, r.Min.Y, r.Max.X-g, r.Max.Y)
	}
	return r
}

// Empty rects are ignored.
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

func computeBPMGroupRect(bpmBox, incBtn, decBtn image.Rectangle, pad int) image.Rectangle {
	return computeGroupRect([]image.Rectangle{bpmBox, incBtn, decBtn}, pad)
}

func ensureGapBtns(left, right *Button, gap int) {
	if left == nil || right == nil {
		return
	}
	lr, rr := left.Rect(), right.Rect()
	// Only on a real overlap: shifting buttons that merely touch would wedge them into the next control.
	if lr.Max.X > rr.Min.X {
		dx := lr.Max.X - rr.Min.X + gap
		rr.Min.X += dx
		rr.Max.X += dx
		right.SetRect(rr)
	}
}

// TextInput.Update focuses the box before dispatch. consume=false lets a press fall through (an EQ
// dB input sits over its mute button); consume=true stops it leaking into a neighbour's expanded rect
// (a BPM tap toggled recording). Both register exact, non-Touch areas, so consuming is safe.
type textInputHitAdapter struct {
	ti      *TextInput
	consume bool
}

func (h *textInputHitAdapter) OnPress(x, y int) InputResult {
	if h.consume {
		return InputConsumed
	}
	return InputIgnored
}
func (h *textInputHitAdapter) OnDrag(x, y int)                     {}
func (h *textInputHitAdapter) OnRelease(x, y int)                  {}
func (h *textInputHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// Verify interface at compile time.
var _ Zone = (*TransportZone)(nil)
