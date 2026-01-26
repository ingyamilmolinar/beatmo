package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
)

// Splitter holds the Y-coordinate of the horizontal divider.
type Splitter struct {
	Y        int     // divider position in screen px
	ratio    float64 // Y / screen height
	dragging bool    // true while the user is moving it
	userSet  bool    // set to true after the user drags at least once
	winW     int     // track window width for bounds
	totalH   int     // track total height for clamping in HandleInput
}

func NewSplitter(totalH int) *Splitter {
	return &Splitter{Y: totalH / 2, ratio: 0.5}
}

// UpdateResize adjusts the divider position based on window resize.
// The caller must provide the total window height and width so the splitter
// can preserve its relative location when the window resizes.
// This should be called each frame; input handling is separate via HandleInput.
func (s *Splitter) UpdateResize(totalH, winW int) {
	s.winW = winW
	s.totalH = totalH
	if totalH > 0 && !s.dragging {
		// Apply saved ratio when not dragging
		if s.userSet {
			s.Y = int(s.ratio * float64(totalH))
		}
		// Clamp to sensible range
		if s.Y < 120 {
			s.Y = 120
		}
		if s.Y > totalH-120 {
			s.Y = totalH - 120
		}
	}
}

// Update is a legacy method that combines UpdateResize and direct input polling.
// Prefer using UpdateResize + InputDispatcher.HandleInput for new code.
func (s *Splitter) Update(totalH, winW int) {
	s.UpdateResize(totalH, winW)
	// Legacy direct input handling (bypassed when using InputDispatcher)
	const grab = 5 // px hit-box around the divider
	if suppressClicksUntilRelease {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			suppressClicksUntilRelease = false
		}
		return
	}
	_, y := cursorPosition()

	// Only allow splitter interaction from the grid pane area (above splitter)
	// or within the grab zone. Block if click originates from drum pane.
	if !s.dragging && y > s.Y+grab {
		return // Cursor is in drum pane - ignore
	}

	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		// start drag if cursor is near the divider
		if !s.dragging && utils.Abs(y-s.Y) <= grab {
			s.dragging = true
			s.userSet = true
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

// --- InputHandler interface implementation ---

// InputBounds returns the interactive area of the splitter (the grab zone).
func (s *Splitter) InputBounds() image.Rectangle {
	const grab = 5
	return image.Rect(0, s.Y-grab, s.winW, s.Y+grab)
}

// ZIndex returns the splitter's z-order for input dispatch.
// Higher than DrumView (100) to ensure splitter gets priority at the boundary.
func (s *Splitter) ZIndex() int { return 150 }

// HandleInput processes mouse input for the splitter.
// Returns InputCaptured during drag, InputConsumed on release, InputIgnored otherwise.
func (s *Splitter) HandleInput(x, y int, pressed bool) InputResult {
	const grab = 5

	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		return InputIgnored
	}

	// Only allow splitter interaction from the grid pane area (above splitter)
	// or within the grab zone. Block if click originates from drum pane.
	if !s.dragging && y > s.Y+grab {
		return InputIgnored
	}

	if pressed {
		wasDragging := s.dragging
		if !s.dragging && utils.Abs(y-s.Y) <= grab {
			s.dragging = true
			s.userSet = true
		}
		if s.dragging {
			// Only update Y position if we were already dragging (not on initial grab)
			// to avoid side effects when just detecting grab initiation
			if wasDragging {
				s.Y = y
				// Clamp to sensible range
				if s.Y < 120 {
					s.Y = 120
				}
				if s.totalH > 0 && s.Y > s.totalH-120 {
					s.Y = s.totalH - 120
				}
				if s.totalH > 0 {
					s.ratio = float64(s.Y) / float64(s.totalH)
				}
			}
			return InputCaptured
		}
	} else if s.dragging {
		s.dragging = false
		return InputConsumed
	}

	return InputIgnored
}

// Capturing returns true while the splitter is being dragged.
func (s *Splitter) Capturing() bool { return s.dragging }

// HandleWheel does not process wheel events for the splitter.
func (s *Splitter) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}
