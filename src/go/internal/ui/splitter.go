package ui

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

// Splitter holds the divider position between grid and drum panes.
// In horizontal mode (default), the divider is a horizontal line at Y.
// In side-by-side mode (landscape mobile), it is a vertical line at X.
type Splitter struct {
	Y           int     // divider position in screen px (horizontal mode)
	X           int     // divider position in screen px (side-by-side mode)
	ratio       float64 // position / relevant dimension
	dragging    bool    // true while the user is moving it
	userSet     bool    // set to true after the user drags at least once
	winW        int     // track window width for bounds
	totalH      int     // track total height for clamping in HandleInput
	horizontal  bool    // true = top/bottom (default), false = left/right
	guardFrames int     // >0 prevents new drag initiation (cooldown after drum capture)

	// handle owns the shared animated line+pill draw (glow + grow on hover).
	// Shared with the EQ-boundary divider via SplitterHandle.
	handle SplitterHandle
}

// Horizontal returns true when layout is stacked (grid top, drum bottom).
func (s *Splitter) Horizontal() bool { return s.horizontal }

// GridRect returns the grid pane rectangle.
func (s *Splitter) GridRect(winW, winH int) image.Rectangle {
	if !s.horizontal {
		return image.Rect(0, 0, s.X, winH)
	}
	return image.Rect(0, 0, winW, s.Y)
}

// DrumRect returns the drum pane rectangle.
func (s *Splitter) DrumRect(winW, winH int) image.Rectangle {
	if !s.horizontal {
		return image.Rect(s.X, 0, winW, winH)
	}
	return image.Rect(0, s.Y, winW, winH)
}

// InGridPane reports whether (x,y) is in the grid pane.
func (s *Splitter) InGridPane(x, y int) bool {
	if !s.horizontal {
		return x < s.X
	}
	return y < s.Y
}

// GridW returns grid pane width.
func (s *Splitter) GridW(winW int) int {
	if !s.horizontal {
		return s.X
	}
	return winW
}

// GridH returns grid pane height.
func (s *Splitter) GridH(winH int) int {
	if !s.horizontal {
		return winH
	}
	return s.Y
}

func NewSplitter(totalH int) *Splitter {
	return &Splitter{Y: totalH / 2, ratio: 0.5, horizontal: true}
}

// UpdateResize adjusts the divider position based on window resize.
// The caller must provide the total window height and width so the splitter
// can preserve its relative location when the window resizes.
// This should be called each frame; input handling is separate via HandleInput.
func (s *Splitter) UpdateResize(totalH, winW int) {
	s.winW = winW
	s.totalH = totalH
	if s.horizontal {
		// --- Stacked (top/bottom) ---
		if totalH > 0 && !s.dragging {
			if s.userSet {
				s.Y = int(math.Round(s.ratio * float64(totalH)))
			}
			minY := 120
			maxY := totalH - 120
			if f := Profile().SplitterMinFraction; f > 0 {
				if q := int(f * float64(totalH)); q > minY {
					minY = q
				}
				if q := totalH - int(f*float64(totalH)); q < maxY {
					maxY = q
				}
			}
			if s.Y < minY {
				s.Y = minY
			}
			if s.Y > maxY {
				s.Y = maxY
			}
		}
	} else {
		// --- Side-by-side (left/right) ---
		if winW > 0 && !s.dragging {
			if s.userSet {
				s.X = int(math.Round(s.ratio * float64(winW)))
			}
			minX := 120
			maxX := winW - 120
			if f := Profile().SplitterMinFraction; f > 0 {
				if q := int(f * float64(winW)); q > minX {
					minX = q
				}
				if q := winW - int(f*float64(winW)); q < maxX {
					maxX = q
				}
			}
			if s.X < minX {
				s.X = minX
			}
			if s.X > maxX {
				s.X = maxX
			}
		}
		// Keep Y at full height so stacked-mode references still work
		s.Y = totalH
	}
}

// Update is a legacy method that combines UpdateResize and direct input polling.
// Prefer using UpdateResize + InputDispatcher.HandleInput for new code.
func (s *Splitter) Update(totalH, winW int) {
	s.UpdateResize(totalH, winW)
	// Legacy direct input handling (bypassed when using InputDispatcher)
	grab := TouchGrabZone() // px hit-box around the divider
	initGrab := grab
	if t := Profile().SplitterGrabThreshold; t > 0 && grab > t {
		initGrab = t
	}
	if suppressClicksUntilRelease {
		if !isMouseButtonPressed(ebiten.MouseButtonLeft) {
			suppressClicksUntilRelease = false
		}
		return
	}
	mx, my := cursorPosition()

	if s.horizontal {
		// --- Stacked mode ---
		if !s.dragging && my > s.Y+initGrab {
			return
		}
		if isMouseButtonPressed(ebiten.MouseButtonLeft) {
			if !s.dragging {
				handleR := s.HandleRect()
				expanded := handleR.Inset(-SpaceSM)
				if image.Pt(mx, my).In(expanded) {
					s.dragging = true
					s.userSet = true
				}
			}
			if s.dragging {
				s.Y = my
				minY := 120
				maxY := totalH - 120
				if f := Profile().SplitterMinFraction; f > 0 {
					if q := int(f * float64(totalH)); q > minY {
						minY = q
					}
					if q := totalH - int(f*float64(totalH)); q < maxY {
						maxY = q
					}
				}
				if s.Y < minY {
					s.Y = minY
				}
				if s.Y > maxY {
					s.Y = maxY
				}
				if totalH > 0 {
					s.ratio = float64(s.Y) / float64(totalH)
				}
			}
		} else {
			s.dragging = false
		}
	} else {
		// --- Side-by-side mode ---
		if !s.dragging && mx > s.X+initGrab {
			return
		}
		if isMouseButtonPressed(ebiten.MouseButtonLeft) {
			if !s.dragging {
				handleR := s.HandleRect()
				expanded := handleR.Inset(-SpaceSM)
				if image.Pt(mx, my).In(expanded) {
					s.dragging = true
					s.userSet = true
				}
			}
			if s.dragging {
				s.X = mx
				minX := 120
				maxX := winW - 120
				if f := Profile().SplitterMinFraction; f > 0 {
					if q := int(f * float64(winW)); q > minX {
						minX = q
					}
					if q := winW - int(f*float64(winW)); q < maxX {
						maxX = q
					}
				}
				if s.X < minX {
					s.X = minX
				}
				if s.X > maxX {
					s.X = maxX
				}
				if winW > 0 {
					s.ratio = float64(s.X) / float64(winW)
				}
			}
		} else {
			s.dragging = false
		}
	}
}

// HandleRect returns the pill handle rect for the current splitter position.
func (s *Splitter) HandleRect() image.Rectangle {
	if s.horizontal {
		return SplitterHandleRect(s.winW/2, s.Y, true)
	}
	return SplitterHandleRect(s.X, s.totalH/2, false)
}

// --- InputHandler interface implementation ---

// InputBounds returns the interactive area of the splitter (the grab zone).
// The below-divider (or right-of-divider) extent is kept small so the splitter
// does not steal taps from drum-area controls. During active drag the
// dispatcher's capture mechanism handles events regardless of InputBounds.
func (s *Splitter) InputBounds() image.Rectangle {
	grab := TouchGrabZone()
	const belowExtent = 4
	// The grab strip must also encompass the full visible handle pill, so the
	// dispatcher routes presses that land on the drawn pill to the splitter.
	// Previously belowExtent (4 px) was smaller than the pill's lower half
	// (thick/2 + SpaceSM), so presses on the bottom of the pill never reached
	// HandleInput and fell through to whatever sat beneath it (the notification
	// bar). HandleInput still self-gates on the pill rect, so widening the
	// strip never steals presses that miss the pill.
	pill := s.HandleRect().Inset(-SpaceSM)
	if !s.horizontal {
		left := s.X - grab
		right := s.X + belowExtent
		if pill.Min.X < left {
			left = pill.Min.X
		}
		if pill.Max.X > right {
			right = pill.Max.X
		}
		return image.Rect(left, 0, right, s.totalH)
	}
	top := s.Y - grab
	bottom := s.Y + belowExtent
	if pill.Min.Y < top {
		top = pill.Min.Y
	}
	if pill.Max.Y > bottom {
		bottom = pill.Max.Y
	}
	return image.Rect(0, top, s.winW, bottom)
}

// ZIndex returns the splitter's z-order for input dispatch.
// Higher than DrumView (100) to ensure splitter gets priority at the boundary.
func (s *Splitter) ZIndex() int { return 150 }

// HandleInput processes mouse input for the splitter.
// Returns InputCaptured during drag, InputConsumed on release, InputIgnored otherwise.
func (s *Splitter) HandleInput(x, y int, pressed bool) InputResult {
	// Guard: block new drag initiation for a few frames after the drum view
	// was capturing. This prevents touch flicker from handing input to the
	// splitter when the user is scrolling in the drum rows.
	if s.guardFrames > 0 && !s.dragging {
		return InputIgnored
	}

	grab := TouchGrabZone()

	// On small screens, use a tighter initiation threshold so the grab zone
	// doesn't extend far into the drum area and steal button taps.
	initGrab := grab
	if t := Profile().SplitterGrabThreshold; t > 0 && grab > t {
		initGrab = t
	}

	if suppressClicksUntilRelease {
		if !pressed {
			suppressClicksUntilRelease = false
		}
		return InputIgnored
	}

	if s.horizontal {
		// --- Stacked mode: drag Y ---
		// Reject presses well below the divider UNLESS they land on the visible
		// handle pill (which extends below s.Y). Without the pill exception, a
		// press on the lower half of the drawn pill fell through to the
		// notification bar beneath it.
		if !s.dragging && y > s.Y+initGrab && !image.Pt(x, y).In(s.HandleRect().Inset(-SpaceSM)) {
			return InputIgnored
		}
		if pressed {
			wasDragging := s.dragging
			if !s.dragging {
				handleR := s.HandleRect()
				expanded := handleR.Inset(-SpaceSM)
				if image.Pt(x, y).In(expanded) {
					s.dragging = true
					s.userSet = true
				}
			}
			if s.dragging {
				if wasDragging {
					s.Y = y
					minY := 120
					maxY := s.totalH - 120
					if f := Profile().SplitterMinFraction; f > 0 {
						if q := int(f * float64(s.totalH)); q > minY {
							minY = q
						}
						if q := s.totalH - int(f*float64(s.totalH)); q < maxY {
							maxY = q
						}
					}
					if s.Y < minY {
						s.Y = minY
					}
					if s.totalH > 0 && s.Y > maxY {
						s.Y = maxY
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
	} else {
		// --- Side-by-side mode: drag X ---
		// Mirror the stacked-mode pill exception (see above).
		if !s.dragging && x > s.X+initGrab && !image.Pt(x, y).In(s.HandleRect().Inset(-SpaceSM)) {
			return InputIgnored
		}
		if pressed {
			wasDragging := s.dragging
			if !s.dragging {
				handleR := s.HandleRect()
				expanded := handleR.Inset(-SpaceSM)
				if image.Pt(x, y).In(expanded) {
					s.dragging = true
					s.userSet = true
				}
			}
			if s.dragging {
				if wasDragging {
					s.X = x
					minX := 120
					maxX := s.winW - 120
					if f := Profile().SplitterMinFraction; f > 0 {
						if q := int(f * float64(s.winW)); q > minX {
							minX = q
						}
						if q := s.winW - int(f*float64(s.winW)); q < maxX {
							maxX = q
						}
					}
					if s.X < minX {
						s.X = minX
					}
					if s.winW > 0 && s.X > maxX {
						s.X = maxX
					}
					if s.winW > 0 {
						s.ratio = float64(s.X) / float64(s.winW)
					}
				}
				return InputCaptured
			}
		} else if s.dragging {
			s.dragging = false
			return InputConsumed
		}
	}

	return InputIgnored
}

// Capturing returns true while the splitter is being dragged.
func (s *Splitter) Capturing() bool { return s.dragging }

// HandleWheel does not process wheel events for the splitter.
func (s *Splitter) HandleWheel(x, y, steps int) InputResult {
	return InputIgnored
}
