package ui

import (
	"image"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ColorWheelProps contains the external state passed to the color picker.
//
// Despite the legacy "Wheel" naming (kept so the portal wiring and every
// caller continue to compile), the picker is now a SWATCH GRID restricted to
// the curated instrument palette (SuggestedInstrumentSwatches). The product
// requirement is that circuit colors come from a known set, so the free
// HSV hue-wheel was removed.
type ColorWheelProps struct {
	// AnchorRect is the button that opens the picker (used for positioning).
	AnchorRect image.Rectangle
	// Bounds is the container bounds for clamping the picker position.
	Bounds image.Rectangle
	// RowHeight is used for calculating picker size.
	RowHeight int
	// CurrentColor is the row's currently-applied color. When it matches a
	// palette swatch, that swatch renders a selected ring. Optional.
	CurrentColor color.Color
	// CurrentColorFn, when set, supersedes CurrentColor with a LIVE read, so
	// the selected ring follows a value that changes while the picker is
	// open (the Theme editor's category swatch under an external theme
	// change — undo, a circuit load) instead of the value seeded at open.
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
	// pressing is true while a pick-press is in flight (applied but not yet
	// released), so Capturing() keeps reporting ownership for the whole
	// gesture even when the press did not close the picker.
	pressing bool
	// applied is the colour picked during this session, which supersedes
	// props.CurrentColor. Props are seeded once at open time, so without this
	// the selected ring and the confirm check would both go stale the moment
	// the user picks anything.
	applied color.Color
}

// swatchCell is one color cell in the grid: its on-screen rect and color.
type swatchCell struct {
	rect image.Rectangle
	col  color.RGBA
}

// ColorWheelComponent is a self-contained swatch-grid color picker restricted
// to the curated instrument palette.
type ColorWheelComponent struct {
	overlayBase
	props ColorWheelProps
	state ColorWheelState
	// card is the shared MenuCard frame (panel, title band, close ×); the
	// picker is the frame's REFERENCE aesthetic, so the card's bands equal the
	// picker's own solveSwatchGrid bands (TestMenuCardReproducesSwatchPickerFrame).
	card     *MenuCard
	closeBtn *Button // alias of card.CloseBtn() kept for existing call/test sites

	cells     []swatchCell
	gridRect  image.Rectangle // body region (label gutter + cells)
	cellsRect image.Rectangle // cell region (gridRect minus the left label gutter)
	titleRect image.Rectangle // header region (title + close button)
	hintRect  image.Rectangle // shade-direction caption band above the cells (empty if suppressed)
	cellW     int             // cell width (for gap-tolerant hit mapping)
	cellH     int             // cell height

	// keyNav is the keyboard cursor over the swatch grid — the same MenuKeyNav
	// the note matrix and the list menus use.
	keyNav *MenuKeyNav
}

// Grid layout tuning. 5 columns (shades, light→deep) × 7 rows (hue families)
// lays out the 35-swatch Vice City palette into a tall, finger-friendly panel.
// A left gutter reserves room for family-name labels. The cell size adapts to
// the panel width and is floored at the touch minimum on mobile.
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

// colorShadeHintLeft / colorShadeHintRight are the end captions of the
// shade-direction hint drawn above the swatch columns (columns run light→deep,
// left→right). Words, not arrow glyphs (→ is a forbidden glyph in internal/ui).
const (
	colorShadeHintLeft  = "Light"
	colorShadeHintRight = "Deep"
)

// labelGutterW returns the width of the left family-name gutter, sized to the
// widest short family label at RoleCaption plus a fixed pad. Deriving the gutter
// from the measured labels (instead of a fixed constant) guarantees every label
// fits fully and never collides with the first swatch column, while keeping the
// gutter as narrow as the names allow. Guard: TestColorPickerFamilyLabelsFitGutter.
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

// Open opens the color picker.
// Note: hold is NOT set here because the portal tree's capture system
// prevents double-dispatch of the opening press. Setting hold would block
// all subsequent input until a mouse release clears it.
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

// currentColor is the colour currently applied to the row: whatever was picked
// in this session, else the value the picker was opened with.
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

// rebuildWheel recomputes the panel rect and the swatch-cell layout.
//
// The panel is sized to comfortably fit a colorSwatchCols × colorSwatchRows
// grid of touch-friendly cells plus a header band, then clamped to stay inside
// Bounds. On mobile the panel widens toward the container so cells meet the
// 44 px touch minimum.
func (c *ColorWheelComponent) rebuildWheel() {
	c.cells = c.cells[:0]
	if c.props.AnchorRect.Empty() || c.props.Bounds.Empty() {
		c.SetBounds(image.Rectangle{})
		c.card = nil
		c.closeBtn = nil
		return
	}

	// Shared with the note-matrix picker — see swatch_grid_geometry.go for the
	// cell-solve / touch-min floor / clamp / re-derive sequence.
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

	// Lay out the swatch cells over the cell region (row-major: family per row,
	// shade light→deep per column).
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

	// Frame via the shared MenuCard: panel + title band + close × (desktop
	// only, per desktopCloseVisible). Rebuilt with the layout so a profile
	// flip re-evaluates the close-× policy. The title/chip are refreshed at
	// draw time (cardSpec) because the pick changes them without a relayout.
	c.card = NewMenuCard(func() { c.Close() })
	c.card.Layout(lay.Panel, c.cardSpec())
	c.closeBtn = c.card.CloseBtn()
}

// cardSpec is the picker's frame content: the current swatch's canonical
// name + chip when the applied color is on the palette, else "Color".
func (c *ColorWheelComponent) cardSpec() MenuCardSpec {
	title := i18n.T(i18n.KeyMenuColor)
	var chip color.Color
	if name, col, ok := c.selectedSwatchName(); ok {
		title, chip = name, col
	}
	return MenuCardSpec{Title: title, Swatch: chip}
}

// pickColorAt returns the palette color for the point (x, y). A direct cell
// hit wins; otherwise, a point inside the grid body maps to the nearest cell
// by stride so inter-cell gap pixels still resolve to a swatch (no dead
// gutters). Returns nil only when the point is outside the grid body entirely.
func (c *ColorWheelComponent) pickColorAt(x, y int) color.Color {
	pt := image.Pt(x, y)
	for _, cell := range c.cells {
		if pt.In(cell.rect) {
			return cell.col
		}
	}
	// Gap-tolerant fallback: map any point inside the cell region to a cell.
	// Hit-testing uses cellsRect (NOT gridRect) so the left label gutter is not
	// a hit area and the stride math agrees with rebuildWheel's cell layout.
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

	// Pressed on a swatch cell.
	//
	// SELECTING APPLIES, RE-SELECTING CONFIRMS: a different colour is applied
	// immediately and the picker stays OPEN, so several can be tried without
	// reopening it; pressing the colour already applied is the confirm gesture
	// and closes (deferred to the release so the press cannot fall through).
	// The note-matrix picker follows the same rule.
	if pressed {
		if col := c.pickColorAt(x, y); col != nil {
			// Cells store color.RGBA; the assertion is total in practice and
			// falls back to "not equal" (a plain apply) if it ever is not.
			rgba, _ := col.(color.RGBA)
			if colorsEqualRGB(c.currentColor(), rgba) {
				c.state.picked = true // confirm → close on release
				return InputCaptured
			}
			c.state.pressing = true
			// Shared with the keyboard path (applySwatch), so pointer and
			// Enter cannot diverge.
			c.applySwatch(rgba)
			return InputCaptured
		}
	}

	// Release after an apply-press: the picker stays open, ready for the next
	// audition or the confirm re-press.
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

// stripShadeSuffix removes a trailing "-NNN" shade suffix from a swatch name,
// yielding the hue-family key: "hot-pink-300" → "hot-pink".
func stripShadeSuffix(name string) string {
	if i := strings.LastIndex(name, "-"); i >= 0 {
		if suffix := name[i+1:]; suffix != "" && isAllDigits(suffix) {
			return name[:i]
		}
	}
	return name
}

// familyLabelFromSwatchName derives a human-readable hue-family label from a
// ramp swatch name by stripping the trailing "-NNN" shade suffix and
// Title-Casing the hyphen-separated words: "hot-pink-300" → "Hot Pink".
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

// colorFamilyShortLabels maps each hue-family key (the swatch name minus its
// "-NNN" shade suffix) to a short, plain color name for the picker's left
// gutter. Short names keep the gutter narrow — so it never wastes horizontal
// space that belongs to the swatches — while staying instantly recognizable
// (and kid-readable). Families absent here fall back to the Title-Cased derived
// label (familyLabelFromSwatchName). Keep every value <= 6 runes; the fit is
// guarded by TestColorPickerFamilyLabels{FitGutter,AreShort}.
var colorFamilyShortLabels = map[string]string{
	"sunset-gold":   "Gold",
	"tangerine":     "Orange",
	"hot-pink":      "Pink",
	"orchid":        "Purple",
	"electric-blue": "Blue",
	"cyan":          "Cyan",
	"mint":          "Mint",
}

// colorFamilyDisplayLabel returns the short label drawn in the left gutter for a
// hue family, falling back to the Title-Cased derived name for any family not in
// colorFamilyShortLabels.
func colorFamilyDisplayLabel(swatchName string) string {
	if short, ok := colorFamilyShortLabels[stripShadeSuffix(swatchName)]; ok {
		return short
	}
	return familyLabelFromSwatchName(swatchName)
}

// canonicalSwatchNames maps each palette swatch (family + shade) to a canonical
// human color name — the closest well-known name for that hue+shade. Surfaced in
// the header readout so the user sees "Rose", not "#FF2D9E". Keyed by the full
// swatch name; keep in sync with the palette (genInstrumentSwatches). Guard:
// TestCanonicalColorNameForEverySwatch.
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

// canonicalColorName returns a canonical, human color name for a specific swatch
// (family + shade), e.g. "hot-pink-300" → "Rose". This is the name surfaced in
// the header readout when that color is applied — a plain human name, never a
// hex code. Swatches absent from the table fall back to the short family label.
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

// selectedSwatchName returns the canonical name and color of the palette swatch
// matching the row's currently-applied color, plus ok=false when the current
// color is off-palette (or unset). Drives the header readout.
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

	// Frame (panel + title band + close ×) through the shared MenuCard. When
	// the row's current color is on the palette, the title reads it back as a
	// canonical name ("Rose") with a chip of that color — never a hex code.
	if c.card != nil {
		c.card.Layout(r, c.cardSpec())
		c.card.Draw(dst)
	}

	// Shade-direction hint above the columns: "Light" at the light end (left),
	// "Deep" at the deep end (right). Skipped when the band was suppressed.
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

	// Swatches are small controls — control role, bounded by each rect's own
	// size. The selection ring insets outward/inward, so it is bounded
	// separately rather than reusing the swatch's radius.
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
		// Keyboard cursor: a bright inset outline. A swatch IS its colour, so
		// this cannot be a tint the way the note matrix highlights a keycap —
		// an outline is the only mark that does not misrepresent the swatch.
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

// ShadeHintRect returns the shade-direction caption band rect (empty when the
// panel is too short to host it). Test accessor.
func (c *ColorWheelComponent) ShadeHintRect() image.Rectangle { return c.hintRect }

// LabelGutterWidth returns the pixel width of the left family-name gutter (the
// span between the grid body's left edge and the first swatch column). Test
// accessor mirroring the geometry Draw uses to place labels.
func (c *ColorWheelComponent) LabelGutterWidth() int {
	return c.cellsRect.Min.X - c.gridRect.Min.X
}

// FamilyDisplayLabels returns the short family label drawn in the left gutter
// for each visible ramp row (top→bottom). Test accessor.
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
