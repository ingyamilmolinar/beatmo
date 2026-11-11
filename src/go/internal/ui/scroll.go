package ui

import (
	"image"
	"math"
)

// VerticalScroller encapsulates common scroll logic (wheel + drag) for vertical
// lists with a fixed number of visible items inside a viewport.
type VerticalScroller struct {
	Total   int
	Visible int
	First   int
	View    image.Rectangle

	dragging       bool
	dragStartY     int
	dragStartFirst int
	dragOffsetY    int
}

// Clamp keeps First within valid bounds based on Total and Visible.
func (s *VerticalScroller) Clamp() {
	if s.Visible < 1 {
		s.Visible = 1
	}
	if s.Visible > s.Total && s.Total >= 0 {
		s.Visible = s.Total
	}
	maxFirst := s.Total - s.Visible
	if maxFirst < 0 {
		maxFirst = 0
	}
	if s.First < 0 {
		s.First = 0
	}
	if s.First > maxFirst {
		s.First = maxFirst
	}
}

// HasScroll reports whether scrolling is necessary.
func (s *VerticalScroller) HasScroll() bool {
	return s.Total > s.Visible
}

// BarRect returns the scrollbar track rectangle of the specified width.
func (s *VerticalScroller) BarRect(width int) image.Rectangle {
	return image.Rect(s.View.Max.X-width, s.View.Min.Y, s.View.Max.X, s.View.Max.Y)
}

// ThumbRect returns the scrollbar thumb rectangle. minH enforces a minimum
// thumb height to keep it usable in small viewports.
func (s *VerticalScroller) ThumbRect(width, minH int) image.Rectangle {
	if !s.HasScroll() || s.View.Empty() {
		return image.Rect(0, 0, 0, 0)
	}
	bar := s.BarRect(width)
	track := bar.Dy()
	if track <= 0 || s.Total <= 0 || s.Visible <= 0 {
		return bar
	}
	thumbH := track * s.Visible / s.Total
	if thumbH < minH {
		thumbH = minH
	}
	if thumbH > track {
		thumbH = track
	}
	maxFirst := s.Total - s.Visible
	pos := 0
	if maxFirst > 0 && track > thumbH {
		pos = (track - thumbH) * s.First / maxFirst
	}
	y := bar.Min.Y + pos
	return image.Rect(bar.Min.X, y, bar.Max.X, y+thumbH)
}

// ScrollBy adjusts First by delta (positive scrolls down). It clamps and
// reports whether the value changed.
func (s *VerticalScroller) ScrollBy(delta int) bool {
	prev := s.First
	s.First += delta
	s.Clamp()
	return s.First != prev
}

// StartDrag begins dragging if the provided point lies within the thumb.
func (s *VerticalScroller) StartDrag(y int, width, minH int) bool {
	thumb := s.ThumbRect(width, minH)
	if !image.Pt(thumb.Min.X, y).In(thumb) && !image.Pt(thumb.Max.X-1, y).In(thumb) {
		return false
	}
	s.dragging = true
	s.dragStartY = y
	s.dragStartFirst = s.First
	s.dragOffsetY = y - thumb.Min.Y
	return true
}

// DragTo updates First based on drag distance. Returns true if First changed.
func (s *VerticalScroller) DragTo(y int, width, minH int) bool {
	if !s.dragging {
		return false
	}
	bar := s.BarRect(width)
	thumb := s.ThumbRect(width, minH)
	track := bar.Dy() - thumb.Dy()
	if track < 1 {
		track = 1
	}
	maxFirst := s.Total - s.Visible
	if maxFirst < 0 {
		maxFirst = 0
	}
	thumbTop := y - s.dragOffsetY
	if thumbTop < bar.Min.Y {
		thumbTop = bar.Min.Y
	}
	maxThumbTop := bar.Max.Y - thumb.Dy()
	if thumbTop > maxThumbTop {
		thumbTop = maxThumbTop
	}
	pos := thumbTop - bar.Min.Y
	newFirst := int(math.Round(float64(pos) * float64(maxFirst) / float64(track)))
	if newFirst < 0 {
		newFirst = 0
	}
	if newFirst > maxFirst {
		newFirst = maxFirst
	}
	changed := newFirst != s.First
	s.First = newFirst
	return changed
}

// EndDrag releases the drag state.
func (s *VerticalScroller) EndDrag() { s.dragging = false }
