package ui

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// audio import used indirectly via callbacks (GetMainVolume/SetMainVolume).

// TransportCallbacks contains callbacks for the TransportZone to communicate
// with the DrumView and audio engine. Zones don't reference Game or each other.
type TransportCallbacks struct {
	OnPlayToggle    func()            // play/pause pressed
	OnStop          func()            // stop pressed
	OnBPMChange     func(bpm int)     // BPM committed (from text or +/-)
	OnFollowChange  func(follow bool) // track toggle
	OnUploadClick   func()            // delegates to DrumView's upload goroutine
	OnImportClick   func()            // delegates to DrumView's import picker
	OnExportClick   func()            // delegates to DrumView's export
	OnViewCycle     func()            // mobile view mode toggle
	OnRecordToggle  func()            // record button pressed
	OnUndo          func()            // undo button pressed
	OnRedo          func()            // redo button pressed
	CanUndo         func() bool       // whether the undo stack is non-empty (drives dim)
	CanRedo         func() bool       // whether the redo stack is non-empty (drives dim)
	IsPlaying       func() bool       // read current playback state
	IsRecording     func() bool       // read current recording state
	GetMainVolume   func() float64    // read master volume
	SetMainVolume   func(v float64)   // set master volume (per-frame, live)
	OnMainVolCommit func()            // fires once at master-vol drag release (emit + undo)
	OnNotifyError   func(msg string)  // display error notification

	// Overlay callbacks: delegate to DrumView's overlay mechanisms.
	OnSubdivClick    func()       // delegates to DrumView's SubdivMenuComponent
	OnOverflowOpen   func()       // delegates to DrumView's overflow menu
	OnMasterVolClick func()       // delegates to DrumView's master vol popup
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
	undoBtn        *Button
	redoBtn        *Button
	bpmDecBtn      *Button
	bpmBox         *TextInput
	paramEditor    *ParamValueEditor // shared numeric editor for BPM (tap-to-popup)
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

	// Previous layout dimensions — drives the blur-on-resize behavior so
	// the BPM box never carries a stale focus ring across orientation
	// changes or window resizes.
	prevLayoutRect image.Rectangle

	// inputBlocked returns true when a popup/overlay is open; while blocked, an
	// in-progress BPM edit is cancelled and a tap on the readout will not open
	// the editor (see blockedAtTick). Set by DrumView wiring.
	inputBlocked func() bool

	// useBottomBar tells layoutMobile that DrumView has allocated a
	// bottom action bar (B3 redesign) and will host vol-icon /
	// view-switch / overflow there — so layoutMobile should clear those
	// rects in the top toolbar and let DrumView place them. When false
	// (ultra-short viewports where the bar collapses, see
	// drumview_layout.go:49-64), layoutMobile falls back to the
	// pre-Task-1.3 two-row layout that keeps those buttons inside the
	// top toolbar so they remain reachable. DrumView sets this via
	// SetUseBottomBar before each Layout call.
	useBottomBar bool

	// barRect is the mobile bottom-action-bar bounds. Set by DrumView.calcLayout
	// after it positions the bar; consumed by hit-area clip overrides for the
	// vol/view/overflow buttons AND by the segmented view-switch (Phase 3).
	barRect image.Rectangle
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
	// Construct each button with a placeholder style; refreshProfileStyles
	// (called at the end of this method and from Layout()) is the single
	// authority for Style + IconColor, so a mobile\u2194desktop transition
	// retints these buttons in real time without relying on construction-
	// time profile sampling.
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

	// Undo / Redo render as vector icons (IconUndo / IconRedo) rather than
	// text labels: the narrow desktop transport cell (~24px) clips a
	// "Undo"/"Redo" text label down to the bare "..." ellipsis, which the
	// user mistook for a broken overflow button. IconColor is retinted each
	// frame (dim when the stack is empty) in syncUndoRedoVisual, called from
	// decayAnims/Update.
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

	z.bpmBox = NewTextInput(image.Rect(0, 0, 0, 0), BPMBoxStyle)
	z.bpmBox.MaxLen = 4
	z.bpmBox.SetText("120")
	z.bpmBox.InputMode = "numeric"
	z.bpmBox.MobileInputID = "bpm"
	z.bpmBox.OnFocusGained = func() { softKeyboardShow("numeric") }
	z.bpmBox.OnFocusLost = func() { softKeyboardHide() }
	z.paramEditor = NewParamValueEditor()

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

	z.trackBtn = NewButton("", nil, func() {
		z.SetFollow(!z.follow)
	})
	// Icon + IconColor are authoritative-set by syncTrackBtnVisual below.
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

	z.viewSwitchBtn = NewButton("", nil, func() {
		if z.callbacks.OnViewCycle != nil {
			z.callbacks.OnViewCycle()
		}
	})
	z.viewSwitchBtn.Icon = string(IconAudio)
	z.viewSwitchBtn.IconColor = colTextSecondary

	z.overflowBtn = NewButton("", nil, func() {
		if z.callbacks.OnOverflowOpen != nil {
			z.callbacks.OnOverflowOpen()
		}
	})
	z.overflowBtn.Icon = string(IconOverflow)
	z.overflowBtn.IconColor = colTextSecondary

	// Single authority for every profile-dependent Style and IconColor.
	// Called once here so the buttons are visually valid before Layout(),
	// and re-called from Layout() so a mobile↔desktop viewport transition
	// retints in real time (no construction-time sampling survives the
	// transition).
	z.refreshProfileStyles()
}

// refreshProfileStyles re-derives every Button Style + IconColor in this
// zone from the current LayoutProfile. Called from initButtons() at
// construction and from Layout() every relayout. Idempotent — safe to
// call every frame; runs in O(buttons).
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
		// recordBtn.IconColor is dynamic (idle / armed) and managed
		// elsewhere — do not overwrite it here.
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
	// Mobile second-row recede: secondary controls (view, overflow) sit
	// on surface-1 fill so the eye reads them as ancillary to the row 0
	// primary cluster (play/stop/record/bpm/subdiv on surface-2). Desktop
	// uses the profile's ViewSwitchStyle / OverflowStyle directly.
	if p.IsMobile() {
		recede := ButtonStyleFromSpec(ComponentButtonSecondary)
		recede.Fill = colSurface1
		if z.viewSwitchBtn != nil {
			z.viewSwitchBtn.Style = recede
		}
		if z.overflowBtn != nil {
			z.overflowBtn.Style = recede
		}
	} else {
		if z.viewSwitchBtn != nil {
			z.viewSwitchBtn.Style = p.ViewSwitchStyle
		}
		if z.overflowBtn != nil {
			z.overflowBtn.Style = p.OverflowStyle
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
	// Re-derive profile-dependent button styles before laying out so a
	// mobile↔desktop transition that occurred between frames is fully
	// reflected in this Layout pass.
	z.refreshProfileStyles()
	z.layoutButtons(rect)
	z.rebuildHitAreas()
}

// BlurInputs clears focus on every text input owned by the transport zone.
// Called automatically when the layout rect's size changes (resize /
// orientation flip) so a stale focus ring can't outlive a viewport
// transition. Exposed so DrumView wiring can blur on overlay open or
// other input-blocking transitions.
func (z *TransportZone) BlurInputs() {
	if z.bpmBox != nil {
		z.bpmBox.focused = false
	}
}

func (z *TransportZone) Update() {
	z.decayAnims()
	z.syncUndoRedoVisual()
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
	if z.paramEditor != nil {
		z.paramEditor.Update()
	}

	// Latch the input-blocked state at Tick time (before the RootTree dispatches
	// input this frame). A foreign-subtree dropdown that dismisses itself during
	// this frame's dispatch would otherwise read as "not blocked" by the time
	// bpmOpenAdapter.OnPress runs — so we snapshot it here, while the overlay is
	// still open, and gate the tap on the snapshot.
	blocked := z.inputBlocked != nil && z.inputBlocked()
	z.blockedAtTick = blocked

	// When input is blocked (a popup/overlay is open), cancel any in-progress
	// BPM edit so it doesn't linger behind the overlay. The readout box is
	// never focused now — editing happens entirely in the shared editor — so
	// its text stays in sync via SetBPM's existing SetText.
	if blocked {
		if z.paramEditor != nil && z.paramEditor.Active() {
			z.paramEditor.cancel()
		}
		return
	}
}

func (z *TransportZone) HitAreas() []HitArea {
	return z.hitAreas
}

// SetBarRect stores the mobile bottom-action-bar bounds. Theme 1+4: the
// bar hosts ONLY the 6-segment view switcher (DrumView registers that
// hit area separately). Vol icon and overflow live in the top toolbar
// now, so we no longer override their ClipRect to the bar — leave them
// pointing at z.rect (the top toolbar rect) so taps in the toolbar
// reach them.
func (z *TransportZone) SetBarRect(bar image.Rectangle) {
	z.barRect = bar
}

// BarRect returns the stored mobile bottom-action-bar rect (or empty on
// desktop / before Layout).
func (z *TransportZone) BarRect() image.Rectangle { return z.barRect }

func (z *TransportZone) Draw(screen *ebiten.Image) {
	if z.rect.Dy() < 8 || z.rect.Dx() < 8 {
		return
	}
	z.renderToolbarControls(screen)
	// Draw the shared BPM editor OVER the (possibly cached) toolbar, straight to
	// screen — never into the toolbar cache. Its per-keystroke text/caret/flash
	// would otherwise require hashing all of that into toolbarStateHash, and the
	// cache-hit path would never invalidate while typing. Mirrors EQPanelZone.Draw.
	if z.paramEditor != nil {
		z.paramEditor.Draw(screen)
	}
}

// HandleKey is a no-op for the transport zone. BPM editing now happens entirely
// in the shared ParamValueEditor, whose own Update() handles Enter (commit) and
// Escape (cancel); the readout box is never focused, so the tree never routes
// keys here for BPM. Method retained to satisfy the Zone interface.
func (z *TransportZone) HandleKey(key ebiten.Key) InputResult {
	return InputIgnored
}

// HandleChars is a no-op for the transport zone. See HandleKey — text entry is
// owned by the shared editor's TextInput, not the readout box.
func (z *TransportZone) HandleChars(chars []rune) InputResult {
	return InputIgnored
}

// --- Public accessors (for DrumView migration bridge) ---

// SetPlaying updates the play button icon to reflect playback state.
// On mobile, the button also swaps between PrimaryActionStyle (saturated
// accent fill — calls the user to action) when stopped and the recede
// PlayBtnStyle (surface fill, accent icon) when playing, so the resting
// state is visually dominant and the playing state recedes.
func (z *TransportZone) SetPlaying(p bool) {
	z.isPlaying = p
	prof := Profile()
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

// SetUseBottomBar declares whether DrumView is hosting the mobile
// vol-icon / view-switch / overflow buttons inside its bottom action
// bar. When true, layoutMobile clears those three rects in the top
// toolbar (DrumView places them in the bar). When false — the bar
// collapsed because the drum-pane is too short — layoutMobile falls
// back to a two-row layout that keeps the buttons inside the top
// toolbar so they remain reachable. Must be called before Layout.
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

	if Profile().IsMobile() {
		z.layoutMobile(topBounds, pad, spec)
	} else {
		z.layoutDesktop(topBounds, pad, spec)
	}
	ensureGapBtns(z.playBtn, z.stopBtn, spec.ControlGap)
}

func (z *TransportZone) layoutMobile(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	// Mobile transport (Theme 4): the top toolbar holds the transport
	// controls + vol icon + overflow kebab. The bottom action bar hosts
	// the 6-segment view switcher (Pads/EQ/Wave/Spec/Mtr/Scope) at full
	// width — placed by DrumView, not here.
	//
	// Single-row layout (preferred — when topBounds height fits one row):
	//   Play | Stop | Record | [−|BPM|+] | Subdiv | Vol | Overflow
	// Two-row fallback (when topBounds is tall enough to split):
	//   Row 0: Play | Stop | Record | [−|BPM|+] | Subdiv
	//   Row 1:                                       Vol .... Overflow
	//
	// We prefer single row whenever it would land each cell at >=
	// TouchMinTarget; otherwise fall back to 2-row to preserve touch
	// height. The legacy binary view-switch button is suppressed on
	// mobile in both layouts (segmented replaces it).
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
		// Row 1: vol-icon left, empty middle, overflow right (3-cell grid).
		row1Grid := NewGridLayout(row1Bounds, []float64{1.0, 1.0, 1.0}, []float64{1})
		z.mainVolIconRect = safeInsetTransport(row1Grid.Cell(0, 0), pad)
		if z.mainVolSlider != nil {
			z.mainVolSlider.SetRect(image.Rectangle{})
			z.mainVolRect = image.Rectangle{}
		}
		if z.overflowBtn != nil {
			z.overflowBtn.SetRect(safeInsetTransport(row1Grid.Cell(2, 0), pad))
		}
	}

	// Row 0 column weights — extend by [vol, overflow] when single-row.
	// Undo / Redo are NOT placed on the mobile primary row: at <=480px the
	// extra cells shrink the record button below the 44px touch-min (see
	// TestMobileRecordButtonNeverSliver). On mobile they live in the overflow
	// menu (TODO) + keyboard; desktop keeps them on the toolbar.
	//
	// NOTE: mobile buttons are intentionally NOT forced into equal squares
	// (unlike desktop's layoutDesktop run). The mobile transport region is
	// only ~188px wide, so seven controls at the 44px touch-min cannot be
	// square — the layout instead fills the cell HEIGHT (≥ touch-min) at a
	// narrow width and relies on ExpandHitArea for the 44px touch target.
	// There is no "spacer void" on mobile to remove. Touch-min ratchet:
	// TestMobileTransportButtons_AtTouchMin.
	weights := []float64{1.0, 1.0, 1.0, 3.0, 1.0}
	if !useTwoRow {
		weights = []float64{1.0, 1.0, 1.0, 3.0, 1.0, 1.0, 1.0}
	}
	row0Grid := NewGridLayout(row0Bounds, weights, []float64{1})

	z.playBtn.SetRect(safeInsetTransport(row0Grid.Cell(0, 0), pad))
	z.stopBtn.SetRect(safeInsetTransport(row0Grid.Cell(1, 0), pad))
	// Record is visually demoted on mobile (B12 in the screenshot
	// critique): start from the play/stop rect, then shrink symmetrically
	// by recordDemoteInsetMobile so the red dot doesn't sit at equal
	// visual weight with play/stop and invite accidental record mid-jam.
	// Hit-test still spans the full cell via the standard touch
	// expansion (ExpandHitArea).
	recordR := safeInsetTransport(row0Grid.Cell(2, 0), pad)
	// B12: visually demote record below play/stop so the red dot doesn't sit
	// at equal visual weight with the primary actions and invite accidental
	// mid-jam record. Demote by HEIGHT only: the mobile transport cells are
	// narrow (play is ~20 px wide), so insetting width too would collapse
	// record to a sliver — that was the reported ~6 px bug. A shorter record
	// button reads as lower visual weight while keeping the full cell width
	// comfortably tappable. The floor guard skips the inset rather than ever
	// shrinking the height below recordDemoteFloorPx; the hit-test still spans
	// the full cell via the standard touch expansion (ExpandHitArea).
	if dy := recordDemoteInsetMobile; recordR.Dy()-2*dy >= recordDemoteFloorPx {
		recordR = image.Rect(recordR.Min.X, recordR.Min.Y+dy, recordR.Max.X, recordR.Max.Y-dy)
	}
	z.recordBtn.SetRect(recordR)
	bpmStepperBounds := row0Grid.Cell(3, 0)
	stepperGrid := NewGridLayout(bpmStepperBounds, []float64{1.0, 2.0, 1.0}, []float64{1})
	z.bpmDecBtn.SetRect(safeInsetTransport(stepperGrid.Cell(0, 0), pad))
	z.bpmBox.Rect = safeInsetTransport(stepperGrid.Cell(1, 0), pad)
	z.bpmIncBtn.SetRect(safeInsetTransport(stepperGrid.Cell(2, 0), pad))

	// Compute BPM group container rect (covers BPM box + inc/dec arrows).
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)
	// Clamp to the stepper cell on mobile — the horizontal layout puts
	// bpmIncBtn flush against the col 3 right edge, so the pill outline
	// pad would otherwise spill into col 4 (subdiv) by `GroupOutlinePad`
	// pixels.
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

	// Single-row mode: append vol icon + overflow on row 0 (cells 5, 6).
	if !useTwoRow {
		z.mainVolIconRect = safeInsetTransport(row0Grid.Cell(5, 0), pad)
		if z.mainVolSlider != nil {
			z.mainVolSlider.SetRect(image.Rectangle{})
			z.mainVolRect = image.Rectangle{}
		}
		if z.overflowBtn != nil {
			z.overflowBtn.SetRect(safeInsetTransport(row0Grid.Cell(6, 0), pad))
		}
	}

	// Undo / Redo are hidden on the mobile primary row (no room at the 44px
	// touch-min). Zero their rects so they are not drawn or hit-registered on
	// mobile; they remain available via keyboard and (TODO) the overflow menu.
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

// transportBPMBoxUnits is the BPM text box width in square-side units. Wide
// enough to show up to 4 digits (maxBPM == 1000) — it is the only wide
// exception in the run.
const transportBPMBoxUnits = 2.0

// transportBPMStepUnits is the desktop BPM ± stepper column width (stacked
// inc/dec chevrons) in square-side units — narrower than a square, the second
// intentional exception.
const transportBPMStepUnits = 0.9

func (z *TransportZone) layoutDesktop(topBounds image.Rectangle, pad int, spec TopBarSpec) {
	// Desktop transport is a single left-aligned run of equal-size SQUARE
	// buttons with a uniform gap. Running order:
	//
	//   Play | Stop | Record | BPM box | BPM ± | Subdiv | Vol | Undo | Redo | Overflow
	//
	// The BPM box and BPM ± stepper are the only non-square exceptions. The
	// run packs at the left of the transport column; any leftover width sits
	// to the right, before the timeline begins. This replaces the former
	// weighted grid whose weight-0.6 "spacer" cell left a black void between
	// Subdiv and Vol and whose uneven weights made the buttons different
	// sizes. Regression guards: TestDesktopTransportButtonsAreEqualSquares,
	// TestDesktopTransportNoSpacerGap, TestDesktopTransportNoOverlap.
	const nGaps = 9 // 10 controls in the run → 9 inter-control gaps
	totalUnits := 8*1.0 + transportBPMBoxUnits + transportBPMStepUnits
	sq, gap, top, bot := transportRunMetrics(topBounds, pad, spec, totalUnits, nGaps)

	x := topBounds.Min.X
	square := func(b *Button) {
		if b == nil {
			return
		}
		b.SetRect(image.Rect(x, top, x+sq, bot))
		x += sq + gap
	}

	square(z.playBtn)
	square(z.stopBtn)
	square(z.recordBtn)

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

	// Desktop: icon-only volume (popup on click) occupies a square slot.
	z.mainVolIconRect = image.Rect(x, top, x+sq, bot)
	x += sq + gap
	if z.mainVolSlider != nil {
		z.mainVolSlider.SetRect(image.Rectangle{})
		z.mainVolRect = image.Rectangle{}
	}

	square(z.undoBtn)
	square(z.redoBtn)
	square(z.overflowBtn)

	// Group container pills derive from the final button rects.
	z.bpmGroupRect = computeBPMGroupRect(z.bpmBox.Rect, z.bpmIncBtn.Rect(), z.bpmDecBtn.Rect(), spec.GroupOutlinePad)
	z.transportGroupRect = computeGroupRect([]image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
	}, spec.GroupOutlinePad)
	// File-ops live behind the overflow menu; no inline group outline.
	z.fileOpsGroupRect = image.Rectangle{}

	// File ops (Upload / Import / Export) live behind the overflow "..." menu
	// on desktop — keep their rects empty so they don't draw or claim hit
	// areas inline. Their OnClick handlers fire from the overflow menu entries.
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
			Rect:   z.bpmBox.Rect,
			ZIndex: zIdx + 1,
			// The readout is display-only; tapping it opens the shared numeric
			// editor (tap-to-popup) rather than focusing the box in place.
			Handler: &bpmOpenAdapter{z: z},
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

// repeatButtonHitAdapter wraps a Repeat-enabled Button as a HitHandler.
// Delegates to Button.Handle which has proper repeat timing (initial delay,
// acceleration) instead of firing every frame.
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

	// Record button pulsing animation when recording. The icon tint swaps
	// from colRecordIdle (dim red dot) to a breathing colRecordActive so the
	// armed/recording state is unmistakable. Cadence comes from the
	// button-toggle-pulse animation token (no magic numbers); the monotonic
	// frame counter drives the phase so the breath is wall-clock smooth.
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

// bpmSpec drives the shared editor for the BPM value: integer in [1,maxBPM],
// rejecting invalid input with the same "Invalid BPM" toast as the legacy path.
func (z *TransportZone) bpmSpec() ValueSpec {
	return ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			if v, ok := parseBPM(strings.TrimSpace(s)); ok {
				return float64(v), true
			}
			if z.callbacks.OnNotifyError != nil {
				z.callbacks.OnNotifyError(i18n.T(i18n.KeyNotifInvalidBPM))
			}
			return 0, false
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
}

// openBPMEditor opens the shared editor over the BPM readout, pre-filled with
// the current tempo. Commit clamps via bpmSpec and writes through SetBPM.
func (z *TransportZone) openBPMEditor() {
	if z.paramEditor == nil {
		z.paramEditor = NewParamValueEditor()
	}
	z.paramEditor.OpenValue(ValueOpen{
		Spec:          z.bpmSpec(),
		Anchor:        z.bpmBox.Rect,
		Clamp:         z.rect,
		MobileInputID: "bpm",
		Get:           func() float64 { return float64(z.bpm) },
		Set:           func(v float64) { z.SetBPM(int(v)) },
	})
}

// bpmOpenAdapter opens the shared BPM editor on press.
type bpmOpenAdapter struct{ z *TransportZone }

func (a *bpmOpenAdapter) OnPress(x, y int) InputResult {
	// Respect the input-blocked gate: when a popup/overlay (in either subtree)
	// is open, a tap on the readout must NOT open the editor beneath it. We read
	// the snapshot taken at Tick (blockedAtTick) rather than re-evaluating now,
	// because a foreign dropdown can be dismissed earlier in this same dispatch
	// frame. Still consume the press so it doesn't leak to a touch-expanded
	// sibling control.
	if a.z.blockedAtTick || (a.z.inputBlocked != nil && a.z.inputBlocked()) {
		return InputConsumed
	}
	a.z.openBPMEditor()
	return InputConsumed
}
func (a *bpmOpenAdapter) OnDrag(x, y int)                     {}
func (a *bpmOpenAdapter) OnRelease(x, y int)                  {}
func (a *bpmOpenAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// --- Track button visual ---

// syncTrackBtnVisual mirrors the play/stop visual model: chrome (Style) stays
// fixed across states; the icon glyph and icon color carry the active/inactive
// signal. Active = primary-bright coral (colFollowActive, the single chrome
// accent per DESIGN.md); inactive = the platform-neutral BPM/secondary-control
// tint.
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

// syncUndoRedoVisual dims the undo/redo button labels when their respective
// stack is empty (CanUndo/CanRedo == false), matching the play/stop visual
// model where the icon/text color carries the active/inactive signal while
// chrome stays fixed. Uses existing theme tokens only (colTextPrimary /
// colTextDisabled) — no new color literals.
func (z *TransportZone) syncUndoRedoVisual() {
	dim := func(btn *Button, can func() bool) {
		if btn == nil {
			return
		}
		enabled := can == nil || can()
		col := colTextPrimary
		if !enabled {
			col = colTextDisabled
		}
		// Undo/Redo are icon buttons: the icon carries the active/inactive
		// signal. TextColor is kept in sync too in case a label is ever set.
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
	// Undo / Redo: hover/press + dim state (CanUndo/CanRedo drive the label
	// tint, so the toolbar cache must invalidate when a stack empties/fills).
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
	// Mobile surface hierarchy: toolbar sits on colSurface1 so the eye reads
	// a clear vertical zone separation from the background and the rows zone
	// (which uses the brighter alternating-stripe colors). Drawn first so all
	// chrome (cluster pills, buttons, hairlines) renders on top.
	if Profile().IsMobile() {
		bounds := cache.Bounds()
		drawRect(cache, bounds, colSurface1, true)
	}
	// Draw group container backgrounds (behind buttons).
	z.drawTransportGroupOffset(cache, offsetX, offsetY)
	z.drawBPMGroupOffset(cache, offsetX, offsetY)
	z.drawFileOpsGroupOffset(cache, offsetX, offsetY)

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

	z.drawToolbarSeparators(cache, offsetX, offsetY)
}

func (z *TransportZone) renderToolbarDirect(dst *ebiten.Image) {
	// Mobile surface hierarchy: see renderToolbarToCache for rationale.
	if Profile().IsMobile() && !z.rect.Empty() {
		drawRect(dst, z.rect, colSurface1, true)
	}
	// Draw group container backgrounds.
	z.drawTransportGroupOffset(dst, 0, 0)
	z.drawBPMGroupOffset(dst, 0, 0)
	z.drawFileOpsGroupOffset(dst, 0, 0)

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
	// NOTE: the shared BPM editor is drawn in (*TransportZone).Draw over the
	// composited toolbar, NOT here — it must never live inside the toolbar cache.
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
}

func (z *TransportZone) drawToolbarSeparators(cache *ebiten.Image, offsetX, offsetY int) {
	if !Profile().DrawToolbarSep {
		return
	}
	sepCol := WithAlpha(genColorBorder, genAlphaTransportSeparator)

	// Mobile: draw a horizontal hairline between transport row 0 (play/stop/
	// record/bpm/subdiv) and row 1 (vol/view/overflow) so the two-row layout
	// reads as deliberate grouping rather than collision-packed controls.
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

// drawMasterVolIconOffset renders the master volume button by delegating to
// the shared drawVolumeButton component (the same one used by per-row
// volume cells). Master differs only in channel wiring: it reads from
// mainVolSlider, has no row tint, and is never user-mutable as "muted".
func (z *TransportZone) drawMasterVolIconOffset(cache *ebiten.Image, offsetX, offsetY int) {
	r := z.mainVolIconRect
	if r.Empty() {
		return
	}
	vol := 0.0
	if z.mainVolSlider != nil {
		vol = z.mainVolSlider.Value
	}
	drawVolumeButton(cache, r.Sub(image.Pt(offsetX, offsetY)), vol, false, nil)
}

// drawBPMGroupOffset draws the BPM group visual container: a colSurface2
// rounded rect with colBorderSubtle border, plus a "BPM" caption label.
// Uses colSurface2 (one step lighter than the surface-1 toolbar background)
// so the pill is visually distinct from the surrounding chrome.
func (z *TransportZone) drawBPMGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.bpmGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colSurface2, RadiusMD, true)
	drawRoundedRect(dst, r, colBorderMedium, RadiusMD, false)

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

// drawPlayAccentOffset is a no-op; the play button's primary-action visual
// is now driven by SetPlaying swapping playBtn.Style between
// PrimaryActionStyle (stopped) and PlayBtnStyle (playing). Kept as a stub
// so the call sites in renderToolbarToCache / renderToolbarDirect remain
// stable.
func (z *TransportZone) drawPlayAccentOffset(dst *ebiten.Image, offsetX, offsetY int) {
}

// drawRecordArmedRingOffset draws a 2px destructive ring outside the record
// button when recording is armed (z.isRecording == true). Signals "armed —
// next play will record". Drawn AFTER the button so the ring sits on top of
// the button border. Mobile only — desktop already has a stronger pulsing
// indicator on the icon.
func (z *TransportZone) drawRecordArmedRingOffset(dst *ebiten.Image, offsetX, offsetY int) {
	if !Profile().IsMobile() {
		return
	}
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
	drawRoundedRect(dst, outer, genColorDestructive, RadiusMD+(-ringInset), false)
	inner := image.Rect(outer.Min.X+1, outer.Min.Y+1, outer.Max.X-1, outer.Max.Y-1)
	drawRoundedRect(dst, inner, genColorDestructive, RadiusMD+(-ringInset)-1, false)
}

// drawRecordIndicatorOffset paints a persistent "REC" caption in
// colRecordActive over the record button while recording. Unlike the
// breathing icon tint (which the cached toolbar can freeze between hash
// changes) and the non-cached pulse halo, this label is gated on the
// isRecording bool that participates in the toolbar cache hash — so the
// recording state is guaranteed to differ pixel-wise from idle the moment
// recording starts, on both desktop and mobile. The label hugs the record
// button's bottom edge, clamped inside the toolbar rect so it never spills.
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

// drawSubdivPillOffset draws a colSurface2 pill background behind the
// subdivision button on mobile. Called BEFORE subdivBtn.Draw so the
// button's own background renders on top — the button uses a
// transparent style on mobile so the pill shows through.
func (z *TransportZone) drawSubdivPillOffset(dst *ebiten.Image, offsetX, offsetY int) {
	if !Profile().IsMobile() {
		return
	}
	r := z.subdivBtn.Rect()
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	drawRoundedRect(dst, r, colSurface2, RadiusMD, true)
	drawRoundedRect(dst, r, colBorderMedium, RadiusMD, false)
}

// drawTransportGroupOffset draws the transport group pill container
// (play+stop+record) as a rounded rect. Uses colSurface2 fill on mobile
// (one step lighter than the surface-1 toolbar) so the cluster reads as
// a distinct grouping; desktop keeps the more subtle colTransportGroupBG
// since the desktop toolbar already has stronger chrome cues.
func (z *TransportZone) drawTransportGroupOffset(dst *ebiten.Image, offsetX, offsetY int) {
	gr := z.transportGroupRect
	if gr.Empty() {
		return
	}
	r := gr.Sub(image.Pt(offsetX, offsetY))
	radius := RadiusMD
	fill := color.Color(colTransportGroupBG)
	border := color.Color(colTransportGroupBorder)
	if Profile().IsMobile() {
		radius = RadiusMD + 2 // 10px for mobile
		fill = colSurface2
		border = colBorderMedium
	}
	drawRoundedRect(dst, r, fill, radius, true)
	drawRoundedRect(dst, r, border, radius, false)
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

// transportRunMetrics computes the uniform square side, inter-control gap, and
// vertical extent for a left-packed run of transport controls.
//
//   - totalUnits is the run width measured in multiples of the square side: a
//     square button is 1 unit; wider exceptions (the BPM box, the ± stepper)
//     contribute >1 / <1 units.
//   - nGaps is the number of inter-control gaps in the run.
//
// The square side equals the bar's inner height, shrunk ONLY when the run
// would otherwise overflow the available width. So a wide bar (desktop's left
// column) yields a tight left-aligned cluster with slack to the right, while a
// narrow bar (a phone) shrinks the squares to fit without ever overflowing.
//
// The gap is uniform and at least 2×GroupOutlinePad so adjacent group "pills"
// (transport, BPM) never overlap.
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

	// Center the square band vertically inside the padded area so every
	// control (squares and the wider/narrower BPM exceptions) shares one row
	// height == sq, even when sq shrank below the bar's inner height to fit.
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
	minDim := r.Dx()
	if r.Dy() < minDim {
		minDim = r.Dy()
	}
	maxPad := (minDim - 2) / 2
	if maxPad < 0 {
		maxPad = 0
	}
	minW := genGeomTransportMinBtnW
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

// recordDemoteInsetMobile is the extra inset (in px, applied on top of
// the row pad) that shrinks the mobile record button visual relative to
// play/stop. See B12 in the screenshot critique — the red dot at parity
// invites accidental mid-jam record.
const recordDemoteInsetMobile = 8

// recordDemoteFloorPx is the minimum drawn size (px) of the demoted mobile
// record button. It stays below the touch-min target (so the button reads as
// deliberately smaller than play/stop per B12) but is large enough that a
// narrow phone can never collapse the cell to a sliver — the reported ~6px
// bug. The hit-test still expands to the full touch target via ExpandHitArea.
const recordDemoteFloorPx = 36

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

// insetTransportCell converts a contiguous grid cell into its drawn button
// rect: it applies the standard vertical/large-cell inset (safeInsetTransport)
// and then guarantees a small horizontal gap so adjacent cells leave a visible
// space instead of touching at narrow widths (where safeInsetTransport's
// min-width guard otherwise refuses to inset). The gap is never taken if it
// would collapse the button, so a button always stays clickable.
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
	// Only nudge on a genuine overlap. Buttons that merely touch (left.Max ==
	// right.Min — the normal case for contiguous grid cells with no inset at
	// narrow widths) must NOT be shifted: pushing `right` rightward would wedge
	// it into the *next* control and reintroduce the overlap this guards
	// against (see TestDesktopTransportNoOverlap).
	if lr.Max.X > rr.Min.X {
		dx := lr.Max.X - rr.Min.X + gap
		rr.Min.X += dx
		rr.Max.X += dx
		right.SetRect(rr)
	}
}

// textInputHitAdapter wraps a TextInput hit area. Focus/blur/caret are driven
// by the legacy TextInput.Update() mouse poll, which runs in the tree's Phase-2
// zone Update — BEFORE Phase-3 input dispatch — so the box is already focused by
// the time OnPress fires.
//
// The `consume` flag decides what OnPress returns to the dispatcher:
//
//   - consume == false (default, EQ dB inputs): return InputIgnored so the
//     press falls through to lower-z handlers. The EQ band dB input sits at
//     zIdx+3 directly over its band's mute button (zIdx+2) and intentionally
//     lets the press through so the mute toggle still fires (regression:
//     TestEQBandMuteHoldNoMultipleToggles).
//   - consume == true (transport BPM box): return InputConsumed so the press is
//     NOT leaked into a neighbouring button's touch-EXPANDED hit rect. On mobile
//     (MinTarget=44) the Record button's expanded rect reaches across the
//     BPM-dec button into the BPM box; the old unconditional InputIgnored let a
//     BPM box tap toggle recording (regression: transport_input_isolation_test.go).
//
// Both sites register EXACT, non-Touch hit areas, so OnPress only fires when the
// cursor is inside the input's real rect — consuming there is always safe.
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
