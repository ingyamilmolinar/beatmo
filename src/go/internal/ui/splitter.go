package ui

import "github.com/hajimehoshi/ebiten/v2"

// Splitter holds the Y-coordinate of the horizontal divider.
type Splitter struct {
    Y        int     // divider position in screen px
    ratio    float64 // Y / screen height
    dragging bool    // true while the user is moving it
}

func NewSplitter(totalH int) *Splitter {
    return &Splitter{Y: totalH / 2, ratio: 0.5}
}

func (s *Splitter) Update() {
	const grab = 5 // px hit-box around the divider

	_, y := cursorPosition()

        if isMouseButtonPressed(ebiten.MouseButtonLeft) {
                // start drag if cursor is near the divider
                if !s.dragging && abs(y-s.Y) <= grab {
                        s.dragging = true
                }
                if s.dragging {
                        s.Y = y

                        // clamp to sensible range
                        _, screenH := screenSize()
                        if s.Y < 120 {
                                s.Y = 120
                        }
                        if s.Y > screenH-120 {
                                s.Y = screenH - 120
                        }
                        if screenH > 0 {
                                s.ratio = float64(s.Y) / float64(screenH)
                        }
                }
        } else {
                s.dragging = false
        }
}
