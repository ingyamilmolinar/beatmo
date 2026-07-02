package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// SegmentedControl is a general-purpose 2-to-N segment toggle. It renders as a
// row of matte keycaps inside a surface-2 tray (the same look drawPillTabAt
// gives the desktop audio tabs): the active segment is a solid sunset-gold
// (#FFB30A) cap, inactive segments are neutral caps, disabled segments are
// greyed and inert. Clicks dispatch to the registered callback with the segment
// index.
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
	// disabled[i] greys segment i and makes it inert: HitTest consumes the tap
	// (so it doesn't fall through to a lower-z handler) but neither activates it
	// nor fires onClick. Lazily sized to len(labels); a nil/short slice means
	// "all enabled". Used to grey the Synth segment for WAV instruments.
	disabled []bool
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

// SetSegmentDisabled greys segment i and makes it inert (or re-enables it).
// Out-of-range indices are ignored.
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

// SetLabels replaces the segment labels in place (e.g. after a UI language
// switch). The active index is preserved when still in range, else reset to 0.
// The slice is copied so callers may reuse theirs.
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

// Draw renders the control as a row of matte keycaps inside a surface-2 tray —
// the same look drawPillTabAt gives the desktop audio tabs, so the mobile
// switcher matches the desktop pills. The active segment is a solid sunset-gold
// (#FFB30A) cap sitting pressed-IN (inverted bevel); inactive segments are
// neutral raised caps; a disabled segment is a greyed, inert cap. Each cap is
// inset 1px inside its cell so the tray shows through the seams as hairline
// dividers. The pre-boxed pill* palette is reused so the path adds no per-call
// color boxing and introduces no inline color literals.
func (s *SegmentedControl) Draw(dst *ebiten.Image) {
	if s.rect.Empty() || len(s.labels) == 0 {
		return
	}
	// Tray backdrop. The keycaps sit on top; the surface-2 tray peeks through the
	// 1px seams between caps as the segment dividers.
	drawRoundedRect(dst, s.rect, colSurface2, RadiusMD, true)
	drawRoundedRect(dst, s.rect, colBorderMedium, RadiusMD, false)

	pillRadius := RadiusMD / 2 // 4px corners — matches the desktop audio tabs
	for i, label := range s.labels {
		cell := insetRect(s.SegmentRect(i), 1) // 1px seam → hairline divider
		if cell.Empty() {
			continue
		}
		dis := s.segmentDisabled(i)
		active := i == s.active && !dis

		// Shared pill palette (see audio_sticky_bar.go) so the mobile switcher is
		// pixel-consistent with the desktop tabs.
		capFill, borderCol, textCol, shellCol := pillCapFillInactive, pillCapBorderInact, pillTextInactive, pillShellInactive
		switch {
		case dis:
			capFill, borderCol, textCol, shellCol = pillCapFillDisabled, pillCapBorderDisab, pillTextDisabled, pillShellDisabled
		case active:
			capFill, borderCol, textCol, shellCol = pillCapFillActive, pillCapBorderActive, pillTextActive, pillShellActive
		}

		// Static keycap (segments aren't Buttons, so no press-spring). The cap is
		// travel-invariant: raised=false anchors it at the cell top, the wall shows
		// at the bottom as the socket.
		capR := keycapCapRect(cell, 0, false)
		if genGeomKeycapWallDepth != 0 {
			drawRoundedButton(dst, cell, shellCol, shellCol, pillRadius, false) // socket/side-wall
		}
		drawKeycapContactShadow(dst, capR, pillRadius)
		drawRoundedButton(dst, capR, capFill, borderCol, pillRadius, false) // cap face
		if !dis {
			if active {
				drawKeycapActiveInset(dst, capR, pillRadius) // lit-amber, pressed-IN
			} else {
				drawKeycapBevel(dst, capR, pillRadius) // matte raised
			}
		}

		// Center label on the cap face.
		tw := TextWidth(label)
		th := TextHeight()
		tx := capR.Min.X + (capR.Dx()-tw)/2
		ty := capR.Min.Y + (capR.Dy()-th)/2
		DrawTextColorAt(dst, label, tx, ty, textCol)
	}
}
