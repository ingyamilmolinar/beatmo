package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
)

// Splitter holds the Y-coordinate of the horizontal divider.
type Splitter struct {
	Y        int     // divider position in screen px
	ratio    float64 // Y / screen height
	dragging bool    // true while the user is moving it
}

func NewSplitter(totalH int) *Splitter {
	return &Splitter{Y: totalH / 2, ratio: 0.5}
}

// Update adjusts the divider position based on the current cursor position
// and window height. The caller must provide the total window height so the
// splitter can preserve its relative location when the window resizes.
func (s *Splitter) Update(totalH int) {
	const grab = 5 // px hit-box around the divider

	_, y := cursorPosition()

	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		// start drag if cursor is near the divider
		if !s.dragging && utils.Abs(y-s.Y) <= grab {
			s.dragging = true
		}
		if s.dragging {
			s.Y = y

			// clamp to sensible range
			if s.Y < 120 {
				s.Y = 120
			}
			if s.Y > totalH-120 {
				s.Y = totalH - 120
			}
			if totalH > 0 {
				s.ratio = float64(s.Y) / float64(totalH)
			}
		}
	} else {
		s.dragging = false
	}
}
