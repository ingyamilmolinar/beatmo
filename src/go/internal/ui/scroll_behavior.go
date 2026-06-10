package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// ScrollBehavior composes VerticalScroller (item-level) and TouchScroller
// (pixel-level momentum) into a single scrollable behaviour. It handles
// wheel, scrollbar drag, and touch input uniformly so callers only need
// to forward raw events and read the result.
type ScrollBehavior struct {
	VS         VerticalScroller // item-level scroll state
	TS         TouchScroller    // pixel-level momentum/direction lock
	Style      ScrollbarStyle
	ItemHeight int     // px per item (for pixel→item conversion)
	pixelAccum float64 // sub-item precision accumulator
	dirty      bool    // true when VS.First changed since last ClearDirty

	// Opt-in "step-by-step" state (used by the synth ControlGrid and the row
	// rack). These power WheelStep / StepDragTo and are independent of the
	// continuous HandleWheel / HandleDragTo / HandleTouch* paths, so scrollbars
	// that don't call the step methods (dropdowns, context menu, sidebar) keep
	// their original feel.
	stepCooldown   int  // frames until the next wheel/step move is allowed
	stepDragging   bool // a stepped thumb drag is in progress
	stepDragStartY int  // pointer Y at the start of the stepped drag
	stepFirst      int  // VS.First at the start of the stepped drag
}

// NewScrollBehavior creates a ScrollBehavior with the given style and item height.
func NewScrollBehavior(style ScrollbarStyle, itemHeight int) *ScrollBehavior {
	return &ScrollBehavior{
		Style:      style,
		ItemHeight: itemHeight,
	}
}

// --- Input handlers (return true when VS.First changed) ---

// HandleWheel processes a mouse wheel event. steps > 0 = scroll up (toward
// lower indices), matching VerticalScroller.ScrollBy semantics.
func (sb *ScrollBehavior) HandleWheel(steps int) bool {
	if sb.VS.ScrollBy(-steps) {
		sb.dirty = true
		return true
	}
	return false
}

// HandleDragStart begins a scrollbar thumb drag if y is within the thumb.
func (sb *ScrollBehavior) HandleDragStart(y int) bool {
	if sb.VS.StartDrag(y, sb.Style.Width, sb.Style.MinThumbH) {
		sb.dirty = true
		return true
	}
	return false
}

// HandleDragTo continues a scrollbar drag to the given y position.
func (sb *ScrollBehavior) HandleDragTo(y int) bool {
	if sb.VS.DragTo(y, sb.Style.Width, sb.Style.MinThumbH) {
		sb.dirty = true
		return true
	}
	return false
}

// HandleDragEnd ends the scrollbar drag.
func (sb *ScrollBehavior) HandleDragEnd() {
	sb.VS.EndDrag()
}

// --- Step-by-step input (opt-in; see the stepCooldown/stepDragging fields) ---

// WheelStep advances the scroll by exactly ONE item in the wheel's direction,
// then refuses further steps until cooldownFrames frames have elapsed (see
// TickStep). The magnitude of steps is ignored on purpose — one notch (or one
// fast flick / hi-res-wheel burst) is one item — so the wheel can't fly through
// the list. steps > 0 is a wheel-up (toward lower indices), matching ScrollBy.
// Returns true when VS.First changed.
func (sb *ScrollBehavior) WheelStep(steps, cooldownFrames int) bool {
	if steps == 0 || sb.stepCooldown > 0 {
		return false
	}
	dir := 1
	if steps > 0 {
		dir = -1
	}
	prev := sb.VS.First
	sb.VS.First += dir
	sb.VS.Clamp()
	if sb.VS.First == prev {
		return false
	}
	sb.stepCooldown = cooldownFrames
	sb.dirty = true
	return true
}

// TickStep advances the per-frame cooldown clock. Call once per frame. Cheap
// no-op when no cooldown is pending.
func (sb *ScrollBehavior) TickStep() {
	if sb.stepCooldown > 0 {
		sb.stepCooldown--
	}
}

// BeginStepDrag starts a stepped thumb drag anchored at y. The caller decides
// when to start it (e.g. on a press inside the thumb). Also raises VS.dragging
// so Dragging()/capture logic sees the drag. Returns false (no capture) when
// the content does not scroll.
func (sb *ScrollBehavior) BeginStepDrag(y int) bool {
	if !sb.HasScroll() {
		return false
	}
	sb.stepDragging = true
	sb.VS.dragging = true
	sb.stepDragStartY = y
	sb.stepFirst = sb.VS.First
	return true
}

// StepDragTo advances the scroll one item per stepPx of pointer travel from the
// grab point, anchored so the drag is fully reversible and never jumps more
// items than the distance dragged warrants. Returns true when VS.First changed.
func (sb *ScrollBehavior) StepDragTo(y, stepPx int) bool {
	if !sb.stepDragging || stepPx <= 0 {
		return false
	}
	rows := (y - sb.stepDragStartY) / stepPx
	newFirst := sb.stepFirst + rows
	maxFirst := sb.VS.Total - sb.VS.Visible
	if maxFirst < 0 {
		maxFirst = 0
	}
	if newFirst < 0 {
		newFirst = 0
	}
	if newFirst > maxFirst {
		newFirst = maxFirst
	}
	if newFirst == sb.VS.First {
		return false
	}
	sb.VS.First = newFirst
	sb.dirty = true
	return true
}

// EndStepDrag releases a stepped thumb drag.
func (sb *ScrollBehavior) EndStepDrag() {
	sb.stepDragging = false
	sb.VS.dragging = false
}

// StepDragging reports whether a stepped thumb drag is in progress.
func (sb *ScrollBehavior) StepDragging() bool { return sb.stepDragging }

// HandleTouchBegin starts tracking a touch at the given coordinates.
func (sb *ScrollBehavior) HandleTouchBegin(x, y int) {
	sb.TS.Begin(x, y)
	sb.pixelAccum = 0
}

// HandleTouchMove processes a touch move and returns true if VS.First changed.
func (sb *ScrollBehavior) HandleTouchMove(x, y int) bool {
	delta := sb.TS.Move(x, y)
	if delta == 0 {
		return false
	}
	return sb.applyPixelDelta(-delta)
}

// HandleTouchEnd ends the touch and captures velocity for momentum.
func (sb *ScrollBehavior) HandleTouchEnd() {
	sb.TS.End()
}

// UpdateMomentum applies per-frame momentum decay. Returns true if VS.First changed.
// Call once per frame after the touch has ended.
func (sb *ScrollBehavior) UpdateMomentum() bool {
	delta := sb.TS.UpdateMomentum()
	if delta == 0 {
		return false
	}
	return sb.applyPixelDelta(-delta)
}

// applyPixelDelta converts a pixel delta to item offset change using the
// sub-item accumulator. Returns true if VS.First changed.
func (sb *ScrollBehavior) applyPixelDelta(px float64) bool {
	if sb.ItemHeight <= 0 {
		return false
	}
	rows := px / float64(sb.ItemHeight)
	sb.pixelAccum += rows
	intRows := int(sb.pixelAccum)
	if intRows == 0 {
		return false
	}
	sb.pixelAccum -= float64(intRows)
	prev := sb.VS.First
	sb.VS.First += intRows
	sb.VS.Clamp()
	// Reset accumulator at boundaries to prevent drift
	if sb.VS.First == 0 || sb.VS.First == sb.VS.Total-sb.VS.Visible {
		sb.pixelAccum = 0
	}
	if sb.VS.First != prev {
		sb.dirty = true
		return true
	}
	return false
}

// --- Geometry ---

// HasScroll reports whether scrolling is necessary.
func (sb *ScrollBehavior) HasScroll() bool {
	return sb.VS.HasScroll()
}

// BarRect returns the scrollbar track rectangle.
func (sb *ScrollBehavior) BarRect() image.Rectangle {
	return sb.VS.BarRect(sb.Style.Width)
}

// ThumbRect returns the scrollbar thumb rectangle.
func (sb *ScrollBehavior) ThumbRect() image.Rectangle {
	return sb.VS.ThumbRect(sb.Style.Width, sb.Style.MinThumbH)
}

// --- Draw ---

// Draw renders the scrollbar using the configured style.
func (sb *ScrollBehavior) Draw(dst *ebiten.Image) {
	if !sb.HasScroll() {
		return
	}
	sb.Style.Draw(dst, sb.BarRect(), sb.ThumbRect())
}

// --- State queries ---

// TouchActive reports whether a touch is currently being tracked.
func (sb *ScrollBehavior) TouchActive() bool { return sb.TS.Active() }

// ScrollingCommitted reports whether a touch scroll has committed (past dead zone).
func (sb *ScrollBehavior) ScrollingCommitted() bool { return sb.TS.ScrollingCommitted() }

// HasMomentum reports whether momentum scrolling is still in progress.
func (sb *ScrollBehavior) HasMomentum() bool { return sb.TS.HasMomentum() }

// Dragging reports whether a scrollbar drag is in progress.
func (sb *ScrollBehavior) Dragging() bool { return sb.VS.dragging }

// Dirty reports whether VS.First has changed since the last ClearDirty call.
func (sb *ScrollBehavior) Dirty() bool { return sb.dirty }

// ClearDirty resets the dirty flag.
func (sb *ScrollBehavior) ClearDirty() { sb.dirty = false }

// ResetTouch clears all touch state (active tracking + momentum).
func (sb *ScrollBehavior) ResetTouch() {
	sb.TS.Reset()
	sb.pixelAccum = 0
}
