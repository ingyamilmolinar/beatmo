package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

const (
	// Ebiten's debug font uses 6px-wide glyphs in a 16px-tall cell.
	debugCharW = 6  // width of a character drawn by DebugPrintAt
	debugCharH = 16 // height of a character drawn by DebugPrintAt
)

// Published once per frame so animations advance without threading a frame through every Draw.
var uiAnimFrame int64

// insetRect returns r shrunk by pad pixels on all sides.
func insetRect(r image.Rectangle, pad int) image.Rectangle {
	if r.Dx() < 2*pad || r.Dy() < 2*pad {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}

// Whole runes, with "..." when truncated.
func clipTextToWidth(text string, maxW int) string {
	return clipTextToWidthMeasure(text, maxW, TextWidth)
}

// measure must be the styled metric the text draws with, so the clip lands on the real glyphs.
func clipTextToWidthMeasure(text string, maxW int, measure func(string) int) string {
	if maxW <= 0 {
		return ""
	}
	if measure(text) <= maxW {
		return text
	}
	ellipsis := "..."
	rs := []rune(text)
	for keepRunes := len(rs) - 1; keepRunes >= 0; keepRunes-- {
		candidate := string(rs[:keepRunes]) + ellipsis
		if measure(candidate) <= maxW {
			return candidate
		}
	}
	return ellipsis
}

type ButtonVisual interface {
	Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool)
}

// Button is a basic clickable component with a rectangular bounds and text label.
type Button struct {
	r    image.Rectangle
	Text string
	// Resolved at draw time, so a language switch updates the label without reconstruction. Overrides Text.
	TextKey i18n.Key
	Style   ButtonVisual
	OnClick func()
	pressed bool
	hovered bool
	Repeat  bool
	held    int
	// 0 rest, 1 bottomed out, negative = release overshoot; stored so the spring allocates nothing and settles to 0.
	pressDepth  float64
	pressTarget float64
	// A one-shot flash on the off→on latch transition.
	engageAnim float64
	// So the engage flash fires exactly on the off→on transition.
	wasActive bool
	toggled   bool
	// Never fires OnClick on either input path (the subdivision selector is disabled while playing).
	Disabled bool
	// For destructive actions: suppresses every other click until release, so a layout change under a held cursor
	// cannot cascade.
	ConsumeOnPress bool
	// Ignored when a text role is set.
	TextScale float64
	// The styled path (crisp Inter at true size), not the bitmap upscale that pixelates. RoleBody is the zero value, so a
	// "set" flag is needed.
	textRole    TextRole
	hasTextRole bool
	// Vector-drawn: the debug font lacks most Unicode glyphs.
	Icon      string
	IconColor color.Color
	// While IconColor still equals the seed the spec is re-read, so a runtime theme recolours the glyph without losing a
	// call-site override.
	specIconSeed color.Color
	// Only without an icon; indices outside the clipped text are ignored.
	Highlights []int
	TextColor  color.Color
	// Draw prefers SpecID over Style. Use HasSpecID(): ComponentID 0 is a valid spec.
	SpecID    ComponentID
	hasSpecID bool
	// For the timeline lock keycap, which fills its strip as a solid block.
	SquareCorners bool
	// The "swatch + text" row pattern, left-aligning the label after the swatch.
	LeadSwatch color.Color
}

func (b *Button) labelArea(cap image.Rectangle) (image.Rectangle, bool) {
	if b.LeadSwatch != nil {
		sr := b.leadSwatchRect(cap)
		gap := SpaceSM
		if Profile().IsMobile() {
			gap = SpaceXS // the slim mobile bar needs less air than a square
		}
		return image.Rect(sr.Max.X+gap, cap.Min.Y, cap.Max.X-SpaceXS, cap.Max.Y), true
	}
	// A 1 px margin: a wider inset turned a single "M" on an 18 px toggle into "...".
	return image.Rect(cap.Min.X+1, cap.Min.Y, cap.Max.X-1, cap.Max.Y), false
}

// On mobile a narrow label cannot spare a square, so it is a slim identity bar instead.
func (b *Button) leadSwatchRect(cap image.Rectangle) image.Rectangle {
	sz := SwatchSizeMD
	if sz > cap.Dy() {
		sz = cap.Dy()
	}
	sx := cap.Min.X + SpaceSM
	if Profile().IsMobile() {
		w := Profile().AccentStripeWidth
		if w < 2 {
			w = 2
		}
		h := cap.Dy() - 2*SpaceSM
		if h < sz {
			h = sz
		}
		sx = cap.Min.X + SpaceXS
		sy := cap.Min.Y + (cap.Dy()-h)/2
		return image.Rect(sx, sy, sx+w, sy+h)
	}
	sy := cap.Min.Y + (cap.Dy()-sz)/2
	return image.Rect(sx, sy, sx+sz, sy+sz)
}

func (b *Button) labelOrigin(cap image.Rectangle, w, h int) (int, int) {
	area, left := b.labelArea(cap)
	x := area.Min.X + (area.Dx()-w)/2
	if left {
		x = area.Min.X
	}
	return x, cap.Min.Y + (cap.Dy()-h)/2
}

// Use this setter, never the field, so ID 0 is honoured. A spec iconColor seeds IconColor as the resting default.
func (b *Button) SetSpec(id ComponentID) {
	b.SpecID = id
	b.hasSpecID = true
	if spec := Spec(id); spec.HasIconColor {
		b.IconColor = spec.IconColor
		b.specIconSeed = spec.IconColor
	}
}

// A call-site override wins; a spec-seeded colour is re-read live so a runtime theme recolours existing buttons.
func (b *Button) effectiveIconColor() color.Color {
	if b.hasSpecID && b.specIconSeed != nil && b.IconColor == b.specIconSeed {
		if spec := Spec(b.SpecID); spec.HasIconColor {
			return spec.IconColor
		}
	}
	return liveCaptured(b.IconColor)
}

// nil when no colour was set.
func (b *Button) effectiveTextColor() color.Color { return liveCaptured(b.TextColor) }

// HasSpecID reports whether SetSpec has been called on this button.
func (b *Button) HasSpecID() bool { return b.hasSpecID }

// Stops other buttons firing while a press is held after a click reflowed the UI under the cursor.
var suppressClicksUntilRelease bool

// The next mouse release clears it.
func SuppressClicksUntilMouseUp() { suppressClicksUntilRelease = true }

// NewButton constructs a button with the given label, style, and optional click handler.
func NewButton(text string, style ButtonVisual, onClick func()) *Button {
	return &Button{Text: text, Style: style, OnClick: onClick}
}

// Prefer this over NewButton.
func NewSpecButton(text string, id ComponentID, onClick func()) *Button {
	b := &Button{Text: text, OnClick: onClick}
	b.SetSpec(id)
	return b
}

// Prefer this for any localized label.
func NewButtonKey(key i18n.Key, id ComponentID, onClick func()) *Button {
	b := &Button{TextKey: key, OnClick: onClick}
	b.SetSpec(id)
	return b
}

// SetTextKey switches the button to live i18n resolution.
func (b *Button) SetTextKey(key i18n.Key) { b.TextKey = key }

func (b *Button) displayText() string {
	if b.TextKey != "" {
		return i18n.T(b.TextKey)
	}
	return b.Text
}

// Rect returns the button's bounds.
func (b *Button) Rect() image.Rectangle { return b.r }

// SetRect sets the button's bounds.
func (b *Button) SetRect(r image.Rectangle) { b.r = r }

// Draw renders the button and its label.
func (b *Button) Draw(dst *ebiten.Image) {
	// Draw is the per-frame tick site; a settled button no-ops.
	b.AdvancePressAnim()
	// One radius for every control regardless of size: per-size radii made the toolbar read as unrelated widgets.
	rad := fitRadius(RadiusControl, b.r)
	if b.SquareCorners {
		rad = RadiusNone
	}
	// Flat-tonal: one filled body; the silhouette is the tone step against the container.
	cap := b.r
	switch {
	case b.Disabled:
		// Greyed-out chrome; never reflect press/hover/active state.
		DisabledButtonStyle.Draw(dst, cap, false, false)
	case b.hasSpecID:
		Render(dst, cap, Spec(b.SpecID), ComponentState{Pressed: b.pressed, Hovered: b.hovered, Active: b.toggled})
	case b.Style != nil:
		b.Style.Draw(dst, cap, b.pressed, b.hovered)
	}
	if !b.Disabled && b.toggled {
		// A tint plus the 2 px bar, never a flood: the accent marks values, not chrome.
		drawRoundedRect(dst, cap, WithAlpha(colAccent, AlphaSubtle), rad, true)
		drawFlatSelectionBar(dst, cap, colAccent, selectionEdgeBottom, 1)
	}
	if b.LeadSwatch != nil {
		sr := b.leadSwatchRect(cap)
		drawRoundedRect(dst, sr, b.LeadSwatch, fitRadius(RadiusControl, sr), true)
	}
	// The debug font draws icon glyphs as white rectangles.
	if b.Icon == "" && b.hasTextRole {
		b.drawStyledLabel(dst, cap)
	} else if b.Icon == "" {
		scale := b.TextScale
		if scale <= 0 {
			scale = 1.0
		}
		// Clip text to fit within the label area.
		area, _ := b.labelArea(cap)
		clipped := clipTextToWidth(b.displayText(), int(float64(area.Dx())/scale))
		spr := TextSprite(clipped)
		// Center (or, with a LeadSwatch, left-align) the scaled text.
		w := int(float64(TextWidth(clipped)) * scale)
		h := int(float64(TextHeight()) * scale)
		x, y := b.labelOrigin(cap, w, h)

		// Draw fuzzy-match highlight rectangles behind matched chars.
		if len(b.Highlights) > 0 {
			drawTextHighlights(dst, clipped, b.Highlights, x, y, scale, TextHeight(), TextWidth)
		}

		var op ebiten.DrawImageOptions
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(x), float64(y))
		textCol := b.effectiveTextColor()
		if b.Disabled {
			textCol = colTextDisabled
		} else if b.toggled && textCol == nil {
			// Bright text on the tinted latched body; dark text was for the old solid flood.
			textCol = colTextPrimary
		}
		if textCol != nil {
			r, g, bb, a := textCol.RGBA()
			if a > 0 {
				op.ColorScale.Scale(float32(r)/float32(a), float32(g)/float32(a), float32(bb)/float32(a), float32(a)/0xffff)
			}
		}
		dst.DrawImage(spr, &op)
	}
	// Icon overlay (font-independent)
	if b.Icon != "" {
		col := b.effectiveIconColor()
		if col == nil {
			col = colButtonBorder
		}
		if b.Disabled {
			col = colTextDisabled
		} else if b.toggled && b.IconColor == nil {
			// The glyph carries the "on" signal alongside the bar.
			col = colAccent
		}
		// Proportional margin: 18% of min(w,h), min 2px.
		dim := cap.Dx()
		if cap.Dy() < dim {
			dim = cap.Dy()
		}
		pad := dim * 18 / 100
		if pad < 2 {
			pad = 2
		}
		box := image.Rect(cap.Min.X+pad, cap.Min.Y+pad, cap.Max.X-pad, cap.Max.Y-pad)
		// Sprite cache path: 1 DrawImage blit instead of many drawRect calls.
		if spr := iconSprite(b.Icon, box.Dx(), box.Dy()); spr != nil {
			var op ebiten.DrawImageOptions
			op.GeoM.Translate(float64(box.Min.X), float64(box.Min.Y))
			cr, cg, cb, ca := col.RGBA()
			if ca > 0 {
				op.ColorScale.Scale(float32(cr)/float32(ca), float32(cg)/float32(ca), float32(cb)/float32(ca), float32(ca)/0xffff)
			}
			dst.DrawImage(spr, &op)
			return
		}
		// For test builds (no sprite cache) and uncached paths.
		drawIconByID(dst, IconID(b.Icon), box, col)
	}
}

// colFuzzyHighlight is the background color for fuzzy-match highlighted chars.
var colFuzzyHighlight color.NRGBA // accent, semi-transparent; derived in rebuildThemeAliases (theme.go)

// measure and cellH MUST be the metric the text renders with, so cell i spans [measure(text[:i]),
// measure(text[:i+1])] and tiles the glyphs; summed rune widths lose kerning and drift.
func drawTextHighlights(dst *ebiten.Image, text string, highlights []int, baseX, baseY int, scale float64, cellH int, measure func(string) int) {
	rs := []rune(text)
	hlSet := make(map[int]bool, len(highlights))
	maxIdx := -1
	for _, idx := range highlights {
		if idx >= 0 && idx < len(rs) {
			hlSet[idx] = true
			if idx > maxIdx {
				maxIdx = idx
			}
		}
	}
	if maxIdx < 0 {
		return
	}
	prevW := 0
	for i := 0; i <= maxIdx; i++ {
		w := measure(string(rs[:i+1]))
		if w < prevW {
			w = prevW // measure is nondecreasing; guard trailing-space quirks
		}
		if hlSet[i] && w > prevW {
			sx := baseX + int(float64(prevW)*scale)
			sw := int(float64(w-prevW) * scale)
			sh := int(float64(cellH) * scale)
			drawRect(dst, image.Rect(sx, baseY, sx+sw, baseY+sh), colFuzzyHighlight, true)
		}
		prevW = w
	}
}

// Portal overlays only see press/drag/release, so hover needs this per-frame seam. Desktop only.
func (b *Button) UpdateHover(mx, my int) {
	if b == nil {
		return
	}
	b.hovered = !b.Disabled && image.Pt(mx, my).In(b.Rect())
}

// RoleBody is the zero value, hence a setter with a "set" flag.
func (b *Button) SetTextRole(role TextRole) {
	b.textRole = role
	b.hasTextRole = true
}

// StyledTextRole reports the styled label role and whether one is set.
func (b *Button) StyledTextRole() (TextRole, bool) { return b.textRole, b.hasTextRole }

// Matches the legacy path's centring, clipping, highlights and colours.
func (b *Button) drawStyledLabel(dst *ebiten.Image, cap image.Rectangle) {
	role := b.textRole
	measure := func(s string) int { return StyledTextWidth(s, role) }
	area, _ := b.labelArea(cap)
	clipped := clipTextToWidthMeasure(b.displayText(), area.Dx(), measure)
	w := measure(clipped)
	h := StyledTextHeight(role)
	x, y := b.labelOrigin(cap, w, h)
	if len(b.Highlights) > 0 {
		drawTextHighlights(dst, clipped, b.Highlights, x, y, 1.0, h, measure)
	}
	textCol := b.effectiveTextColor()
	if b.Disabled {
		textCol = colTextDisabled
	} else if b.toggled && textCol == nil {
		textCol = genColorBackground
	}
	if textCol == nil {
		textCol = colTextPrimary
	}
	DrawTextStyled(dst, clipped, x, y, role, textCol)
}

// Clamped to the button, with the same clip and scale as Draw.
//
//nolint:unused // used by test files with test build tag
func (b *Button) textRect() image.Rectangle {
	if b.hasTextRole {
		measure := func(s string) int { return StyledTextWidth(s, b.textRole) }
		area, _ := b.labelArea(b.r)
		clipped := clipTextToWidthMeasure(b.displayText(), area.Dx(), measure)
		w := measure(clipped)
		h := StyledTextHeight(b.textRole)
		x, y := b.labelOrigin(b.r, w, h)
		return image.Rect(x, y, x+w, y+h).Intersect(b.r)
	}
	scale := b.TextScale
	if scale <= 0 {
		scale = 1.0
	}
	area, _ := b.labelArea(b.r)
	clipped := clipTextToWidth(b.displayText(), int(float64(area.Dx())/scale))
	w := int(float64(TextWidth(clipped)) * scale)
	h := int(float64(TextHeight()) * scale)
	x, y := b.labelOrigin(b.r, w, h)
	return image.Rect(x, y, x+w, y+h).Intersect(b.r)
}

// On touch the hit area grows to the minimum target.
func (b *Button) HandleInputResult(mx, my int, pressed bool) InputResult {
	if b.Disabled {
		b.pressed = false
		b.hovered = false
		b.held = 0
		return InputIgnored
	}
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		b.pressed = false
		b.held = 0
		return InputIgnored
	}
	inside := image.Pt(mx, my).In(b.HitRect())
	return b.applyPress(inside, pressed)
}

// THE hit geometry: a handler routing presses must use this, not Rect(), or a press in the touch target falls
// through. A zero-area rect stays non-interactive.
func (b *Button) HitRect() image.Rectangle {
	hitRect := b.r
	if minTarget := TouchMinTarget(); minTarget > 0 && !hitRect.Empty() {
		if hitRect.Dx() < minTarget {
			expand := (minTarget - hitRect.Dx()) / 2
			hitRect.Min.X -= expand
			hitRect.Max.X += expand
		}
		if hitRect.Dy() < minTarget {
			expand := (minTarget - hitRect.Dy()) / 2
			hitRect.Min.Y -= expand
			hitRect.Max.Y += expand
		}
	}
	return hitRect
}

// Shared by HandleInputResult and the tree adapter so both get the same edge firing, repeat and release
// settle; inside=true trusts an upstream hit decision.
func (b *Button) applyPress(inside, pressed bool) InputResult {
	b.hovered = inside
	if pressed && inside {
		b.held++
		if b.held == 1 {
			// Press edge: ease the cap down toward full travel.
			emitUITap(b.displayText(), b.Icon)
			b.pressTarget = 1
			if b.OnClick != nil {
				b.OnClick()
			}
			if b.ConsumeOnPress {
				suppressClicksUntilRelease = true
			}
		} else if b.Repeat && b.repeatTick() {
			if b.OnClick != nil {
				b.OnClick()
			}
		}
		b.pressed = true
		return InputConsumed
	}
	if b.pressed {
		// Release kicks the cap UP for the overshoot before it springs back to 0.
		b.pressTarget = 0
		if b.pressDepth > 0 {
			b.pressDepth = -genGeomKeycapReleaseOvershootFrac
		}
	}
	b.pressed = false
	b.held = 0
	return InputIgnored
}

// The one seam buttonHitAdapter uses, so tree-routed buttons share the exact press lifecycle.
func (b *Button) PressFromTree(pressed bool) InputResult {
	if b.Disabled {
		b.pressed = false
		b.hovered = false
		b.held = 0
		return InputConsumed
	}
	return b.applyPress(true, pressed)
}

// Stay-open menus reusing a Button must call this before re-dispatching: they consume without capture, so no
// release reaches the button and held stays at 1, swallowing the next click.
func (b *Button) ResetPress() {
	if b.pressed {
		b.pressTarget = 0
		if b.pressDepth > 0 {
			b.pressDepth = -genGeomKeycapReleaseOvershootFrac
		}
	}
	b.pressed = false
	b.hovered = false
	b.held = 0
}

// Heals a button whose release a newly opened menu swallowed (stuck down, un-clickable). Keeps hover, so it is safe
// to call every pointer-up frame; with the pointer up it cannot abort a press.
func (b *Button) ReleaseStuckPress() {
	if !b.pressed && b.held == 0 {
		return
	}
	if b.pressed && b.pressDepth > 0 {
		b.pressDepth = -genGeomKeycapReleaseOvershootFrac
	}
	b.pressTarget = 0
	b.pressed = false
	b.held = 0
}

// Allocation-free; a no-op once settled.
func (b *Button) AdvancePressAnim() {
	if b.pressDepth != b.pressTarget {
		r := genAnimButtonPressDecay.Rate
		b.pressDepth = b.pressTarget + (b.pressDepth-b.pressTarget)*r
		if math.Abs(b.pressDepth-b.pressTarget) < genAnimButtonPressDecay.Threshold {
			b.pressDepth = b.pressTarget
		}
	}
	if b.engageAnim > 0 {
		if v, alive := DecayStep(b.engageAnim, genAnimButtonEngageFlash); alive {
			b.engageAnim = v
		} else {
			b.engageAnim = 0
		}
	}
}

func (b *Button) fillColor() color.Color {
	if b.hasSpecID {
		return Spec(b.SpecID).Fill
	}
	return TokenSurface2()
}

func (b *Button) Pressed() bool { return b.pressed }

func (b *Button) SetToggled(on bool) { b.toggled = on }

// Toggled reports whether the button is in the latched/active state.
func (b *Button) Toggled() bool { return b.toggled }

// The shared buttonRepeat curve with the legacy firing rule, double-fire included.
func (b *Button) repeatTick() bool {
	return buttonRepeat.tickLegacyModulo(b.held)
}

// GridLayout splits a rectangle into rows and columns using fractional weights.
type GridLayout struct {
	bounds     image.Rectangle
	colWeights []float64
	rowWeights []float64
	colPos     []int
	rowPos     []int
}

// NewGridLayout creates a layout for the given bounds.
func NewGridLayout(b image.Rectangle, cols, rows []float64) *GridLayout {
	g := &GridLayout{bounds: b, colWeights: cols, rowWeights: rows}
	g.recalc()
	return g
}

func (g *GridLayout) recalc() {
	totalW := 0.0
	for _, w := range g.colWeights {
		totalW += w
	}
	totalH := 0.0
	for _, h := range g.rowWeights {
		totalH += h
	}
	// Rounded fractions of the total spread the error (±1 px) instead of piling it into the last cell.
	g.colPos = make([]int, len(g.colWeights)+1)
	cumW := 0.0
	for i, w := range g.colWeights {
		g.colPos[i] = g.bounds.Min.X + int(cumW/totalW*float64(g.bounds.Dx()))
		cumW += w
	}
	g.colPos[len(g.colWeights)] = g.bounds.Max.X

	g.rowPos = make([]int, len(g.rowWeights)+1)
	cumH := 0.0
	for i, h := range g.rowWeights {
		g.rowPos[i] = g.bounds.Min.Y + int(cumH/totalH*float64(g.bounds.Dy()))
		cumH += h
	}
	g.rowPos[len(g.rowWeights)] = g.bounds.Max.Y
}

// Cell returns the rectangle for the specified cell.
func (g *GridLayout) Cell(col, row int) image.Rectangle {
	return image.Rect(g.colPos[col], g.rowPos[row], g.colPos[col+1], g.rowPos[row+1])
}

func (g *GridLayout) SubGrid(col, row int, cols, rows []float64) *GridLayout {
	return NewGridLayout(g.Cell(col, row), cols, rows)
}

// Moving or resizing a parent repositions its children, so a whole section moves by one cell assignment.
type LayoutGroup struct {
	ID       string
	grid     *GridLayout
	parent   *LayoutGroup
	pCol     int // parent cell column
	pRow     int // parent cell row
	children []*LayoutGroup
	visible  bool
}

// NewLayoutGroup creates a top-level layout group.
func NewLayoutGroup(id string, bounds image.Rectangle, cols, rows []float64) *LayoutGroup {
	return &LayoutGroup{
		ID:      id,
		grid:    NewGridLayout(bounds, cols, rows),
		visible: true,
	}
}

// AddChild creates a child group anchored to the parent's cell at (col, row).
func (g *LayoutGroup) AddChild(id string, col, row int, cols, rows []float64) *LayoutGroup {
	child := &LayoutGroup{
		ID:      id,
		grid:    NewGridLayout(g.grid.Cell(col, row), cols, rows),
		parent:  g,
		pCol:    col,
		pRow:    row,
		visible: true,
	}
	g.children = append(g.children, child)
	return child
}

// Bounds returns the absolute rectangle of this group.
func (g *LayoutGroup) Bounds() image.Rectangle {
	if !g.visible {
		return image.Rectangle{}
	}
	return g.grid.bounds
}

// Cell returns the absolute cell rectangle within this group.
func (g *LayoutGroup) Cell(col, row int) image.Rectangle {
	if !g.visible {
		return image.Rectangle{}
	}
	return g.grid.Cell(col, row)
}

// SetBounds updates the bounds and cascades to all children.
func (g *LayoutGroup) SetBounds(b image.Rectangle) {
	g.grid.bounds = b
	g.grid.recalc()
	for _, c := range g.children {
		c.SetBounds(g.grid.Cell(c.pCol, c.pRow))
	}
}

// SetVisible hides or shows this group and all its children.
func (g *LayoutGroup) SetVisible(v bool) {
	g.visible = v
	for _, c := range g.children {
		c.SetVisible(v)
	}
}

// Relayout recalculates this group's bounds from its parent cell and cascades.
func (g *LayoutGroup) Relayout() {
	if g.parent != nil {
		g.SetBounds(g.parent.grid.Cell(g.pCol, g.pRow))
	} else {
		g.grid.recalc()
		for _, c := range g.children {
			c.SetBounds(g.grid.Cell(c.pCol, c.pRow))
		}
	}
}

// Find looks up a child group by ID in the subtree (depth-first).
func (g *LayoutGroup) Find(id string) *LayoutGroup {
	if g.ID == id {
		return g
	}
	for _, c := range g.children {
		if found := c.Find(id); found != nil {
			return found
		}
	}
	return nil
}
