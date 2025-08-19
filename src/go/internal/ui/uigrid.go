package ui

import (
	"image"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
        // Ebiten's debug font uses a 6x13 glyph. Using 7 previously caused text
        // input cursors to drift ahead of the character being edited.
        debugCharW = 6  // width of a character drawn by DebugPrintAt
        debugCharH = 13 // height of a character drawn by DebugPrintAt
)

// insetRect returns r shrunk by pad pixels on all sides.
func insetRect(r image.Rectangle, pad int) image.Rectangle {
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
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
}

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
	tr := b.textRect()
	ebitenutil.DebugPrintAt(dst, b.Text, tr.Min.X, tr.Min.Y)
}

// textRect returns the rectangle occupied by the button's text when drawn.
func (b *Button) textRect() image.Rectangle {
	w := debugCharW * utf8.RuneCountInString(b.Text)
	h := debugCharH
	x := b.r.Min.X + (b.r.Dx()-w)/2
	y := b.r.Min.Y + (b.r.Dy()-h)/2
	return image.Rect(x, y, x+w, y+h)
}

// Handle processes a mouse click at (mx,my). It triggers OnClick when pressed inside.
func (b *Button) Handle(mx, my int, pressed bool) bool {
	inside := image.Pt(mx, my).In(b.r)
	b.hovered = inside
	if pressed && inside {
		b.held++
		if b.held == 1 {
			if b.OnClick != nil {
				b.OnClick()
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
	g.colPos = make([]int, len(g.colWeights)+1)
	x := g.bounds.Min.X
	for i, w := range g.colWeights {
		width := int(float64(g.bounds.Dx()) * (w / totalW))
		g.colPos[i] = x
		x += width
	}
	g.colPos[len(g.colWeights)] = g.bounds.Max.X

	g.rowPos = make([]int, len(g.rowWeights)+1)
	y := g.bounds.Min.Y
	for i, h := range g.rowWeights {
		height := int(float64(g.bounds.Dy()) * (h / totalH))
		g.rowPos[i] = y
		y += height
	}
	g.rowPos[len(g.rowWeights)] = g.bounds.Max.Y
}

// Cell returns the rectangle for the specified cell.
func (g *GridLayout) Cell(col, row int) image.Rectangle {
	return image.Rect(g.colPos[col], g.rowPos[row], g.colPos[col+1], g.rowPos[row+1])
}
