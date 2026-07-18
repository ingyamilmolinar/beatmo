package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

/* ─────────────── input handling ───────────────────────────────────────── */

// handleUndoRedoKeys dispatches global undo/redo keyboard shortcuts:
// Ctrl/Cmd+Z = undo, Ctrl/Cmd+Shift+Z and Ctrl/Cmd+Y = redo. It is
// edge-triggered (just-pressed) so a held chord fires once. Returns true if a
// key was consumed. Works on desktop (Control) and browser/macOS (Meta/Cmd).
// handleUndoRedoKeys dispatches the ungated shortcut tier (undo/redo). Kept as
// the Game.Update entry point and bool-returning test anchor; the actual key
// logic lives in the undo node (g.undoShortcut, keyboard_shortcuts.go). Returns
// whether an ungated node claimed the frame.
func (g *Game) handleUndoRedoKeys() bool {
	if g.keyboardRouter == nil {
		return false
	}
	return g.keyboardRouter.dispatchUngated()
}

func (g *Game) handleEditor() {
	// Undo/redo shortcuts are dispatched unconditionally from Game.Update (so
	// they work over panels/popups/during drags), NOT here — handleEditor is
	// cursor-gated. Do not re-add the dispatch here or a single chord fires twice.
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
		if g.marquee.active {
			g.marquee = marqueeDrag{}
		}
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// Armed grid mode: mouse release in the grid places/cancels via the same
	// shared handler the touch path uses.
	if g.modes.Active() != GridModeNone {
		// While the break-circuit dialog is open the portal owns input;
		// grid releases must not place/cancel behind it.
		if g.breakConfirmActive() {
			g.leftPrev = left
			return
		}
		x, y := cursorPosition()
		// Skip the mouse release that corresponds to the popup/sidebar button
		// press that armed the mode.
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

	// Guard: a popup was closed recently (e.g., by handleTapInGrid processing
	// a gesture tap on the previous frame). The press/release state from the
	// same touch is stale — don't create nodes or set pendingClick. Shift
	// gestures bypass the guard: the sidebar deliberately closes itself on a
	// shift+press and yields that SAME press to the link-drag/marquee block
	// below, which never creates nodes (see NodeSidebar.HandleInput).
	if g.sidebar.ClosedGuard() > 0 && !shift {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	if y < gridTopOffset() || !g.split.InGridPane(x, y) {
		// A marquee drag/release that leaves the grid pane must be cancelled
		// here, NOT left dangling: this early return sits before the
		// shift/marquee branch below, so handleMarquee's release path never
		// runs while the cursor is outside the pane. Left unattended,
		// g.marquee.active stays true (dead camera pan — panOK gates on it
		// globally) and re-entering the grid pane with the button up replays
		// a stale release against frozen start/cur coords, potentially
		// creating a phantom group. This is a cancel, not a release: no
		// group creation, no menu open.
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
		// For link drag, prioritize screen-hit for nodes (verbatim from the
		// pre-marquee implementation — only the trailing "else" below, taken
		// when the press lands on empty space, is new).
		// Gated on the press EDGE (left && !g.leftPrev): without it, a plain
		// left-drag already in progress (e.g. camera pan) that then has Shift
		// pressed mid-hold would arm a link-drag/marquee here using the
		// CURRENT (already-dragged) cursor position instead of a fresh press
		// position, corrupting the gesture.
		if left && !g.leftPrev && !g.linkDrag.active && !g.marquee.active && shift {
			if n := g.nodeAtScreen(x, y); n != nil {
				g.linkDrag = dragLink{from: n, active: true}
			} else if gid, ok := g.groupBadgeAt(x, y); ok {
				// Badge click behavior wins even with Shift held — a press
				// landing on a group badge must never arm the marquee (or
				// create a group from the gesture).
				g.sidebar.Close()
				g.openGroupMenuForBadge(gid, x, y)
				g.leftPrev = left
				return
			} else if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				// Guide-snapped node: the crosshair is visibly locked onto a
				// node (the press sits inside the snap radius but outside the
				// small sprite rect — the norm when zoomed out on a large
				// circuit). The connect gesture must honor the same snap a
				// plain click does, or shift+click "on" a node arms the
				// marquee instead of the helper arrow. Ranked below the badge
				// so badge presses keep winning.
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
				// Release inside a node's snap annulus connects to it — the
				// same guide snap the press honors.
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
			// Arrow tip locks onto a guide-snapped destination node so the
			// user sees which node the release will connect to; otherwise it
			// follows the grid-snapped cursor as before.
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
		// Fallthrough (shift held / continuing, but neither linkDrag nor
		// marquee got armed — e.g. a badge press-return above, which
		// deliberately arms neither): keep leftPrev in sync so the release
		// edge on the NEXT frame is still detected correctly by whichever
		// branch runs then, instead of leaking a stale left=true into a
		// later, unrelated gesture.
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

	// Desktop mouse-hold on a node → quick-action popup (touch reaches the
	// same popup via GestureLongPress; gate on !touchOverrideActive so the
	// two detectors never double-fire on one touch).
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
			// Badge hit (checked BEFORE clickNode handling): open that
			// group's menu instead of treating the release as a node
			// click/add-node — never creates a node, never opens the
			// sidebar.
			if gid, ok := g.groupBadgeAt(x, y); ok {
				g.sidebar.Close()
				g.openGroupMenuForBadge(gid, x, y)
			} else if n := g.clickNode; n != nil {
				g.selectNodeOpenSidebar(n)
				g.groupMenu.Close()
			} else if gt := g.guideTargetAt(x, y); gt.Kind == GuideNode {
				// Guide snapped to an existing node: select it (same as a
				// direct node click) instead of creating a neighbor.
				g.selectNodeOpenSidebar(gt.Node)
				g.groupMenu.Close()
			} else {
				// Empty intersection → add/select at the guide's commit cell
				// (edge-fraction cell or free cell); stitchEdgesAt splits any
				// edge passing through it.
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

// handleNodeMenuButtons dispatches a touch-input press/release to the open
// node sidebar, guarding on the sidebar being open with a node selected.
// Returns true if the sidebar consumed the input.
func (g *Game) handleNodeMenuButtons(x, y int, left bool) bool {
	if !g.sidebar.IsOpen() || g.sidebar.Node() == nil {
		return false
	}
	result := g.sidebar.HandleInput(x, y, left)
	return result != InputIgnored
}
