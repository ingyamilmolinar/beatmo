package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─────────────── input handling ───────────────────────────────────────── */

func (g *Game) handleEditor() {
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	right := isMouseButtonPressed(ebiten.MouseButtonRight)
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)

	if g.split.dragging {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// Cancel editor state when multi-touch is active (pinch/pan).
	// A prior single-finger touch may have set pendingClick via the
	// touch-to-mouse override; transitioning to two fingers must not
	// be treated as a mouse release that completes the click.
	if globalTouchState.ActiveTouchCount() >= 2 {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// ESC closes node sidebar
	if g.sidebar.IsOpen() && isKeyPressed(ebiten.KeyEscape) {
		g.sidebar.Close()
		g.leftPrev = left
		return
	}

	// ESC cancels connect mode
	if g.connectMode && isKeyPressed(ebiten.KeyEscape) {
		g.cancelConnectMode()
		g.leftPrev = left
		return
	}

	// Move mode: handle placement and ESC
	if g.moveMode && g.movingNode != nil {
		x, y := cursorPosition()
		if g.moveConfirm {
			// Confirmation dialog: check for clicks on confirm/cancel buttons
			if left && !g.leftPrev {
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
			}
			g.leftPrev = left
			return
		}
		// ESC cancels move mode
		if isKeyPressed(ebiten.KeyEscape) {
			g.cancelMoveMode()
			g.leftPrev = left
			return
		}
		// Skip the mouse release that corresponds to the MOVE button press
		if g.moveSkipRelease {
			if !left {
				g.moveSkipRelease = false
			}
			g.leftPrev = left
			return
		}
		// Click release in grid places the node
		if !left && g.leftPrev && g.split.InGridPane(x, y) && y >= gridTopOffset() {
			wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
			wy := (float64(y-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
			_, _, ni, nj := g.grid.Snap(wx, wy)
			// Check if destination is occupied by another node
			if existing := g.nodeAt(ni, nj); existing != nil && existing.ID != g.movingNode.ID {
				g.cancelMoveMode()
				g.leftPrev = left
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
		}
		g.leftPrev = left
		return
	}

	// coords -> world
	x, y := cursorPosition()

	// Guard: a popup was closed recently (e.g., by handleTapInGrid processing
	// a gesture tap on the previous frame). The press/release state from the
	// same touch is stale — don't create nodes or set pendingClick.
	if g.sidebar.ClosedGuard() > 0 {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	if y < gridTopOffset() || !g.split.InGridPane(x, y) {
		g.pendingClick = false
		g.leftPrev = left
		return
	}
	wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(y-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	gx, gy, i, j := g.grid.Snap(wx, wy)

	// ---------------- delete node (right-click) ----------------
	if right && !shift && !left {
		if n := g.nodeAtScreen(x, y); n != nil {
			g.logger.Debugf("[GAME] Deleting node: %d at grid=(%d,%d)", n.ID, i, j)
			g.deleteNode(n)
		}
		return
	}

	// ---------------- link drag (shift held OR drag in progress) ----
	if g.linkDrag.active || shift {
		// For link drag, prioritize screen-hit for nodes.
		if left && !g.linkDrag.active && shift {
			if n := g.nodeAtScreen(x, y); n != nil {
				g.linkDrag = dragLink{from: n, active: true}
			}
		} else if g.linkDrag.active && !left {
			if n2 := g.nodeAtScreen(x, y); n2 != nil && n2 != g.linkDrag.from {
				tFrom := g.graph.Nodes[g.linkDrag.from.ID].Type
				tTo := g.graph.Nodes[n2.ID].Type
				if tFrom != model.NodeTypeInvisible && tTo != model.NodeTypeInvisible {
					if right {
						g.deleteEdge(g.linkDrag.from, n2)
					} else {
						g.addEdge(g.linkDrag.from, n2)
					}
				}
			}
			g.linkDrag = dragLink{}
		} else if g.linkDrag.active && left {
			g.linkDrag.toX, g.linkDrag.toY = gx, gy
		}
		return
	}

	// click handling based on press+release without drag
	if left && !g.leftPrev {
		g.clickI, g.clickJ = i, j
		g.clickNode = g.nodeAtScreen(x, y)
		g.pendingClick = true
		g.camDragged = false
		g.logger.Tracef("[INPUT/MOUSE] down screen=(%d,%d) grid=(%d,%d)", x, y, i, j)
		// Handle origin selection immediately when a row requested it and a visible
		// node is pressed.
		if g.pendingStartRow >= 0 {
			// Use screen hit to prioritize existing nodes under the cursor,
			// regardless of exact grid snapping.
			if n := g.nodeAtScreen(x, y); n != nil {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok && mn.Type != model.NodeTypeInvisible {
					row := g.pendingStartRow
					// Disallow selecting a node that belongs to a different circuit only
					// when that other circuit is currently audible during playback.
					if other, ok := g.nodeRows[n.ID]; ok && other != row {
						if g.Playing() && g.rowIsAudible(other) {
							// Keep selection active; ignore this click.
							g.pendingClick = false
							g.camDragged = false
							g.leftPrev = left
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
					g.pendingClick = false
					g.camDragged = false
					g.leftPrev = left
					return
				}
			}
		}
	}
	if !left && g.leftPrev {
		// Node menu button clicks are handled on press; fall through on release.
		if g.pendingClick && !g.camDragged {
			g.logger.Tracef("[INPUT/MOUSE] up screen=(%d,%d) grid=(%d,%d)", x, y, i, j)
			// If clicking over an existing node (screen hit), select and open menu
			if n := g.clickNode; n != nil {
				if g.sel != nil {
					g.sel.Selected = false
				}
				g.sel = n
				n.Selected = true
				g.sidebar.Open(n)
				g.computeSelNeighbors()
				g.coordBadgeNode = n
				g.coordBadgeFrame = g.frame
			} else {
				// Empty intersection → add/select regular node and close menu
				g.logger.Tracef("[INPUT/NODE] add/select grid=(%d,%d)", g.clickI, g.clickJ)
				n := g.tryAddNode(g.clickI, g.clickJ, model.NodeTypeRegular)
				if g.sel != n {
					if g.sel != nil {
						g.logger.Tracef("[INPUT/NODE] deselect grid=(%d,%d)", g.sel.I, g.sel.J)
						g.sel.Selected = false
					}
					g.logger.Tracef("[INPUT/NODE] select grid=(%d,%d)", n.I, n.J)
					g.sel = n
					n.Selected = true
					g.computeSelNeighbors()
				}
				g.sidebar.Close()
			}
		}
		g.pendingClick = false
		g.camDragged = false
	}
	if isKeyPressed(ebiten.KeyS) && g.sel != nil {
		if g.start != nil {
			g.start.Start = false
			g.logger.Debugf("[GAME] Unsetting start node: %d,%d", g.start.I, g.start.J)
		}
		g.start = g.sel
		g.start.Start = true
		g.graph.StartNodeID = g.sel.ID
		g.logger.Infof("[GAME] Setting start node: %d,%d", g.start.I, g.start.J)
		g.updateBeatInfos()
	}
	g.leftPrev = left
}

// handleNodeMenuButtons is retained as a compatibility shim for touch input.
// It delegates to the sidebar's HandleInput method.
func (g *Game) handleNodeMenuButtons(x, y int, left bool) bool {
	if !g.sidebar.IsOpen() || g.sidebar.Node() == nil {
		return false
	}
	result := g.sidebar.HandleInput(x, y, left)
	return result != InputIgnored
}
