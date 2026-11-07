package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─────────────── input handling ───────────────────────────────────────── */

// The ungated tier; the key logic lives in g.undoShortcut.
func (g *Game) handleUndoRedoKeys() bool {
	if g.keyboardRouter == nil {
		return false
	}
	return g.keyboardRouter.dispatchUngated()
}

func (g *Game) handleEditor() {
	// Undo/redo dispatch from Game.Update, ungated, never here: this path is cursor-gated, and
	// dispatching here too fires a chord twice.
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	right := isMouseButtonPressed(ebiten.MouseButtonRight)
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)

	if g.split.dragging {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// A prior single touch may have set pendingClick; going to two fingers is not a release.
	if globalTouchState.ActiveTouchCount() >= 2 {
		if g.marquee.active {
			g.marquee = marqueeDrag{}
		}
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// A grid-anchored portal owns input, so a click on it must never create or select a node behind it.
	if g.modalOverlayActive() {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// Shares the touch path's handler.
	if g.modes.Active() != GridModeNone {
		// The dialog's portal owns input.
		if g.breakConfirmActive() {
			g.leftPrev = left
			return
		}
		x, y := cursorPosition()
		// The release of the press that armed the mode.
		if g.moveSkipRelease {
			if !left {
				g.moveSkipRelease = false
			}
			g.leftPrev = left
			return
		}
		if !left && g.leftPrev && g.split.InGridPane(x, y) && y >= gridTopOffset() {
			g.handleArmedModeTap(x, y)
		}
		g.leftPrev = left
		return
	}

	// coords -> world
	x, y := cursorPosition()

	// A popup just closed, so this touch's press/release is stale. Shift bypasses: the sidebar closes
	// itself on shift+press and yields that press to the link/marquee branch, which never creates nodes.
	if g.sidebar.ClosedGuard() > 0 && !shift {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	if y < gridTopOffset() || !g.split.InGridPane(x, y) {
		// Cancel, not release: this early return skips handleMarquee, so a marquee left active would
		// freeze camera panning and replay a stale release later (a phantom group).
		if g.marquee.active {
			g.marquee = marqueeDrag{}
		}
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
			g.logger.Debugf("[game] Deleting node: %d at grid=(%d,%d)", n.ID, i, j)
			g.deleteNode(n)
		}
		return
	}

	// ---------------- link drag / marquee (shift held OR either in progress) ----
	if g.linkDrag.active || g.marquee.active || shift {
		// Gated on the press EDGE: Shift pressed mid-drag must not arm a link/marquee from the already-
		// dragged position.
		if left && !g.leftPrev && !g.linkDrag.active && !g.marquee.active && shift {
			if n := g.nodeAtScreen(x, y); n != nil {
				g.linkDrag = dragLink{from: n, active: true}
			} else if gid, ok := g.groupBadgeAt(x, y); ok {
				// A press on a badge never arms the marquee, even with Shift.
				g.sidebar.Close()
				g.openGroupMenuForBadge(gid, x, y)
				g.leftPrev = left
				return
			} else if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				// Honour the crosshair's snap like a plain click does, or shift+click "on" a zoomed-out node arms
				// the marquee. Ranked below the badge.
				g.linkDrag = dragLink{from: gt.Node, active: true}
			} else {
				// Marquee claims Shift+press on EMPTY space.
				g.handleMarquee(left, x, y)
				g.leftPrev = left
				return
			}
		} else if g.linkDrag.active && !left {
			n2 := g.nodeAtScreen(x, y)
			if n2 == nil {
				// The same snap the press honours.
				if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
					n2 = gt.Node
				}
			}
			if n2 != nil && n2 != g.linkDrag.from {
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
			// Locks onto the snapped node, so the user sees what the release connects to.
			if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				g.linkDrag.toX, g.linkDrag.toY = gt.Node.X, gt.Node.Y
			} else {
				g.linkDrag.toX, g.linkDrag.toY = gx, gy
			}
		} else if g.marquee.active {
			// Marquee drag continuing, or its release frame.
			g.handleMarquee(left, x, y)
			g.leftPrev = left
			return
		}
		// Keep leftPrev in sync so the next release edge is detected and no stale left=true leaks into
		// a later gesture.
		g.leftPrev = left
		return
	}

	// click handling based on press+release without drag
	if left && !g.leftPrev {
		g.clickI, g.clickJ = i, j
		g.clickNode = g.nodeAtScreen(x, y)
		g.pendingClick = true
		g.camDragged = false
		g.holdFrames = 0
		g.holdStartX, g.holdStartY = x, y
		g.logger.Tracef("[input/mouse] down screen=(%d,%d) grid=(%d,%d)", x, y, i, j)
		if g.pendingStartRow >= 0 {
			// Screen hit first, so an existing node wins regardless of grid snapping.
			if n := g.nodeAtScreen(x, y); n != nil {
				if mn, ok := g.graph.GetNodeByID(n.ID); ok && mn.Type != model.NodeTypeInvisible {
					row := g.pendingStartRow
					// Only refused while that other circuit is audible during playback.
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

	// !touchOverrideActive, so this and GestureLongPress never both fire on one touch.
	if left && g.leftPrev && g.pendingClick && g.clickNode != nil && !touchOverrideActive {
		if g.camDragged || abs(x-g.holdStartX) > tapMaxMovePx || abs(y-g.holdStartY) > tapMaxMovePx {
			g.holdFrames = -1 // moved: hold is dead for this press
		} else if g.holdFrames >= 0 {
			g.holdFrames++
			if g.holdFrames >= mouseHoldPopupFrames {
				g.holdFrames = -1
				g.pendingClick = false // the hold consumed this press: no click on release
				g.showLongPressPopup(g.clickNode, x, y)
				g.leftPrev = left
				return
			}
		}
	}
	if !left && g.leftPrev {
		// Node menu button clicks are handled on press; fall through on release.
		if g.pendingClick && !g.camDragged {
			g.logger.Tracef("[input/mouse] up screen=(%d,%d) grid=(%d,%d)", x, y, i, j)
			// Before clickNode: a badge release opens its menu, never a node action.
			if gid, ok := g.groupBadgeAt(x, y); ok {
				g.sidebar.Close()
				g.openGroupMenuForBadge(gid, x, y)
			} else if n := g.clickNode; n != nil {
				g.selectNodeOpenSidebar(n)
				g.groupMenu.Close()
			} else if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				g.selectNodeOpenSidebar(gt.Node)
				g.groupMenu.Close()
			} else {
				// stitchEdgesAt splits any edge through the commit cell.
				g.logger.Tracef("[input/node] add/select grid=(%d,%d)", gt.I, gt.J)
				n := g.tryAddNode(gt.I, gt.J, model.NodeTypeRegular)
				if g.sel != n {
					if g.sel != nil {
						g.logger.Tracef("[input/node] deselect grid=(%d,%d)", g.sel.I, g.sel.J)
						g.sel.Selected = false
					}
					g.logger.Tracef("[input/node] select grid=(%d,%d)", n.I, n.J)
					g.sel = n
					n.Selected = true
					g.computeSelNeighbors()
				}
				g.sidebar.Close()
				g.groupMenu.Close()
			}
		}
		g.pendingClick = false
		g.camDragged = false
	}
	if isKeyPressed(ebiten.KeyS) && g.sel != nil {
		if g.start != nil {
			g.start.Start = false
			g.logger.Debugf("[game] Unsetting start node: %d,%d", g.start.I, g.start.J)
		}
		g.start = g.sel
		g.start.Start = true
		g.graph.StartNodeID = g.sel.ID
		g.logger.Debugf("[game] setting start node: %d,%d", g.start.I, g.start.J)
		g.updateBeatInfos()
	}
	g.leftPrev = left
}

func (g *Game) handleNodeMenuButtons(x, y int, left bool) bool {
	if !g.sidebar.IsOpen() || g.sidebar.Node() == nil {
		return false
	}
	result := g.sidebar.HandleInput(x, y, left)
	return result != InputIgnored
}
