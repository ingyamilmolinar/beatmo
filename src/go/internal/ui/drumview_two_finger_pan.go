package ui

// Two-finger pan over the drum cell grid.
//
// A two-finger pan whose center sits over the cell grid (the "steps" region
// where a single-finger drag already scrolls the timeline) is routed here
// instead of panning the grid/graph camera. It mirrors the single-finger grid
// drag (gridDragHitAdapter in timeline_zone.go): direction-locked, with a
// left/right swipe moving the timeline view (dv.Offset) and an up/down swipe
// scrolling the instrument rows. The direction lock is what fixes the reported
// bug — a left/right swipe can no longer leak into vertical row scrolling.

// twoFingerPanDeadZone is the travel (px) before the gesture commits to a
// horizontal or vertical axis. Matches gridDragHitAdapter's deadZone.
const twoFingerPanDeadZone = 8

// twoFingerPanState holds the per-gesture anchor + direction lock for a
// two-finger pan over the cell grid.
type twoFingerPanState struct {
	active      bool
	startX      int
	startY      int
	startOffset int
	startRowOff int
	locked      bool
	vertical    bool
}

// cellGridFrontmostAt reports whether the drum cell grid (the timeline grid
// drag surface) is the front-most hit at (x,y). This reuses the tree's z-order
// arbitration so the audio panel, portals, and row controls naturally win where
// they overlap — the two-finger pan engages exactly where a single-finger drag
// would move the timeline.
func (dv *DrumView) cellGridFrontmostAt(x, y int) bool {
	if dv.tree == nil {
		return false
	}
	hits := dv.tree.HitIndexRef().At(x, y)
	return len(hits) > 0 && hits[0].Tag == "timeline-grid-drag"
}

// HandleTwoFingerPan consumes a two-finger pan whose center is (centerX,
// centerY). It returns true when the gesture belongs to the cell grid (and was
// handled here), false when the caller should fall back to camera panning.
//
// Once the gesture is anchored over the cell grid it keeps ownership for the
// rest of the gesture (even if the center drifts) so a stray frame never bleeds
// into a camera pan.
func (dv *DrumView) HandleTwoFingerPan(centerX, centerY int) bool {
	st := &dv.twoFingerPan

	if !st.active {
		if !dv.cellGridFrontmostAt(centerX, centerY) {
			return false // not over the cell grid → let the camera handle it
		}
		// Anchor the gesture; no movement on the first frame.
		st.active = true
		st.startX = centerX
		st.startY = centerY
		st.startOffset = dv.Offset
		st.startRowOff = dv.rowOffset
		st.locked = false
		st.vertical = false
		return true
	}

	if !st.locked {
		dx := abs(centerX - st.startX)
		dy := abs(centerY - st.startY)
		if dx < twoFingerPanDeadZone && dy < twoFingerPanDeadZone {
			return true // still in the dead zone
		}
		st.locked = true
		st.vertical = dy > dx
	}

	if st.vertical {
		rowH := dv.rowHeight()
		if rowH > 0 && dv.timelineZone != nil && dv.timelineZone.callbacks.OnRowScrollDrag != nil {
			// Match gridDragHitAdapter: dragging up (center moves up) scrolls
			// the rows down (increasing offset).
			target := st.startRowOff + (st.startY-centerY)/rowH
			dv.timelineZone.callbacks.OnRowScrollDrag(target)
		}
		return true
	}

	cell := dv.cell
	if cell < 1 {
		cell = 1
	}
	// Match gridDragHitAdapter horizontal mapping: fingers moving left
	// (center decreases) advances the timeline.
	delta := (st.startX - centerX) / cell
	newOffset := st.startOffset + delta
	if newOffset < 0 {
		newOffset = 0
	}
	if dv.timelineZone != nil && dv.timelineZone.callbacks.OnOffsetChange != nil {
		dv.timelineZone.callbacks.OnOffsetChange(newOffset)
	}
	return true
}

// EndTwoFingerPan clears any in-progress two-finger pan. Called when fewer than
// two fingers remain so the next gesture re-anchors cleanly.
func (dv *DrumView) EndTwoFingerPan() {
	dv.twoFingerPan = twoFingerPanState{}
}
