package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// The one correct wheel / thumb / touch-scroll state machine for every list menu.
type MenuScroll struct {
	sb  *ScrollBehavior
	tap DeferredTap
}

func NewMenuScroll(style ScrollbarStyle, itemHeight int) *MenuScroll {
	return &MenuScroll{sb: NewScrollBehavior(style, itemHeight)}
}

// Safe every frame: Clamp preserves VS.First.
func (m *MenuScroll) Configure(view image.Rectangle, total, visible int) {
	m.sb.VS.View = view
	m.sb.VS.Total = total
	m.sb.VS.Visible = visible
	m.sb.VS.Clamp()
}

// OffsetPx is the pixel offset to subtract from item base-Y during layout/draw.
func (m *MenuScroll) OffsetPx() int { return m.sb.VS.First * m.sb.ItemHeight }

// HasScroll reports whether the content overflows the viewport.
func (m *MenuScroll) HasScroll() bool { return m.sb.HasScroll() }

// Respects magnitude with no cooldown: the one wheel path for list menus. The rack and control
// grids keep the clicky WheelStep on purpose.
func (m *MenuScroll) HandleWheel(steps int) bool {
	return m.sb.HandleWheel(steps)
}

// Once per frame while the menu exists.
func (m *MenuScroll) UpdateMomentum() bool {
	if !m.sb.HasMomentum() {
		return false
	}
	return m.sb.UpdateMomentum()
}

// The shared momentum driver; rebuild runs only when the offset changed.
func (m *MenuScroll) Tick(rebuild func()) bool {
	if !m.UpdateMomentum() {
		return false
	}
	if rebuild != nil {
		rebuild()
	}
	return true
}

// Draw renders the scrollbar (no-op when content fits).
func (m *MenuScroll) Draw(dst *ebiten.Image) { m.sb.Draw(dst) }

// For menus whose layout drives VS.View / VS.First directly.
func (m *MenuScroll) ScrollBehavior() *ScrollBehavior { return m.sb }

// DeferredTap exposes the tap helper for callers that need to cancel it.
func (m *MenuScroll) DeferredTap() *DeferredTap { return &m.tap }

// Callers map it to InputCaptured rather than InputConsumed.
func (m *MenuScroll) TapActive() bool { return m.tap.Active() }

// For menus that keep immediate-tap content input; relayout may be nil.
func (m *MenuScroll) HandleScrollbarDrag(pt image.Point, pressed bool, relayout func()) bool {
	sb := m.sb
	if sb.Dragging() {
		if pressed {
			if sb.HandleDragTo(pt.Y) && relayout != nil {
				relayout()
			}
		} else {
			sb.HandleDragEnd()
		}
		return true
	}
	if sb.HasScroll() && pressed && pt.In(sb.ThumbRect()) {
		sb.HandleDragStart(pt.Y)
		return true
	}
	return false
}

// Menus keep their own content layout and draw; this owns only the scroll mechanics.
type MenuScrollInput struct {
	Pt        image.Point
	Pressed   bool
	PopupRect image.Rectangle // whole popup; taps inside are consumed
	ItemView  image.Rectangle // scroll viewport (excludes any pinned header)

	// nil for menus that rebuild buttons every frame.
	Relayout func()
	// FireTapAt fires the menu button under (x,y) on a committed mobile tap.
	FireTapAt  func(x, y int)
	DesktopHit func(pt image.Point, pressed bool) bool
	// CloseBtn, when non-nil, is checked before items on mobile (highest z).
	CloseBtn *Button
}

// The full desktop + mobile state machine, matching the context menu's reference behaviour.
func (m *MenuScroll) HandleInput(in MenuScrollInput) bool {
	pt := in.Pt
	left := in.Pressed
	sb := m.sb

	if Profile().IsMobile() {
		// 1. Scrollbar drag in progress.
		if sb.Dragging() {
			if left {
				if sb.HandleDragTo(pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				sb.HandleDragEnd()
			}
			return true
		}
		// 2. Scroll committed → move; on release cancel the deferred tap.
		if sb.ScrollingCommitted() {
			if left {
				if sb.HandleTouchMove(pt.X, pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				m.tap.Cancel()
				sb.HandleTouchEnd()
			}
			return true
		}
		// 3. Touch active but still in the dead zone.
		if sb.TouchActive() {
			if left {
				if sb.HandleTouchMove(pt.X, pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				wasTap := !sb.ScrollingCommitted()
				sb.HandleTouchEnd()
				if wasTap && in.FireTapAt != nil {
					m.tap.End(in.FireTapAt)
				} else {
					m.tap.Cancel()
				}
			}
			return true
		}
		// 4. Scrollbar thumb click.
		if sb.HasScroll() && left && pt.In(sb.ThumbRect()) {
			sb.HandleDragStart(pt.Y)
			return true
		}
		// 5. Close button (highest z, before item area).
		if in.CloseBtn != nil && left && pt.In(in.CloseBtn.Rect()) {
			if in.CloseBtn.OnClick != nil {
				in.CloseBtn.OnClick()
			}
			return true
		}
		// 6. Press in item viewport → begin deferred tap + touch scroll.
		if left && pt.In(in.ItemView) && !sb.TouchActive() && !sb.Dragging() {
			if m.tap.Begin(pt.X, pt.Y) {
				sb.HandleTouchBegin(pt.X, pt.Y)
				return true
			}
		}
		// 7. Header area (above viewport) → consume without action.
		if pt.In(in.PopupRect) && pt.Y < in.ItemView.Min.Y {
			return true
		}
	} else {
		// Desktop: scrollbar drag, then immediate button hit.
		if sb.Dragging() {
			if left {
				if sb.HandleDragTo(pt.Y) && in.Relayout != nil {
					in.Relayout()
				}
			} else {
				sb.HandleDragEnd()
			}
			return true
		}
		if sb.HasScroll() && left && pt.In(sb.ThumbRect()) {
			sb.HandleDragStart(pt.Y)
			return true
		}
		if in.DesktopHit != nil && in.DesktopHit(pt, left) {
			return true
		}
	}

	// Consume any tap inside the popup (click-outside close handled by the tree).
	if pt.In(in.PopupRect) {
		return true
	}
	return false
}
