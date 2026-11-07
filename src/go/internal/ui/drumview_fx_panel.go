package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// fxSearchMobileInputID is the native-input id of the FX "add effect" search.
const fxSearchMobileInputID = "fx-search"

// Text = fxToggleTag+"on"/"off" tells drawFXPanel to render a pill switch, not a checkbox.
const fxToggleTag = "\x00fxtoggle:"

type addEffectButtonStyle struct{}

func (addEffectButtonStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) {
	if r.Empty() {
		return
	}
	var fill color.Color = colSurface1
	if pressed {
		fill = adjustColor(fill, -20)
	} else if hovered {
		fill = adjustColor(fill, 12)
	}
	rad := fitRadius(RadiusControl, r)
	drawRoundedRect(dst, r, fill, rad, true)
	drawDashedRoundedBorder(dst, r, colBorderMedium, rad)
}

// Shared with the Synth stage pills, so its ON tint stays; the FX panel uses
// drawFXTogglePillCol with the accent (one chrome accent).
func drawFXTogglePill(dst *ebiten.Image, r image.Rectangle, enabled bool) {
	drawFXTogglePillCol(dst, r, enabled, colPlayGreen)
}

func drawFXTogglePillCol(dst *ebiten.Image, r image.Rectangle, enabled bool, onCol color.Color) {
	trackW, trackH := FXToggleTrackW(), FXToggleTrackH()
	thumbD := FXToggleThumbD()

	// Center the track vertically and left-align within the rect.
	cx := r.Min.X + (r.Dx()-trackW)/2
	cy := r.Min.Y + (r.Dy()-trackH)/2
	trackR := image.Rect(cx, cy, cx+trackW, cy+trackH)

	// Track color: ON = onCol, OFF = colSurface3.
	var trackCol color.Color
	if enabled {
		trackCol = onCol
	} else {
		trackCol = colSurface3
	}
	drawCapsule(dst, trackR, trackCol, true)

	// Thumb: circle centered vertically in the track.
	thumbY := cy + (trackH-thumbD)/2
	var thumbX int
	if enabled {
		thumbX = cx + trackW - thumbD - (trackH-thumbD)/2
	} else {
		thumbX = cx + (trackH-thumbD)/2
	}
	thumbR := image.Rect(thumbX, thumbY, thumbX+thumbD, thumbY+thumbD)

	// Thumb color: ON = white, OFF = colTextSecondary.
	var thumbCol color.Color
	if enabled {
		thumbCol = genColorBorder
	} else {
		thumbCol = colTextSecondary
	}
	drawCapsule(dst, thumbR, thumbCol, true)
}

// For a stage that is the instrument's only sound source: it must stay on, so the pill reads
// "locked on", not a toggle that would silence the instrument.
func drawFXTogglePillDisabled(dst *ebiten.Image, r image.Rectangle) {
	trackW, trackH := FXToggleTrackW(), FXToggleTrackH()
	thumbD := FXToggleThumbD()

	cx := r.Min.X + (r.Dx()-trackW)/2
	cy := r.Min.Y + (r.Dy()-trackH)/2
	trackR := image.Rect(cx, cy, cx+trackW, cy+trackH)
	// Dimmed ON track — the muted tint is the "locked" signal.
	drawCapsule(dst, trackR, WithAlpha(colPlayGreen, AlphaSubtle), true)

	// Thumb in the ON (right) position, greyed to read non-interactive.
	thumbY := cy + (trackH-thumbD)/2
	thumbX := cx + trackW - thumbD - (trackH-thumbD)/2
	thumbR := image.Rect(thumbX, thumbY, thumbX+thumbD, thumbY+thumbD)
	drawCapsule(dst, thumbR, TokenTextDisabled(), true)
}

func (dv *DrumView) fxAddRowEffectFor(btn *Button) (audio.EffectType, bool) {
	for i, b := range dv.fxAddRowBtn {
		if b == btn {
			return dv.fxAddRowEffect[i], true
		}
	}
	return "", false
}

func hoverStateOf(b *Button) menuItemState {
	if b != nil && (b.hovered || b.pressed) {
		return menuItemHover
	}
	return menuItemRest
}

// isFXToggleBtn returns whether a button is an FX panel toggle, and its state.
func isFXToggleBtn(btn *Button) (isToggle bool, enabled bool) {
	if btn == nil {
		return false, false
	}
	if strings.HasPrefix(btn.Text, fxToggleTag) {
		return true, btn.Text == fxToggleTag+"on"
	}
	// Legacy fallback for tests that search by "✓" or " ".
	return false, false
}

// toggleFXPanel opens or closes the FX panel for the given row.
func (dv *DrumView) toggleFXPanel(row int) {
	if dv.IsFXPanelOpen() && dv.fxPanelRow == row {
		dv.closeFXPanel()
		return
	}
	// Close all other overlays for mutual exclusivity.
	dv.CloseAllPopups()
	dv.openFXPanel(row)
}

// Pixel mode (ItemHeight=1), so the FX list reuses the same MenuScroll engine as every menu.
func (dv *DrumView) ensureFXScroll() *MenuScroll {
	if dv.fxScroll == nil {
		dv.fxScroll = NewMenuScroll(ScrollbarStyleForPlatform(), 1)
	}
	return dv.fxScroll
}

func (dv *DrumView) fxScrollOffset() int {
	if dv.fxScroll == nil {
		return 0
	}
	return dv.fxScroll.OffsetPx()
}

func (dv *DrumView) fxHasScroll() bool {
	return dv.fxScroll != nil && dv.fxScroll.HasScroll()
}

const fxWheelStepPx = 20

func (dv *DrumView) openFXPanel(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	dv.fxPanelRow = row
	// Fresh scroll for this open (offset 0), styled for the current platform.
	dv.fxScroll = NewMenuScroll(ScrollbarStyleForPlatform(), 1)
	dv.fxExpandedSlots = map[int]bool{}
	dv.fxListScrollPx = 0
	// The search box widget persists across opens, so a stale query must be cleared explicitly.
	dv.clearFXAddSearch()
	dv.buildFXPanel()      // compute correct fxPanelRect FIRST
	dv.openFXPanelPortal() // register portal with correct rect
}

func (dv *DrumView) closeFXPanel() {
	dv.closeFXPanelPortal()
	dv.fxAddMenuOpen = false
	dv.fxPanelBtns = nil
	dv.fxAddEffectBtn = nil
	dv.fxPanelSliders = nil
	dv.fxSliderDragging = false
	dv.fxPanelDeferredTap.Cancel()
	dv.fxScroll = nil
	dv.fxPanelRect = image.Rectangle{}
	dv.fxAddSearchRect = image.Rectangle{}
	dv.clearFXAddSearch()
}

func (dv *DrumView) OpenFXPanel(row int) {
	if dv.IsFXPanelOpen() && dv.fxPanelRow == row {
		return
	}
	dv.CloseAllPopups()
	dv.openFXPanel(row)
}

// The row's instrument color, so the panel reads as owned by the instrument it edits.
func (dv *DrumView) fxPanelAccent() color.Color {
	if dv.fxPanelRow >= 0 && dv.fxPanelRow < len(dv.Rows) {
		if c := dv.Rows[dv.fxPanelRow].Color; c != nil {
			return c
		}
	}
	return colAccent
}

// Tracked by identity, so it never depends on being the last button.
func (dv *DrumView) fxPanelCloseBtn() *Button { return dv.fxPanelCloseButton }

// CloseFXPanel closes the FX panel.
func (dv *DrumView) CloseFXPanel() { dv.closeFXPanel() }

// FXPanelRow returns the row index of the currently open FX panel, or -1.
func (dv *DrumView) FXPanelRow() int {
	if !dv.IsFXPanelOpen() {
		return -1
	}
	return dv.fxPanelRow
}

func (dv *DrumView) FXPanelRect() (image.Rectangle, bool) {
	if !dv.IsFXPanelOpen() {
		return image.Rectangle{}, false
	}
	if dv.fxPanelRect.Empty() {
		return image.Rectangle{}, false
	}
	return dv.fxPanelRect, true
}

// propagateFXSliderValue pushes the current slider value to the audio engine.
func (dv *DrumView) propagateFXSliderValue(idx int) {
	if idx >= len(dv.fxPanelSliders) || idx >= len(dv.fxSliderBindings) {
		return
	}
	sl := dv.fxPanelSliders[idx]
	b := dv.fxSliderBindings[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	instID := dv.Rows[dv.fxPanelRow].Instrument
	audio.SetInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
}

// commitFXSlider emits + records an insert-FX param change once, at release.
func (dv *DrumView) commitFXSlider(idx int) {
	if idx < 0 || idx >= len(dv.fxSliderBindings) {
		return
	}
	b := dv.fxSliderBindings[idx]
	sl := dv.fxPanelSliders[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	instID := dv.Rows[dv.fxPanelRow].Instrument
	emitInsertEffectParam(channel.Of(instID), b.slotIndex, b.paramName, actual)
	dv.recordUndoStep(hooks.EventInsertEffectParam)
}

// effectTypeName returns the display name for an effect type from the registry.
func effectTypeName(t audio.EffectType) string {
	regs := audio.EffectRegistrations()
	if r, ok := regs[t]; ok {
		return r.DisplayName
	}
	return string(t)
}

// effectTypeOrder returns all effect types in registration order.
func effectTypeOrder() []audio.EffectType {
	return audio.EffectTypeOrder()
}

var fxCategoryOrder = []string{"creative", "modulation", "dynamics", "utility"}

var fxCategoryDisplay = map[string]string{
	"creative":   "Creative",
	"modulation": "Modulation",
	"dynamics":   "Dynamics",
	"utility":    "Utility",
}

// Falls back to the raw category so an unmapped one never renders a blank header.
func fxCategoryLabel(cat string) string {
	if label, ok := fxCategoryDisplay[cat]; ok {
		return label
	}
	return cat
}

// Unknown categories sort last.
func fxCategoryOrderIndex(cat string) int {
	for i, c := range fxCategoryOrder {
		if c == cat {
			return i
		}
	}
	return len(fxCategoryOrder)
}

// Stable, so registration order holds within a category.
func fxGroupEffectsByCategory(order []audio.EffectType) []audio.EffectType {
	regs := audio.EffectRegistrations()
	out := make([]audio.EffectType, len(order))
	copy(out, order)
	sort.SliceStable(out, func(i, j int) bool {
		ci, cj := "", ""
		if r := regs[out[i]]; r != nil {
			ci = r.Category
		}
		if r := regs[out[j]]; r != nil {
			cj = r.Category
		}
		return fxCategoryOrderIndex(ci) < fxCategoryOrderIndex(cj)
	})
	return out
}

func fxAddCategoryHeaderCount(effects []audio.EffectType) int {
	regs := audio.EffectRegistrations()
	haveLast := false
	lastCat := ""
	n := 0
	for _, et := range effects {
		cat := ""
		if r := regs[et]; r != nil {
			cat = r.Category
		}
		if !haveLast || cat != lastCat {
			haveLast = true
			lastCat = cat
			n++
		}
	}
	return n
}

// Y is the scrolled screen top, recorded in the same pass as the effect rows so draw and build
// cannot drift.
type fxAddHeaderRow struct {
	Y     int
	Label string
}

// Always empty while searching.
func (dv *DrumView) fxAddGroupHeaders() []string {
	out := make([]string, len(dv.fxAddHeaderRows))
	for i, h := range dv.fxAddHeaderRows {
		out[i] = h.Label
	}
	return out
}

func (dv *DrumView) fxAddInstanceCount(et audio.EffectType) int {
	if dv.fxPanelRow < 0 || dv.fxPanelRow >= len(dv.Rows) {
		return 0
	}
	instID := dv.Rows[dv.fxPanelRow].Instrument
	n := 0
	for _, e := range audio.GetInsertEffects(instID) {
		if e.Type == et {
			n++
		}
	}
	return n
}

// Searching shows the category (results leave their group); idle shows "×N" when the chain
// already holds that effect.
func (dv *DrumView) fxAddRowTrail(et audio.EffectType) string {
	if strings.TrimSpace(dv.fxAddSearchQuery) != "" {
		if r := audio.EffectRegistrations()[et]; r != nil {
			return fxCategoryLabel(r.Category)
		}
		return ""
	}
	if n := dv.fxAddInstanceCount(et); n > 0 {
		return "×" + strconv.Itoa(n)
	}
	return ""
}

// Also refreshes fxAddSearchHighlights for drawMenuRow.
func (dv *DrumView) fxAddVisibleEffects() []audio.EffectType {
	order := effectTypeOrder()
	q := strings.TrimSpace(dv.fxAddSearchQuery)
	if q == "" {
		dv.fxAddSearchHighlights = nil
		return fxGroupEffectsByCategory(order)
	}
	regs := audio.EffectRegistrations()
	items := make([]MenuSearchItem, 0, len(order))
	for _, et := range order {
		if r := regs[et]; r != nil {
			items = append(items, MenuSearchItem{Key: string(et), Label: r.DisplayName})
		}
	}
	res := (&MenuSearcher{}).Search(q, items)
	out := make([]audio.EffectType, 0, len(res))
	hl := make(map[audio.EffectType][]int, len(res))
	for _, r := range res {
		et, err := audio.ParseEffectType(r.Key)
		if err != nil {
			continue
		}
		out = append(out, et)
		hl[et] = r.Highlights
	}
	dv.fxAddSearchHighlights = hl
	return out
}

// Programmatic changes only: on the typing path SetText would jump the caret to the end.
func (dv *DrumView) setFXAddSearchQuery(q string) {
	if dv.fxAddSearchQuery == q {
		return
	}
	dv.fxAddSearchQuery = q
	if dv.fxAddSearch != nil {
		dv.fxAddSearch.SetText(q)
	}
	if dv.fxScroll != nil { // reset add-menu scroll to top on every query change
		dv.fxScroll.ScrollBehavior().VS.First = 0
	}
	dv.buildFXPanel()
	dv.refreshFXPortalHitAreas()
}

// No rebuild: callers rebuild right after.
func (dv *DrumView) clearFXAddSearch() {
	dv.fxAddSearchQuery = ""
	dv.fxAddSearchHighlights = nil
	if dv.fxAddSearch != nil {
		dv.fxAddSearch.SetText("")
		dv.fxAddSearch.SetFocus(false)
	}
}

// Must rebuild: unlike the overflow menu, the FX panel only rebuilds on explicit events.
func (dv *DrumView) updateFXAddSearch() {
	if !dv.fxAddMenuOpen || dv.fxAddSearch == nil {
		return
	}
	// Mobile: the query is typed into the native <input>; mirror it first.
	if v, ok := dv.fxAddSearch.NativeLiveValue(); ok {
		dv.fxAddSearch.SetText(v)
	}
	dv.fxAddSearch.Update()
	if nv := dv.fxAddSearch.Value(); nv != dv.fxAddSearchQuery {
		dv.fxAddSearchQuery = nv
		dv.buildFXPanel()
		dv.refreshFXPortalHitAreas()
		emitSearchChanged("fx-add-menu", nv)
	}
}

// Clears a query first, then closes the add menu; false outside the add menu, so the portal's
// Esc closes the whole panel.
func (dv *DrumView) fxPanelHandleEscape() bool {
	if !dv.fxAddMenuOpen {
		return false
	}
	if strings.TrimSpace(dv.fxAddSearchQuery) != "" {
		dv.setFXAddSearchQuery("")
		return true
	}
	dv.closeFXAddMenu()
	return true
}

// Restores the list at the scroll offset it was left at.
func (dv *DrumView) closeFXAddMenu() {
	dv.fxAddMenuOpen = false
	dv.clearFXAddSearch()
	if dv.fxScroll != nil {
		dv.fxScroll.ScrollBehavior().VS.First = dv.fxListScrollPx
	}
	dv.fxListScrollPx = 0
	dv.buildFXPanel()
	dv.refreshFXPortalHitAreas()
}

// fxSliderBinding tracks which effect param a slider controls.
type fxSliderBinding struct {
	slotIndex int
	paramName string
	def       audio.EffectParamDef
}

func (dv *DrumView) fxShowParams(slotIdx int, enabled bool) bool {
	if !enabled {
		return false
	}
	if !Profile().UseBottomSheet {
		return true
	}
	return dv.fxExpandedSlots[slotIdx]
}

// Shared by build and draw, so hit rects and painted rows come from the same values.
type fxMetrics struct {
	lineH   int // effect header row height
	headerH int // panel title band height
	paramH  int // one param-slider row height
	footerH int // "+ Add Effect" footer height
}

func fxPanelMetrics() fxMetrics {
	if Profile().IsMobile() {
		return fxMetrics{
			lineH:   touchMinTargetPx,
			headerH: touchMinTargetPx,
			paramH:  touchMinTargetPx - 4,
			footerH: touchMinTargetPx,
		}
	}
	return fxMetrics{lineH: 28, headerH: 32, paramH: 26, footerH: 32}
}

func (dv *DrumView) buildFXPanel() {
	row := dv.fxPanelRow
	if row < 0 || row >= len(dv.Rows) {
		dv.closeFXPanelPortal()
		return
	}

	// The FX column has zero width on mobile.
	anchor := image.Rectangle{}
	if row < len(dv.rowFXBtns()) {
		anchor = dv.rowFXBtns()[row].Rect()
	}
	if anchor.Empty() && row < len(dv.rowLabels()) {
		anchor = dv.rowLabels()[row].Rect()
	}

	r := dv.Rows[row]
	instID := r.Instrument
	effects := audio.GetInsertEffects(instID)

	m := fxPanelMetrics()
	lineH, headerH, paramH, footerH := m.lineH, m.headerH, m.paramH, m.footerH
	var panelW int
	mobile := Profile().IsMobile()
	if mobile {
		panelW = dv.Bounds.Dx() - 32 // near full-width with 16px margin each side
		if panelW < 250 {
			panelW = 250
		}
	} else {
		panelW = 340
	}

	// Compute dynamic slider left offset based on longest visible param label.
	if !mobile {
		maxLabelW := 0
		for _, e := range effects {
			if e.Enabled {
				cat := audio.InsertEffectCatalog()
				if defs, ok := cat[e.Type]; ok {
					for _, def := range defs {
						val := e.Params[def.Name]
						label := fxParamLabel(def, val)
						if w := TextWidth(label); w > maxLabelW {
							maxLabelW = w
						}
					}
				}
			}
		}
		dv.fxSliderLeft = maxLabelW + 16 // 8px left pad + 8px gap
		if dv.fxSliderLeft < 90 {
			dv.fxSliderLeft = 90
		}
		// At least a 120 px usable slider track.
		minPanelW := SpaceXL + dv.fxSliderLeft + 120 + 8
		if panelW < minPanelW {
			panelW = minPanelW
		}
		if panelW > dv.Bounds.Dx()-16 {
			panelW = dv.Bounds.Dx() - 16
		}
	}

	searchH := 0
	if dv.fxAddMenuOpen {
		searchH = lineH
	}

	// The picker REPLACES the list in the same frame, never a footer under it, and is never shorter
	// than the list, so it always covers what it opened from.
	listH := headerH + footerH
	if len(effects) == 0 {
		listH += lineH // empty-state hint row above the Add button
	}
	for slotIdx, e := range effects {
		listH += lineH
		if dv.fxShowParams(slotIdx, e.Enabled) {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				listH += len(defs) * paramH
			}
		}
	}
	listed := effects // effect cards laid out in this view (none under the picker)
	if dv.fxAddMenuOpen {
		listed = nil
	}

	// Compute total content height (including header, search band, and footer).
	contentH := headerH + searchH
	var visibleEffectTypes []audio.EffectType
	fxAddSearching := strings.TrimSpace(dv.fxAddSearchQuery) != ""
	if dv.fxAddMenuOpen {
		visibleEffectTypes = dv.fxAddVisibleEffects()
		if len(visibleEffectTypes) == 0 {
			contentH += lineH + 2*lineH // leading Back row + centered no-match hint band
		} else {
			hdrN := 0
			if !fxAddSearching {
				hdrN = fxAddCategoryHeaderCount(visibleEffectTypes) // idle only: category group headers
			}
			contentH += len(visibleEffectTypes)*lineH + lineH + hdrN*lineH // N type rows + leading Back row + headers
		}
	} else {
		contentH = listH
	}
	if contentH < 80 {
		contentH = 80
	}

	// Limit panel height to available space.
	maxH := dv.Bounds.Dy() - 20
	panelH := contentH
	if dv.fxAddMenuOpen && panelH < listH {
		panelH = listH
	}
	if panelH > maxH {
		panelH = maxH
	}

	scrollContentH := contentH - headerH - searchH
	viewportH := panelH - headerH - searchH

	// Position: prefer above the anchor, fall back to below
	wantY := anchor.Min.Y - panelH
	if wantY < dv.Bounds.Min.Y {
		wantY = anchor.Max.Y
	}
	if wantY+panelH > dv.Bounds.Max.Y {
		wantY = dv.Bounds.Max.Y - panelH
	}
	if wantY < dv.Bounds.Min.Y {
		wantY = dv.Bounds.Min.Y
	}

	var wantX int
	if mobile {
		wantX = (dv.Bounds.Min.X + dv.Bounds.Max.X - panelW) / 2
	} else {
		wantX = anchor.Min.X - panelW + anchor.Dx()
	}
	if wantX < dv.Bounds.Min.X {
		wantX = dv.Bounds.Min.X
	}
	if wantX+panelW > dv.Bounds.Max.X {
		wantX = dv.Bounds.Max.X - panelW
	}

	dv.fxPanelRect = image.Rect(wantX, wantY, wantX+panelW, wantY+panelH)
	if dv.fxAddMenuOpen {
		dv.fxAddSearchRect = image.Rect(wantX, wantY+headerH, wantX+panelW, wantY+headerH+searchH)
		dv.fxAddSearch = ensureMenuSearchBox(dv.fxAddSearch, dv.fxAddSearchRect, dv.textFields,
			TextFieldSpec{ID: fxSearchMobileInputID, Owner: "fx-panel"})
	} else {
		dv.fxAddSearchRect = image.Rectangle{}
	}
	dv.fxViewportRect = image.Rect(wantX, wantY+headerH+searchH, wantX+panelW, wantY+panelH)

	dv.ensureFXScroll().Configure(dv.fxViewportRect, scrollContentH, viewportH)

	// Build buttons and sliders
	dv.fxPanelBtns = nil
	dv.fxPanelCloseButton = nil
	dv.fxAddEffectBtn = nil
	dv.fxPanelSliders = nil
	dv.fxSliderBindings = nil

	y := dv.fxPanelRect.Min.Y + headerH + searchH - dv.fxScrollOffset()
	x := dv.fxPanelRect.Min.X
	w := panelW

	for slotIdx, e := range listed {
		si := slotIdx
		enabled := e.Enabled

		// A little wider than the pill, so the hit area extends past it.
		toggleBtnW := FXToggleTrackW() + SpaceSM
		btnW := 24
		btnGap := 3
		if mobile {
			btnW = 36
			btnGap = 4
		}

		// Inset by the scrollbar's width so the × never overlaps the thumb.
		rightInset := 4
		if dv.fxHasScroll() {
			rightInset += ScrollbarStyleForPlatform().Width + SpaceXS
		}

		// Skip creating buttons for rows entirely outside viewport.
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		if inView {
			// Toggle button — tagged text so drawFXPanel renders a pill switch.
			toggleText := fxToggleTag + "on"
			if !enabled {
				toggleText = fxToggleTag + "off"
			}
			toggle := NewButton(toggleText, InstButtonStyle, nil)
			toggle.SetRect(image.Rect(x+4, y+2, x+4+toggleBtnW, y+lineH-2))
			toggle.OnClick = func() {
				audio.ToggleInsertEffect(instID, si, !enabled)
				emitInsertEffectToggled(channel.Of(instID), si, !enabled)
				dv.recordUndoStep(hooks.EventInsertEffectToggled)
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, toggle)

			if mobile && enabled {
				expBtn := NewButton("", InstButtonStyle, nil)
				if dv.fxExpandedSlots[si] {
					expBtn.Icon = string(IconChevronDown)
				} else {
					expBtn.Icon = string(IconChevronRight)
				}
				expBtn.IconColor = colTextSecondary
				expX := x + 4 + toggleBtnW + btnGap
				expBtn.SetRect(image.Rect(expX, y+2, expX+btnW, y+lineH-2))
				expBtn.OnClick = func() {
					if dv.fxExpandedSlots == nil {
						dv.fxExpandedSlots = map[int]bool{}
					}
					dv.fxExpandedSlots[si] = !dv.fxExpandedSlots[si]
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, expBtn)
			}

			// Move up button — IconChevronUp per §5c.
			if slotIdx > 0 {
				up := NewButton("", InstButtonStyle, nil)
				up.Icon = string(IconChevronUp)
				up.IconColor = colTextSecondary
				// A gutter before the destructive ×, so a mis-aimed reorder click cannot delete the effect.
				upX := x + w - 3*(btnW+btnGap) - rightInset + btnGap - SpaceMD
				up.SetRect(image.Rect(upX, y+2, upX+btnW, y+lineH-2))
				up.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si-1)
					emitInsertEffectMoved(channel.Of(instID), si, si-1)
					dv.recordUndoStep(hooks.EventInsertEffectMoved)
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, up)
			}

			// Move down button — IconChevronDown per §5c (icon economy: chevrons reused for reorder).
			if slotIdx < len(effects)-1 {
				down := NewButton("", InstButtonStyle, nil)
				down.Icon = string(IconChevronDown)
				down.IconColor = colTextSecondary
				downX := x + w - 2*(btnW+btnGap) - rightInset + btnGap - SpaceMD
				down.SetRect(image.Rect(downX, y+2, downX+btnW, y+lineH-2))
				down.OnClick = func() {
					audio.MoveInsertEffect(instID, si, si+1)
					emitInsertEffectMoved(channel.Of(instID), si, si+1)
					dv.recordUndoStep(hooks.EventInsertEffectMoved)
					dv.buildFXPanel()
					dv.refreshFXPortalHitAreas()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, down)
			}

			// Remove button — IconClose per §5c (no raw "✕").
			remove := NewButton("", InstButtonStyle, nil)
			remove.Icon = string(IconClose)
			remove.IconColor = colError
			removeX := x + w - btnW - rightInset
			remove.SetRect(image.Rect(removeX, y+2, removeX+btnW, y+lineH-2))
			remove.OnClick = func() {
				audio.RemoveInsertEffect(instID, si)
				emitInsertEffectRemoved(channel.Of(instID), si)
				dv.recordUndoStep(hooks.EventInsertEffectRemoved)
				dv.buildFXPanel()
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, remove)
		}

		y += lineH

		paramIndent := SpaceXL // 16px
		if dv.fxShowParams(si, e.Enabled) {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				for _, def := range defs {
					slTop := y
					slBot := y + paramH
					slInView := slBot > dv.fxViewportRect.Min.Y && slTop < dv.fxViewportRect.Max.Y

					if slInView {
						val := e.Params[def.Name]
						norm := (val - def.Min) / (def.Max - def.Min)
						if norm < 0 {
							norm = 0
						}
						if norm > 1 {
							norm = 1
						}
						sl := NewSlider(norm)
						sliderLeft := x + paramIndent + dv.fxSliderLeft
						if mobile {
							sliderLeft = x + paramIndent + 100
						}
						sl.SetRect(image.Rect(sliderLeft, y+2, x+w-4, y+paramH-2))
						// No percent label: the row already shows the raw value.
						sl.TrackH = 6
						sl.SuppressLabel = true
						if dv.fxPanelRow >= 0 && dv.fxPanelRow < len(dv.Rows) {
							sl.FillCol = rowShadeToRGBA(rowToggleBaseRGBA(dv.Rows[dv.fxPanelRow].Color))
						}
						dv.fxPanelSliders = append(dv.fxPanelSliders, sl)
						dv.fxSliderBindings = append(dv.fxSliderBindings, fxSliderBinding{
							slotIndex: si,
							paramName: def.Name,
							def:       def,
						})
					}
					y += paramH
				}
			}
		}
	}

	// Footer: effect type picker or "Add Effect" button
	dv.fxAddRowEffect = nil
	dv.fxAddRowBtn = nil
	dv.fxAddMenuBackBtn = nil
	dv.fxAddHeaderRows = nil
	if dv.fxAddMenuOpen {
		bTop := y
		bBot := y + lineH
		bInView := bBot > dv.fxViewportRect.Min.Y && bTop < dv.fxViewportRect.Max.Y
		if bInView {
			backBtn := NewButton("", InstButtonStyle, nil)
			backBtn.SetRect(insetRect(image.Rect(x, y, x+w, y+lineH), SpaceXS))
			backBtn.OnClick = dv.closeFXAddMenu
			dv.fxPanelBtns = append(dv.fxPanelBtns, backBtn)
			dv.fxAddMenuBackBtn = backBtn
		}
		y += lineH

		regs := audio.EffectRegistrations()
		haveLastCat := false
		lastCat := ""
		for _, et := range visibleEffectTypes {
			etCopy := et

			if !fxAddSearching {
				cat := ""
				if r := regs[et]; r != nil {
					cat = r.Category
				}
				if !haveLastCat || cat != lastCat {
					haveLastCat = true
					lastCat = cat
					dv.fxAddHeaderRows = append(dv.fxAddHeaderRows, fxAddHeaderRow{Y: y, Label: fxCategoryLabel(cat)})
					y += lineH
				}
			}

			fTop := y
			fBot := y + lineH
			fInView := fBot > dv.fxViewportRect.Min.Y && fTop < dv.fxViewportRect.Max.Y
			if fInView {
				// Text stays the display name for lookups; drawMenuRow paints the row, never btn.Draw.
				name := effectTypeName(et)
				btn := NewButton(name, InstButtonStyle, nil)
				btn.SetRect(insetRect(image.Rect(x, y, x+w, y+lineH), SpaceXS))
				btn.OnClick = func() {
					slot := audio.AddInsertEffect(instID, etCopy, nil)
					emitInsertEffectAdded(channel.Of(instID), slot, string(etCopy))
					dv.recordUndoStep(hooks.EventInsertEffectAdded)
					// Scrolled to the end (Configure clamps) so the new effect is in view.
					dv.fxListScrollPx = math.MaxInt32
					dv.closeFXAddMenu()
				}
				dv.fxPanelBtns = append(dv.fxPanelBtns, btn)
				dv.fxAddRowEffect = append(dv.fxAddRowEffect, etCopy)
				dv.fxAddRowBtn = append(dv.fxAddRowBtn, btn)
			}
			y += lineH
		}
	} else {
		if len(effects) == 0 {
			y += lineH // empty-state hint row (drawn by drawFXPanel)
		}
		aTop := y
		aBot := y + footerH
		aInView := aBot > dv.fxViewportRect.Min.Y && aTop < dv.fxViewportRect.Max.Y
		if aInView {
			addBtn := NewButton(i18n.T(i18n.KeyAddEffect), addEffectButtonStyle{}, nil)
			addBtn.TextColor = colTextSecondary
			addBtn.SetRect(image.Rect(x+4, y+4, x+w-4, y+footerH-4))
			addBtn.OnClick = func() {
				dv.fxAddMenuOpen = true
				if dv.fxScroll != nil { // picker opens at its own top; Back restores the list's offset
					dv.fxListScrollPx = dv.fxScroll.ScrollBehavior().VS.First
					dv.fxScroll.ScrollBehavior().VS.First = 0
				}
				dv.buildFXPanel()
				if dv.fxAddSearch != nil {
					dv.fxAddSearch.SetFocus(true)
				}
				dv.refreshFXPortalHitAreas()
			}
			dv.fxPanelBtns = append(dv.fxPanelBtns, addBtn)
			dv.fxAddEffectBtn = addBtn
		}
	}

	// Desktop only (nil on mobile, where tap-away closes); appended only when non-nil.
	dv.fxPanelCloseButton = newPopupCloseButton(func() { dv.closeFXPanel() })
	if dv.fxPanelCloseButton != nil {
		dv.fxPanelCloseButton.SetRect(closeButtonRect(dv.fxPanelRect, SpaceXS))
		dv.fxPanelBtns = append(dv.fxPanelBtns, dv.fxPanelCloseButton)
	}
}

func (dv *DrumView) fxAddMenuTitle() string {
	if dv.fxPanelRow < 0 || dv.fxPanelRow >= len(dv.Rows) {
		return "FX"
	}
	return fmt.Sprintf("FX · %s", dv.Rows[dv.fxPanelRow].Name)
}

func (dv *DrumView) drawFXPanelHeader(dst *ebiten.Image, headerR image.Rectangle, row int) {
	if dv.fxPanelCard == nil {
		dv.fxPanelCard = NewMenuCard(nil)
	}
	dv.fxPanelCard.SetCloseButton(dv.fxPanelCloseButton)
	dv.fxPanelCard.Layout(image.Rect(headerR.Min.X, headerR.Min.Y, headerR.Max.X, dv.fxPanelRect.Max.Y),
		MenuCardSpec{Title: dv.fxAddMenuTitle(), Swatch: dv.Rows[row].Color, Accent: dv.Rows[row].Color})
	drawRoundedRect(dst, headerR, genColorSliderTrackFill, fitRadius(RadiusContainer, headerR), true)
	dv.fxPanelCard.DrawBand(dst)
}

func (dv *DrumView) drawFXAddSearchBand(dst *ebiten.Image, searchR image.Rectangle) {
	if searchR.Empty() || dv.fxAddSearch == nil {
		return
	}
	drawRect(dst, searchR, genColorSliderTrackFill, true)
	drawMenuSearchBox(dst, dv.fxAddSearch)
}

func drawFXAddEmptyState(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	label := i18n.T(i18n.KeyMenuNoMatches)
	hint := i18n.T(i18n.KeyMenuNoMatchesHint)
	lh := StyledTextHeight(RoleBody)
	ch := StyledTextHeight(RoleCaption)
	totalH := lh + SpaceXS + ch
	topY := r.Min.Y + (r.Dy()-totalH)/2
	if topY < r.Min.Y {
		topY = r.Min.Y
	}
	lw := StyledTextWidth(label, RoleBody)
	DrawTextStyled(dst, label, r.Min.X+(r.Dx()-lw)/2, topY, RoleBody, colTextPrimary)
	hw := StyledTextWidth(hint, RoleCaption)
	DrawTextStyled(dst, hint, r.Min.X+(r.Dx()-hw)/2, topY+lh+SpaceXS, RoleCaption, colTextSecondary)
}

// After a mid-frame rebuild changes geometry.
func (dv *DrumView) refreshFXPortalHitAreas() {
	if dv.tree != nil && dv.portal().Has("fx-panel") {
		dv.portal().RefreshEntry("fx-panel")
	}
}

// drawFXPanel renders the FX panel overlay.
func (dv *DrumView) drawFXPanel(dst *ebiten.Image) {
	if !dv.IsFXPanelOpen() {
		return
	}
	r := dv.fxPanelRect
	if r.Empty() {
		return
	}
	row := dv.fxPanelRow
	if row < 0 || row >= len(dv.Rows) {
		return
	}

	// Background
	drawScrim(dst)
	drawPanel(dst, r)

	fm := fxPanelMetrics()
	lineH, headerH, paramH := fm.lineH, fm.headerH, fm.paramH
	mobile := Profile().IsMobile()

	// Header (fixed, not scrolled)
	headerR := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+headerH)
	dv.drawFXPanelHeader(dst, headerR, row)

	// Read from the rect build computed, so build and draw cannot drift.
	searchH := 0
	if !dv.fxAddSearchRect.Empty() {
		searchH = dv.fxAddSearchRect.Dy()
		dv.drawFXAddSearchBand(dst, dv.fxAddSearchRect)
	}

	effects := audio.GetInsertEffects(dv.Rows[row].Instrument)
	if dv.fxAddMenuOpen {
		effects = nil
	}
	y := r.Min.Y + headerH + searchH - dv.fxScrollOffset()

	paramIndent := SpaceXL // 16px indent for param labels

	// Leaves room for the right-cluster controls and scrollbar.
	cardRight := r.Max.X - 4
	if dv.fxHasScroll() {
		cardRight -= ScrollbarStyleForPlatform().Width + SpaceXS
	}

	fxAccent := dv.fxPanelAccent()
	for slotIdx, e := range effects {
		rowTop := y
		rowBot := y + lineH
		inView := rowBot > dv.fxViewportRect.Min.Y && rowTop < dv.fxViewportRect.Max.Y

		// Whether this effect's params are shown inline (expanded) or summarized.
		paramsShown := dv.fxShowParams(slotIdx, e.Enabled)

		// One card per effect, header and params together, so each reads as a unit; an enabled card's
		// band is instrument-tinted (identity, never the chrome accent).
		cardH := lineH
		if paramsShown {
			if defs, ok := audio.InsertEffectCatalog()[e.Type]; ok {
				cardH += len(defs) * paramH
			}
		}
		cardR := image.Rect(r.Min.X+4, rowTop+1, cardRight, rowTop+cardH-2)
		if cardR.Dx() > 0 && cardR.Max.Y > dv.fxViewportRect.Min.Y && cardR.Min.Y < dv.fxViewportRect.Max.Y {
			fill := colSurface2
			if !e.Enabled {
				fill = colSurface1
			}
			drawRoundedRect(dst, cardR, fill, fitRadius(RadiusControl, cardR), true)
			if e.Enabled {
				bandR := image.Rect(cardR.Min.X, cardR.Min.Y, cardR.Max.X, rowTop+lineH-1)
				drawRoundedRect(dst, bandR, WithAlphaFromColor(fxAccent, AlphaFaint), fitRadius(RadiusControl, bandR), true)
			}
		}

		if inView {
			name := effectTypeName(e.Type)
			if name == "" {
				name = string(e.Type)
			}
			// Name offset accounts for wider pill toggle (36px desktop, 44px mobile).
			nameX := r.Min.X + 40
			if mobile {
				nameX = r.Min.X + 48
				if e.Enabled {
					nameX = r.Min.X + 88 // room for expand button
				}
			}
			nameCol := colTextPrimary
			if !e.Enabled {
				nameCol = colTextSecondary
			}
			DrawTextColorAt(dst, name, nameX, y+(lineH-12)/2, nameCol)

			// A collapsed effect still shows its state as a one-line summary.
			if !paramsShown {
				if summary := fxParamSummary(e); summary != "" {
					nameW := TextWidth(name)
					sumX := nameX + nameW + SpaceSM
					maxSumW := cardRight - sumX - 4
					if maxSumW > 24 {
						summary = clipTextToWidth(summary, maxSumW)
						DrawTextColorAtScale(dst, summary, sumX, y+(lineH-9)/2, colTextSecondary, fxSummaryScale)
					}
				}
			}
		}
		y += lineH

		if paramsShown {
			cat := audio.InsertEffectCatalog()
			if defs, ok := cat[e.Type]; ok {
				for _, def := range defs {
					slTop := y
					slBot := y + paramH
					slInView := slBot > dv.fxViewportRect.Min.Y && slTop < dv.fxViewportRect.Max.Y

					if slInView {
						val := e.Params[def.Name]
						label := fxParamLabel(def, val)
						maxLabelW := dv.fxSliderLeft - 8
						if mobile {
							maxLabelW = 92
						}
						label = clipTextToWidth(label, maxLabelW)
						DrawTextAt(dst, label, r.Min.X+8+paramIndent, y+(paramH-12)/2)
					}
					y += paramH
				}
			}
		}
	}

	if len(effects) == 0 && !dv.fxAddMenuOpen {
		hint := i18n.T(i18n.KeyFXEmptyHint)
		hint = clipTextToWidth(hint, r.Dx()-2*SpaceMD)
		hw := TextWidth(hint)
		hx := r.Min.X + (r.Dx()-hw)/2
		DrawTextColorAt(dst, hint, hx, y+(lineH-12)/2, colTextSecondary)
	} else if dv.fxAddMenuOpen && len(dv.fxAddVisibleEffects()) == 0 {
		emptyTop := dv.fxViewportRect.Min.Y
		if dv.fxAddMenuBackBtn != nil {
			emptyTop = dv.fxAddMenuBackBtn.Rect().Max.Y
		}
		emptyR := image.Rect(dv.fxViewportRect.Min.X, emptyTop, dv.fxViewportRect.Max.X, dv.fxViewportRect.Max.Y)
		drawFXAddEmptyState(dst, emptyR)
	}

	for _, h := range dv.fxAddHeaderRows {
		hTop, hBot := h.Y, h.Y+lineH
		if hBot > dv.fxViewportRect.Min.Y && hTop < dv.fxViewportRect.Max.Y {
			hRect := insetRect(image.Rect(r.Min.X, h.Y, r.Max.X, h.Y+lineH), SpaceXS)
			drawMenuGroupHeader(dst, hRect, h.Label)
		}
	}

	for _, btn := range dv.fxPanelBtns {
		if btn == nil {
			continue
		}
		// Close button is fixed (not scrolled) — matched by identity, not slice tail.
		if btn == dv.fxPanelCloseButton {
			btn.Draw(dst)
			continue
		}
		br := btn.Rect()
		if br.Max.Y > dv.fxViewportRect.Min.Y && br.Min.Y < dv.fxViewportRect.Max.Y {
			switch {
			case btn == dv.fxAddMenuBackBtn:
				// Add-menu dismiss row: shared flat menu row, leading chevron-left.
				drawMenuRow(dst, btn, MenuRowSpec{
					Accent:   fxAccent,
					State:    hoverStateOf(btn),
					IconID:   IconChevronLeft,
					IconTint: colMenuIcon,
					Label:    i18n.T(i18n.KeyMenuBack),
				})
			default:
				if et, ok := dv.fxAddRowEffectFor(btn); ok {
					drawMenuRow(dst, btn, MenuRowSpec{
						Accent:     fxAccent,
						State:      hoverStateOf(btn),
						IconID:     IconFx,
						IconTint:   colMenuIcon,
						Label:      audio.EffectRegistrations()[et].DisplayName,
						TrailLabel: dv.fxAddRowTrail(et),
						Highlights: dv.fxAddSearchHighlights[et],
					})
				} else if isToggle, enabled := isFXToggleBtn(btn); isToggle {
					// The instrument's color, not the green default the synth stage pills share.
					drawFXTogglePillCol(dst, br, enabled, fxAccent)
				} else {
					btn.Draw(dst)
				}
			}
		}
	}

	// Draw sliders (skip those outside viewport)
	for _, sl := range dv.fxPanelSliders {
		if sl == nil {
			continue
		}
		sr := sl.Rect()
		if sr.Max.Y > dv.fxViewportRect.Min.Y && sr.Min.Y < dv.fxViewportRect.Max.Y {
			sl.Draw(dst)
		}
	}

	// Drawn last: rows can scroll up under the pinned bands.
	dv.drawFXPanelHeader(dst, headerR, row)
	if !dv.fxAddSearchRect.Empty() {
		dv.drawFXAddSearchBand(dst, dv.fxAddSearchRect)
	}
	// Redraw close button on top of header
	if closeBtn := dv.fxPanelCloseBtn(); closeBtn != nil {
		closeBtn.Draw(dst)
	}

	if dv.fxScroll != nil {
		dv.fxScroll.Draw(dst)
	}
}

func (dv *DrumView) handleFXPanelInput(mx, my int, left bool) bool {
	if !dv.IsFXPanelOpen() {
		return false
	}

	p := image.Pt(mx, my)

	// First, so a tap on the search band always wins: the clear-× wipes the query, anywhere else focuses.
	if left && dv.fxAddMenuOpen && dv.fxAddSearch != nil &&
		!dv.fxAddSearchRect.Empty() && p.In(dv.fxAddSearchRect) {
		if dv.fxAddSearchQuery != "" && menuSearchAdornmentsActive(dv.fxAddSearch) &&
			p.In(menuSearchClearRect(dv.fxAddSearch)) {
			dv.setFXAddSearchQuery("")
			return true
		}
		dv.fxAddSearch.SetFocus(true)
		return true
	}

	// Before click-outside and deferred taps, so a drag that leaves the panel is not interrupted.
	if dv.fxSliderDragging {
		idx := dv.fxSliderDragIdx
		if idx < len(dv.fxPanelSliders) {
			sl := dv.fxPanelSliders[idx]
			before := sl.Value
			sl.HandleInputResult(mx, my, left)
			// Only a value change reaches the audio: this runs every input tick of a held slider.
			if sl.Value != before {
				dv.propagateFXSliderValue(idx)
			}
		}
		if !left {
			dv.fxSliderDragging = false
			dv.commitFXSlider(idx)
		}
		return true
	}

	if Profile().IsMobile() {
		sb := dv.ensureFXScroll().ScrollBehavior()
		relayout := func() { dv.buildFXPanel(); dv.refreshFXPortalHitAreas() }

		// Continue committed scroll gesture.
		if sb.ScrollingCommitted() {
			if left {
				if sb.HandleTouchMove(mx, my) {
					relayout()
				}
			} else {
				// Touch ended — capture velocity for momentum.
				sb.HandleTouchEnd()
			}
			return true
		}

		// Touch is being tracked but hasn't committed to scroll yet.
		if sb.TouchActive() {
			if left {
				moved := sb.HandleTouchMove(mx, my)
				if sb.ScrollingCommitted() {
					// Scroll just committed — cancel deferred tap.
					dv.fxPanelDeferredTap.Cancel()
					if moved {
						relayout()
					}
					return true
				}
				// Not committed yet — keep tracking.
				return true
			}
			// Touch ended before committing to scroll → fire as tap.
			sb.HandleTouchEnd()
			if dv.fxPanelDeferredTap.Active() {
				dv.fxPanelDeferredTap.End(dv.fireFXPanelTapAt)
			}
			return true
		}

		// Check if touch-begin lands on a slider → start drag directly.
		if left && !dv.fxPanelDeferredTap.Active() {
			for i, sl := range dv.fxPanelSliders {
				if sl != nil && p.In(sl.Rect()) {
					sl.HandleInputResult(mx, my, left)
					dv.fxSliderDragging = true
					dv.fxSliderDragIdx = i
					dv.propagateFXSliderValue(i)
					return true
				}
			}
		}

		// New touch begin in viewport: start both deferred tap and scroll tracking.
		if left && !dv.fxPanelDeferredTap.Active() && p.In(dv.fxPanelRect) {
			if dv.fxHasScroll() && p.In(dv.fxViewportRect) {
				// Start scroll tracking alongside deferred tap.
				sb.HandleTouchBegin(mx, my)
			}
			if dv.fxPanelDeferredTap.Begin(mx, my) {
				return true
			}
		}

		// Active deferred tap waiting for release (no scroll committed).
		if dv.fxPanelDeferredTap.Active() {
			if !left {
				dv.fxPanelDeferredTap.End(dv.fireFXPanelTapAt)
				sb.ResetTouch()
			}
			return true
		}
	}

	for _, btn := range dv.fxPanelBtns {
		if btn != nil && btn.HandleInputResult(mx, my, left) != InputIgnored {
			return true
		}
	}

	for i, sl := range dv.fxPanelSliders {
		if sl != nil && sl.HandleInputResult(mx, my, left) != InputIgnored {
			if sl.dragging {
				dv.fxSliderDragging = true
				dv.fxSliderDragIdx = i
			}
			dv.propagateFXSliderValue(i)
			return true
		}
	}

	if p.In(dv.fxPanelRect) {
		return true
	}

	return false
}

// In reverse: the close button is appended last and overlaps the others.
func (dv *DrumView) fireFXPanelTapAt(x, y int) {
	pt := image.Pt(x, y)
	for i := len(dv.fxPanelBtns) - 1; i >= 0; i-- {
		btn := dv.fxPanelBtns[i]
		if btn != nil && pt.In(btn.Rect()) && btn.OnClick != nil {
			btn.OnClick()
			return
		}
	}
	// Check sliders: map position to value update.
	for i, sl := range dv.fxPanelSliders {
		if sl != nil && pt.In(sl.Rect()) {
			slR := sl.Rect()
			norm := float64(x-slR.Min.X) / float64(slR.Dx())
			if norm < 0 {
				norm = 0
			}
			if norm > 1 {
				norm = 1
			}
			sl.Value = norm
			if i < len(dv.fxSliderBindings) {
				b := dv.fxSliderBindings[i]
				actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
				instID := dv.Rows[dv.fxPanelRow].Instrument
				audio.SetInsertEffectParam(instID, b.slotIndex, b.paramName, actual)
				emitInsertEffectParam(channel.Of(instID), b.slotIndex, b.paramName, actual)
				dv.recordUndoStep(hooks.EventInsertEffectParam)
			}
			return
		}
	}
}

// Shares fxFormatParam with the summary, so one value never shows two precisions.
func fxParamLabel(def audio.EffectParamDef, val float64) string {
	return fmt.Sprintf("%s: %s", capitalize(def.Name), fxFormatParam(def, val))
}

func capitalize(s string) string {
	return audio.PrettyName(s)
}

const fxSummaryScale = 0.8

// Read live from the slot (catalog default when absent); "" when the effect has no params.
func fxParamSummary(e audio.EffectSlot) string {
	defs, ok := audio.InsertEffectCatalog()[e.Type]
	if !ok || len(defs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, def := range defs {
		val, has := e.Params[def.Name]
		if !has {
			val = def.Default
		}
		if i > 0 {
			b.WriteString(" · ") // middle dot separator
		}
		b.WriteString(capitalize(def.Name))
		b.WriteByte(' ')
		b.WriteString(fxFormatParam(def, val))
	}
	return b.String()
}

// Precision and unit come from the parameter's definition, never the value, so the digit count
// holds still while a slider drags.
func fxFormatParam(def audio.EffectParamDef, val float64) string {
	switch def.Unit {
	case "Hz":
		// Audio frequencies (Max >= 1000) read in integer Hz, kHz above 1000; LFO rates keep a decimal.
		if def.Max >= 1000 {
			if val >= 1000 {
				return fmt.Sprintf("%.1f kHz", val/1000)
			}
			return fmt.Sprintf("%.0f Hz", val)
		}
		return fmt.Sprintf("%.*f Hz", fxDecimalsForSpan(def.Max-def.Min), val)
	case "ms":
		return fmt.Sprintf("%.0f ms", val)
	case "dB":
		return fmt.Sprintf("%.1f dB", val)
	case "st":
		return fmt.Sprintf("%.0f st", val)
	case "%":
		return fmt.Sprintf("%.0f%%", val)
	default:
		return fmt.Sprintf("%.*f", fxDecimalsForSpan(def.Max-def.Min), val)
	}
}

// A function of the range alone, so it is constant for a parameter.
func fxDecimalsForSpan(span float64) int {
	switch {
	case span <= 1:
		return 2
	case span <= 30:
		return 1
	default:
		return 0
	}
}
