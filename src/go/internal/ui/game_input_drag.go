package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// enqueueUI schedules a UI-side action to be applied after input handling in Update.
func (g *Game) enqueueUI(fn func()) {
	if fn != nil {
		g.uiQueue = append(g.uiQueue, fn)
	}
}

// blocksAt reports whether any UI overlay blocks interaction at (x,y).
func (g *Game) blocksAt(x, y int) bool {
	if image.Pt(x, y).In(g.gridHelpButtonRect()) {
		return true
	}
	if g.drum != nil && g.drum.BlocksAt(x, y) {
		return true
	}
	// Node popup does not block editor handling; zoom +/- buttons are disabled.
	return false
}

func (g *Game) handleLinkDrag(left, right bool, gx, gy float64, i, j int) {
	shift := isKeyPressed(ebiten.KeyShiftLeft) ||
		isKeyPressed(ebiten.KeyShiftRight)

	// start drag
	if left && !g.linkDrag.active && shift {
		if n := g.nodeAt(i, j); n != nil {
			g.logger.Debugf("[game] Start link drag: node=%d at grid=(%d,%d)", n.ID, n.I, n.J)
			g.linkDrag = dragLink{from: n, active: true}
		}
	}
	// update preview
	if g.linkDrag.active && left {
		g.linkDrag.toX, g.linkDrag.toY = gx, gy
		return
	}
	// release → commit or delete (only between visible nodes)
	if g.linkDrag.active && !left {
		if n2 := g.nodeAt(i, j); n2 != nil && n2 != g.linkDrag.from {
			tFrom := g.graph.Nodes[g.linkDrag.from.ID].Type
			tTo := g.graph.Nodes[n2.ID].Type
			if tFrom != model.NodeTypeInvisible && tTo != model.NodeTypeInvisible {
				if right {
					g.logger.Debugf("[game] Deleting edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
					g.deleteEdge(g.linkDrag.from, n2)
				} else {
					g.logger.Debugf("[game] Adding edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
					g.addEdge(g.linkDrag.from, n2)
				}
			} else {
				g.logger.Debugf("[game] Ignoring link to invisible node at grid=(%d,%d)", i, j)
			}
		}
		g.logger.Debugf("[game] End link drag at grid=(%d,%d)", i, j)
		g.linkDrag = dragLink{}
	}
}

// menuHit reports whether a screen-space point lies within the node sidebar.
func (g *Game) menuHit(x, y int) bool {
	return g.sidebar.Hit(x, y) || image.Pt(x, y).In(g.gridHelpButtonRect())
}

// modalOverlayActive reports whether a blocking (modal/scrim) overlay — such as
// the mobile synth-knob wheel popup or a volume popup — is open in the drum
// view's input trees. The out-of-tree Game-level gesture/camera handlers consult
// this so a gesture over the scrim never leaks to the grid, camera, or timeline
// beneath; the overlay closes via the tree's click-outside / Esc path instead.
func (g *Game) modalOverlayActive() bool {
	return g.drum != nil && g.drum.PortalHasBlocking()
}

func (g *Game) spawnPulseFromRow(row, start int) {
	g.logger.Tracef("[pulse] spawn row=%d from=%d", row, start)
	if row < 0 || row >= len(g.beatInfosByRow) {
		return
	}
	path := g.beatInfosByRow[row]
	if len(path) == 0 {
		g.logger.Debugf("[game] spawn pulse: no beat information available for row %d", row)
		return
	}
	curIdxWrapped := g.wrapBeatIndexRow(row, start)
	beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
	fromBeatInfo := path[curIdxWrapped]
	g.nextBeatIdxs[row] = start
	idx := g.nextBeatIdxs[row]
	g.highlightBeat(row, idx, fromBeatInfo, beatDuration)
	if row == 0 {
		g.setPrimaryStep(idx)
	}
	g.nextBeatIdxs[row] = idx + 1
	nextInfo := g.beatInfoAtRow(row, start+1)
	nextIdxWrapped := g.wrapBeatIndexRow(row, start+1)
	unit := g.grid.Unit()
	x1 := float64(fromBeatInfo.I) * unit
	y1 := float64(fromBeatInfo.J) * unit
	x2 := float64(nextInfo.I) * unit
	y2 := float64(nextInfo.J) * unit
	dist := hypot(x2-x1, y2-y1)
	beats := dist / g.grid.Step
	if beats <= 0 {
		beats = 1
	}
	p := &pulse{
		x1:           x1,
		y1:           y1,
		x2:           x2,
		y2:           y2,
		speed:        float64(g.grid.MaxDiv()) / float64(beatDuration),
		fromBeatInfo: fromBeatInfo,
		toBeatInfo:   nextInfo,
		pathIdx:      nextIdxWrapped,
		lastIdx:      start,
		from:         g.nodeByID(fromBeatInfo.NodeID),
		to:           g.nodeByID(nextInfo.NodeID),
		path:         path,
		row:          row,
		segBeats:     beats,
	}
	g.activePulses = append(g.activePulses, p)
	if row == 0 {
		g.activePulse = p
	}
}
