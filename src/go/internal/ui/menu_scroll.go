package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// MenuScroll bundles a ScrollBehavior and a DeferredTap into the single shared
// scroll component used by every list menu (context, overflow, subdivision,
// instrument). It owns the one correct wheel / thumb-drag / touch-scroll input
// state machine — the overflow menu previously hand-rolled this glue and got it
// half-wired (no offset applied, no drag/touch), which is the bug this fixes.
type MenuScroll struct {
	sb  *ScrollBehavior
	tap DeferredTap
}

// NewMenuScroll creates a MenuScroll with the given scrollbar style and the
// per-item pixel height used for offset and pixel<->item conversion.
func NewMenuScroll(style ScrollbarStyle, itemHeight int) *MenuScroll {
	return &MenuScroll{sb: NewScrollBehavior(style, itemHeight)}
}

// Configure refreshes the scroll viewport and item counts for the current
// layout. Safe to call every frame: it preserves the current scroll position
// (VS.First) via Clamp. Call whenever item count, viewport, or page changes.
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

// HandleWheel applies a wheel event; returns true when the offset changed.
func (m *MenuScroll) HandleWheel(steps int) bool { return m.sb.HandleWheel(steps) }

// Draw renders the scrollbar (no-op when content fits).
func (m *MenuScroll) Draw(dst *ebiten.Image) { m.sb.Draw(dst) }

// ScrollBehavior exposes the underlying engine for menus (instrument menu) whose
// bespoke content layout drives VS.View / VS.First directly.
func (m *MenuScroll) ScrollBehavior() *ScrollBehavior { return m.sb }

// DeferredTap exposes the tap helper for callers that need to cancel it.
func (m *MenuScroll) DeferredTap() *DeferredTap { return &m.tap }

// TapActive reports an in-flight mobile deferred tap. Callers map this to
// InputCaptured (vs InputConsumed) for the overlay portal.
func (m *MenuScroll) TapActive() bool { return m.tap.Active() }

// HandleScrollbarDrag processes scrollbar thumb interaction only (start on press
// inside the thumb, continue/end on subsequent events). It is the shared
// primitive for menus that keep their own immediate-tap content-input
// (subdivision, instrument). Returns true when the scrollbar consumed the event.
// relayout (may be nil) is called when the offset changes mid-drag.
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

// MenuScrollInput carries the per-event inputs and content callbacks for one
// HandleInput call. Each menu keeps its own content layout/draw; MenuScroll owns
// only the scroll mechanics.
type MenuScrollInput struct {
	Pt        image.Point
	Pressed   bool
	PopupRect image.Rectangle // whole popup; taps inside are consumed
	ItemView  image.Rectangle // scroll viewport (excludes any pinned header)

	// Relayout re-applies OffsetPx() to cached item buttons after the offset
	// changes. Menus that rebuild their buttons every frame may pass nil.
	Relayout func()
	// FireTapAt fires the menu button under (x,y) on a committed mobile tap.
	FireTapAt func(x, y int)
	// DesktopHit performs immediate hit-testing of the menu's buttons on
	// desktop; returns true when a button consumed the event.
	DesktopHit func(pt image.Point, pressed bool) bool
	// CloseBtn, when non-nil, is checked before items on mobile (highest z).
	CloseBtn *Button
}

// HandleInput runs the complete desktop+mobile scroll input state machine,
// behavior-identical to the (correct) context menu reference implementation.
// Returns true when the event was handled (consumed/captured).
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
