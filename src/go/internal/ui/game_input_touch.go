package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// handleTapInGrid handles a tap gesture in the grid area.
// This provides direct tap-to-click handling without relying on the mouse state machine.
func (g *Game) handleTapInGrid(x, y int) {
	// Connect mode: tap selects target node for edge creation.
	if g.connectMode && g.connectFromNode != nil {
		g.handleConnectModeTap(x, y)
		return
	}

	// Move mode: tap places node at destination
	if g.moveMode && g.movingNode != nil {
		if g.moveConfirm {
			// Confirmation dialog: check tap on confirm/cancel buttons
			dw, dh := 280, 60
			dx := (g.split.GridW(g.winW) - dw) / 2
			dy := (g.split.GridH(g.winH) - dh) / 2
			confirmRect := image.Rect(dx+40, dy+30, dx+120, dy+50)
			cancelRect := image.Rect(dx+160, dy+30, dx+240, dy+50)
			pt := image.Pt(x, y)
			if pt.In(confirmRect) {
				g.moveNode(g.movingNode, g.moveConfirmI, g.moveConfirmJ)
				g.cancelMoveMode()
			} else if pt.In(cancelRect) {
				g.cancelMoveMode()
			}
			return
		}
		wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
		wy := (float64(y-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
		_, _, ni, nj := g.grid.Snap(wx, wy)
		if existing := g.nodeAt(ni, nj); existing != nil && existing.ID != g.movingNode.ID {
			g.cancelMoveMode()
			return
		}
		loss := g.moveNodeEdgeLoss(g.movingNode, ni, nj)
		if loss > 0 {
			g.moveConfirm = true
			g.moveConfirmI = ni
			g.moveConfirmJ = nj
			g.moveEdgeLoss = loss
		} else {
			g.moveNode(g.movingNode, ni, nj)
			g.cancelMoveMode()
		}
		return
	}

	// Check for sidebar interaction first
	if g.sidebar.IsOpen() && g.sidebar.Hit(x, y) {
		// If the sidebar has active scroll/touch tracking (from the
		// InputDispatcher's captured frames), send only a release to let
		// the sidebar's deferred-tap + scroll pattern resolve correctly:
		// - If scroll committed: cancels deferred tap, no section toggle.
		// - If scroll NOT committed: fires deferred tap → section toggles.
		// This prevents the instant press+release from bypassing scroll
		// disambiguation.
		if g.sidebar.scroll.TouchActive() || g.sidebar.scroll.Dragging() {
			g.sidebar.HandleInput(x, y, false)
			return
		}
		// Touch-to-mouse override normally handles sidebar presses via
		// handleEditor. When suppress is active the override already
		// processed this tap; just consume the gesture.
		if !suppressClicksUntilRelease {
			g.handleNodeMenuButtons(x, y, true)
			g.handleNodeMenuButtons(x, y, false)
		}
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

	// Tap on empty space creates node
	wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(y-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	_, _, i, j := g.grid.Snap(wx, wy)
	n := g.tryAddNode(i, j, model.NodeTypeRegular)
	if n != nil {
		if g.sel != nil && g.sel != n {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
	}
	// Don't open sidebar for new nodes (consistent with mouse behavior)
	g.sidebar.Close()
}

// handleTouchLongPress handles a long press gesture, equivalent to right-click.
// In the grid pane, deletes the node under the touch point.
// In the drum pane, opens the mobile context menu for the row label.
func (g *Game) handleTouchLongPress(x, y int) {
	// Audio panel: Levels icon-row long-press → tooltip with the
	// unabbreviated readout value (Phase 4 audio-panel redesign).
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

// handleTouchPinch handles a pinch zoom gesture.
// Uses direct 1:1 scale mapping: camera scale tracks finger distance ratio.
//
// Three regions can absorb a pinch, decided by the gesture center:
//   - Inside the drum rows zone on mobile (Theme 3): scales dv.rowZoom.
//   - Inside the grid pane: scales the camera (the original behavior).
//   - Anywhere else: ignored.
func (g *Game) handleTouchPinch(centerX, centerY int, scale float64) {
	// Drum rows zone has priority on mobile so two-finger pinches over
	// the rack are absorbed (do not bleed into camera zoom). The legacy
	// "pinch to scale row height" behavior was retired alongside the
	// row-zoom chip refactor — per-row dimensions are fixed, and the
	// chips now resize the drum-view pane instead.
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

// handleTouchTwoFingerPan handles a two-finger pan gesture.
func (g *Game) handleTouchTwoFingerPan(deltaX, deltaY int) {
	g.cam.OffsetX += float64(deltaX)
	g.cam.OffsetY += float64(deltaY)
	g.cam.Snap()
}

// IsTouchActive returns true if any touch is currently active.
func (g *Game) IsTouchActive() bool {
	return globalTouchState.ActiveTouchCount() > 0
}
