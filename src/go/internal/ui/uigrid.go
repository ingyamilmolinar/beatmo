package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	// Ebiten's debug font uses 6px-wide glyphs in a 16px-tall cell.
	debugCharW = 6  // width of a character drawn by DebugPrintAt
	debugCharH = 16 // height of a character drawn by DebugPrintAt
)

// insetRect returns r shrunk by pad pixels on all sides.
func insetRect(r image.Rectangle, pad int) image.Rectangle {
	if r.Dx() < 2*pad || r.Dy() < 2*pad {
		return image.Rectangle{}
	}
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}

// clipTextToWidth trims text so that its rendered width does not exceed maxW.
// It preserves whole runes and appends "..." when truncating. Uses proper font
// metrics when available, falling back to debug font character width.
func clipTextToWidth(text string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if TextWidth(text) <= maxW {
		return text
	}
	ellipsis := "..."
	rs := []rune(text)
	for keepRunes := len(rs) - 1; keepRunes >= 0; keepRunes-- {
		candidate := string(rs[:keepRunes]) + ellipsis
		if TextWidth(candidate) <= maxW {
			return candidate
		}
	}
	return ellipsis
}

// ButtonVisual is implemented by styles capable of drawing a button.
// pressed indicates the mouse button is currently down; hovered indicates the
// cursor is over the control so styles can provide hover feedback.
type ButtonVisual interface {
	Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool)
}

// Button is a basic clickable component with a rectangular bounds and text label.
type Button struct {
	r       image.Rectangle
	Text    string
	Style   ButtonVisual
	OnClick func()
	pressed bool
	hovered bool
	Repeat  bool
	held    int
	// ConsumeOnPress: when true, after the first press triggers OnClick, suppress
	// any further button clicks across the UI until the mouse is released. Use
	// this for destructive actions to avoid cascading operations when the layout
	// changes under a held cursor.
	ConsumeOnPress bool
	// TextScale scales the button label text when > 0. A value of 1.5 renders
	// the debug font at 150%. Zero or negative uses the default 1× scale.
	TextScale float64
	// Optional icon to draw inside the button. When set, the icon is drawn
	// using simple vector primitives so it renders under the default Ebiten
	// debug font (which lacks many Unicode glyphs). Supported values:
	// "play", "pause", "stop", "pencil", "close". IconColor defaults to
	// colButtonBorder when zero.
	Icon      string
	IconColor color.Color
}

// Global guard to prevent multiple buttons from firing while a mouse press is
// held after a non-repeat click caused the UI to reflow under the cursor.
var suppressClicksUntilRelease bool

// SuppressClicksUntilMouseUp enables the global guard; the next mouse release
// clears it inside Handle.
func SuppressClicksUntilMouseUp() { suppressClicksUntilRelease = true }

// NewButton constructs a button with the given label, style, and optional click handler.
func NewButton(text string, style ButtonVisual, onClick func()) *Button {
	return &Button{Text: text, Style: style, OnClick: onClick}
}

// Rect returns the button's bounds.
func (b *Button) Rect() image.Rectangle { return b.r }

// SetRect sets the button's bounds.
func (b *Button) SetRect(r image.Rectangle) { b.r = r }

// Draw renders the button and its label.
func (b *Button) Draw(dst *ebiten.Image) {
	if b.Style != nil {
		b.Style.Draw(dst, b.r, b.pressed, b.hovered)
	}
	// Skip text rendering when an icon is set — the icon is the visual
	// representation and the debug font can't render Unicode icon glyphs
	// (they appear as small white rectangles).
	if b.Icon == "" {
		scale := b.TextScale
		if scale <= 0 {
			scale = 1.0
		}
		// Clip text to fit within the button rect.
		clipped := clipTextToWidth(b.Text, b.r.Dx()-2*buttonPad)
		spr := TextSprite(clipped)
		// Center the scaled text within the button.
		w := int(float64(TextWidth(clipped)) * scale)
		h := int(float64(TextHeight()) * scale)
		x := b.r.Min.X + (b.r.Dx()-w)/2
		y := b.r.Min.Y + (b.r.Dy()-h)/2
		var op ebiten.DrawImageOptions
		op.GeoM.Scale(scale, scale)
		op.GeoM.Translate(float64(x), float64(y))
		dst.DrawImage(spr, &op)
	}
	// Icon overlay (font-independent)
	if b.Icon != "" {
		col := b.IconColor
		if col == nil {
			col = colButtonBorder
		}
		// Proportional margin: 18% of min(w,h), min 2px.
		dim := b.r.Dx()
		if b.r.Dy() < dim {
			dim = b.r.Dy()
		}
		pad := dim * 18 / 100
		if pad < 2 {
			pad = 2
		}
		box := image.Rect(b.r.Min.X+pad, b.r.Min.Y+pad, b.r.Max.X-pad, b.r.Max.Y-pad)
		switch b.Icon {
		case "play":
			drawPlayIcon(dst, box, col)
		case "pause":
			drawPauseIcon(dst, box, col)
		case "stop":
			drawStopIcon(dst, box, col)
		case "pencil":
			drawPencilIcon(dst, box, col)
		case "save":
			drawSaveIcon(dst, box, col)
		case "close":
			drawCloseIcon(dst, box, col)
		case "overflow":
			drawOverflowIcon(dst, box, col)
		case "plus":
			drawPlusIcon(dst, box, col)
		case "minus":
			drawMinusIcon(dst, box, col)
		case "rows":
			drawRowsIcon(dst, box, col)
		case "audio":
			drawAudioIcon(dst, box, col)
		case "chevron-up":
			drawChevronUpIcon(dst, box, col)
		case "chevron-down":
			drawChevronDownIcon(dst, box, col)
		case "track":
			drawTrackIcon(dst, box, col)
		case "track-off":
			drawTrackOffIcon(dst, box, col)
		case "upload":
			drawUploadIcon(dst, box, col)
		case "import":
			drawImportIcon(dst, box, col)
		case "export":
			drawExportIcon(dst, box, col)
		}
	}
}

// textRect returns the rectangle occupied by the button's text when drawn,
// clamped to the button bounds. Matches Draw(): clips text to button width
// and applies TextScale.
//
//nolint:unused // used by test files with test build tag
func (b *Button) textRect() image.Rectangle {
	scale := b.TextScale
	if scale <= 0 {
		scale = 1.0
	}
	clipped := clipTextToWidth(b.Text, b.r.Dx()-2*buttonPad)
	w := int(float64(TextWidth(clipped)) * scale)
	h := int(float64(TextHeight()) * scale)
	x := b.r.Min.X + (b.r.Dx()-w)/2
	y := b.r.Min.Y + (b.r.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h).Intersect(b.r)
}

// Handle processes a mouse click at (mx,my). It triggers OnClick when pressed inside.
// On touch devices, the hit area is expanded to meet minimum touch target size.
func (b *Button) Handle(mx, my int, pressed bool) bool {
	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		b.pressed = false
		b.held = 0
		return false
	}
	// Expand hit area for touch devices (skip if button has zero-area rect,
	// e.g. hidden columns on mobile that should not be interactable).
	hitRect := b.r
	if minTarget := TouchMinTarget(); minTarget > 0 && !hitRect.Empty() {
		// Expand hit area if button is smaller than minimum touch target
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
	inside := image.Pt(mx, my).In(hitRect)
	b.hovered = inside
	if pressed && inside {
		b.held++
		if b.held == 1 {
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
		return true
	}
	b.pressed = false
	b.held = 0
	return false
}

func (b *Button) repeatTick() bool {
	d := b.held
	if d <= 60 {
		return false
	}
	step := d - 60
	accel := step / 30
	if accel > 5 {
		accel = 5
	}
	interval := 6 - accel
	return step%interval == 0
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
	// Compute positions as rounded fractions of the total to distribute
	// rounding error evenly (±1px per cell) instead of accumulating it
	// in the last cell.
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

// SubGrid creates a new GridLayout within the specified cell, allowing nested
// grids with different column/row structures.
func (g *GridLayout) SubGrid(col, row int, cols, rows []float64) *GridLayout {
	return NewGridLayout(g.Cell(col, row), cols, rows)
}

// LayoutGroup is a named, hierarchical GridLayout that tracks parent-child
// relationships. Moving or resizing the parent automatically repositions
// all children. This enables relocating entire UI sections (e.g., transport
// controls, row controls) by changing a single parent cell assignment.
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
