package ui

import (
	"image"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

const (
	debugCharW = 7  // width of a character drawn by DebugPrintAt
	debugCharH = 13 // height of a character drawn by DebugPrintAt
)

// insetRect returns r shrunk by pad pixels on all sides.
func insetRect(r image.Rectangle, pad int) image.Rectangle {
	return image.Rect(r.Min.X+pad, r.Min.Y+pad, r.Max.X-pad, r.Max.Y-pad)
}

// ButtonVisual is implemented by styles capable of drawing a button.
type ButtonVisual interface {
	Draw(dst *ebiten.Image, r image.Rectangle, pressed bool)
}

// Button is a basic clickable component with a rectangular bounds and text label.
type Button struct {
	r       image.Rectangle
	Text    string
	Style   ButtonVisual
	OnClick func()
	pressed bool
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
		b.Style.Draw(dst, b.r, b.pressed)
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
	if !pressed {
		b.pressed = false
		return false
	}
	if image.Pt(mx, my).In(b.r) {
		if !b.pressed && b.OnClick != nil {
			b.OnClick()
		}
		b.pressed = true
		return true
	}
	return false
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
