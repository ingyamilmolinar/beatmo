package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

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
	ebitenutil.DebugPrintAt(dst, b.Text, b.r.Min.X+5, b.r.Min.Y+14)
}

// Handle processes a mouse click at (mx,my). It triggers OnClick when pressed inside.
func (b *Button) Handle(mx, my int, pressed bool) bool {
	if !pressed {
		b.pressed = false
		return false
	}
	if image.Pt(mx, my).In(b.r) {
		b.pressed = true
		if b.OnClick != nil {
			b.OnClick()
		}
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
