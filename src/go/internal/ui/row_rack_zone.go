package ui

import (
	"image"
	"image/color"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// hasActiveEffects reports whether the slot list contains any enabled effect.
// The FX button highlights only when at least one slot is enabled — disabled
// slots do not contribute to the audio chain and must not light the button.
func hasActiveEffects(slots []audio.EffectSlot) bool {
	for _, s := range slots {
		if s.Enabled {
			return true
		}
	}
	return false
}

// activeEffectsCount returns the number of enabled effect slots.
func activeEffectsCount(slots []audio.EffectSlot) int {
	n := 0
	for _, s := range slots {
		if s.Enabled {
			n++
		}
	}
	return n
}

// drawFXBadge overlays a small accent badge on the top-right of the FX button
// when one or more effects are enabled. Single-effect rows show only the dot;
// multi-effect rows show a numeric count rendered in caption-size text. The
// badge replaces the previous icon-swap convention so the FX button keeps a
// consistent identity (IconFx) across states.
func drawFXBadge(dst *ebiten.Image, btnRect image.Rectangle, count int) {
	if count <= 0 || btnRect.Empty() {
		return
	}
	radius := 5
	if count > 1 {
		radius = 7
	}
	cx := btnRect.Max.X - radius - 1
	cy := btnRect.Min.Y + radius + 1
	r := image.Rect(cx-radius, cy-radius, cx+radius, cy+radius)
	drawRoundedRect(dst, r, colAccentBright, radius, true)
	if count > 1 {
		label := strconv.Itoa(count)
		scale := FontSizeCaption / FontSizeBody
		w := int(float64(TextWidth(label)) * scale)
		h := int(float64(TextHeight()) * scale)
		tx := cx - w/2
		ty := cy - h/2
		DrawTextColorAtScale(dst, label, tx, ty, colTextPrimary, scale)
	}
}

// addRowButtonStyle is a ButtonVisual for the add-row "+" button.
// It draws a rounded rect with colSurface1 fill and a dashed colBorderMedium border.
type addRowButtonStyle struct{}

// addRowBtnKey identifies a cached add-row button sprite.
type addRowBtnKey struct {
	w, h    int
	pressed bool
	hovered bool
}

var addRowBtnCache = map[addRowBtnKey]*ebiten.Image{}

func (addRowButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	if r.Empty() {
		return
	}
	k := addRowBtnKey{w: r.Dx(), h: r.Dy(), pressed: pressed, hovered: hovered}
	if spr, ok := addRowBtnCache[k]; ok {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(spr, &op)
		return
	}
	var fill color.Color = colSurface1
	if pressed {
		fill = adjustColor(fill, -20)
	} else if hovered {
		fill = adjustColor(fill, 12)
	}
	spr := ebiten.NewImage(r.Dx(), r.Dy())
	zr := image.Rect(0, 0, r.Dx(), r.Dy())
	drawRoundedRect(spr, zr, fill, RadiusMD, true)
	drawDashedRoundedBorder(spr, zr, colBorderMedium, RadiusMD)
	addRowBtnCache[k] = spr
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(spr, &op)
}

// drawDashedRoundedBorder draws a dashed border (4px on, 4px off) on a rounded rect.
// It draws straight edges with dashes and solid rounded corners.
func drawDashedRoundedBorder(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int) {
	if r.Empty() {
		return
	}
	maxR := minI(r.Dx(), r.Dy()) / 2
	if radius > maxR {
		radius = maxR
	}
	if radius < 1 {
		radius = 0
	}
	const dashOn = 4
	const dashOff = 4
	dashCycle := dashOn + dashOff

	// Top edge (dashed)
	for x := r.Min.X + radius; x < r.Max.X-radius; x++ {
		if (x-r.Min.X-radius)%dashCycle < dashOn {
			drawRect(dst, image.Rect(x, r.Min.Y, x+1, r.Min.Y+1), c, true)
		}
	}
	// Bottom edge (dashed)
	for x := r.Min.X + radius; x < r.Max.X-radius; x++ {
		if (x-r.Min.X-radius)%dashCycle < dashOn {
			drawRect(dst, image.Rect(x, r.Max.Y-1, x+1, r.Max.Y), c, true)
		}
	}
	// Left edge (dashed)
	for y := r.Min.Y + radius; y < r.Max.Y-radius; y++ {
		if (y-r.Min.Y-radius)%dashCycle < dashOn {
			drawRect(dst, image.Rect(r.Min.X, y, r.Min.X+1, y+1), c, true)
		}
	}
	// Right edge (dashed)
	for y := r.Min.Y + radius; y < r.Max.Y-radius; y++ {
		if (y-r.Min.Y-radius)%dashCycle < dashOn {
			drawRect(dst, image.Rect(r.Max.X-1, y, r.Max.X, y+1), c, true)
		}
	}
	// Corner arcs (solid, 1px stroked) — same as drawRoundedRect stroke corners.
	if radius > 0 {
		for i := 0; i < radius; i++ {
			fi := float64(i)
			fr := float64(radius)
			inset := int(fr - math.Sqrt(fi*(2*fr-fi)))
			drawRect(dst, image.Rect(r.Min.X+inset, r.Min.Y+i, r.Min.X+inset+1, r.Min.Y+i+1), c, true)
			drawRect(dst, image.Rect(r.Max.X-inset-1, r.Min.Y+i, r.Max.X-inset, r.Min.Y+i+1), c, true)
			drawRect(dst, image.Rect(r.Min.X+inset, r.Max.Y-1-i, r.Min.X+inset+1, r.Max.Y-i), c, true)
			drawRect(dst, image.Rect(r.Max.X-inset-1, r.Max.Y-1-i, r.Max.X-inset, r.Max.Y-i), c, true)
		}
	}
}

// RowRackCallbacks contains callbacks for the RowRackZone to communicate
// with the DrumView. Zones don't reference Game or each other directly.
type RowRackCallbacks struct {
	OnMuteToggle   func(row int)
	OnSoloToggle   func(row int)
	OnDeleteRow    func(row int)
	OnOriginReq    func(row int)
	OnAddRow       func()
	OnRowSelect    func(row int)
	OnVolumeChange func(row int, vol float64)

	// Overlay callbacks: delegate to DrumView's overlay mechanisms.
	OnContextMenuOpen func(row int)
	OnInstMenuOpen    func(row int)
	OnColorWheelOpen  func(row int)
	OnRenameOpen      func(row int)
	OnFXPanelToggle   func(row int)
	OnVolPopupOpen    func(row int)
	VolumePopup       *SliderPopup // forwarded to rowVolIconHitAdapter for drag-through
	OnSaveInstrument  func(row int)
	OnScrollChanged   func() // called when zone scroll offset changes

	// Read-only accessors for shared state.
	Rows              func() []*DrumRow
	IsInstrumentAvail func(id string) bool
	RowHeight         func() int
	DeleteConfirm     func() (row int, frame int64)
	RenameRow         func() int
	IsMobileEQMode    func() bool
	Frame             func() int64
}

// rowEntry consolidates per-row widgets (replaces 10+ parallel slices).
type rowEntry struct {
	label     *Button
	editBtn   *Button
	saveBtn   *Button
	colorBtn  *Button
	volSlider *Slider
	muteBtn   *Button
	soloBtn   *Button
	fxBtn     *Button
	originBtn *Button
	deleteBtn *Button
	menuBtn   *Button
	group     RowButtonGroup
}

// RowRackZone implements the Zone interface for the instrument row rack.
// It owns per-row buttons/sliders, the add-row button, row scrolling,
// and the row volume slider group.
type RowRackZone struct {
	rect         image.Rectangle
	screenBounds image.Rectangle // full screen bounds for FAB positioning
	needLayout   bool
	callbacks    RowRackCallbacks
	portal       *OverlayPortal

	entries     []rowEntry
	addRowBtn   *Button
	rowVolGroup *SliderGroup
	rowScroll   *ScrollBehavior

	// addRowRightReserve is the width (px) the addRowBtn must leave
	// unoccupied on its right edge — used by drumview_layout.go to host
	// the row-zoom +/- chips inline with the addRowBtn at the bottom of
	// the rack column. Set via SetAddRowRightReserve before Layout runs.
	// Zero means full-width addRowBtn (legacy behavior).
	addRowRightReserve int

	rowOffset       int
	selRow          int
	visRowsOverride int // if > 0, overrides VisibleRows() calculation

	// Row controls cache.
	controlsCache       *ebiten.Image
	controlsCacheDirty  bool
	controlsCacheRowOff int
	controlsCacheVis    int
	controlsCacheRect   image.Rectangle
	// Test-only: counts how many times drawRowControlsToCache has been
	// invoked. Each rebuild allocates many vector.Path objects via the icon
	// renderer; a soak test asserts this stays close to zero during
	// steady-state playback.
	controlsCacheRebuilds int64
	// Test-only invalidation reason counters. Each Draw that finds the
	// cache invalid increments exactly one of these so a regression test
	// can say *why* the cache rebuilt.
	cacheInvalidNil       int64
	cacheInvalidDirty     int64
	cacheInvalidRowOff    int64
	cacheInvalidVis       int64
	cacheInvalidEmptyRect int64
	cacheInvalidMuteDesync int64
	cacheInvalidSoloDesync int64
	// Test-only: invocation counters to localize per-frame churn.
	layoutInvocations           int64
	layoutFromNeedsLayout       int64
	layoutFromRectChange        int64
	repositionInvocations       int64
	lastLayoutRect              image.Rectangle
	lastLayoutRectInitialized   bool

	// Hit areas cache (rebuilt on Layout).
	hitAreas []HitArea
}

// NewRowRackZone creates a RowRackZone with the provided callbacks.
func NewRowRackZone(cb RowRackCallbacks) *RowRackZone {
	z := &RowRackZone{
		needLayout:         true,
		callbacks:          cb,
		controlsCacheDirty: true,
	}
	z.addRowBtn = NewButton("", addRowButtonStyle{}, func() {
		if z.callbacks.OnAddRow != nil {
			z.callbacks.OnAddRow()
		}
	})
	z.addRowBtn.Icon = string(IconPlus)
	z.addRowBtn.ConsumeOnPress = true
	z.rowVolGroup = NewSliderGroup(nil, func(idx int, val float64) {
		if z.callbacks.OnVolPopupOpen != nil {
			z.callbacks.OnVolPopupOpen(idx)
		}
		z.rowVolGroup.Release()
	})
	z.rowScroll = NewScrollBehavior(ScrollbarStyleForPlatform(), TouchRowHeight())
	return z
}

// --- Zone interface ---

func (z *RowRackZone) ID() string { return "row-rack" }

func (z *RowRackZone) NeedsLayout() bool { return z.needLayout }

func (z *RowRackZone) Invalidate() { z.needLayout = true }

func (z *RowRackZone) Layout(rect image.Rectangle) {
	z.layoutInvocations++
	if z.needLayout {
		z.layoutFromNeedsLayout++
	} else if z.lastLayoutRectInitialized && z.lastLayoutRect != rect {
		z.layoutFromRectChange++
	}
	z.lastLayoutRect = rect
	z.lastLayoutRectInitialized = true
	z.rect = rect
	z.needLayout = false
	// Profile-driven scrollbar appearance: re-derive every Layout so a
	// mobile↔desktop transition correctly retheme the row scrollbar (mobile
	// uses a wider thumb / larger min-thumb height for touch ergonomics).
	if z.rowScroll != nil {
		want := ScrollbarStyleForPlatform()
		if z.rowScroll.Style != want {
			z.rowScroll.Style = want
		}
	}
	z.rebuildEntries()
	z.rebuildHitAreas()
}

// applyRowLabelTypography unconditionally writes the profile-correct
// TextScale and TextColor onto a row label button. Idempotent. Called from
// rebuildEntries (cold path) and indirectly via applyRowLabelTypographyIfChanged
// from repositionEntries (hot path).
func applyRowLabelTypography(b *Button) {
	if b == nil {
		return
	}
	if Profile().IsMobile() {
		b.TextScale = FontSizeLabel / FontSizeBody
		b.TextColor = colTextPrimary
	} else {
		b.TextScale = 0
		b.TextColor = nil
	}
}

// applyRowLabelTypographyIfChanged updates the label typography to match
// the current profile and returns true iff anything actually changed. Used
// by repositionEntries to gate cache invalidation — re-applying identical
// values must NOT trigger a controls-cache rebuild (the WASM OOM hot path).
func applyRowLabelTypographyIfChanged(b *Button) bool {
	if b == nil {
		return false
	}
	wantScale := float64(0)
	var wantColor color.Color
	if Profile().IsMobile() {
		wantScale = FontSizeLabel / FontSizeBody
		wantColor = colTextPrimary
	}
	changed := false
	if b.TextScale != wantScale {
		b.TextScale = wantScale
		changed = true
	}
	if !colorEqualOrBothNil(b.TextColor, wantColor) {
		b.TextColor = wantColor
		changed = true
	}
	return changed
}

// colorEqualOrBothNil handles nil-vs-concrete interface comparison for
// color.Color. Used by applyRowLabelTypographyIfChanged to avoid spurious
// cache invalidation when the typography is unchanged.
func colorEqualOrBothNil(a, b color.Color) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func (z *RowRackZone) Update() {
	// Check if row count changed; re-layout if so.
	rows := z.rows()
	if len(rows) != len(z.entries) {
		z.needLayout = true
	}
	// Tick the wheel/step cooldown once per frame so the stepped scrollbar
	// re-arms (see WheelStep). Cheap no-op when nothing is pending.
	z.rowScroll.TickStep()
	// Scroll momentum.
	if z.rowScroll.HasMomentum() {
		z.syncScroll()
		if z.rowScroll.UpdateMomentum() {
			z.flushScroll()
			if z.callbacks.OnScrollChanged != nil {
				z.callbacks.OnScrollChanged()
			}
		}
	}
	// Sync mute/solo visuals (only write when changed to avoid cache-line dirtying).
	for i, e := range z.entries {
		if i < len(rows) {
			if e.muteBtn != nil && e.muteBtn.pressed != rows[i].Muted {
				e.muteBtn.pressed = rows[i].Muted
			}
			if e.soloBtn != nil && e.soloBtn.pressed != rows[i].Solo {
				e.soloBtn.pressed = rows[i].Solo
			}
		}
	}
}

func (z *RowRackZone) HitAreas() []HitArea {
	return z.hitAreas
}

func (z *RowRackZone) Draw(screen *ebiten.Image) {
	if z.isMobileEQMode() {
		return
	}
	z.drawRowControls(screen)
	if z.addRowBtn != nil && !z.addRowBtn.Rect().Empty() {
		z.addRowBtn.Draw(screen)
	}
	rows := z.rows()
	if len(rows) > z.VisibleRows() {
		z.syncScroll()
		z.rowScroll.Draw(screen)
	}
}

func (z *RowRackZone) HandleKey(key ebiten.Key) InputResult { return InputIgnored }
func (z *RowRackZone) HandleChars(chars []rune) InputResult { return InputIgnored }

// --- Public accessors (computed from entries) ---

func (z *RowRackZone) RowLabels() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].label
	}
	return out
}
func (z *RowRackZone) RowEditBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].editBtn
	}
	return out
}
func (z *RowRackZone) RowSaveBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].saveBtn
	}
	return out
}
func (z *RowRackZone) RowColorBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].colorBtn
	}
	return out
}
func (z *RowRackZone) RowDeleteBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].deleteBtn
	}
	return out
}
func (z *RowRackZone) RowVolSliders() []*Slider {
	out := make([]*Slider, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].volSlider
	}
	return out
}
func (z *RowRackZone) RowOriginBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].originBtn
	}
	return out
}
func (z *RowRackZone) RowMuteBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].muteBtn
	}
	return out
}
func (z *RowRackZone) RowSoloBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].soloBtn
	}
	return out
}
func (z *RowRackZone) RowMenuBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].menuBtn
	}
	return out
}
func (z *RowRackZone) RowFXBtns() []*Button {
	out := make([]*Button, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].fxBtn
	}
	return out
}
func (z *RowRackZone) RowGroups() []RowButtonGroup {
	out := make([]RowButtonGroup, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].group
	}
	return out
}
func (z *RowRackZone) EntryCount() int                   { return len(z.entries) }
func (z *RowRackZone) AddRowButton() *Button             { return z.addRowBtn }
func (z *RowRackZone) RowOffset() int                    { return z.rowOffset }
func (z *RowRackZone) SetRowOffset(off int) {
	if off != z.rowOffset {
		z.rowOffset = off
		z.needLayout = true
	}
}
func (z *RowRackZone) SelRow() int { return z.selRow }
func (z *RowRackZone) SetSelRow(r int) {
	if r != z.selRow {
		z.selRow = r
		z.needLayout = true
	}
}
func (z *RowRackZone) RowScroll() *ScrollBehavior        { return z.rowScroll }
func (z *RowRackZone) RowVolGroup() *SliderGroup         { return z.rowVolGroup }
func (z *RowRackZone) SetPortal(p *OverlayPortal)        { z.portal = p }
func (z *RowRackZone) MarkDirty()                        { z.controlsCacheDirty = true }
func (z *RowRackZone) SetScreenBounds(r image.Rectangle) {
	if r != z.screenBounds {
		z.screenBounds = r
		z.needLayout = true
	}
}
func (z *RowRackZone) SetVisibleRowsOverride(n int) {
	if n != z.visRowsOverride {
		z.visRowsOverride = n
		z.needLayout = true
	}
}

// VisibleRows returns the number of visible rows given current layout.
func (z *RowRackZone) VisibleRows() int {
	if z.visRowsOverride > 0 {
		return z.visRowsOverride
	}
	rh := z.rowHeight()
	if rh <= 0 {
		return 0
	}
	h := z.rect.Dy()
	if Profile().ReserveAddRowSpace {
		h -= rh // reserve one rowHeight for the "+" footer (desktop only)
	}
	if h < 0 {
		h = 0
	}
	return h / rh
}

// --- Internal helpers ---

func (z *RowRackZone) rows() []*DrumRow {
	if z.callbacks.Rows != nil {
		return z.callbacks.Rows()
	}
	return nil
}

func (z *RowRackZone) rowHeight() int {
	if z.callbacks.RowHeight != nil {
		return z.callbacks.RowHeight()
	}
	return TouchRowHeight()
}

// volSliderSlice builds a []*Slider from entries for SetSliders.
func (z *RowRackZone) volSliderSlice() []*Slider {
	out := make([]*Slider, len(z.entries))
	for i := range z.entries {
		out[i] = z.entries[i].volSlider
	}
	return out
}

func (z *RowRackZone) isMobileEQMode() bool {
	if z.callbacks.IsMobileEQMode != nil {
		return z.callbacks.IsMobileEQMode()
	}
	return false
}

// rebuildEntries creates rowEntry per row with button closures.
// When the row count hasn't changed, it repositions existing widgets
// instead of recreating them (preserving slider capture state).
func (z *RowRackZone) rebuildEntries() {
	rows := z.rows()
	vis := z.VisibleRows()
	if Profile().IsMobile() && z.isMobileEQMode() {
		vis = 0
	}
	panelRect := z.rect

	// Fast path: if row count is unchanged, just reposition existing entries.
	if len(rows) == len(z.entries) {
		z.repositionEntries(rows, vis, panelRect)
		return
	}

	z.entries = z.entries[:0]

	for i, r := range rows {
		rowRect := z.rowRectForIndex(i, panelRect.Min.Y, vis, panelRect)

		style := Profile().RowLabelStyle
		if z.callbacks.IsInstrumentAvail != nil && !z.callbacks.IsInstrumentAvail(r.Instrument) {
			style = MissingInstStyle
		}
		idx := i

		lbl := NewButton(r.Name, style, nil)
		// Mobile typography upgrade (larger TextScale + on-surface TextColor)
		// is applied unconditionally every Layout via applyRowLabelTypography,
		// so a mobile↔desktop transition retints in real time without relying
		// on construction-time profile sampling.
		applyRowLabelTypography(lbl)
		lbl.OnClick = func() {
			// Skip if rename is active for this row.
			if z.callbacks.RenameRow != nil && z.callbacks.RenameRow() == idx {
				return
			}
			if Profile().IsMobile() {
				if z.callbacks.OnContextMenuOpen != nil {
					z.callbacks.OnContextMenuOpen(idx)
				}
			} else {
				if z.callbacks.OnInstMenuOpen != nil {
					z.callbacks.OnInstMenuOpen(idx)
				}
			}
		}

		edit := NewButton("", InstButtonStyle, nil)
		edit.Icon = "pencil"
		edit.OnClick = func() {
			if z.callbacks.OnRenameOpen != nil {
				z.callbacks.OnRenameOpen(idx)
			}
		}

		save := NewButton("", InstButtonStyle, nil)
		save.Icon = "save"
		save.OnClick = func() {
			if z.callbacks.OnSaveInstrument != nil {
				z.callbacks.OnSaveInstrument(idx)
			}
		}

		colorBtn := z.makeColorButton(idx)

		slider := NewSlider(r.Volume)

		mute := NewButton("", InstButtonStyle, nil)
		mute.Icon = string(IconMute)
		mute.OnClick = func() {
			if z.callbacks.OnMuteToggle != nil {
				z.callbacks.OnMuteToggle(idx)
			}
		}

		solo := NewButton("", InstButtonStyle, nil)
		solo.Icon = string(IconSolo)
		solo.OnClick = func() {
			if z.callbacks.OnSoloToggle != nil {
				z.callbacks.OnSoloToggle(idx)
			}
		}

		origin := NewButton("", InstButtonStyle, nil)
		origin.Icon = string(IconTarget)
		origin.OnClick = func() {
			if z.callbacks.OnOriginReq != nil {
				z.callbacks.OnOriginReq(idx)
			}
		}

		del := NewButton("", DeleteButtonStyle, nil)
		del.Icon = string(IconClose)
		if len(rows) > 1 {
			del.OnClick = func() {
				if z.callbacks.OnDeleteRow != nil {
					z.callbacks.OnDeleteRow(idx)
				}
				z.MarkDirty()
			}
		} else {
			del.Style = DisabledButtonStyle
		}

		menu := NewButton("", RowKebabChipStyle, nil)
		menu.Icon = "overflow"
		menu.OnClick = func() {
			if z.callbacks.OnContextMenuOpen != nil {
				z.callbacks.OnContextMenuOpen(idx)
			}
		}

		fx := NewButton("", InstButtonStyle, nil)
		fx.Icon = string(IconFx)
		fx.OnClick = func() {
			if z.callbacks.OnFXPanelToggle != nil {
				z.callbacks.OnFXPanelToggle(idx)
			}
		}

		e := rowEntry{
			label:     lbl,
			editBtn:   edit,
			saveBtn:   save,
			colorBtn:  colorBtn,
			volSlider: slider,
			muteBtn:   mute,
			soloBtn:   solo,
			fxBtn:     fx,
			originBtn: origin,
			deleteBtn: del,
			menuBtn:   menu,
			group: RowButtonGroup{
				Mute: mute, Solo: solo, FX: fx, Origin: origin, Delete: del,
				Edit: edit, Save: save, Menu: menu, Label: lbl,
			},
		}

		z.entries = append(z.entries, e)

		// Position widgets.
		z.positionRowWidgets(i, rowRect)
	}

	// Sync slider group with rebuilt slider slice.
	z.rowVolGroup.SetSliders(z.volSliderSlice())

	// Position the "+" button.
	z.positionAddRowBtn(panelRect.Min.Y, panelRect)

	// Set container bounds for slider group.
	rowsBottom := panelRect.Min.Y + vis*z.rowHeight()
	z.rowVolGroup.SetContainerBounds(image.Rect(panelRect.Min.X, panelRect.Min.Y, panelRect.Max.X, rowsBottom))

	// Invalidate controls cache.
	z.controlsCacheDirty = true
}

// repositionEntries updates widget rects for existing entries without
// recreating button/slider instances. This preserves capture state
// (e.g., active slider drags) across Layout() calls.
//
// Cache invalidation is *conditional*: a re-position with no observable
// change (same rects, same label text/style, same volume) leaves the
// controls cache valid. updateRowRects (drumview_cache_background.go) calls
// us every Draw as a layout safety net; without conditional invalidation
// the cache would rebuild every frame, allocating ~30 vector.Path objects
// per visible row × len(rows) — the production WASM OOM hot path.
func (z *RowRackZone) repositionEntries(rows []*DrumRow, vis int, panelRect image.Rectangle) {
	z.repositionInvocations++
	dirty := false
	for i := range z.entries {
		rowRect := z.rowRectForIndex(i, panelRect.Min.Y, vis, panelRect)

		// Update label text/style in case name or availability changed.
		if i < len(rows) {
			e := &z.entries[i]
			if e.label.Text != rows[i].Name {
				e.label.Text = rows[i].Name
				dirty = true
			}
			style := Profile().RowLabelStyle
			if z.callbacks.IsInstrumentAvail != nil && !z.callbacks.IsInstrumentAvail(rows[i].Instrument) {
				style = MissingInstStyle
			}
			if e.label.Style != style {
				e.label.Style = style
				dirty = true
			}
			// Profile-driven typography: mobile labels bump TextScale and
			// override TextColor so kits parse at glance; desktop uses the
			// Button defaults. Re-applied every reposition so a viewport
			// transition retints in real time.
			if applyRowLabelTypographyIfChanged(e.label) {
				dirty = true
			}
			if e.volSlider.Value != rows[i].Volume {
				e.volSlider.Value = rows[i].Volume
				dirty = true
			}
		}

		// Snapshot the widget rects before repositioning so we can detect
		// whether positionRowWidgets actually moved anything. The rect set
		// here mirrors the rects positionRowWidgets writes.
		e := &z.entries[i]
		before := [10]image.Rectangle{
			e.label.Rect(), e.muteBtn.Rect(), e.soloBtn.Rect(),
			e.volSlider.Rect(), e.deleteBtn.Rect(), e.originBtn.Rect(),
			e.editBtn.Rect(), e.colorBtn.Rect(), e.saveBtn.Rect(), e.menuBtn.Rect(),
		}
		z.positionRowWidgets(i, rowRect)
		after := [10]image.Rectangle{
			e.label.Rect(), e.muteBtn.Rect(), e.soloBtn.Rect(),
			e.volSlider.Rect(), e.deleteBtn.Rect(), e.originBtn.Rect(),
			e.editBtn.Rect(), e.colorBtn.Rect(), e.saveBtn.Rect(), e.menuBtn.Rect(),
		}
		if before != after {
			dirty = true
		}
	}

	// Sync slider group (preserves active index since count is unchanged).
	z.rowVolGroup.SetSliders(z.volSliderSlice())

	// Position the "+" button.
	z.positionAddRowBtn(panelRect.Min.Y, panelRect)

	// Set container bounds for slider group.
	rowsBottom := panelRect.Min.Y + vis*z.rowHeight()
	z.rowVolGroup.SetContainerBounds(image.Rect(panelRect.Min.X, panelRect.Min.Y, panelRect.Max.X, rowsBottom))

	if dirty {
		z.controlsCacheDirty = true
	}
}

func (z *RowRackZone) makeColorButton(idx int) *Button {
	colorFn := func() color.Color {
		rows := z.rows()
		if idx >= 0 && idx < len(rows) {
			return rows[idx].Color
		}
		return genColorRowRackColorFallback
	}
	b := NewButton("", ColorSwatchStyle{Color: colorFn, Border: colButtonBorder}, nil)
	b.OnClick = func() {
		if z.callbacks.OnColorWheelOpen != nil {
			z.callbacks.OnColorWheelOpen(idx)
		}
	}
	return b
}

// rowRectForIndex computes the bounding rect for row i.
func (z *RowRackZone) rowRectForIndex(i, rowsTop, vis int, panelRect image.Rectangle) image.Rectangle {
	if i < z.rowOffset || i >= z.rowOffset+vis {
		return image.Rectangle{}
	}
	rh := z.rowHeight()
	y := rowsTop + (i-z.rowOffset)*rh
	return image.Rect(panelRect.Min.X, y, panelRect.Max.X, y+rh)
}

// positionRowWidgets sets rects for all per-row controls at index i.
func (z *RowRackZone) positionRowWidgets(i int, rowRect image.Rectangle) {
	if i >= len(z.entries) {
		return
	}
	e := &z.entries[i]
	g := NewGridLayout(rowRect, rowControlWeights(), []float64{1})
	e.label.SetRect(insetRect(g.Cell(0, 0), SpaceXS))
	if Profile().IsMobile() {
		// Mobile: Label | Vol | M | S | FX inline. Color/Rename/Origin/Delete
		// stay in the bottom-sheet context menu (tap label to open).
		e.menuBtn.SetRect(image.Rectangle{})
		e.editBtn.SetRect(image.Rectangle{})
		e.saveBtn.SetRect(image.Rectangle{})
		e.colorBtn.SetRect(image.Rectangle{})
		e.volSlider.SetRect(insetRect(g.Cell(1, 0), SpaceXS))
		e.muteBtn.SetRect(insetRect(g.Cell(2, 0), SpaceXS))
		e.soloBtn.SetRect(insetRect(g.Cell(3, 0), SpaceXS))
		e.fxBtn.SetRect(insetRect(g.Cell(4, 0), SpaceXS))
		e.originBtn.SetRect(image.Rectangle{})
		e.deleteBtn.SetRect(image.Rectangle{})
	} else {
		// Desktop: Label | VolBar | M | S | FX | ⋯
		// Hide edit/save/color/origin/delete — they live in overflow menu.
		e.editBtn.SetRect(image.Rectangle{})
		e.saveBtn.SetRect(image.Rectangle{})
		e.colorBtn.SetRect(image.Rectangle{})
		e.volSlider.SetRect(insetRect(g.Cell(1, 0), SpaceXS))
		e.muteBtn.SetRect(insetRect(g.Cell(2, 0), SpaceXS))
		e.soloBtn.SetRect(insetRect(g.Cell(3, 0), SpaceXS))
		e.fxBtn.SetRect(insetRect(g.Cell(4, 0), SpaceXS))
		e.originBtn.SetRect(image.Rectangle{})
		e.deleteBtn.SetRect(image.Rectangle{})
		e.menuBtn.SetRect(insetRect(g.Cell(5, 0), SpaceXS))
	}
}

// positionAddRowBtn sets the "+" button rect.
//
// **Anchoring contract:** the addRowBtn is always pinned to the bottom
// of the rack column (`panelRect.Max.Y - rh`) — predictable click
// targets matter more than tracking the row count.
//
// Tight-layout invariant: the rack rect itself is contracted in
// `drumview_layout.go` to fit `rendered_rows + 1` slots exactly, with
// any modulo slack absorbed by the bottom action bar growing upward
// (mobile only). That means `panelRect.Max.Y - rh` is always flush
// with the last rendered row's bottom AND the bar's top — no orphan
// rack-background strip can appear above OR below the button.
func (z *RowRackZone) positionAddRowBtn(rowsTop int, panelRect image.Rectangle) {
	rh := z.rowHeight()
	addY := panelRect.Max.Y - rh
	if addY < rowsTop {
		addY = rowsTop
	}
	// Mobile EQ mode hides the rack entirely; suppress the button so it does
	// not render through the bottom sheet.
	if Profile().IsMobile() && z.isMobileEQMode() {
		z.addRowBtn.SetRect(image.Rectangle{})
		return
	}
	// Guard: skip positioning if zone rect is empty (transient layout state).
	if panelRect.Empty() {
		z.addRowBtn.SetRect(image.Rectangle{})
		return
	}
	z.addRowBtn.Text = ""
	z.addRowBtn.Icon = string(IconPlus)
	addMaxX := panelRect.Max.X
	if z.addRowRightReserve > 0 && addMaxX-z.addRowRightReserve > panelRect.Min.X {
		addMaxX -= z.addRowRightReserve
	}
	z.addRowBtn.SetRect(insetRect(image.Rect(panelRect.Min.X, addY, addMaxX, addY+rh), SpaceXS))
}

// SetAddRowRightReserve declares how many pixels the addRow button must
// leave free on its right edge. Drumview_layout.go uses this to inline
// the row-zoom +/- chips beside the addRow button so they stop
// overlapping the FX cells of the topmost rows. Pass 0 to disable.
func (z *RowRackZone) SetAddRowRightReserve(px int) {
	if z.addRowRightReserve == px {
		return
	}
	z.addRowRightReserve = px
	z.needLayout = true
}

// AddRowBtnRect returns the current rect of the add-row button. Used by
// drumview_layout.go to position the row-zoom +/- chips inline with the
// addRow button on mobile (Theme 3 fix — row-zoom chips no longer
// overlap the FX cells of rows 0/1).
func (z *RowRackZone) AddRowBtnRect() image.Rectangle {
	if z.addRowBtn == nil {
		return image.Rectangle{}
	}
	return z.addRowBtn.Rect()
}

// --- Scroll helpers ---

func (z *RowRackZone) syncScroll() {
	rows := z.rows()
	z.rowScroll.VS.Total = len(rows)
	z.rowScroll.VS.Visible = z.VisibleRows()
	z.rowScroll.VS.First = z.rowOffset
	z.rowScroll.ItemHeight = z.rowHeight()
	z.rowScroll.VS.View = z.rect
}

func (z *RowRackZone) flushScroll() {
	if z.rowScroll.VS.First != z.rowOffset {
		z.rowOffset = z.rowScroll.VS.First
		z.needLayout = true
	}
}

// --- Hit area construction ---

func (z *RowRackZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]
	const zIdx = 120

	rows := z.rows()
	vis := z.VisibleRows()

	for i := range z.entries {
		if i < z.rowOffset || i >= z.rowOffset+vis || i >= len(rows) {
			continue
		}
		e := &z.entries[i]
		tag := func(name string) string {
			return "row-rack-" + name
		}

		renameRow := -1
		if z.callbacks.RenameRow != nil {
			renameRow = z.callbacks.RenameRow()
		}

		// Button hit areas for each row control.
		// Skip label when rename is active for this row (text input overlaps).
		var labelBtn *Button
		if renameRow != i {
			labelBtn = e.label
		}
		btns := []struct {
			btn   *Button
			tag   string
			touch bool
		}{
			{labelBtn, tag("label"), true},
			{e.muteBtn, tag("mute"), true},
			{e.soloBtn, tag("solo"), true},
			{e.fxBtn, tag("fx"), true},
			{e.originBtn, tag("origin"), true},
			{e.deleteBtn, tag("delete"), true},
			{e.editBtn, tag("edit"), true},
			{e.saveBtn, tag("save"), true},
			{e.menuBtn, tag("menu"), true},
		}
		for _, sb := range btns {
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

		// Color button.
		if e.colorBtn != nil && !e.colorBtn.Rect().Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:     e.colorBtn.Rect(),
				ZIndex:   zIdx,
				Handler:  &buttonHitAdapter{btn: e.colorBtn},
				Tag:      tag("color"),
				Touch:    true,
				ClipRect: z.rect,
			})
		}
	}

	// Row volume slider group — z+1 so sliders get first dispatch over
	// label buttons at the same z. Fall-through ensures clicks outside
	// the slider track still reach the label.
	if z.rowVolGroup != nil {
		groupBounds := z.rowVolGroup.InputBounds()
		if !groupBounds.Empty() {
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:     groupBounds,
				ZIndex:   zIdx + 1,
				Handler:  &sliderGroupHitAdapter{group: z.rowVolGroup},
				Tag:      "row-rack-vol-slider",
				Touch:    true,
				ClipRect: z.rect,
			})
		}
	}

	// Volume icon areas (both platforms — click opens popup).
	{
		for i := z.rowOffset; i < z.rowOffset+vis && i < len(z.entries); i++ {
			r := z.entries[i].volSlider.Rect()
			if r.Empty() {
				continue
			}
			idx := i
			z.hitAreas = append(z.hitAreas, HitArea{
				Rect:   r,
				ZIndex: zIdx + 1, // above slider group
				Handler: &rowVolIconHitAdapter{
					onClick: func() {
						if z.callbacks.OnVolPopupOpen != nil {
							z.callbacks.OnVolPopupOpen(idx)
						}
					},
					popup: z.callbacks.VolumePopup,
				},
				Tag:      "row-rack-vol-icon",
				Touch:    true,
				ClipRect: z.rect,
			})
		}
	}

	// Scroll wheel area.
	if !z.rect.Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:     z.rect,
			ZIndex:   zIdx - 1, // below buttons
			Handler:  &rowRackScrollHitAdapter{zone: z},
			Tag:      "row-rack-scroll",
			ClipRect: z.rect,
		})
	}

	// Add row button (FAB): higher z-index to float above EQ panel hit areas.
	if z.addRowBtn != nil && !z.addRowBtn.Rect().Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.addRowBtn.Rect(),
			ZIndex:  ZBaseMax, // 199: above all base zones including EQ (130)
			Handler: &buttonHitAdapter{btn: z.addRowBtn},
			Tag:     "row-rack-add",
			Touch:   true,
		})
	}
}

// --- Hit handler adapters ---

// rowVolIconHitAdapter handles the mobile per-row volume icon click.
// OnPress opens the popup and returns InputCaptured so that subsequent
// drag/release events are routed here, enabling tap-and-slide in one gesture.
type rowVolIconHitAdapter struct {
	onClick func()
	popup   *SliderPopup
}

func (h *rowVolIconHitAdapter) OnPress(x, y int) InputResult {
	if h.onClick != nil {
		h.onClick()
	}
	return InputCaptured
}

func (h *rowVolIconHitAdapter) OnDrag(x, y int) {
	if h.popup != nil {
		h.popup.HandleInput(x, y, true)
	}
}

func (h *rowVolIconHitAdapter) OnRelease(x, y int) {
	if h.popup != nil {
		h.popup.HandleInput(x, y, false)
	}
}
func (h *rowVolIconHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// rowRackScrollHitAdapter handles scroll wheel events over the row rack.
type rowRackScrollHitAdapter struct {
	zone *RowRackZone
}

func (h *rowRackScrollHitAdapter) OnPress(x, y int) InputResult { return InputIgnored }
func (h *rowRackScrollHitAdapter) OnDrag(x, y int)              {}
func (h *rowRackScrollHitAdapter) OnRelease(x, y int)           {}

func (h *rowRackScrollHitAdapter) OnWheel(x, y, steps int) InputResult {
	if steps == 0 {
		return InputIgnored
	}
	// Step-by-step: one row per notch, throttled by the cooldown so a spun
	// wheel can't fly through the list. Consume regardless so the wheel never
	// leaks past the rack while the cursor is over it.
	h.zone.syncScroll()
	if h.zone.rowScroll.WheelStep(steps, controlGridScrollCooldownFrames) {
		h.zone.flushScroll()
		if h.zone.callbacks.OnScrollChanged != nil {
			h.zone.callbacks.OnScrollChanged()
		}
	}
	return InputConsumed
}

// --- Drawing ---

func (z *RowRackZone) controlsCacheValid() bool {
	if z.controlsCache == nil {
		z.cacheInvalidNil++
		return false
	}
	if z.controlsCacheDirty {
		z.cacheInvalidDirty++
		return false
	}
	vis := z.VisibleRows()
	if z.controlsCacheRowOff != z.rowOffset {
		z.cacheInvalidRowOff++
		return false
	}
	if z.controlsCacheVis != vis {
		z.cacheInvalidVis++
		return false
	}
	if z.controlsCacheRect.Empty() {
		z.cacheInvalidEmptyRect++
		return false
	}
	rows := z.rows()
	for i := z.rowOffset; i < z.rowOffset+vis && i < len(rows) && i < len(z.entries); i++ {
		e := &z.entries[i]
		if !e.muteBtn.Rect().Empty() && e.muteBtn.pressed != rows[i].Muted {
			z.cacheInvalidMuteDesync++
			return false
		}
		if !e.soloBtn.Rect().Empty() && e.soloBtn.pressed != rows[i].Solo {
			z.cacheInvalidSoloDesync++
			return false
		}
		if !e.fxBtn.Rect().Empty() && e.fxBtn.pressed != hasActiveEffects(rows[i].Effects) {
			return false
		}
	}
	return true
}

func (z *RowRackZone) computeControlsBounds() image.Rectangle {
	vis := z.VisibleRows()
	rows := z.rows()
	if vis == 0 || len(rows) == 0 {
		return image.Rectangle{}
	}
	startRow := z.rowOffset
	endRow := z.rowOffset + vis
	if endRow > len(rows) {
		endRow = len(rows)
	}
	if startRow >= endRow {
		return image.Rectangle{}
	}
	var minX, minY, maxX, maxY int
	first := true
	expandRect := func(r image.Rectangle) {
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
	for i := startRow; i < endRow && i < len(z.entries); i++ {
		e := &z.entries[i]
		expandRect(e.label.Rect())
		expandRect(e.muteBtn.Rect())
		expandRect(e.soloBtn.Rect())
		expandRect(e.volSlider.Rect())
		expandRect(e.deleteBtn.Rect())
		expandRect(e.originBtn.Rect())
		expandRect(e.editBtn.Rect())
		expandRect(e.colorBtn.Rect())
		expandRect(e.saveBtn.Rect())
		expandRect(e.menuBtn.Rect())
		expandRect(e.fxBtn.Rect())
	}
	if first {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func (z *RowRackZone) drawRowControls(dst *ebiten.Image) {
	if z.controlsCacheValid() {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(z.controlsCacheRect.Min.X), float64(z.controlsCacheRect.Min.Y))
		dst.DrawImage(z.controlsCache, &op)
		return
	}
	bounds := z.computeControlsBounds()
	if bounds.Empty() {
		z.drawRowControlsDirect(dst)
		return
	}
	w, h := bounds.Dx(), bounds.Dy()
	if z.controlsCache == nil || z.controlsCache.Bounds().Dx() != w || z.controlsCache.Bounds().Dy() != h {
		releaseImage(z.controlsCache)
		z.controlsCache = newTrackedImage("rowRackZone.controlsCache", w, h)
	} else {
		z.controlsCache.Clear()
	}
	z.drawRowControlsToCache(z.controlsCache, bounds.Min.X, bounds.Min.Y)
	z.controlsCacheRebuilds++
	z.controlsCacheDirty = false
	z.controlsCacheRowOff = z.rowOffset
	z.controlsCacheVis = z.VisibleRows()
	z.controlsCacheRect = bounds
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	dst.DrawImage(z.controlsCache, &op)
}

func (z *RowRackZone) drawRowControlsToCache(cache *ebiten.Image, offsetX, offsetY int) {
	rows := z.rows()
	vis := z.VisibleRows()
	mobile := Profile().IsMobile()
	renameRow := -1
	if z.callbacks.RenameRow != nil {
		renameRow = z.callbacks.RenameRow()
	}
	deleteConfirmRow := -1
	var deleteConfirmFrame int64
	if z.callbacks.DeleteConfirm != nil {
		deleteConfirmRow, deleteConfirmFrame = z.callbacks.DeleteConfirm()
	}
	frame := int64(0)
	if z.callbacks.Frame != nil {
		frame = z.callbacks.Frame()
	}

	for i := range rows {
		if i < z.rowOffset || i >= z.rowOffset+vis || i >= len(z.entries) {
			continue
		}
		e := &z.entries[i]
		if mobile && i%2 == 1 {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				zebraR := lblR.Sub(image.Pt(offsetX, offsetY))
				drawRect(cache, zebraR, WithAlpha(genColorBorder, genAlphaRowRackZebra), true)
			}
		}
		// Draw accent stripe for all platforms (mobile and desktop).
		{
			lblR := e.label.Rect()
			if !lblR.Empty() {
				stripeR := lblR.Sub(image.Pt(offsetX, offsetY))
				drawAccentStripe(cache, stripeR, rows[i].Color)
			}
		}
		if renameRow != i {
			drawBtnOff(cache, e.label, offsetX, offsetY)
		}
		drawBtnOff(cache, e.editBtn, offsetX, offsetY)
		drawBtnOff(cache, e.saveBtn, offsetX, offsetY)
		if !e.menuBtn.Rect().Empty() {
			drawBtnOff(cache, e.menuBtn, offsetX, offsetY)
		}
		drawBtnOff(cache, e.colorBtn, offsetX, offsetY)
		drawVolCellOff(cache, e.volSlider, rows[i].Volume, offsetX, offsetY, rows[i].Color, rows[i].Muted)
		if !e.muteBtn.Rect().Empty() {
			syncToggleVisual(e.muteBtn, rows[i].Muted,
				MuteActiveStyle, InstButtonStyle,
				"", "", nil, nil)
			drawBtnOff(cache, e.muteBtn, offsetX, offsetY)
		}
		if !e.soloBtn.Rect().Empty() {
			syncToggleVisual(e.soloBtn, rows[i].Solo,
				SoloActiveStyle, InstButtonStyle,
				"", "", nil, nil)
			drawBtnOff(cache, e.soloBtn, offsetX, offsetY)
		}
		if !e.fxBtn.Rect().Empty() {
			// Highlight only when at least one effect is *enabled*; a slot list
			// of all-disabled effects must render inactive. We tag pressed with
			// the active flag so controlsCacheValid() can detect state flips and
			// rebuild the cache in real time when the user toggles or removes
			// effects.
			active := hasActiveEffects(rows[i].Effects)
			if active {
				e.fxBtn.Style = FXActiveStyle
			} else {
				e.fxBtn.Style = InstButtonStyle
			}
			e.fxBtn.pressed = active
			drawBtnOff(cache, e.fxBtn, offsetX, offsetY)
			if active {
				badgeR := e.fxBtn.Rect().Sub(image.Pt(offsetX, offsetY))
				drawFXBadge(cache, badgeR, activeEffectsCount(rows[i].Effects))
			}
		}
		drawBtnOff(cache, e.originBtn, offsetX, offsetY)
		if deleteConfirmRow == i && (frame-deleteConfirmFrame) < 120 {
			// Stateful confirm: show "!!" as a warning glyph (text) so it
			// reads as distinct from the resting close-icon state.
			e.deleteBtn.Text = "!!"
			e.deleteBtn.Icon = ""
			e.deleteBtn.Style = DeleteConfirmButtonStyle
		} else {
			e.deleteBtn.Text = ""
			e.deleteBtn.Icon = string(IconClose)
			if len(rows) > 1 {
				e.deleteBtn.Style = DeleteButtonStyle
			}
		}
		drawBtnOff(cache, e.deleteBtn, offsetX, offsetY)
		if mobile {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				sepY := lblR.Max.Y - offsetY
				sepR := image.Rect(lblR.Min.X-offsetX, sepY, lblR.Max.X-offsetX, sepY+1)
				drawRect(cache, sepR, WithAlpha(genColorBorder, genAlphaRowRackSeparator), true)
			}
		}
	}
}

func (z *RowRackZone) drawRowControlsDirect(dst *ebiten.Image) {
	rows := z.rows()
	vis := z.VisibleRows()
	mobile := Profile().IsMobile()
	renameRow := -1
	if z.callbacks.RenameRow != nil {
		renameRow = z.callbacks.RenameRow()
	}
	deleteConfirmRow := -1
	var deleteConfirmFrame int64
	if z.callbacks.DeleteConfirm != nil {
		deleteConfirmRow, deleteConfirmFrame = z.callbacks.DeleteConfirm()
	}
	frame := int64(0)
	if z.callbacks.Frame != nil {
		frame = z.callbacks.Frame()
	}

	for i := range rows {
		if i < z.rowOffset || i >= z.rowOffset+vis || i >= len(z.entries) {
			continue
		}
		e := &z.entries[i]
		if mobile && i%2 == 1 {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				drawRect(dst, lblR, WithAlpha(genColorBorder, genAlphaRowRackDim), true)
			}
		}
		// Draw accent stripe for all platforms (mobile and desktop).
		{
			lblR := e.label.Rect()
			if !lblR.Empty() {
				drawAccentStripe(dst, lblR, rows[i].Color)
			}
		}
		if renameRow != i {
			e.label.Draw(dst)
		}
		e.editBtn.Draw(dst)
		e.saveBtn.Draw(dst)
		if !e.menuBtn.Rect().Empty() {
			e.menuBtn.Draw(dst)
		}
		e.colorBtn.Draw(dst)
		drawVolCell(dst, e.volSlider, rows[i].Volume, rows[i].Color, rows[i].Muted)
		if !e.muteBtn.Rect().Empty() {
			if rows[i].Muted {
				e.muteBtn.Style = MuteActiveStyle
				e.muteBtn.pressed = false
			} else {
				e.muteBtn.Style = InstButtonStyle
				e.muteBtn.pressed = false
			}
			e.muteBtn.Draw(dst)
		}
		if !e.soloBtn.Rect().Empty() {
			if rows[i].Solo {
				e.soloBtn.Style = SoloActiveStyle
				e.soloBtn.pressed = false
			} else {
				e.soloBtn.Style = InstButtonStyle
				e.soloBtn.pressed = false
			}
			e.soloBtn.Draw(dst)
		}
		e.originBtn.Draw(dst)
		if deleteConfirmRow == i && (frame-deleteConfirmFrame) < 120 {
			// Stateful confirm: show "!!" as a warning glyph (text) so it
			// reads as distinct from the resting close-icon state.
			e.deleteBtn.Text = "!!"
			e.deleteBtn.Icon = ""
			e.deleteBtn.Style = DeleteConfirmButtonStyle
		} else {
			e.deleteBtn.Text = ""
			e.deleteBtn.Icon = string(IconClose)
			if len(rows) > 1 {
				e.deleteBtn.Style = DeleteButtonStyle
			}
		}
		e.deleteBtn.Draw(dst)
		if !e.fxBtn.Rect().Empty() {
			active := hasActiveEffects(rows[i].Effects)
			if active {
				e.fxBtn.Style = FXActiveStyle
			} else {
				e.fxBtn.Style = InstButtonStyle
			}
			e.fxBtn.pressed = active
			e.fxBtn.Draw(dst)
			if active {
				drawFXBadge(dst, e.fxBtn.Rect(), activeEffectsCount(rows[i].Effects))
			}
		}
		if mobile {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				sepY := lblR.Max.Y
				drawRect(dst, image.Rect(lblR.Min.X, sepY, lblR.Max.X, sepY+1), WithAlpha(genColorBorder, genAlphaRowRackSeparator), true)
			}
		}
	}
}

// drawVolIcon renders a per-row volume button into dst. Thin wrapper around
// drawVolumeButton — the canonical, shared volume-button component used by
// both per-row volume cells AND the master volume button. Differences from
// master are limited to channel wiring (the slider's value source) and the
// optional row-color tint.
func drawVolIcon(dst *ebiten.Image, s *Slider, vol float64, muted bool, rowColor color.Color) {
	drawVolumeButton(dst, s.Rect(), vol, muted, rowColor)
}

// drawVolIconOff is the offset-cache counterpart of drawVolIcon.
func drawVolIconOff(cache *ebiten.Image, s *Slider, vol float64, offsetX, offsetY int, muted bool, rowColor color.Color) {
	r := s.Rect()
	if r.Empty() {
		return
	}
	drawVolumeButton(cache, r.Sub(image.Pt(offsetX, offsetY)), vol, muted, rowColor)
}

// volIconColor returns the appropriate color for a volume icon.
// Uses instrument color tint when volume > 0 and not muted, colTextDisabled otherwise.
func volIconColor(vol float64, muted bool, rowColor color.Color) color.Color {
	if vol <= 0 || muted {
		return colTextDisabled
	}
	if rowColor != nil {
		return WithAlphaFromColor(rowColor, AlphaOverlay)
	}
	return colVolumeIconOn
}

// drawVolCell draws the volume cell: speaker glyph + thin level underline.
// The underline is drawn by drawVolIcon via drawSliderLevelIndicator, so the
// at-a-glance level reading is consistent across mobile and desktop and
// across per-row and master volume controls.
func drawVolCell(dst *ebiten.Image, s *Slider, vol float64, rowColor color.Color, muted bool) {
	drawVolIcon(dst, s, vol, muted, rowColor)
}

// drawVolCellOff is the offset-cache counterpart of drawVolCell.
func drawVolCellOff(cache *ebiten.Image, s *Slider, vol float64, offsetX, offsetY int, rowColor color.Color, muted bool) {
	drawVolIconOff(cache, s, vol, offsetX, offsetY, muted, rowColor)
}

// Verify interface at compile time.
var _ Zone = (*RowRackZone)(nil)
