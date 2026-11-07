package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Disabled slots are not in the audio chain, so they must not light the button.
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

// The FX panel lists them all.
const fxCountMaxDots = 4

// One dot per enabled effect along the chip's bottom edge, in the letter color so it contrasts with
// any fill; a corner badge covered the X.
func drawFXBadge(dst *ebiten.Image, btnRect image.Rectangle, count int, instColor color.Color) {
	if count <= 0 || btnRect.Empty() {
		return
	}
	n := count
	if n > fxCountMaxDots {
		n = fxCountMaxDots
	}
	d := Profile().DensityValues().ParamCardGaugeH
	if d < 2 {
		d = 2
	}
	stride := d + 2
	w := n*stride - 2
	x0 := btnRect.Min.X + (btnRect.Dx()-w)/2
	y1 := btnRect.Max.Y - SpaceXS
	col := rowToggleLabelColor(instColor, roleFX, true)
	for k := 0; k < n; k++ {
		x := x0 + k*stride
		drawCapsule(dst, image.Rect(x, y1-d, x+d, y1), col, true)
	}
}

// The neutral row body with a "+" swatch and a muted label, aligned like the rows above. No dashed
// outline: borders at rest are forbidden.
type addRowButtonStyle struct{}

// addRowBtnKey identifies a cached add-row button sprite.
type addRowBtnKey struct {
	w, h    int
	pressed bool
	hovered bool
	locale  i18n.Locale
}

var addRowBtnCache = map[addRowBtnKey]*ebiten.Image{}

// The key carries no color, so a theme change must clear it.
func invalidateAddRowBtnCache() { addRowBtnCache = map[addRowBtnKey]*ebiten.Image{} }

func (addRowButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	if r.Empty() {
		return
	}
	k := addRowBtnKey{w: r.Dx(), h: r.Dy(), pressed: pressed, hovered: hovered, locale: i18n.ActiveLocale()}
	if spr, ok := addRowBtnCache[k]; ok {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		dst.DrawImage(spr, &op)
		return
	}
	spr := ebiten.NewImage(r.Dx(), r.Dy())
	zr := image.Rect(0, 0, r.Dx(), r.Dy())
	InstButtonStyle.Draw(spr, zr, pressed, hovered)
	// "+" in the (desktop) swatch slot, then the label.
	sz := SwatchSizeMD
	if sz > zr.Dy() {
		sz = zr.Dy()
	}
	pr := image.Rect(SpaceSM, (zr.Dy()-sz)/2, SpaceSM+sz, (zr.Dy()-sz)/2+sz)
	DrawIcon(spr, IconPlus, pr, colTextSecondary)
	lx := pr.Max.X + SpaceSM
	label := clipTextToWidthMeasure(i18n.T(i18n.KeyRackAddInstrument), zr.Dx()-lx-SpaceSM,
		func(t string) int { return StyledTextWidth(t, RoleBody) })
	ty := (zr.Dy() - StyledTextHeight(RoleBody)) / 2
	DrawTextStyled(spr, label, lx, ty, RoleBody, colTextSecondary)
	addRowBtnCache[k] = spr
	var op ebiten.DrawImageOptions
	op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
	dst.DrawImage(spr, &op)
}

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

// The on/off visual is applied per draw by applyRowToggle.
func newRowToggleButton(letter string) *Button {
	b := NewButton(letter, rowToggleStyle(nil, roleMute, false), nil)
	b.SetTextRole(RoleBody)
	b.TextColor = colTextSecondary
	return b
}

// An 18 px mobile chip cannot fit a body-size "FX", and a clipped "..." is not a label.
func fitRowToggleRole(b *Button) {
	if b == nil {
		return
	}
	role := RoleBody
	if StyledTextWidth(b.Text, RoleBody) > b.Rect().Dx()-2 {
		role = RoleCaption
	}
	if r, ok := b.StyledTextRole(); !ok || r != role {
		b.SetTextRole(role)
	}
}

// The rack fallback for rows built without a color (headless tests).
func rowIdentityColor(c color.Color) color.Color {
	if c == nil {
		return genColorRowRackColorFallback
	}
	return c
}

// Zones never reference Game or each other directly.
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
	IsInstrumentAvail func(id instrument.ID) bool
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

	// Set before Layout; 0 = full width.
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
	// Each rebuild allocates many vector.Paths; a soak test asserts it stays near zero while playing.
	controlsCacheRebuilds int64
	// One increments per invalid Draw, so a test can say why the cache rebuilt.
	cacheInvalidNil        int64
	cacheInvalidDirty      int64
	cacheInvalidRowOff     int64
	cacheInvalidVis        int64
	cacheInvalidEmptyRect  int64
	cacheInvalidMuteDesync int64
	cacheInvalidSoloDesync int64
	// Test-only: invocation counters to localize per-frame churn.
	layoutInvocations         int64
	layoutFromNeedsLayout     int64
	layoutFromRectChange      int64
	repositionInvocations     int64
	lastLayoutRect            image.Rectangle
	lastLayoutRectInitialized bool

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
	// A taller rack can leave a scrolled offset past the last valid window; the scrollbar path only
	// fixes it while content overflows, so the first rows would stay hidden.
	z.clampRowOffset()
	// Every Layout, so a mobile↔desktop switch re-themes the scrollbar.
	if z.rowScroll != nil {
		want := ScrollbarStyleForPlatform()
		// The scrollbar is the only cue that hidden rows exist, and the shared faint thumb was invisible on
		// the dark rack; contrast, not width, makes it discoverable.
		want.ThumbColor = WithAlpha(genColorPrimary, genAlphaStrong)
		want.TrackColor = WithAlpha(genColorBorder, genAlphaMedium)
		if z.rowScroll.Style != want {
			z.rowScroll.Style = want
		}
	}
	z.rebuildEntries()
	z.rebuildHitAreas()
}

// Idempotent.
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

// true only on a real change: re-applying identical values must not rebuild the controls cache.
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

// Avoids spurious invalidation from nil-vs-concrete interface comparison.
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
	// Re-arms the stepped scrollbar; a no-op when nothing is pending.
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
func (z *RowRackZone) EntryCount() int       { return len(z.entries) }
func (z *RowRackZone) AddRowButton() *Button { return z.addRowBtn }
func (z *RowRackZone) RowOffset() int        { return z.rowOffset }
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
func (z *RowRackZone) RowScroll() *ScrollBehavior { return z.rowScroll }
func (z *RowRackZone) RowVolGroup() *SliderGroup  { return z.rowVolGroup }
func (z *RowRackZone) SetPortal(p *OverlayPortal) { z.portal = p }
func (z *RowRackZone) MarkDirty()                 { z.controlsCacheDirty = true }
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

// An unchanged row count repositions instead of recreating, preserving slider capture.
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
		// Every Layout, so a viewport switch retints without construction-time sampling.
		applyRowLabelTypography(lbl)
		lbl.OnClick = func() {
			// Skip if rename is active for this row.
			if z.callbacks.RenameRow != nil && z.callbacks.RenameRow() == idx {
				return
			}
			// On every platform; the picker becomes a bottom sheet through the profile.
			if z.callbacks.OnInstMenuOpen != nil {
				z.callbacks.OnInstMenuOpen(idx)
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

		mute := newRowToggleButton(rowMuteLabel)
		mute.OnClick = func() {
			if z.callbacks.OnMuteToggle != nil {
				z.callbacks.OnMuteToggle(idx)
			}
		}

		solo := newRowToggleButton(rowSoloLabel)
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

		menu := NewButton("", rowKebabStyle(nil), nil)
		menu.Icon = "overflow"
		menu.IconColor = colTextSecondary
		menu.OnClick = func() {
			if z.callbacks.OnContextMenuOpen != nil {
				z.callbacks.OnContextMenuOpen(idx)
			}
		}

		fx := newRowToggleButton(rowFXLabel)
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

// Invalidation is conditional: this runs every Draw as a safety net, and an unconditional rebuild
// allocated ~30 vector.Paths per row per frame (the WASM OOM hot path).
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
			// Re-applied every reposition so a viewport switch retints live.
			if applyRowLabelTypographyIfChanged(e.label) {
				dirty = true
			}
			if e.volSlider.Value != rows[i].Volume {
				e.volSlider.Value = rows[i].Volume
				dirty = true
			}
		}

		// Snapshot to tell whether positionRowWidgets moved anything.
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

// [Label … flexes] [Vol] [M] [S] [FX] [⋯] on both platforms: the cluster is right-anchored at fixed
// widths and the label takes what remains, down to a readable minimum. Other actions live in the ⋯ menu.
func (z *RowRackZone) positionRowWidgets(i int, rowRect image.Rectangle) {
	if i >= len(z.entries) {
		return
	}
	e := &z.entries[i]

	// Controls that only exist inside the overflow / context menu.
	e.editBtn.SetRect(image.Rectangle{})
	e.saveBtn.SetRect(image.Rectangle{})
	e.colorBtn.SetRect(image.Rectangle{})
	e.originBtn.SetRect(image.Rectangle{})
	e.deleteBtn.SetRect(image.Rectangle{})

	z.layoutRowControls(e, rowRect)
}

// Every control in the row shares one band, so a row never holds several heights.
func rowControlBand(rowRect image.Rectangle) (top, bottom int) {
	top, bottom = rowRect.Min.Y+SpaceXS, rowRect.Max.Y-SpaceXS
	if bottom-top < 1 {
		return rowRect.Min.Y, rowRect.Max.Y
	}
	return top, bottom
}

// Two gaps, each with one meaning: name, level, switches.
func rowGroupGap() int { return SpaceSM }

// Shared by the add-row row so its edges line up.
func (z *RowRackZone) rowContentSpan(r image.Rectangle) (minX, maxX int) {
	return r.Min.X + SpaceXS, r.Max.X - SpaceXS - z.scrollReserve()
}

// Reserved always, so controls stay put whether or not the scrollbar shows.
func (z *RowRackZone) scrollReserve() int {
	if z.rowScroll == nil || z.rowScroll.Style.Width <= 0 {
		return 0
	}
	gap := Profile().ControlGap
	if gap < SpaceXS {
		gap = SpaceXS
	}
	return z.rowScroll.Style.Width + gap
}

// DrumView sizes the rack column to labelW + this, so the column is exactly its content.
func (z *RowRackZone) FixedControlsWidth() int {
	btnW := RowControlBtnSize()
	volW := btnW * 2
	if Profile().IsMobile() {
		volW = btnW
	}
	gap := Profile().ControlGap
	if gap < SpaceXS {
		gap = SpaceXS
	}
	return 2*SpaceXS + rowGroupGap() + volW + rowGroupGap() + 4*btnW + 3*gap + z.scrollReserve()
}

func (z *RowRackZone) layoutRowControls(e *rowEntry, rowRect image.Rectangle) {
	btnW := RowControlBtnSize()
	volW := btnW * 2
	gap := Profile().ControlGap
	if gap < SpaceXS {
		gap = SpaceXS
	}
	groupGap := rowGroupGap()
	labelMinW := btnW * 2
	// On mobile the volume is a compact icon (precision lives in the popup), so one key width is enough.
	if Profile().IsMobile() {
		volW = btnW
	}

	left, right := z.rowContentSpan(rowRect)
	// Width consumed by the cluster: volume + group gap + 4 toggles + 3 gaps.
	clusterW := volW + groupGap + btnW*4 + gap*3
	avail := right - left

	// Shrink the controls proportionally rather than lose the label.
	if labelMinW+groupGap+clusterW > avail && clusterW > 0 {
		space := avail - labelMinW - groupGap
		if space < btnW {
			space = btnW
		}
		if space < clusterW {
			scale := float64(space) / float64(clusterW)
			btnW = max(1, int(float64(btnW)*scale))
			volW = max(1, int(float64(volW)*scale))
		}
	}

	top, bottom := rowControlBand(rowRect)
	// Place toggles from the right edge inward: ⋯, FX, Solo, Mute.
	placeBtn := func(b *Button) {
		b.SetRect(image.Rect(right-btnW, top, right, bottom))
		right -= btnW + gap
	}
	placeBtn(e.menuBtn)
	placeBtn(e.fxBtn)
	placeBtn(e.soloBtn)
	placeBtn(e.muteBtn)
	fitRowToggleRole(e.muteBtn)
	fitRowToggleRole(e.soloBtn)
	fitRowToggleRole(e.fxBtn)
	right += gap - groupGap

	e.volSlider.SetRect(image.Rect(right-volW, top, right, bottom))
	right -= volW + groupGap

	// Label flexes to fill everything to the left of the volume cell.
	if right < left {
		right = left
	}
	e.label.SetRect(image.Rect(left, top, right, bottom))
}

// Directly UNDER the last visible row, sharing its span and band; when rows overflow, pinned to the
// rack bottom where one row height was kept free. Always pinning left a gap when the pane was taller.
func (z *RowRackZone) positionAddRowBtn(rowsTop int, panelRect image.Rectangle) {
	rh := z.rowHeight()
	// The rack is hidden there, so it would render through the bottom sheet.
	if Profile().IsMobile() && z.isMobileEQMode() {
		z.addRowBtn.SetRect(image.Rectangle{})
		return
	}
	// Guard: skip positioning if zone rect is empty (transient layout state).
	if panelRect.Empty() {
		z.addRowBtn.SetRect(image.Rectangle{})
		return
	}
	shown := len(z.rows()) - z.rowOffset
	if vis := z.VisibleRows(); shown > vis {
		shown = vis
	}
	if shown < 0 {
		shown = 0
	}
	addY := rowsTop + shown*rh
	if addY+rh > panelRect.Max.Y {
		addY = panelRect.Max.Y - rh
	}
	if addY < rowsTop {
		addY = rowsTop
	}
	z.addRowBtn.Text = ""
	slot := image.Rect(panelRect.Min.X, addY, panelRect.Max.X, addY+rh)
	minX, maxX := z.rowContentSpan(slot)
	if z.addRowRightReserve > 0 && maxX-z.addRowRightReserve > minX {
		maxX -= z.addRowRightReserve
	}
	top, bottom := rowControlBand(slot)
	z.addRowBtn.SetRect(image.Rect(minX, top, maxX, bottom))
}

// 0 disables.
func (z *RowRackZone) SetAddRowRightReserve(px int) {
	if z.addRowRightReserve == px {
		return
	}
	z.addRowRightReserve = px
	z.needLayout = true
}

func (z *RowRackZone) AddRowBtnRect() image.Rectangle {
	if z.addRowBtn == nil {
		return image.Rectangle{}
	}
	return z.addRowBtn.Rect()
}

// --- Scroll helpers ---

// Sets needLayout only when it moves, so callers inside Layout do not re-loop.
func (z *RowRackZone) clampRowOffset() {
	maxOffset := len(z.rows()) - z.VisibleRows()
	if maxOffset < 0 {
		maxOffset = 0
	}
	off := z.rowOffset
	if off > maxOffset {
		off = maxOffset
	}
	if off < 0 {
		off = 0
	}
	if off != z.rowOffset {
		z.rowOffset = off
		z.needLayout = true
	}
}

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

		// No label hit while renaming: the text input overlaps it.
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

	// z+1 so sliders win over the label; clicks off the track still reach the label.
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

// InputCaptured, so tap-and-slide works in one gesture.
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

// The single wheel-scroll behaviour for both the rack and the cell grid, so they cannot drift.
// Throttled so a spun wheel cannot fly through the list.
func (z *RowRackZone) ScrollByWheel(steps int) bool {
	if steps == 0 {
		return false
	}
	z.syncScroll()
	if z.rowScroll.WheelStep(steps, controlGridScrollCooldownFrames) {
		z.flushScroll()
		if z.callbacks.OnScrollChanged != nil {
			z.callbacks.OnScrollChanged()
		}
		return true
	}
	return false
}

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
	// Always consumed, so the wheel never leaks past the rack.
	h.zone.ScrollByWheel(steps)
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
		if !e.fxBtn.Rect().Empty() && e.fxBtn.pressed != (audio.ActiveInsertEffectCount(rows[i].Instrument) > 0) {
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
		// Painted by the label itself, so it sits on the opaque body.
		e.label.LeadSwatch = rowIdentityColor(rows[i].Color)
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
			applyRowToggle(e.muteBtn, rows[i].Color, roleMute, rows[i].Muted)
			drawBtnOff(cache, e.muteBtn, offsetX, offsetY)
		}
		if !e.soloBtn.Rect().Empty() {
			applyRowToggle(e.soloBtn, rows[i].Color, roleSolo, rows[i].Solo)
			drawBtnOff(cache, e.soloBtn, offsetX, offsetY)
		}
		if !e.fxBtn.Rect().Empty() {
			// Only enabled effects light it; pressed carries the state so the cache rebuilds on a toggle.
			active := (audio.ActiveInsertEffectCount(rows[i].Instrument) > 0)
			applyRowToggle(e.fxBtn, rows[i].Color, roleFX, active)
			drawBtnOff(cache, e.fxBtn, offsetX, offsetY)
			if active {
				badgeR := e.fxBtn.Rect().Sub(image.Pt(offsetX, offsetY))
				drawFXBadge(cache, badgeR, audio.ActiveInsertEffectCount(rows[i].Instrument), rows[i].Color)
			}
		}
		drawBtnOff(cache, e.originBtn, offsetX, offsetY)
		if deleteConfirmRow == i && (frame-deleteConfirmFrame) < 120 {
			// A text warning glyph, distinct from the resting close icon.
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
		e.label.LeadSwatch = rowIdentityColor(rows[i].Color)
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
			applyRowToggle(e.muteBtn, rows[i].Color, roleMute, rows[i].Muted)
			e.muteBtn.Draw(dst)
		}
		if !e.soloBtn.Rect().Empty() {
			applyRowToggle(e.soloBtn, rows[i].Color, roleSolo, rows[i].Solo)
			e.soloBtn.Draw(dst)
		}
		e.originBtn.Draw(dst)
		if deleteConfirmRow == i && (frame-deleteConfirmFrame) < 120 {
			// A text warning glyph, distinct from the resting close icon.
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
			active := (audio.ActiveInsertEffectCount(rows[i].Instrument) > 0)
			applyRowToggle(e.fxBtn, rows[i].Color, roleFX, active)
			e.fxBtn.Draw(dst)
			if active {
				drawFXBadge(dst, e.fxBtn.Rect(), audio.ActiveInsertEffectCount(rows[i].Instrument), rows[i].Color)
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

// The same volume button as master; only the wiring and tint differ.
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

// Exactly the row color the grid nodes use (DrumRow.Color, full opacity), so slider and nodes match.
func volIconColor(vol float64, muted bool, rowColor color.Color) color.Color {
	if vol <= 0 || muted {
		return colTextDisabled
	}
	if rowColor != nil {
		return rowColor
	}
	return colVolumeIconOn
}

// The same level underline on every platform and for master too.
func drawVolCell(dst *ebiten.Image, s *Slider, vol float64, rowColor color.Color, muted bool) {
	drawVolIcon(dst, s, vol, muted, rowColor)
}

// drawVolCellOff is the offset-cache counterpart of drawVolCell.
func drawVolCellOff(cache *ebiten.Image, s *Slider, vol float64, offsetX, offsetY int, rowColor color.Color, muted bool) {
	drawVolIconOff(cache, s, vol, offsetX, offsetY, muted, rowColor)
}

// Verify interface at compile time.
var _ Zone = (*RowRackZone)(nil)
