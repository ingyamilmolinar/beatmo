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
	// OnColorPick is called when a swatch is picked.
	OnColorPick func(c color.Color)
	// OnClose is called when the picker should close.
	OnClose func()
}

// ColorWheelState contains the internal state for the color picker component.
type ColorWheelState struct {
	open   bool
	hold   bool // Capture flag (set when opened, released on mouse-up)
	picked bool // Set when a color is picked; defers Close() until mouse release
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
	props    ColorWheelProps
	state    ColorWheelState
	closeBtn *Button

	cells     []swatchCell
	gridRect  image.Rectangle // body region (label gutter + cells)
	cellsRect image.Rectangle // cell region (gridRect minus the left label gutter)
	titleRect image.Rectangle // header region (title + close button)
	cellW     int             // cell width (for gap-tolerant hit mapping)
	cellH     int             // cell height
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
	colorRampLabelW    = 56 // left gutter width for family-name labels
)

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
	c.rebuildWheel()
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
	if c.props.OnClose != nil {
		c.props.OnClose()
	}
}

// IsOpen returns whether the color picker is currently open.
func (c *ColorWheelComponent) IsOpen() bool {
	return c.state.open
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
		c.closeBtn = nil
		return
	}

	bounds := c.props.Bounds

	// Cell edge: floor at the touch minimum on mobile (so each cell is a
	// 44 px tap target) and at colorSwatchCellMin on desktop. Grow the cell
	// to use available width when the container is roomy.
	cellMin := colorSwatchCellMin
	if tm := TouchMinTarget(); Profile().IsMobile() && tm > cellMin {
		cellMin = tm
	}

	// Available width for the grid (mobile favors the full container width).
	availW := bounds.Dx()
	if !Profile().UseBottomSheet {
		// Desktop: don't span the whole rack — pick a sensible panel width.
		if availW > 360 {
			availW = 360
		}
	}
	// Solve cell edge from the available width. The left label gutter is fixed,
	// so cells only consume the remainder:
	// availW = 2*pad + labelW + cols*cell + (cols-1)*gap.
	cellFromW := (availW - 2*colorSwatchPad - colorRampLabelW - (colorSwatchCols-1)*colorSwatchGap) / colorSwatchCols
	cell := max(cellMin, cellFromW)

	panelW := 2*colorSwatchPad + colorRampLabelW + colorSwatchCols*cell + (colorSwatchCols-1)*colorSwatchGap
	gridH := colorSwatchRows*cell + (colorSwatchRows-1)*colorSwatchGap
	panelH := colorSwatchHeader + colorSwatchPad + gridH + colorSwatchPad

	// Clamp the panel size to the container so it always fits fully inside
	// Bounds (the rack), which keeps it inside the DrumView. The grid-cell
	// dimensions are re-derived from the FINAL gridRect below, so a shrunk
	// panel produces correspondingly smaller cells that still tile the body —
	// never the collapsed-into-the-header failure the naive clamp caused.
	if panelW > bounds.Dx() {
		panelW = bounds.Dx()
	}
	if panelH > bounds.Dy() {
		panelH = bounds.Dy()
	}

	// Position the panel near the anchor, preferring whichever vertical
	// direction has more room (mirrors the instrument menu), then clamp inside
	// Bounds.
	rect := image.Rect(0, 0, panelW, panelH)
	if Profile().UseBottomSheet {
		// Mobile: center horizontally, anchor toward the bottom of the container.
		cx := bounds.Min.X + (bounds.Dx()-panelW)/2
		rect = rect.Add(image.Pt(cx, bounds.Max.Y-panelH))
	} else {
		spaceDown := bounds.Max.Y - c.props.AnchorRect.Max.Y
		spaceUp := c.props.AnchorRect.Min.Y - bounds.Min.Y
		originY := c.props.AnchorRect.Max.Y
		if spaceDown < panelH && spaceUp > spaceDown {
			originY = c.props.AnchorRect.Min.Y - panelH // open upward
		}
		rect = rect.Add(image.Pt(c.props.AnchorRect.Min.X, originY))
	}

	// Clamp fully inside bounds.
	if rect.Min.X < bounds.Min.X {
		rect = rect.Add(image.Pt(bounds.Min.X-rect.Min.X, 0))
	}
	if rect.Min.Y < bounds.Min.Y {
		rect = rect.Add(image.Pt(0, bounds.Min.Y-rect.Min.Y))
	}
	if rect.Max.X > bounds.Max.X {
		rect = rect.Add(image.Pt(bounds.Max.X-rect.Max.X, 0))
	}
	if rect.Max.Y > bounds.Max.Y {
		rect = rect.Add(image.Pt(0, bounds.Max.Y-rect.Max.Y))
	}

	c.SetBounds(rect)

	// Header + padding adapt down when the panel was clamped short, so the
	// grid body always keeps a usable height (a tiny rack must still show
	// clickable cells filling most of the panel rather than collapsing into
	// the header band). Header is capped at a third of the panel and padding
	// shrinks to fit so the grid spans the lower ~two-thirds — keeping the
	// panel's vertical center inside a swatch cell at any clamped size.
	header := colorSwatchHeader
	if maxHeader := rect.Dy() / 3; header > maxHeader {
		header = maxHeader
	}
	pad := colorSwatchPad
	// Cap padding so header+2*pad never eats more than half the panel; this
	// guarantees the grid body spans the lower half, so the panel's vertical
	// center always falls inside a swatch cell regardless of clamping.
	if maxPad := (rect.Dy()/2 - header) / 2; maxPad >= 0 && pad > maxPad {
		pad = maxPad
	}
	if pad < 0 {
		pad = 0
	}

	// Header band (title left, close button right).
	c.titleRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+header)
	// Grid body below the header (includes the left label gutter).
	c.gridRect = image.Rect(
		rect.Min.X+pad,
		rect.Min.Y+header+pad,
		rect.Max.X-pad,
		rect.Max.Y-pad,
	)
	// Cell region = grid body minus the left label gutter. Cells are laid out
	// and hit-tested over this rect (NOT gridRect) so the label gutter is never
	// a hit area. rebuildWheel and pickColorAt MUST agree on cellsRect.
	labelW := colorRampLabelW
	if maxLabel := c.gridRect.Dx() / 2; labelW > maxLabel {
		labelW = maxLabel
	}
	if labelW < 0 {
		labelW = 0
	}
	c.cellsRect = image.Rect(
		c.gridRect.Min.X+labelW,
		c.gridRect.Min.Y,
		c.gridRect.Max.X,
		c.gridRect.Max.Y,
	)

	// Final cell dimensions derived from the laid-out cell region so cells
	// always fit inside cellsRect (handles any residual clamping).
	cw := (c.cellsRect.Dx() - (colorSwatchCols-1)*colorSwatchGap) / colorSwatchCols
	ch := (c.cellsRect.Dy() - (colorSwatchRows-1)*colorSwatchGap) / colorSwatchRows
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	c.cellW, c.cellH = cw, ch

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

	// Close button inside the header (top-right) — sized via the shared
	// closeButtonRect helper so it is consistent with every other pop-up.
	cr := closeButtonRect(c.titleRect, SpaceXS)
	c.closeBtn = NewButton("", PopupButtonStyle, func() { c.Close() })
	c.closeBtn.Icon = "close"
	c.closeBtn.IconColor = closeIconColor()
	c.closeBtn.SetRect(cr)
	c.closeBtn.ConsumeOnPress = true
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

	// Close button (highest z-order).
	if c.closeBtn != nil && c.closeBtn.HandleInputResult(x, y, pressed) != InputIgnored {
		return InputConsumed
	}

	pt := image.Pt(x, y)

	// Pressed on a swatch cell → pick that color (deferred close on release).
	if pressed {
		if col := c.pickColorAt(x, y); col != nil {
			if c.props.OnColorPick != nil {
				c.props.OnColorPick(col)
			}
			c.state.picked = true
			return InputCaptured
		}
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

// familyLabelFromSwatchName derives a human-readable hue-family label from a
// ramp swatch name by stripping the trailing "-NNN" shade suffix and
// Title-Casing the hyphen-separated words: "hot-pink-300" → "Hot Pink".
func familyLabelFromSwatchName(name string) string {
	// Strip the trailing shade suffix ("-NNN") if the last segment is numeric.
	if i := strings.LastIndex(name, "-"); i >= 0 {
		if suffix := name[i+1:]; suffix != "" && isAllDigits(suffix) {
			name = name[:i]
		}
	}
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

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

	// Panel chrome + unified menu title (shared drawMenuTitle, no swatch).
	drawPanel(dst, r)
	drawMenuHeaderBand(dst, c.titleRect)
	titleRect := image.Rect(c.titleRect.Min.X+colorSwatchPad, c.titleRect.Min.Y, c.titleRect.Max.X, c.titleRect.Max.Y)
	drawMenuTitle(dst, titleRect, i18n.T(i18n.KeyMenuColor), nil)

	// Family-name labels in the left gutter, vertically centered on each row.
	sw := c.swatches()
	lh := StyledTextHeight(RoleCaption)
	for row := 0; row < colorSwatchRows; row++ {
		first := row * colorSwatchCols
		if first >= len(c.cells) || first >= len(sw) {
			break
		}
		label := familyLabelFromSwatchName(sw[first].Name)
		cellRect := c.cells[first].rect
		ly := cellRect.Min.Y + (cellRect.Dy()-lh)/2
		DrawTextStyled(dst, label, c.gridRect.Min.X, ly, RoleCaption, colTextSecondary)
	}

	radius := SpaceXS
	for _, cell := range c.cells {
		drawRoundedRect(dst, cell.rect, cell.col, radius, true)
		// Subtle border so light swatches read against the panel.
		drawRoundedRect(dst, cell.rect, colPanelBorder, radius, false)
		// Selected ring on the swatch matching the row's current color.
		if colorsEqualRGB(c.props.CurrentColor, cell.col) {
			ring := cell.rect.Inset(-2)
			drawRoundedRect(dst, ring, genColorFocusRing, radius, false)
			drawRoundedRect(dst, cell.rect.Inset(2), colTextPrimary, radius, false)
		}
	}

	if c.closeBtn != nil {
		c.closeBtn.Draw(dst)
	}
}

// CloseBtn returns the close button (for testing).
func (c *ColorWheelComponent) CloseBtn() *Button { return c.closeBtn }

// Capturing returns whether the component is capturing input.
func (c *ColorWheelComponent) Capturing() bool {
	return c.state.hold || c.state.picked
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
