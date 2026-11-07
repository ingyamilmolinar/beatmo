package ui

// A two-finger pan over the cell grid moves the timeline or the rows, direction-locked, instead
// of the camera, so a left/right swipe cannot leak into row scrolling.

const twoFingerPanDeadZone = 8

type twoFingerPanState struct {
	active      bool
	startX      int
	startY      int
	startOffset int
	startRowOff int
	locked      bool
	vertical    bool
}

// Uses the tree's z-order, so the panel, portals and row controls win where they overlap.
func (dv *DrumView) cellGridFrontmostAt(x, y int) bool {
	if dv.tree == nil {
		return false
	}
	hits := dv.tree.HitIndexRef().At(x, y)
	return len(hits) > 0 && hits[0].Tag == "timeline-grid-drag"
}

// Once anchored over the grid it owns the whole gesture, so a drifting centre never becomes a
// camera pan.
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
			target := st.startRowOff + (st.startY-centerY)/rowH
			dv.timelineZone.callbacks.OnRowScrollDrag(target)
		}
		return true
	}

	cell := dv.cell
	if cell < 1 {
		cell = 1
	}
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

func (dv *DrumView) EndTwoFingerPan() {
	dv.twoFingerPan = twoFingerPanState{}
}
