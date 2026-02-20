package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

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

	rowOffset       int
	selRow          int
	visRowsOverride int // if > 0, overrides VisibleRows() calculation

	// Row controls cache.
	controlsCache       *ebiten.Image
	controlsCacheDirty  bool
	controlsCacheRowOff int
	controlsCacheVis    int
	controlsCacheRect   image.Rectangle

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
	z.addRowBtn = NewButton("+", addRowButtonStyle{}, func() {
		if z.callbacks.OnAddRow != nil {
			z.callbacks.OnAddRow()
		}
	})
	z.addRowBtn.TextColor = colTextSecondary
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
	z.rect = rect
	z.needLayout = false
	z.rebuildEntries()
	z.rebuildHitAreas()
}

func (z *RowRackZone) Update() {
	// Check if row count changed; re-layout if so.
	rows := z.rows()
	if len(rows) != len(z.entries) {
		z.needLayout = true
	}
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

		mute := NewButton("M", InstButtonStyle, nil)
		mute.OnClick = func() {
			if z.callbacks.OnMuteToggle != nil {
				z.callbacks.OnMuteToggle(idx)
			}
		}

		solo := NewButton("S", InstButtonStyle, nil)
		solo.OnClick = func() {
			if z.callbacks.OnSoloToggle != nil {
				z.callbacks.OnSoloToggle(idx)
			}
		}

		origin := NewButton("O", InstButtonStyle, nil)
		origin.OnClick = func() {
			if z.callbacks.OnOriginReq != nil {
				z.callbacks.OnOriginReq(idx)
			}
		}

		del := NewButton("X", InstButtonStyle, nil)
		del.ConsumeOnPress = true
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

		menu := NewButton("", InstButtonStyle, nil)
		menu.Icon = "overflow"
		menu.OnClick = func() {
			if z.callbacks.OnContextMenuOpen != nil {
				z.callbacks.OnContextMenuOpen(idx)
			}
		}

		fx := NewButton("FX", InstButtonStyle, nil)
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
	z.positionAddRowBtn(panelRect.Min.Y, panelRect, vis)

	// Set container bounds for slider group.
	rowsBottom := panelRect.Min.Y + vis*z.rowHeight()
	z.rowVolGroup.SetContainerBounds(image.Rect(panelRect.Min.X, panelRect.Min.Y, panelRect.Max.X, rowsBottom))

	// Invalidate controls cache.
	z.controlsCacheDirty = true
}

// repositionEntries updates widget rects for existing entries without
// recreating button/slider instances. This preserves capture state
// (e.g., active slider drags) across Layout() calls.
func (z *RowRackZone) repositionEntries(rows []*DrumRow, vis int, panelRect image.Rectangle) {
	for i := range z.entries {
		rowRect := z.rowRectForIndex(i, panelRect.Min.Y, vis, panelRect)

		// Update label text/style in case name or availability changed.
		if i < len(rows) {
			e := &z.entries[i]
			e.label.Text = rows[i].Name
			style := Profile().RowLabelStyle
			if z.callbacks.IsInstrumentAvail != nil && !z.callbacks.IsInstrumentAvail(rows[i].Instrument) {
				style = MissingInstStyle
			}
			e.label.Style = style
			e.volSlider.Value = rows[i].Volume
		}

		// Reposition all widgets.
		z.positionRowWidgets(i, rowRect)
	}

	// Sync slider group (preserves active index since count is unchanged).
	z.rowVolGroup.SetSliders(z.volSliderSlice())

	// Position the "+" button.
	z.positionAddRowBtn(panelRect.Min.Y, panelRect, vis)

	// Set container bounds for slider group.
	rowsBottom := panelRect.Min.Y + vis*z.rowHeight()
	z.rowVolGroup.SetContainerBounds(image.Rect(panelRect.Min.X, panelRect.Min.Y, panelRect.Max.X, rowsBottom))

	// Invalidate controls cache.
	z.controlsCacheDirty = true
}

func (z *RowRackZone) makeColorButton(idx int) *Button {
	colorFn := func() color.Color {
		rows := z.rows()
		if idx >= 0 && idx < len(rows) {
			return rows[idx].Color
		}
		return color.RGBA{200, 200, 200, 255}
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
	e.label.SetRect(insetRect(g.Cell(0, 0), buttonPad))
	if Profile().IsMobile() {
		// Mobile: only label + volume icon; other controls live in context menu.
		e.menuBtn.SetRect(image.Rectangle{})
		e.editBtn.SetRect(image.Rectangle{})
		e.saveBtn.SetRect(image.Rectangle{})
		e.colorBtn.SetRect(image.Rectangle{})
		e.volSlider.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		e.muteBtn.SetRect(image.Rectangle{})
		e.soloBtn.SetRect(image.Rectangle{})
		e.fxBtn.SetRect(image.Rectangle{})
		e.originBtn.SetRect(image.Rectangle{})
		e.deleteBtn.SetRect(image.Rectangle{})
	} else {
		// Desktop: Label | VolBar | M | S | FX | ⋯
		// Hide edit/save/color/origin/delete — they live in overflow menu.
		e.editBtn.SetRect(image.Rectangle{})
		e.saveBtn.SetRect(image.Rectangle{})
		e.colorBtn.SetRect(image.Rectangle{})
		e.volSlider.SetRect(insetRect(g.Cell(1, 0), buttonPad))
		e.muteBtn.SetRect(insetRect(g.Cell(2, 0), buttonPad))
		e.soloBtn.SetRect(insetRect(g.Cell(3, 0), buttonPad))
		e.fxBtn.SetRect(insetRect(g.Cell(4, 0), buttonPad))
		e.originBtn.SetRect(image.Rectangle{})
		e.deleteBtn.SetRect(image.Rectangle{})
		e.menuBtn.SetRect(insetRect(g.Cell(5, 0), buttonPad))
	}
}

// positionAddRowBtn sets the "+" button rect.
func (z *RowRackZone) positionAddRowBtn(rowsTop int, panelRect image.Rectangle, vis int) {
	rows := z.rows()
	nBelow := len(rows) - z.rowOffset
	if nBelow > vis {
		nBelow = vis
	}
	rh := z.rowHeight()
	addY := rowsTop + nBelow*rh
	rackBottom := panelRect.Max.Y
	if addY+rh > rackBottom {
		addY = rackBottom - rh
	}
	if addY < rowsTop {
		addY = rowsTop
	}
	if Profile().IsMobile() {
		if z.isMobileEQMode() {
			z.addRowBtn.SetRect(image.Rectangle{})
		} else {
			// Guard: skip positioning if zone rect is empty (transient layout state).
			if panelRect.Empty() {
				z.addRowBtn.SetRect(image.Rectangle{})
				return
			}
			z.addRowBtn.Text = "+"
			z.addRowBtn.Icon = ""
			btnW := 28
			btnH := 24
			// Use screen bounds for FAB X position (not the narrow rack panel).
			fabMaxX := panelRect.Max.X
			if !z.screenBounds.Empty() {
				fabMaxX = z.screenBounds.Max.X
			}
			fabX := fabMaxX - btnW - 20
			fabY := panelRect.Max.Y - btnH - 8
			if fabY < rowsTop {
				fabY = rowsTop
			}
			fabRect := image.Rect(fabX, fabY, fabX+btnW, fabY+btnH)
			// Clamp FAB rect to screenBounds to prevent rendering outside the drum view.
			if !z.screenBounds.Empty() {
				fabRect = fabRect.Intersect(z.screenBounds)
				if fabRect.Empty() {
					z.addRowBtn.SetRect(image.Rectangle{})
					return
				}
			}
			z.addRowBtn.SetRect(fabRect)
		}
	} else {
		// Guard: skip positioning if zone rect is empty (transient layout state).
		if panelRect.Empty() {
			z.addRowBtn.SetRect(image.Rectangle{})
			return
		}
		z.addRowBtn.SetRect(insetRect(image.Rect(panelRect.Min.X, addY, panelRect.Max.X, addY+rh), buttonPad))
	}
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
	h.zone.syncScroll()
	if h.zone.rowScroll.HandleWheel(steps) {
		h.zone.flushScroll()
		if h.zone.callbacks.OnScrollChanged != nil {
			h.zone.callbacks.OnScrollChanged()
		}
		return InputConsumed
	}
	return InputIgnored
}

// --- Drawing ---

func (z *RowRackZone) controlsCacheValid() bool {
	if z.controlsCache == nil || z.controlsCacheDirty {
		return false
	}
	vis := z.VisibleRows()
	if z.controlsCacheRowOff != z.rowOffset || z.controlsCacheVis != vis {
		return false
	}
	if z.controlsCacheRect.Empty() {
		return false
	}
	rows := z.rows()
	for i := z.rowOffset; i < z.rowOffset+vis && i < len(rows) && i < len(z.entries); i++ {
		e := &z.entries[i]
		if !e.muteBtn.Rect().Empty() && e.muteBtn.pressed != rows[i].Muted {
			return false
		}
		if !e.soloBtn.Rect().Empty() && e.soloBtn.pressed != rows[i].Solo {
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
		z.controlsCache = ebiten.NewImage(w, h)
	} else {
		z.controlsCache.Clear()
	}
	z.drawRowControlsToCache(z.controlsCache, bounds.Min.X, bounds.Min.Y)
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
				drawRect(cache, zebraR, color.NRGBA{255, 255, 255, 10}, true)
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
			e.muteBtn.pressed = rows[i].Muted
			if rows[i].Muted {
				e.muteBtn.Style = MuteActiveStyle
			} else {
				e.muteBtn.Style = InstButtonStyle
			}
			drawBtnOff(cache, e.muteBtn, offsetX, offsetY)
		}
		if !e.soloBtn.Rect().Empty() {
			e.soloBtn.pressed = rows[i].Solo
			if rows[i].Solo {
				e.soloBtn.Style = SoloActiveStyle
			} else {
				e.soloBtn.Style = InstButtonStyle
			}
			drawBtnOff(cache, e.soloBtn, offsetX, offsetY)
		}
		if !e.fxBtn.Rect().Empty() {
			if len(rows[i].Effects) > 0 {
				e.fxBtn.Style = FXActiveStyle
			} else {
				e.fxBtn.Style = InstButtonStyle
			}
			drawBtnOff(cache, e.fxBtn, offsetX, offsetY)
		}
		drawBtnOff(cache, e.originBtn, offsetX, offsetY)
		if deleteConfirmRow == i && (frame-deleteConfirmFrame) < 120 {
			e.deleteBtn.Text = "!!"
			e.deleteBtn.Style = DeleteConfirmButtonStyle
		} else {
			e.deleteBtn.Text = "X"
			if len(rows) > 1 {
				e.deleteBtn.Style = InstButtonStyle
			}
		}
		drawBtnOff(cache, e.deleteBtn, offsetX, offsetY)
		if mobile {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				sepY := lblR.Max.Y - offsetY
				sepR := image.Rect(lblR.Min.X-offsetX, sepY, lblR.Max.X-offsetX, sepY+1)
				drawRect(cache, sepR, color.NRGBA{255, 255, 255, 16}, true)
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
				drawRect(dst, lblR, color.NRGBA{255, 255, 255, 6}, true)
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
			e.deleteBtn.Text = "!!"
			e.deleteBtn.Style = DeleteConfirmButtonStyle
		} else {
			e.deleteBtn.Text = "X"
			if len(rows) > 1 {
				e.deleteBtn.Style = InstButtonStyle
			}
		}
		e.deleteBtn.Draw(dst)
		if !e.fxBtn.Rect().Empty() {
			if len(rows[i].Effects) > 0 {
				e.fxBtn.Style = FXActiveStyle
			} else {
				e.fxBtn.Style = InstButtonStyle
			}
			e.fxBtn.Draw(dst)
		}
		if mobile {
			lblR := e.label.Rect()
			if !lblR.Empty() {
				sepY := lblR.Max.Y
				drawRect(dst, image.Rect(lblR.Min.X, sepY, lblR.Max.X, sepY+1), color.NRGBA{255, 255, 255, 16}, true)
			}
		}
	}
}

// drawVolIcon draws a small speaker icon in the slider's rect (mobile).
func drawVolIcon(dst *ebiten.Image, s *Slider, vol float64, muted bool, rowColor color.Color) {
	r := s.Rect()
	if r.Empty() {
		return
	}
	// Determine icon color: instrument color tint when active, disabled when muted/zero.
	iconCol := volIconColor(vol, muted, rowColor)
	cellH := r.Dy()
	// On mobile, use IconSizeMD (20px) as the target icon height.
	iconH := cellH * 60 / 100
	if Profile().IsMobile() && iconH < IconSizeMD {
		iconH = IconSizeMD
	}
	if iconH < 8 {
		iconH = 8
	}
	iconW := iconH * 3 / 4
	if iconW < 6 {
		iconW = 6
	}
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	bodyW := iconW / 3
	if bodyW < 2 {
		bodyW = 2
	}
	bodyH := iconH / 3
	if bodyH < 2 {
		bodyH = 2
	}
	drawRect(dst, image.Rect(cx-bodyW/2, cy-bodyH/2, cx-bodyW/2+bodyW, cy-bodyH/2+bodyH), iconCol, true)
	coneW := iconW / 3
	if coneW < 2 {
		coneW = 2
	}
	coneH := iconH * 2 / 3
	if coneH < 3 {
		coneH = 3
	}
	coneX := cx - bodyW/2 + bodyW
	drawRect(dst, image.Rect(coneX, cy-coneH/2, coneX+coneW, cy+coneH/2), iconCol, true)
	if vol > 0 {
		barH := int(float64(iconH) * vol * 0.8)
		if barH < 2 {
			barH = 2
		}
		barX := coneX + coneW + 2
		drawRect(dst, image.Rect(barX, cy-barH/2, barX+2, cy+barH/2), iconCol, true)
	}
}

// drawVolIconOff draws a speaker icon into a cache image with coordinate offset.
func drawVolIconOff(cache *ebiten.Image, s *Slider, vol float64, offsetX, offsetY int, muted bool, rowColor color.Color) {
	r := s.Rect()
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	// Determine icon color: instrument color tint when active, disabled when muted/zero.
	iconCol := volIconColor(vol, muted, rowColor)
	cellH := r.Dy()
	// On mobile, use IconSizeMD (20px) as the target icon height.
	iconH := cellH * 60 / 100
	if Profile().IsMobile() && iconH < IconSizeMD {
		iconH = IconSizeMD
	}
	if iconH < 8 {
		iconH = 8
	}
	iconW := iconH * 3 / 4
	if iconW < 6 {
		iconW = 6
	}
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + r.Dy()/2
	bodyW := iconW / 3
	if bodyW < 2 {
		bodyW = 2
	}
	bodyH := iconH / 3
	if bodyH < 2 {
		bodyH = 2
	}
	drawRect(cache, image.Rect(cx-bodyW/2, cy-bodyH/2, cx-bodyW/2+bodyW, cy-bodyH/2+bodyH), iconCol, true)
	coneW := iconW / 3
	if coneW < 2 {
		coneW = 2
	}
	coneH := iconH * 2 / 3
	if coneH < 3 {
		coneH = 3
	}
	coneX := cx - bodyW/2 + bodyW
	drawRect(cache, image.Rect(coneX, cy-coneH/2, coneX+coneW, cy+coneH/2), iconCol, true)
	if vol > 0 {
		barH := int(float64(iconH) * vol * 0.8)
		if barH < 2 {
			barH = 2
		}
		barX := coneX + coneW + 2
		drawRect(cache, image.Rect(barX, cy-barH/2, barX+2, cy+barH/2), iconCol, true)
	}
}

// volIconColor returns the appropriate color for a volume icon.
// Uses instrument color tint when volume > 0 and not muted, colTextDisabled otherwise.
func volIconColor(vol float64, muted bool, rowColor color.Color) color.Color {
	if vol <= 0 || muted {
		return colTextDisabled
	}
	if rowColor != nil {
		cr, cg, cb, _ := rowColor.RGBA()
		return color.NRGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), 220}
	}
	return colVolumeIconOn
}

// drawVolCell draws a speaker icon + mini volume bar in the slider's rect.
// On mobile the cell is too narrow for the bar, so only the icon is drawn.
func drawVolCell(dst *ebiten.Image, s *Slider, vol float64, rowColor color.Color, muted bool) {
	drawVolIcon(dst, s, vol, muted, rowColor)
	if Profile().IsMobile() {
		return
	}
	r := s.Rect()
	if r.Empty() {
		return
	}
	drawVolBar(dst, r, vol, rowColor)
}

// drawVolCellOff draws icon + mini volume bar into a cache image with offset.
func drawVolCellOff(cache *ebiten.Image, s *Slider, vol float64, offsetX, offsetY int, rowColor color.Color, muted bool) {
	drawVolIconOff(cache, s, vol, offsetX, offsetY, muted, rowColor)
	if Profile().IsMobile() {
		return
	}
	r := s.Rect()
	if r.Empty() {
		return
	}
	r = r.Sub(image.Pt(offsetX, offsetY))
	drawVolBar(cache, r, vol, rowColor)
}

// drawVolBar draws a mini horizontal volume bar inside rect r.
// Track: 4px tall, colSurface1 background, full width of cell.
// Fill: instrument color at 60% opacity, width proportional to volume.
func drawVolBar(dst *ebiten.Image, r image.Rectangle, vol float64, rowColor color.Color) {
	const trackH = 4
	// Position the bar to the right of center (leaving room for the speaker icon).
	barX := r.Min.X + r.Dx()/2 + 2
	barW := r.Max.X - barX - 2
	if barW < 4 {
		return
	}
	cy := r.Min.Y + r.Dy()/2
	trackRect := image.Rect(barX, cy-trackH/2, barX+barW, cy-trackH/2+trackH)
	drawRect(dst, trackRect, colSurface1, true)

	// Fill proportional to volume with instrument color at 60% alpha.
	fillW := int(math.Round(float64(barW) * vol))
	if fillW > 0 {
		cr, cg, cb, _ := rowColor.RGBA()
		fillCol := color.NRGBA{uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), 153} // 60% of 255
		fillRect := image.Rect(barX, cy-trackH/2, barX+fillW, cy-trackH/2+trackH)
		drawRect(dst, fillRect, fillCol, true)
	}
}

// Verify interface at compile time.
var _ Zone = (*RowRackZone)(nil)
