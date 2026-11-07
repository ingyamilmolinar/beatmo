package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Move placement is edge-only: a cell not on an existing edge cancels the mode and the node stays.
func (g *Game) handleArmedModeTap(x, y int) bool {
	switch g.modes.Active() {
	case GridModeMove:
		gt := g.guideTargetAt(x, y)
		n := g.modes.Subject()
		if gt.Kind != GuideEdge {
			g.modes.Cancel("invalid-target")
			return true
		}
		if g.moveHealPlan(n).class == healUnhealable {
			// The mode stays armed; the confirm portal decides.
			g.beginBreakConfirm(n, gt.I, gt.J)
			return true
		}
		if g.moveNode(n, gt.I, gt.J) {
			g.modes.Commit()
		} else {
			g.modes.Cancel("move-failed")
		}
		return true
	case GridModeCopy:
		gt := g.guideTargetAt(x, y)
		if existing := g.nodeAt(gt.I, gt.J); existing != nil {
			// The confirm takes over; the mode's job is done.
			g.beginPasteConfirm(gt.I, gt.J)
			g.modes.Commit()
			return true
		}
		g.pasteNodeFrom(gt.I, gt.J, g.clipboard)
		g.modes.Commit()
		return true
	case GridModeConnect:
		g.handleConnectModeTap(x, y)
		return true
	}
	return false
}

func (g *Game) handleTapInGrid(x, y int) {
	// An outside tap on a blocking overlay only closes it (through the tree), never reaches the grid.
	if g.modalOverlayActive() {
		return
	}
	// The rotate notice covers the screen.
	if g.landscapeUnsupported() {
		return
	}
	if g.inTopRightButtonZone(x, y) {
		// The dispatcher already toggled the menu on the press edge, so consume the release-time tap for the
		// whole cluster: toggling again double-fires, falling through creates a stray node.
		return
	}

	// Armed grid mode (move/copy/connect): the tap is the placement/target.
	if g.handleArmedModeTap(x, y) {
		return
	}

	// A gesture the sidebar owned at press stays its own at lift: a confirming picker cell closes the
	// picker mid-gesture and floats outside the card, so a release-time Hit would close the card too.
	if g.sidebar.IsOpen() && !g.sidebar.Hit(x, y) && g.inputDispatcher.PressOwner() == InputHandler(g.sidebar) {
		if g.sidebar.scroll.TouchActive() || g.sidebar.scroll.Dragging() {
			g.sidebar.HandleInput(x, y, false)
		}
		return
	}
	// Check for sidebar interaction first
	if g.sidebar.IsOpen() && g.sidebar.Hit(x, y) {
		// With scroll tracking active, send only a release so the deferred tap and scroll resolve normally.
		if g.sidebar.scroll.TouchActive() || g.sidebar.scroll.Dragging() {
			g.sidebar.HandleInput(x, y, false)
			return
		}
		// With suppress active the override already processed this tap.
		if !suppressClicksUntilRelease {
			g.handleNodeMenuButtons(x, y, true)
			g.handleNodeMenuButtons(x, y, false)
		}
		return
	}
	// Before the generic outside-close, or a tap on a badge would just close the sidebar and be swallowed.
	if gid, ok := g.groupBadgeAt(x, y); ok {
		g.sidebar.Close()
		g.openGroupMenuForBadge(gid, x, y)
		return
	}
	if g.sidebar.IsOpen() {
		// Tap outside sidebar closes it
		g.sidebar.Close()
		return
	}

	// Guard: sidebar was closed recently. Don't create nodes at the tap position.
	if g.sidebar.ClosedGuard() > 0 {
		return
	}

	// Handle origin selection if a row requested it
	if g.pendingStartRow >= 0 {
		if n := g.nodeAtScreen(x, y); n != nil {
			if mn, ok := g.graph.GetNodeByID(n.ID); ok && mn.Type != model.NodeTypeInvisible {
				row := g.pendingStartRow
				// Disallow selecting a node from another audible circuit
				if other, ok := g.nodeRows[n.ID]; ok && other != row {
					if g.Playing() && g.rowIsAudible(other) {
						return
					}
				}
				if row >= 0 && row < len(g.drum.Rows) {
					if old := g.drum.Rows[row].Node; old != nil {
						old.Start = false
					}
					g.drum.Rows[row].Origin = n.ID
					g.drum.Rows[row].Node = n
					n.Start = true
					if row == 0 {
						g.start = n
						g.graph.StartNodeID = n.ID
					}
					g.updateBeatInfos()
				}
				g.pendingStartRow = -1
				return
			}
		}
	}

	// Check if tapping on existing node
	if n := g.nodeAtScreen(x, y); n != nil {
		// Select node and open menu
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.sidebar.Open(n)
		g.computeSelNeighbors()
		g.coordBadgeNode = n
		g.coordBadgeFrame = g.frame
		return
	}

	// Snaps to the nearest node or edge cell within the radius; a free cell otherwise.
	g.commitGuideTarget(x, y)
}

// Grid pane: delete the node. Drum pane: the row's context menu.
func (g *Game) handleTouchLongPress(x, y int) {
	// A blocking overlay or the landscape notice owns input.
	if g.modalOverlayActive() || g.landscapeUnsupported() {
		return
	}
	// Shows the unabbreviated readout.
	if g.drum != nil && g.drum.eqPanelZone != nil {
		if g.drum.eqPanelZone.HandleLevelsAggregateLongPress(x, y) {
			return
		}
	}
	// Check if long-press is in the drum pane on a row label (mobile context menu).
	if Profile().IsMobile() && g.drum != nil && image.Pt(x, y).In(g.drum.Bounds) {
		for i, lbl := range g.drum.rowLabels() {
			if image.Pt(x, y).In(lbl.Rect()) {
				g.drum.openContextMenu(i)
				return
			}
		}
	}

	if y < gridTopOffset() || !g.split.InGridPane(x, y) {
		return
	}

	// Long-press on a node: show quick-action popup (all platforms).
	if n := g.nodeAtScreen(x, y); n != nil {
		g.showLongPressPopup(n, x, y)
	}
}

// Scale tracks the finger distance 1:1. The drum rows on mobile absorb the pinch; the grid pane
// zooms the camera; anywhere else it is ignored.
func (g *Game) handleTouchPinch(centerX, centerY int, scale float64) {
	// A blocking overlay or the landscape notice owns input.
	if g.modalOverlayActive() || g.landscapeUnsupported() {
		return
	}
	// Absorbed over the rack so it never bleeds into camera zoom; row sizes are fixed.
	if Profile().IsMobile() && g.drum != nil {
		rr := g.drum.rowsRect()
		if !rr.Empty() && image.Pt(centerX, centerY).In(rr) {
			return
		}
	}

	if centerY < gridTopOffset() || !g.split.InGridPane(centerX, centerY) {
		return
	}

	// Record baseline on first pinch event of this gesture
	if g.pinchBaseScale == 0 {
		g.pinchBaseScale = g.cam.Scale
		g.pinchBaseGestureScale = scale
		return
	}

	// Direct 1:1 mapping: camera tracks finger distance ratio
	ratio := scale / g.pinchBaseGestureScale
	targetScale := g.pinchBaseScale * ratio

	if targetScale < 0.1 {
		targetScale = 0.1
	} else if targetScale > 10.0 {
		targetScale = 10.0
	}

	// Anchor zoom at pinch center (same transform as zoomAtScreen)
	sx, sy := float64(centerX), float64(centerY)
	wx := (sx - g.cam.OffsetX) / g.cam.Scale
	wy := (sy - float64(gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	g.cam.OffsetX = sx - wx*targetScale
	g.cam.OffsetY = sy - float64(gridTopOffset()) - wy*targetScale
	g.cam.Scale = targetScale
	g.cam.Snap()
}

// Over the cell grid it moves the timeline or rows (direction-locked), never the camera.
func (g *Game) handleTouchTwoFingerPan(deltaX, deltaY, centerX, centerY int) {
	// A blocking overlay or the landscape notice owns input.
	if g.modalOverlayActive() || g.landscapeUnsupported() {
		return
	}
	if g.drum != nil && g.drum.HandleTwoFingerPan(centerX, centerY) {
		return
	}
	g.cam.OffsetX += float64(deltaX)
	g.cam.OffsetY += float64(deltaY)
	g.cam.Snap()
}

// IsTouchActive returns true if any touch is currently active.
func (g *Game) IsTouchActive() bool {
	return globalTouchState.ActiveTouchCount() > 0
}
