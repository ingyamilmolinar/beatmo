package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// SegmentedControl is a general-purpose 2-to-N segment toggle. The active
// segment renders with a colored fill (TokenAccent at AlphaMedium); the
// other segments render with the surface-2 chip color. Clicks dispatch
// to the registered callback with the segment index.
//
// Compared to a row of plain Buttons, the segmented control provides:
//   - a single visual container that reads as a unified control;
//   - automatic active-state painting (no per-button toggle wiring);
//   - guaranteed equal-width segments, anchored to TouchMinTarget.
//
// Used by the mobile view-mode switcher (Pads / Audio) — see B4 in the
// screenshot critique. Tests pin the active-segment fill color and the
// hit-rect partition.
type SegmentedControl struct {
	rect    image.Rectangle
	labels  []string
	active  int
	onClick func(int)
}

// NewSegmentedControl returns a SegmentedControl with the given labels and
// click handler. `active` is the initial selected index.
func NewSegmentedControl(labels []string, active int, onClick func(int)) *SegmentedControl {
	if active < 0 || active >= len(labels) {
		active = 0
	}
	return &SegmentedControl{
		labels:  append([]string(nil), labels...),
		active:  active,
		onClick: onClick,
	}
}

// SetRect positions the control. Width is divided evenly across segments.
func (s *SegmentedControl) SetRect(r image.Rectangle) { s.rect = r }

// Rect returns the current bounds (full container, not per-segment).
func (s *SegmentedControl) Rect() image.Rectangle { return s.rect }

// SetActive selects a segment without firing the click handler.
func (s *SegmentedControl) SetActive(i int) {
	if i >= 0 && i < len(s.labels) {
		s.active = i
	}
}

// Active returns the currently selected segment index.
func (s *SegmentedControl) Active() int { return s.active }

// SegmentRect returns the bounds of segment `i`, or the zero rect if i is
// out of range.
func (s *SegmentedControl) SegmentRect(i int) image.Rectangle {
	if i < 0 || i >= len(s.labels) || s.rect.Empty() {
		return image.Rectangle{}
	}
	w := s.rect.Dx() / len(s.labels)
	x0 := s.rect.Min.X + i*w
	x1 := x0 + w
	if i == len(s.labels)-1 {
		// Last segment absorbs any rounding remainder so the right edge
		// matches s.rect.Max.X exactly.
		x1 = s.rect.Max.X
	}
	return image.Rect(x0, s.rect.Min.Y, x1, s.rect.Max.Y)
}

// HitTest dispatches a click at (x, y). Returns true if the click landed
// inside the control. Invokes onClick with the chosen segment index;
// SetActive is called as a side effect so the caller doesn't have to.
func (s *SegmentedControl) HitTest(x, y int) bool {
	if !image.Pt(x, y).In(s.rect) {
		return false
	}
	for i := range s.labels {
		if image.Pt(x, y).In(s.SegmentRect(i)) {
			s.active = i
			if s.onClick != nil {
				s.onClick(i)
			}
			return true
		}
	}
	return false
}

// Draw renders the control. Active segment uses TokenAccent at
// AlphaMedium; inactive uses surface-2.
func (s *SegmentedControl) Draw(dst *ebiten.Image) {
	if s.rect.Empty() || len(s.labels) == 0 {
		return
	}
	// Container surface.
	drawRoundedRect(dst, s.rect, colSurface2, RadiusMD, true)
	drawRoundedRect(dst, s.rect, colBorderMedium, RadiusMD, false)
	// Per-segment fills + labels.
	for i, label := range s.labels {
		segR := s.SegmentRect(i)
		if i == s.active {
			fill := color.Color(WithAlpha(TokenAccent(), genAlphaMedium))
			drawRoundedRect(dst, insetRect(segR, 2), fill, RadiusSM, true)
		}
		// Center label: assumes single-line, fits the segment width.
		tw := TextWidth(label)
		th := TextHeight()
		tx := segR.Min.X + (segR.Dx()-tw)/2
		ty := segR.Min.Y + (segR.Dy()-th)/2
		DrawTextAt(dst, label, tx, ty)
	}
}
