package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// One container with automatic active painting and equal-width segments anchored to TouchMinTarget, rather than a
// row of independently wired buttons.
type SegmentedControl struct {
	rect    image.Rectangle
	labels  []string
	active  int
	onClick func(int)
	// A disabled segment consumes its tap (no fall-through) but neither activates nor fires. A short slice means all enabled.
	disabled []bool
}

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

func (s *SegmentedControl) SetSegmentDisabled(i int, disabled bool) {
	if i < 0 || i >= len(s.labels) {
		return
	}
	if len(s.disabled) < len(s.labels) {
		grown := make([]bool, len(s.labels))
		copy(grown, s.disabled)
		s.disabled = grown
	}
	s.disabled[i] = disabled
}

// segmentDisabled reports whether segment i is currently disabled.
func (s *SegmentedControl) segmentDisabled(i int) bool {
	return i >= 0 && i < len(s.disabled) && s.disabled[i]
}

// The active index survives when still in range; the slice is copied.
func (s *SegmentedControl) SetLabels(labels []string) {
	s.labels = append([]string(nil), labels...)
	if s.active < 0 || s.active >= len(s.labels) {
		s.active = 0
	}
}

// Labels returns a copy of the current segment labels (for tests/inspection).
func (s *SegmentedControl) Labels() []string {
	return append([]string(nil), s.labels...)
}

func (s *SegmentedControl) SegmentRect(i int) image.Rectangle {
	if i < 0 || i >= len(s.labels) || s.rect.Empty() {
		return image.Rectangle{}
	}
	w := s.rect.Dx() / len(s.labels)
	x0 := s.rect.Min.X + i*w
	x1 := x0 + w
	if i == len(s.labels)-1 {
		// The last segment takes the rounding remainder so the right edge is exact.
		x1 = s.rect.Max.X
	}
	return image.Rect(x0, s.rect.Min.Y, x1, s.rect.Max.Y)
}

// Calls SetActive itself.
func (s *SegmentedControl) HitTest(x, y int) bool {
	if !image.Pt(x, y).In(s.rect) {
		return false
	}
	for i := range s.labels {
		if image.Pt(x, y).In(s.SegmentRect(i)) {
			if s.segmentDisabled(i) {
				// Consume the tap (don't fall through) but do nothing.
				return true
			}
			s.active = i
			if s.onClick != nil {
				s.onClick(i)
			}
			return true
		}
	}
	return false
}

// The same look as the desktop pills; each cap is inset 1 px so the tray shows through as dividers. The pre-boxed
// pill palette keeps it allocation- and literal-free.
func (s *SegmentedControl) Draw(dst *ebiten.Image) {
	if s.rect.Empty() || len(s.labels) == 0 {
		return
	}
	// A surface-1 tray so the surface-2 members read as sitting IN it; no outline.
	drawRoundedRect(dst, s.rect, colSurface1, RadiusContainer, true)

	pillRadius := RadiusControl // every control shares one radius (DESIGN.md "Shapes")
	for i, label := range s.labels {
		cell := insetRect(s.SegmentRect(i), 1) // 1px seam → hairline divider
		if cell.Empty() {
			continue
		}
		dis := s.segmentDisabled(i)
		active := i == s.active && !dis

		// The shared pill palette, pixel-consistent with the desktop tabs.
		capFill, textCol := pillCapFillInactive, pillTextInactive
		switch {
		case dis:
			capFill, textCol = pillCapFillDisabled, pillTextDisabled
		case active:
			// Bright legend on the accent tint (the solid-amber flood is gone).
			capFill, textCol = pillCapFillInactive, colTextPrimary
		}

		// Flat-tonal: body fill plus a 2 px accent bar when active.
		capR := cell
		drawFlatButtonBody(dst, capR, capFill, pillRadius)
		if active && !dis {
			drawRoundedRect(dst, capR, WithAlpha(colAccent, AlphaSubtle), pillRadius, true)
			drawFlatSelectionBar(dst, capR, colAccent, selectionEdgeBottom, 1)
		}

		// Center label on the cap face.
		tw := TextWidth(label)
		th := TextHeight()
		tx := capR.Min.X + (capR.Dx()-tw)/2
		ty := capR.Min.Y + (capR.Dy()-th)/2
		DrawTextColorAt(dst, label, tx, ty, textCol)
	}
}
