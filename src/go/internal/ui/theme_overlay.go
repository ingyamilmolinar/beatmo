package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// The Settings card's width, so the corner-button panels read as siblings.
const (
	themeRowH          = 44 // one list row (preset / category chip row)
	themeChipSize      = 28 // desktop chip edge; floored at TouchMinTarget() on mobile
	themePillH         = 32
	themePillW         = 138
	themeResetAllArmMs = 2000
	themeStripSwatch   = 14 // one square of the preset swatch strip
	themeStripCount    = 5  // background, surfaces, accent, viz, text
)

// themeRowKind is what one scrolling list row shows.
type themeRowKind uint8

const (
	themeRowPreset themeRowKind = iota
	themeRowAdjustHeader
	themeRowChip
)

type themeRow struct {
	kind themeRowKind
	id   string // preset id or category id
	rect image.Rectangle
}

// Every pick applies live.
type ThemeOverlay struct {
	g      *Game
	closed bool
	card   *MenuCard
	rect   image.Rectangle
	place  func(w, h int) image.Rectangle
	maxH   func() int
	scope  hooks.ThemeScope

	mineBtn, circuitBtn *Button
	overrideBtn         *Button
	resetAllBtn         *Button
	resetAllArmed       bool
	resetAllArmedAt     int64

	presetBtns map[string]*Button // preset id → row button
	adjustBtn  *Button            // "Adjust colors" header row
	adjustOpen bool
	resetBtns  map[string]*Button // category id → reset button
	chipRects  map[string]image.Rectangle
	rows       []themeRow

	// Nil-safe.
	onChip func(categoryID string)

	scroll     *MenuScroll
	scrollRowH int
	rowsView   image.Rectangle
	rowH       int

	// The portal lays an overlay out only on open but republishes HitAreas every frame, so a changed row set replays
	// Layout.
	lastAnchor, lastBounds image.Rectangle
	// Over the SAME row list the mouse sees; lazy.
	nav *MenuKeyNav
}

// NewThemeOverlay builds the editor bound to g (nil-safe for layout tests).
func NewThemeOverlay(g *Game) *ThemeOverlay {
	o := &ThemeOverlay{g: g, scope: initialThemeScope(g), presetBtns: map[string]*Button{}, resetBtns: map[string]*Button{}, chipRects: map[string]image.Rectangle{}, rowH: themeRowH}
	o.card = NewMenuCard(o.Close)
	o.mineBtn = NewButtonKey(i18n.KeyThemeScopeMine, ComponentButtonSecondary, func() { o.scope = hooks.ThemeScopeUser })
	o.circuitBtn = NewButtonKey(i18n.KeyThemeScopeCircuit, ComponentButtonSecondary, func() { o.scope = hooks.ThemeScopeCircuit })
	o.overrideBtn = NewButtonKey(i18n.KeyThemeOverride, ComponentButtonSecondary, func() {
		if o.g != nil {
			o.g.setThemeOverride(!o.g.userTheme.OverrideCircuits)
		}
	})
	o.resetAllBtn = NewButtonKey(i18n.KeyThemeResetAll, ComponentButtonDestructive, o.onResetAll)
	for _, p := range themePresets {
		id := p.ID
		o.presetBtns[id] = NewButton("", ContextMenuItemStyle, func() { o.pickPreset(id) })
	}
	o.adjustBtn = NewButton("", ContextMenuItemStyle, func() { o.SetAdjustExpanded(!o.adjustOpen) })
	for _, c := range themeCategories {
		id := c.ID
		b := NewSpecButton("", ComponentButtonSecondary, func() { o.resetCategory(id) })
		b.Icon = string(IconUndo)
		o.resetBtns[id] = b
	}
	o.scroll = NewMenuScroll(DropdownScrollbarStyle, themeRowH)
	o.scrollRowH = themeRowH
	return o
}

// The document actually in effect, so the highlighted preset matches what the user sees.
func initialThemeScope(g *Game) hooks.ThemeScope {
	if g != nil && !g.userTheme.OverrideCircuits && !g.circuitTheme.IsEmpty() {
		return hooks.ThemeScopeCircuit
	}
	return hooks.ThemeScopeUser
}

// nil centres it.
func (o *ThemeOverlay) SetPositioner(place func(w, h int) image.Rectangle) { o.place = place }

// SetMaxHeight injects the height budget the positioner can honour.
func (o *ThemeOverlay) SetMaxHeight(fn func() int) { o.maxH = fn }

// Close marks the overlay for removal on the next portal sweep.
func (o *ThemeOverlay) Close() { o.closed = true }

// ShouldClose reports whether Close() has been invoked.
func (o *ThemeOverlay) ShouldClose() bool { return o.closed }

// Rect is the panel rect (valid after Layout).
func (o *ThemeOverlay) Rect() image.Rectangle { return o.rect }

// Card exposes the shared frame (tests / discipline).
func (o *ThemeOverlay) Card() *MenuCard { return o.card }

// Scope reports which document the editor writes: "user" or "circuit".
func (o *ThemeOverlay) Scope() hooks.ThemeScope { return o.scope }

// ScopePillRects returns the "My theme" / "This circuit" pill rects.
func (o *ThemeOverlay) ScopePillRects() (mine, circuit image.Rectangle) {
	return o.mineBtn.Rect(), o.circuitBtn.Rect()
}

// OverrideRect is the "Always use my theme" toggle rect.
func (o *ThemeOverlay) OverrideRect() image.Rectangle { return o.overrideBtn.Rect() }

// ResetAllRect is the footer "Reset all" button rect.
func (o *ThemeOverlay) ResetAllRect() image.Rectangle { return o.resetAllBtn.Rect() }

// PresetRowRect is the list row of a preset (screen coords; may be scrolled out).
func (o *ThemeOverlay) PresetRowRect(id string) image.Rectangle {
	if b := o.presetBtns[id]; b != nil {
		return b.Rect()
	}
	return image.Rectangle{}
}

// AdjustHeaderRect is the "Adjust colors" expander row.
func (o *ThemeOverlay) AdjustHeaderRect() image.Rectangle { return o.adjustBtn.Rect() }

// AdjustExpanded reports whether the category chips are shown.
func (o *ThemeOverlay) AdjustExpanded() bool { return o.adjustOpen }

// SetAdjustExpanded shows/hides the category chips and re-lays the rows.
func (o *ThemeOverlay) SetAdjustExpanded(on bool) {
	o.adjustOpen = on
	if !o.lastBounds.Empty() {
		o.Layout(o.lastAnchor, o.lastBounds)
	}
	// A cursor past the end clears rather than landing on whatever row slid under it.
	if o.keyNav().Selected() >= len(o.rowList()) {
		o.keyNav().Reset()
	}
}

// Every row is actionable, so there is no unselectable chrome to skip.

func (o *ThemeOverlay) keyNav() *MenuKeyNav {
	if o.nav == nil {
		o.nav = NewMenuKeyNav()
	}
	return o.nav
}

// MenuItemCount implements MenuKeyNavHost.
func (o *ThemeOverlay) MenuItemCount() int { return len(o.rows) }

// MenuItemSelectable implements MenuKeyNavHost.
func (o *ThemeOverlay) MenuItemSelectable(i int) bool { return i >= 0 && i < len(o.rows) }

// MenuActivate implements MenuKeyNavHost: identical to clicking the row.
func (o *ThemeOverlay) MenuActivate(i int) {
	if !o.MenuItemSelectable(i) {
		return
	}
	r := o.rows[i]
	switch r.kind {
	case themeRowPreset:
		o.pickPreset(r.id)
	case themeRowAdjustHeader:
		o.SetAdjustExpanded(!o.adjustOpen)
	case themeRowChip:
		if o.onChip != nil {
			o.onChip(r.id)
		}
	}
}

// MenuScrollBehavior implements MenuKeyNavHost.
func (o *ThemeOverlay) MenuScrollBehavior() *ScrollBehavior {
	if o.scroll == nil {
		return nil
	}
	return o.scroll.ScrollBehavior()
}

// Its arrows move the cursor, not the camera.
func (o *ThemeOverlay) ClaimsKeyboard() bool { return !o.closed }

// MenuKeyNavForTest exposes the keyboard cursor for tests.
func (o *ThemeOverlay) MenuKeyNavForTest() *MenuKeyNav { return o.keyNav() }

// ChipRect is the swatch chip rect of a category row (empty while collapsed).
func (o *ThemeOverlay) ChipRect(id string) image.Rectangle { return o.chipRects[id] }

// ResetRect is the per-row reset button rect of a category (empty while collapsed).
func (o *ThemeOverlay) ResetRect(id string) image.Rectangle {
	if !o.adjustOpen {
		return image.Rectangle{}
	}
	if b := o.resetBtns[id]; b != nil {
		return b.Rect()
	}
	return image.Rectangle{}
}

// scopeDoc returns the document the editor is currently writing.
func (o *ThemeOverlay) scopeDoc() theme.Doc {
	if o.g == nil {
		return theme.Doc{}
	}
	if o.scope == hooks.ThemeScopeCircuit {
		return o.g.circuitTheme
	}
	return o.g.userTheme.Doc
}

// The scope's own preset, else the shipped default.
func (o *ThemeOverlay) activePresetID() string {
	if id := o.scopeDoc().Preset; id != "" {
		return id
	}
	return defaultThemePresetID
}

func (o *ThemeOverlay) customized() bool {
	d := o.scopeDoc()
	return len(d.Categories) > 0 || len(d.Tokens) > 0
}

func (o *ThemeOverlay) pickPreset(id string) {
	if o.g == nil {
		return
	}
	if o.scope == hooks.ThemeScopeCircuit {
		o.g.setCircuitThemePreset(id)
	} else {
		o.g.setUserThemePreset(id)
	}
}

// set: the scope sets it (ring); inherited: the shown colour comes from the other source.
func (o *ThemeOverlay) chipState(id string) (set, inherited bool) {
	if _, ok := o.scopeDoc().Categories[id]; ok {
		return true, false
	}
	_, inherited = ActiveTheme().Categories[id]
	return false, inherited
}

// The live value of its first token.
func (o *ThemeOverlay) chipColor(id string) color.RGBA {
	var none color.RGBA
	c, ok := themeCategoryByID(id)
	if !ok || len(c.Tokens) == 0 {
		return none
	}
	if p, ok := genColorByName[c.Tokens[0]]; ok {
		return *p
	}
	return none
}

// What the picker rings as current: the scope's pick, the preset's, else the anchor.
func (o *ThemeOverlay) chipSwatch(id string) color.RGBA {
	if name, ok := ActiveTheme().Categories[id]; ok {
		if c, ok := swatchRGBA(name); ok {
			return c
		}
	}
	if p, ok := themePresetByID(ActiveTheme().Preset); ok {
		if c, ok := swatchRGBA(p.Categories[id]); ok {
			return c
		}
	}
	c, _ := themeCategoryByID(id)
	col, _ := swatchRGBA(c.Anchor)
	return col
}

func (o *ThemeOverlay) resetCategory(id string) {
	if o.g == nil {
		return
	}
	if o.scope == hooks.ThemeScopeCircuit {
		o.g.clearCircuitThemeCategory(id)
	} else {
		o.g.clearUserThemeCategory(id)
	}
}

// Two taps (arm, then fire within the window), like the row Delete button.
func (o *ThemeOverlay) onResetAll() {
	now := nowUnixMilli()
	if !o.resetAllArmed || now-o.resetAllArmedAt > themeResetAllArmMs {
		o.resetAllArmed, o.resetAllArmedAt = true, now
		o.resetAllBtn.SetSpec(ComponentButtonDestructiveConfirm)
		return
	}
	o.disarmResetAll()
	if o.g == nil {
		return
	}
	if o.scope == hooks.ThemeScopeCircuit {
		o.g.resetCircuitTheme()
	} else {
		o.g.resetUserTheme()
	}
}

func (o *ThemeOverlay) disarmResetAll() {
	o.resetAllArmed = false
	o.resetAllBtn.SetSpec(ComponentButtonDestructive)
}

func (o *ThemeOverlay) chipSize() int {
	if Profile().IsMobile() && TouchMinTarget() > themeChipSize {
		return TouchMinTarget()
	}
	return themeChipSize
}

func (o *ThemeOverlay) rowList() []themeRow {
	rows := make([]themeRow, 0, len(themePresets)+1+len(themeCategories))
	for _, p := range themePresets {
		rows = append(rows, themeRow{kind: themeRowPreset, id: p.ID})
	}
	rows = append(rows, themeRow{kind: themeRowAdjustHeader})
	if o.adjustOpen {
		for _, c := range themeCategories {
			rows = append(rows, themeRow{kind: themeRowChip, id: c.ID})
		}
	}
	return rows
}

func (o *ThemeOverlay) cardSpec() MenuCardSpec {
	return MenuCardSpec{Title: i18n.T(i18n.KeyThemeTitle), IconID: IconPalette, Footer: []*Button{o.resetAllBtn}}
}

func (o *ThemeOverlay) Layout(anchor, screenBounds image.Rectangle) {
	o.lastAnchor, o.lastBounds = anchor, screenBounds
	b := screenBounds
	w := menuWEditor() // editor-card class, like Settings
	if maxW := b.Dx() - 2*24; maxW > 0 && w > maxW {
		w = maxW
	}
	o.rowH = themeRowH
	if cs := o.chipSize(); cs+2*SpaceXS > o.rowH {
		o.rowH = cs + 2*SpaceXS
	}
	if Profile().IsMobile() && TouchMinTarget() > o.rowH {
		o.rowH = TouchMinTarget()
	}
	rows := o.rowList()
	h := menuCardTitleH + menuCardPad + themePillH + SpaceSM + themePillH + SpaceMD + len(rows)*o.rowH + SpaceSM + menuCardFooterH + menuCardPad
	maxH := b.Dy() - 2*12
	if o.maxH != nil {
		maxH = o.maxH() - 12
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	if o.place != nil {
		o.rect = o.place(w, h)
	} else {
		cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
		o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	}
	o.card.Layout(o.rect, o.cardSpec())
	body := o.card.Body()
	x, y := body.Min.X, body.Min.Y
	pillW := themePillW
	if 2*pillW+SpaceMD > body.Dx() {
		pillW = (body.Dx() - SpaceMD) / 2
	}
	o.mineBtn.SetRect(image.Rect(x, y, x+pillW, y+themePillH))
	o.circuitBtn.SetRect(image.Rect(x+pillW+SpaceMD, y, x+2*pillW+SpaceMD, y+themePillH))
	y += themePillH + SpaceSM
	o.overrideBtn.SetRect(image.Rect(x, y, body.Max.X, y+themePillH))
	y += themePillH + SpaceMD
	o.rowsView = image.Rect(body.Min.X, y, body.Max.X, body.Max.Y)
	vis := o.rowsView.Dy() / o.rowH
	if vis < 1 {
		vis = 1
	}
	// Re-laid every frame: keep the scroll unless the row stride changed.
	if o.scrollRowH != o.rowH {
		o.scroll = NewMenuScroll(DropdownScrollbarStyle, o.rowH)
		o.scrollRowH = o.rowH
	}
	o.scroll.Configure(o.rowsView, len(rows), vis)
	o.rows = rows
	o.layoutRows()
}

// layoutRows positions every list row from the current scroll offset.
func (o *ThemeOverlay) layoutRows() {
	right := o.rowsView.Max.X - dropdownScrollbarWidth()
	cs := o.chipSize()
	ry := o.rowsView.Min.Y - o.scroll.OffsetPx()
	for id := range o.chipRects {
		delete(o.chipRects, id)
	}
	for i := range o.rows {
		r := &o.rows[i]
		r.rect = image.Rect(o.rowsView.Min.X, ry, right, ry+o.rowH)
		switch r.kind {
		case themeRowPreset:
			o.presetBtns[r.id].SetRect(r.rect)
		case themeRowAdjustHeader:
			o.adjustBtn.SetRect(r.rect)
		case themeRowChip:
			reset := image.Rect(right-cs, ry+(o.rowH-cs)/2, right, ry+(o.rowH-cs)/2+cs)
			o.resetBtns[r.id].SetRect(reset)
			chipX := reset.Min.X - SpaceSM - cs
			o.chipRects[r.id] = image.Rect(chipX, ry+(o.rowH-cs)/2, chipX+cs, ry+(o.rowH-cs)/2+cs)
		}
		ry += o.rowH
	}
}

// Lets a stale Reset-all arm time out.
func (o *ThemeOverlay) Update() {
	if o.scroll != nil && o.scroll.UpdateMomentum() {
		o.layoutRows()
	}
	if o.resetAllArmed && nowUnixMilli()-o.resetAllArmedAt > themeResetAllArmMs {
		o.disarmResetAll()
	}
	o.keyNav().Update(o)
}

// HoverAt refreshes hover on the frame controls and the list rows.
func (o *ThemeOverlay) HoverAt(mx, my int) {
	o.card.HoverAt(mx, my)
	for _, b := range o.presetBtns {
		b.UpdateHover(mx, my)
	}
	o.adjustBtn.UpdateHover(mx, my)
	// The pointer takes the ONE shared highlight over (menu_keynav.go).
	if pt := image.Pt(mx, my); pt.In(o.rowsView) {
		for i, r := range o.rows {
			if pt.In(r.rect) {
				o.keyNav().SetSelected(i)
				break
			}
		}
	}
}

// Chips are hit-tested here by rect, so a chip needs no Button.
type themePanelHandler struct{ o *ThemeOverlay }

func (h themePanelHandler) OnPress(x, y int) InputResult {
	o := h.o
	if o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, o.layoutRows) {
		return InputCaptured
	}
	pt := image.Pt(x, y)
	if pt.In(o.rowsView) {
		for id, cr := range o.chipRects {
			if pt.In(cr) {
				if o.onChip != nil {
					o.onChip(id)
				}
				return InputConsumed
			}
		}
	}
	return InputConsumed
}

func (h themePanelHandler) OnDrag(x, y int) {
	h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, h.o.layoutRows)
}

func (h themePanelHandler) OnRelease(x, y int) {
	h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), false, nil)
}

func (h themePanelHandler) OnWheel(x, y, steps int) InputResult {
	if h.o.scroll.HandleWheel(steps) {
		h.o.layoutRows()
	}
	return InputConsumed
}

// Buttons FIRST (they win the equal-z sort), the catch-all LAST; rows clip to the viewport so a scrolled-away row
// cannot be pressed.
func (o *ThemeOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	areas := o.card.ControlAreas(ZOverlayMin, "theme")
	areas = append(areas,
		HitArea{Rect: o.mineBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-scope-mine", Handler: &buttonHitAdapter{btn: o.mineBtn}},
		HitArea{Rect: o.circuitBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-scope-circuit", Handler: &buttonHitAdapter{btn: o.circuitBtn}},
		HitArea{Rect: o.overrideBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-override", Handler: &buttonHitAdapter{btn: o.overrideBtn}},
	)
	for _, r := range o.rows {
		var b *Button
		tag := "theme-row-"
		switch r.kind {
		case themeRowPreset:
			b, tag = o.presetBtns[r.id], "theme-preset-"+r.id
		case themeRowAdjustHeader:
			b, tag = o.adjustBtn, "theme-adjust"
		case themeRowChip:
			b, tag = o.resetBtns[r.id], "theme-reset-"+r.id
		}
		if b == nil {
			continue
		}
		if cr := b.Rect().Intersect(o.rowsView); !cr.Empty() {
			areas = append(areas, HitArea{Rect: cr, ZIndex: ZOverlayMin, Tag: tag, Handler: &buttonHitAdapter{btn: b}})
		}
	}
	return append(areas, o.card.CatchAll(ZOverlayMin, "theme", themePanelHandler{o: o}))
}

// Background, surfaces, accent, viz, text.
func presetSwatchStrip(dst *ebiten.Image, r image.Rectangle, p ThemePreset) {
	order := [themeStripCount]string{"background", "surfaces", "accent", "viz", "text"}
	x := r.Min.X
	for _, cat := range order {
		col, _ := swatchRGBA(p.Categories[cat])
		sq := image.Rect(x, r.Min.Y, x+themeStripSwatch, r.Min.Y+themeStripSwatch)
		drawRoundedRect(dst, sq, col, fitRadius(RadiusControl, sq), true)
		x += themeStripSwatch + 2
	}
}

// presetStripW is the strip's total width.
func presetStripW() int { return themeStripCount*themeStripSwatch + (themeStripCount-1)*2 }

func (o *ThemeOverlay) Draw(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	o.DrawPanel(screen) // the portal paints the scrim (menuCardEntry: Scrim)
}

// Re-run after the pane dividers so they never cross the window-bounded panel.
func (o *ThemeOverlay) DrawPanel(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	o.card.Layout(o.rect, o.cardSpec())
	o.card.Draw(screen)
	dst := screen.SubImage(o.rect).(*ebiten.Image)
	o.mineBtn.SetToggled(o.scope == hooks.ThemeScopeUser)
	o.circuitBtn.SetToggled(o.scope == hooks.ThemeScopeCircuit)
	o.mineBtn.Draw(dst)
	o.circuitBtn.Draw(dst)
	o.overrideBtn.SetToggled(o.g != nil && o.g.userTheme.OverrideCircuits)
	o.overrideBtn.Draw(dst)
	if o.rowsView.Empty() {
		return
	}
	rows := dst.SubImage(o.rowsView).(*ebiten.Image)
	active := o.activePresetID()
	customized := o.customized()
	labelH := StyledTextHeight(RoleBody)
	glossH := StyledTextHeight(RoleCaption)
	sel := o.keyNav().Selected()
	for i, r := range o.rows {
		if !r.rect.Overlaps(o.rowsView) {
			continue
		}
		// The menu-row hover tint, so the arrows show where Enter lands.
		if i == sel && r.kind != themeRowPreset {
			drawMenuItemBackgroundAccent(rows, r.rect, menuItemHover, colAccent)
		}
		switch r.kind {
		case themeRowPreset:
			p, _ := themePresetByID(r.id)
			btn := o.presetBtns[r.id]
			state := menuItemRest
			switch {
			case r.id == active:
				state = menuItemActive
			case btn.hovered || btn.pressed || i == sel:
				state = menuItemHover
			}
			trail := i18n.T(p.GlossKey)
			if r.id == active && customized {
				trail = i18n.T(i18n.KeyThemeCustomized)
			}
			drawMenuRow(rows, btn, MenuRowSpec{State: state, Label: i18n.T(p.NameKey), TrailLabel: trail, LeadW: presetStripW()})
			sy := r.rect.Min.Y + (r.rect.Dy()-themeStripSwatch)/2
			presetSwatchStrip(rows, image.Rect(r.rect.Min.X+SpaceSM, sy, r.rect.Min.X+SpaceSM+presetStripW(), sy+themeStripSwatch), p)
		case themeRowAdjustHeader:
			drawMenuGroupHeader(rows, r.rect, i18n.T(i18n.KeyThemeAdjust))
			s := IconSizeSM
			cr := image.Rect(r.rect.Max.X-SpaceMD-s, r.rect.Min.Y+(r.rect.Dy()-s)/2, r.rect.Max.X-SpaceMD, r.rect.Min.Y+(r.rect.Dy()-s)/2+s)
			drawMenuChevron(rows, cr, o.adjustOpen, colTextSecondary)
		case themeRowChip:
			c, _ := themeCategoryByID(r.id)
			chip := o.chipRects[r.id]
			textX := r.rect.Min.X + SpaceMD
			textW := chip.Min.X - SpaceSM - textX
			ty := r.rect.Min.Y + (r.rect.Dy()-labelH-glossH)/2
			DrawTextStyled(rows, truncCaptionStyled(i18n.T(c.LabelKey), textW, RoleBody), textX, ty, RoleBody, colTextPrimary)
			DrawTextStyled(rows, truncCaptionStyled(i18n.T(c.GlossKey), textW, RoleCaption), textX, ty+labelH, RoleCaption, colTextSecondary)
			set, inherited := o.chipState(r.id)
			drawRoundedRect(rows, chip, o.chipColor(r.id), RadiusControl, true)
			switch {
			case set:
				drawRoundedRect(rows, chip.Inset(-2), colTextPrimary, RadiusControl, false)
			case inherited:
				drawRoundedRect(rows, chip, colBorderMedium, RadiusControl, false)
			default:
				drawRoundedRect(rows, chip, colBorderMedium, RadiusControl, false)
			}
			o.resetBtns[r.id].Draw(rows)
		}
	}
	o.scroll.Draw(dst)
}

// windowRect is the whole window in screen coordinates.
func (g *Game) windowRect() image.Rectangle { return image.Rect(0, 0, g.winW, g.winH) }

// Through menuCardEntry, so Esc, × and an outside tap close it like every menu.
func (dv *DrumView) openThemeOverlay() {
	if dv.tree == nil {
		return
	}
	p := dv.portal()
	if p == nil || p.Has(themeOverlayID) {
		return
	}
	ov := NewThemeOverlay(dv.game)
	anchor := image.Rectangle{}
	if g := dv.game; g != nil {
		anchor = g.gridThemeButtonRect()
		// Window-bounded, unlike Settings/Logs: the preset list needs the height, and the scrim covers both panes.
		ov.SetPositioner(func(w, h int) image.Rectangle {
			return anchoredPopupRect(g.gridThemeButtonRect(), w, h, g.windowRect(), GridAnchorOpts{Avoid: []image.Rectangle{g.gridTopButtonsReservedRect()}})
		})
		ov.SetMaxHeight(func() int { return g.winH - g.gridTopButtonsReservedRect().Max.Y })
		ov.onChip = func(id string) { g.openThemeSwatchPicker(ov, id) }
	}
	p.Open(menuCardEntry(themeOverlayID, ov, anchor, false, nil))
}
