package ui

import (
	"image"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The "Wheel" name stays for the wiring; it is a swatch grid because circuit colors must come
// from the palette.
type ColorWheelProps struct {
	// AnchorRect is the button that opens the picker (used for positioning).
	AnchorRect image.Rectangle
	// Bounds is the container bounds for clamping the picker position.
	Bounds image.Rectangle
	// RowHeight is used for calculating picker size.
	RowHeight    int
	CurrentColor color.Color
	// A live read, so the ring follows a value that changes while the picker is open (an external
	// theme change).
	CurrentColorFn func() color.Color
	// OnColorPick is called when a swatch is picked.
	OnColorPick func(c color.Color)
	// OnClose is called when the picker should close.
	OnClose func()
}

// ColorWheelState contains the internal state for the color picker component.
type ColorWheelState struct {
	open   bool
	hold   bool // Capture flag (set when opened, released on mouse-up)
	picked bool // Set when the CONFIRM gesture fires; defers Close() until mouse release
	// Set while a pick-press is held, so Capturing() covers the whole gesture.
	pressing bool
	// Props are seeded once at open, so without this the ring and confirm check go stale after a pick.
	applied color.Color
}

// swatchCell is one color cell in the grid: its on-screen rect and color.
type swatchCell struct {
	rect image.Rectangle
	col  color.RGBA
}

type ColorWheelComponent struct {
	overlayBase
	props ColorWheelProps
	state ColorWheelState
	// The picker is the frame's reference look: the card's bands equal solveSwatchGrid's.
	card     *MenuCard
	closeBtn *Button // alias of card.CloseBtn() kept for existing call/test sites

	cells     []swatchCell
	gridRect  image.Rectangle // body region (label gutter + cells)
	cellsRect image.Rectangle // cell region (gridRect minus the left label gutter)
	titleRect image.Rectangle // header region (title + close button)
	hintRect  image.Rectangle // shade-direction caption band above the cells (empty if suppressed)
	cellW     int             // cell width (for gap-tolerant hit mapping)
	cellH     int             // cell height

	keyNav *MenuKeyNav
}

// 5 shades × 7 hue families; cells adapt to the width, floored at the touch minimum on mobile.
const (
	colorSwatchCols    = 5
	colorSwatchRows    = 7
	colorSwatchGap     = 8  // gap between cells
	colorSwatchPad     = 12 // panel inner padding
	colorSwatchHeader  = 32 // header band (title + close)
	colorSwatchCellMin = 40 // desktop minimum cell edge
	colorRampLabelPad  = 12 // gap between the family label and the first swatch
	colorRampLabelMin  = 32 // floor for the left gutter (guards a degenerate palette)
	colorShadeHintH    = 15 // height of the "Light … Deep" shade-direction caption band
)

// Words, not arrow glyphs, which are forbidden in this package.
const (
	colorShadeHintLeft  = "Light"
	colorShadeHintRight = "Deep"
)

// Measured from the labels, so every one fits without touching the first column.
func (c *ColorWheelComponent) labelGutterW() int {
	sw := c.swatches()
	maxW := 0
	for row := 0; row < colorSwatchRows; row++ {
		first := row * colorSwatchCols
		if first >= len(sw) {
			break
		}
		if w := StyledTextWidth(colorFamilyDisplayLabel(sw[first].Name), RoleCaption); w > maxW {
			maxW = w
		}
	}
	if g := maxW + colorRampLabelPad; g > colorRampLabelMin {
		return g
	}
	return colorRampLabelMin
}

// NewColorWheelComponent creates a new swatch-grid color picker component.
func NewColorWheelComponent() *ColorWheelComponent {
	return &ColorWheelComponent{
		overlayBase: newOverlayBase(),
	}
}

// SetProps updates the external props.
func (c *ColorWheelComponent) SetProps(p ColorWheelProps) {
	c.props = p
}

// Props returns the current props.
func (c *ColorWheelComponent) Props() ColorWheelProps { return c.props }

// No hold: the portal's capture already prevents the opening press from double-dispatching,
// and a hold would block input until the next release.
func (c *ColorWheelComponent) Open() {
	c.state.open = true
	c.state.applied = nil
	c.state.pressing = false
	c.rebuildWheel()
	// Start the cursor on the colour already applied.
	if c.keyNav != nil {
		c.keyNav.Reset()
	}
	c.syncCursorToApplied()
}

// ClearHold clears the hold state (for testing when opened via direct callback).
func (c *ColorWheelComponent) ClearHold() {
	c.state.hold = false
}

// Close closes the color picker.
func (c *ColorWheelComponent) Close() {
	c.state.open = false
	c.state.hold = false
	c.state.picked = false
	c.state.pressing = false
	c.state.applied = nil
	if c.props.OnClose != nil {
		c.props.OnClose()
	}
}

// IsOpen returns whether the color picker is currently open.
func (c *ColorWheelComponent) IsOpen() bool {
	return c.state.open
}

func (c *ColorWheelComponent) currentColor() color.Color {
	if c.state.applied != nil {
		return c.state.applied
	}
	if c.props.CurrentColorFn != nil {
		return c.props.CurrentColorFn()
	}
	return c.props.CurrentColor
}

// swatches returns the curated palette to render.
func (c *ColorWheelComponent) swatches() []GenInstrumentSwatch {
	return SuggestedInstrumentSwatches
}

// Clamped to Bounds; on mobile it widens so cells meet the 44 px touch minimum.
func (c *ColorWheelComponent) rebuildWheel() {
	c.cells = c.cells[:0]
	if c.props.AnchorRect.Empty() || c.props.Bounds.Empty() {
		c.SetBounds(image.Rectangle{})
		c.card = nil
		c.closeBtn = nil
		return
	}

	lay := solveSwatchGrid(swatchGridSpec{
		Anchor:      c.props.AnchorRect,
		Bounds:      c.props.Bounds,
		Cols:        colorSwatchCols,
		Rows:        colorSwatchRows,
		Gutter:      c.labelGutterW(),
		Gap:         colorSwatchGap,
		Pad:         colorSwatchPad,
		Header:      colorSwatchHeader,
		CellMin:     colorSwatchCellMin,
		DesktopMaxW: 360,
		BandH:       colorShadeHintH,
	})
	c.SetBounds(lay.Panel)
	c.titleRect = lay.Title
	c.gridRect = lay.Grid
	c.cellsRect = lay.Cells
	c.hintRect = lay.Band
	c.cellW, c.cellH = lay.CellW, lay.CellH
	cw, ch := lay.CellW, lay.CellH

	sw := c.swatches()
	for i, s := range sw {
		col := i % colorSwatchCols
		row := i / colorSwatchCols
		if row >= colorSwatchRows {
			break
		}
		x0 := c.cellsRect.Min.X + col*(cw+colorSwatchGap)
		y0 := c.cellsRect.Min.Y + row*(ch+colorSwatchGap)
		cellRect := image.Rect(x0, y0, x0+cw, y0+ch)
		c.cells = append(c.cells, swatchCell{rect: cellRect, col: s.RGBA})
	}

	// Rebuilt with the layout so a profile flip re-evaluates the close-× policy; the title comes
	// from cardSpec at draw because a pick changes it without a relayout.
	c.card = NewMenuCard(func() { c.Close() })
	c.card.Layout(lay.Panel, c.cardSpec())
	c.closeBtn = c.card.CloseBtn()
}

func (c *ColorWheelComponent) cardSpec() MenuCardSpec {
	title := i18n.T(i18n.KeyMenuColor)
	var chip color.Color
	if name, col, ok := c.selectedSwatchName(); ok {
		title, chip = name, col
	}
	return MenuCardSpec{Title: title, Swatch: chip}
}

// A point in the gaps maps to the nearest cell, so there are no dead gutters; nil only outside
// the grid body.
func (c *ColorWheelComponent) pickColorAt(x, y int) color.Color {
	pt := image.Pt(x, y)
	for _, cell := range c.cells {
		if pt.In(cell.rect) {
			return cell.col
		}
	}
	// cellsRect, not gridRect: the label gutter is not a hit area.
	if pt.In(c.cellsRect) {
		strideX := c.cellW + colorSwatchGap
		strideY := c.cellH + colorSwatchGap
		if strideX < 1 {
			strideX = 1
		}
		if strideY < 1 {
			strideY = 1
		}
		col := (x - c.cellsRect.Min.X) / strideX
		row := (y - c.cellsRect.Min.Y) / strideY
		col = clamp(col, 0, colorSwatchCols-1)
		row = clamp(row, 0, colorSwatchRows-1)
		idx := row*colorSwatchCols + col
		if idx >= 0 && idx < len(c.cells) {
			return c.cells[idx].col
		}
	}
	return nil
}

// HandleInput processes mouse input for the color picker.
func (c *ColorWheelComponent) HandleInput(x, y int, pressed bool) InputResult {
	if !c.state.open {
		return InputIgnored
	}

	// If hold is active (just opened), capture until release.
	if c.state.hold {
		if !pressed {
			c.state.hold = false
		}
		return InputCaptured
	}

	// Deferred close after color pick: stay open until mouse release.
	if c.state.picked {
		if !pressed {
			c.Close()
			return InputConsumed
		}
		return InputCaptured
	}

	// Frame controls (close ×) — highest z-order.
	if c.card != nil && c.card.HandleInput(x, y, pressed) != InputIgnored {
		return InputConsumed
	}

	pt := image.Pt(x, y)

	// Selecting applies and keeps the picker open, so colours can be tried; re-pressing the applied
	// one confirms and closes (on release, so the press cannot fall through).
	if pressed {
		if col := c.pickColorAt(x, y); col != nil {
			rgba, _ := col.(color.RGBA)
			if colorsEqualRGB(c.currentColor(), rgba) {
				c.state.picked = true // confirm → close on release
				return InputCaptured
			}
			c.state.pressing = true
			// Shared with Enter, so pointer and keyboard cannot diverge.
			c.applySwatch(rgba)
			return InputCaptured
		}
	}

	if !pressed && c.state.pressing {
		c.state.pressing = false
		return InputConsumed
	}

	// Pressed inside the panel but not on a swatch → absorb (no pick, no close).
	if pressed && pt.In(c.bounds) {
		return InputConsumed
	}

	// Pressed outside the panel → close.
	if pressed && !pt.In(c.bounds) {
		c.Close()
		return InputConsumed
	}

	// Within bounds but not pressed → consume to prevent click-through.
	if pt.In(c.bounds) {
		return InputConsumed
	}

	return InputIgnored
}

func stripShadeSuffix(name string) string {
	if i := strings.LastIndex(name, "-"); i >= 0 {
		if suffix := name[i+1:]; suffix != "" && isAllDigits(suffix) {
			return name[:i]
		}
	}
	return name
}

func familyLabelFromSwatchName(name string) string {
	name = stripShadeSuffix(name)
	words := strings.Split(name, "-")
	for i, w := range words {
		if w == "" {
			continue
		}
		rs := []rune(w)
		rs[0] = unicode.ToUpper(rs[0])
		words[i] = string(rs)
	}
	return strings.Join(words, " ")
}

// Short names keep the gutter narrow; at most 6 runes each.
var colorFamilyShortLabels = map[string]string{
	"sunset-gold":   "Gold",
	"tangerine":     "Orange",
	"hot-pink":      "Pink",
	"orchid":        "Purple",
	"electric-blue": "Blue",
	"cyan":          "Cyan",
	"mint":          "Mint",
}

func colorFamilyDisplayLabel(swatchName string) string {
	if short, ok := colorFamilyShortLabels[stripShadeSuffix(swatchName)]; ok {
		return short
	}
	return familyLabelFromSwatchName(swatchName)
}

// The header names the applied color ("Rose"), never a hex code; keep in sync with the palette.
var canonicalSwatchNames = map[string]string{
	// Sunset Gold ramp (light → deep).
	"sunset-gold-100": "Pale Gold", "sunset-gold-200": "Gold", "sunset-gold-300": "Amber", "sunset-gold-400": "Marigold", "sunset-gold-500": "Ochre",
	// Tangerine ramp.
	"tangerine-100": "Peach", "tangerine-200": "Apricot", "tangerine-300": "Tangerine", "tangerine-400": "Orange", "tangerine-500": "Burnt Orange",
	// Hot Pink ramp.
	"hot-pink-100": "Pink", "hot-pink-200": "Hot Pink", "hot-pink-300": "Rose", "hot-pink-400": "Magenta", "hot-pink-500": "Fuchsia",
	// Orchid ramp.
	"orchid-100": "Lavender", "orchid-200": "Orchid", "orchid-300": "Violet", "orchid-400": "Purple", "orchid-500": "Indigo",
	// Electric Blue ramp.
	"electric-blue-100": "Sky Blue", "electric-blue-200": "Cornflower", "electric-blue-300": "Azure", "electric-blue-400": "Royal Blue", "electric-blue-500": "Navy",
	// Cyan ramp.
	"cyan-100": "Ice", "cyan-200": "Aqua", "cyan-300": "Cyan", "cyan-400": "Teal", "cyan-500": "Deep Teal",
	// Mint ramp.
	"mint-100": "Mint", "mint-200": "Spring Green", "mint-300": "Emerald", "mint-400": "Green", "mint-500": "Forest Green",
}

func canonicalColorName(swatchName string) string {
	if n, ok := canonicalSwatchNames[swatchName]; ok {
		return n
	}
	return colorFamilyDisplayLabel(swatchName)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (c *ColorWheelComponent) selectedSwatchName() (string, color.Color, bool) {
	if c.currentColor() == nil {
		return "", nil, false
	}
	for _, s := range c.swatches() {
		if colorsEqualRGB(c.currentColor(), s.RGBA) {
			return canonicalColorName(s.Name), s.RGBA, true
		}
	}
	return "", nil, false
}

// colorsEqualRGB reports whether two colors match on RGB (ignoring alpha).
func colorsEqualRGB(a color.Color, b color.RGBA) bool {
	if a == nil {
		return false
	}
	ar, ag, ab, _ := a.RGBA()
	return uint8(ar>>8) == b.R && uint8(ag>>8) == b.G && uint8(ab>>8) == b.B
}

// Draw renders the swatch-grid color picker.
func (c *ColorWheelComponent) Draw(dst *ebiten.Image) {
	if !c.state.open {
		return
	}
	r := c.bounds
	if r.Empty() {
		return
	}

	if c.card != nil {
		c.card.Layout(r, c.cardSpec())
		c.card.Draw(dst)
	}

	if !c.hintRect.Empty() {
		hh := StyledTextHeight(RoleMicro)
		hy := c.hintRect.Min.Y + (c.hintRect.Dy()-hh)/2
		DrawTextStyled(dst, colorShadeHintLeft, c.hintRect.Min.X, hy, RoleMicro, colTextSecondary)
		rw := StyledTextWidth(colorShadeHintRight, RoleMicro)
		DrawTextStyled(dst, colorShadeHintRight, c.hintRect.Max.X-rw, hy, RoleMicro, colTextSecondary)
	}

	// Family-name labels in the left gutter, vertically centered on each row.
	sw := c.swatches()
	lh := StyledTextHeight(RoleCaption)
	for row := 0; row < colorSwatchRows; row++ {
		first := row * colorSwatchCols
		if first >= len(c.cells) || first >= len(sw) {
			break
		}
		label := colorFamilyDisplayLabel(sw[first].Name)
		cellRect := c.cells[first].rect
		ly := cellRect.Min.Y + (cellRect.Dy()-lh)/2
		DrawTextStyled(dst, label, c.gridRect.Min.X, ly, RoleCaption, colTextSecondary)
	}

	// The ring insets beyond the swatch, so it is bounded separately from the swatch's radius.
	cursor := c.KeyCursorSwatch()
	for i, cell := range c.cells {
		radius := fitRadius(RadiusControl, cell.rect)
		drawRoundedRect(dst, cell.rect, cell.col, radius, true)
		// Subtle border so light swatches read against the panel.
		drawRoundedRect(dst, cell.rect, colPanelBorder, radius, false)
		// Selected ring on the swatch matching the row's current color.
		if colorsEqualRGB(c.currentColor(), cell.col) {
			ring := cell.rect.Inset(-2)
			inner := cell.rect.Inset(2)
			drawRoundedRect(dst, ring, genColorFocusRing, fitRadius(RadiusControl, ring), false)
			drawRoundedRect(dst, inner, colTextPrimary, fitRadius(RadiusControl, inner), false)
		}
		// An outline, not a tint: a swatch IS its colour, and a tint would misrepresent it.
		if i == cursor {
			in := cell.rect.Inset(3)
			drawRoundedRect(dst, in, colTextPrimary, fitRadius(RadiusControl, in), false)
			drawRoundedRect(dst, cell.rect, colTextPrimary, radius, false)
		}
	}

}

// CloseBtn returns the close button (for testing).
func (c *ColorWheelComponent) CloseBtn() *Button { return c.closeBtn }

// Capturing returns whether the component is capturing input.
func (c *ColorWheelComponent) Capturing() bool {
	return c.state.hold || c.state.picked || c.state.pressing
}

// InputBounds returns the picker panel bounds for overlay compatibility.
func (c *ColorWheelComponent) InputBounds() image.Rectangle {
	if !c.state.open {
		return image.Rectangle{}
	}
	return c.bounds
}

// WheelRect returns the current picker panel rectangle (for testing/debugging).
func (c *ColorWheelComponent) WheelRect() image.Rectangle {
	return c.bounds
}

func (c *ColorWheelComponent) ShadeHintRect() image.Rectangle { return c.hintRect }

func (c *ColorWheelComponent) LabelGutterWidth() int {
	return c.cellsRect.Min.X - c.gridRect.Min.X
}

func (c *ColorWheelComponent) FamilyDisplayLabels() []string {
	sw := c.swatches()
	var out []string
	for row := 0; row < colorSwatchRows; row++ {
		first := row * colorSwatchCols
		if first >= len(sw) {
			break
		}
		out = append(out, colorFamilyDisplayLabel(sw[first].Name))
	}
	return out
}

// SwatchCells returns the laid-out swatch cell rects + colors (for testing).
func (c *ColorWheelComponent) SwatchCells() []image.Rectangle {
	out := make([]image.Rectangle, len(c.cells))
	for i, cell := range c.cells {
		out[i] = cell.rect
	}
	return out
}

// HandleWheel consumes wheel events to prevent pass-through.
func (c *ColorWheelComponent) HandleWheel(x, y, steps int) InputResult {
	if !c.state.open {
		return InputIgnored
	}
	return InputConsumed
}
