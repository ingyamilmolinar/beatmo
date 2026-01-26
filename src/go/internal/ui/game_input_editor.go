package ui

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
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

	// coords -> world
	x, y := cursorPosition()
	if y < topOffset || y >= g.split.Y {
		g.pendingClick = false
		g.leftPrev = left
		return
	}
	wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(y-topOffset) - g.cam.OffsetY) / g.cam.Scale
	gx, gy, i, j := g.grid.Snap(wx, wy)

	// Node popup buttons: unified handling via Button components
	if g.nodeMenuOpen && g.nodeMenuNode != nil {
		if g.handleNodeMenuButtons(x, y, left) {
			g.leftPrev = left
			return
		}
		// If the click is inside the popup panel but not on a button, swallow
		// the event to avoid creating grid nodes underneath.
		if g.menuHit(x, y) {
			g.leftPrev = left
			return
		}
	}

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
				g.nodeMenuOpen = true
				g.nodeMenuNode = n
				g.computeSelNeighbors()
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
				g.nodeMenuOpen = false
				g.nodeMenuNode = nil
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

// handleNodeMenuButtons processes clicks on node popup controls with a
// deterministic z-ordered hit test so visually topmost controls receive input.
func (g *Game) handleNodeMenuButtons(x, y int, left bool) bool {
	g.updateNodeMenuRects()
	if g.nodeMenuBtns == nil {
		return false
	}
	// Build an ordered list of control ids with menu-open items at higher z.
	order := make([]string, 0, len(g.nodeMenuRects))
	if g.nodeGrooveOpen {
		// Groove dropdown items first (topmost)
		for id := range g.nodeMenuRects {
			if strings.HasPrefix(id, "groove:") {
				order = append(order, id)
			}
		}
	}
	if g.nodeLogicOpen {
		// Dropdown items first (topmost)
		for id := range g.nodeMenuRects {
			if strings.HasPrefix(id, "logic:") {
				order = append(order, id)
			}
		}
	}
	// Parameter +/- next (always interactive when visible)
	order = append(order, "ln-", "ln+", "lp-", "lp+", "gp-", "gp+")
	// Then the logic button itself
	order = append(order, "logic", "grv")
	// Other controls beneath
	order = append(order, "vol-", "vol+", "pit-", "pit+", "dur-", "dur+", "aud")

	// Close logic dropdown when open and clicking outside dropdown and logic
	if g.nodeLogicOpen && left && !g.leftPrev {
		inside := false
		if r, ok := g.nodeMenuRects["logic"]; ok && image.Pt(x, y).In(r) {
			inside = true
		}
		for id, r := range g.nodeMenuRects {
			if strings.HasPrefix(id, "logic:") && image.Pt(x, y).In(r) {
				inside = true
				break
			}
		}
		if !inside {
			g.nodeLogicOpen = false
		}
	}
	// Close groove dropdown similarly
	if g.nodeGrooveOpen && left && !g.leftPrev {
		inside := false
		if r, ok := g.nodeMenuRects["grv"]; ok && image.Pt(x, y).In(r) {
			inside = true
		}
		for id, r := range g.nodeMenuRects {
			if strings.HasPrefix(id, "groove:") && image.Pt(x, y).In(r) {
				inside = true
				break
			}
		}
		if !inside {
			g.nodeGrooveOpen = false
		}
	}

	// Hit test in order
	for _, id := range order {
		btn, ok := g.nodeMenuBtns[id]
		if !ok {
			continue
		}
		r, ok := g.nodeMenuRects[id]
		if !ok {
			continue
		}
		btn.SetRect(r)
		if btn.Handle(x, y, left) {
			return true
		}
	}
	return false
}

// handleNodeMenuClick processes a click on the property popup controls if
// present. Returns true when the click was consumed.
// handleNodeMenuClick removed in favor of unified Button handling.
