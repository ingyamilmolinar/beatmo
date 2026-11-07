package ui

import (
	"image"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Read once, never per frame; tests may override them.
var (
	envRenderSafe      = os.Getenv("RENDER_SAFE") == "1"
	envScreenEdges     = os.Getenv("SCREEN_EDGES") == "1"
	envNoGridDraw      = os.Getenv("NO_GRID_DRAW") == "1"
	envNoGridTileCache = os.Getenv("NO_GRID_TILE_CACHE") == "1"
	envNoPixelSnap     = os.Getenv("NO_PIXEL_SNAP") == "1"
	envNoEdgeCache     = os.Getenv("NO_EDGE_CACHE") == "1"
	envNoSpriteNodes   = os.Getenv("NO_SPRITE_NODES") == "1"
)

// Dispatch only: computes the frame's setup once into gridDrawCtx/gridDrawScreen, then the tree draws.
func (g *Game) drawGridPane(screen *ebiten.Image) {
	gridRect := g.split.GridRect(g.winW, g.winH)
	var top *ebiten.Image
	if g.gridPaneSubParent == screen && g.gridPaneSubRect == gridRect && g.gridPaneSub != nil {
		top = g.gridPaneSub
	} else {
		top = screen.SubImage(gridRect).(*ebiten.Image)
		g.gridPaneSubParent = screen
		g.gridPaneSubRect = gridRect
		g.gridPaneSub = top
	}
	// Always the same subimage, so cached and direct draws match.
	dst := top
	// Render mode flags
	renderSafe := envRenderSafe
	screenEdges := renderSafe || RuntimeProf().ScreenEdgesDefault || envScreenEdges || g.simpleDraw

	// The whole ctx is filled before any branch reads it, so no draw sees a partial context.
	ctx := &g.gridDrawCtx
	*ctx = gridPaneDrawCtx{}
	ctx.gridRect = gridRect
	ctx.renderSafe = renderSafe
	ctx.screenEdges = screenEdges
	g.gridDrawScreen = screen

	// camera matrix for world drawings (shift down by bar height)
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	if envNoPixelSnap {
		offX = g.cam.OffsetX
		offY = g.cam.OffsetY
	}
	camScale := unitPx / g.grid.Unit()
	var cam ebiten.GeoM
	cam.Scale(camScale, camScale)
	cam.Translate(offX, offY+float64(gridTopOffset()))
	// World coordinates snap to whole pixels so nodes and edges stay aligned with the grid tiles.

	// Detect fast pan to gate expensive UI ensures (browser only by default).
	if RuntimeProf().FastPanDetect {
		dx := math.Abs(offX - g.lastCamOffX)
		dy := math.Abs(offY - g.lastCamOffY)
		if dx+dy > 2 {
			g.panFastFrames = 6
		} else if g.panFastFrames > 0 {
			g.panFastFrames--
		}
		g.lastCamOffX = offX
		g.lastCamOffY = offY
	}

	// Visible world rect for culling
	minX, maxX, minY, maxY := visibleWorldRect(g.cam, g.split.GridW(g.winW), g.split.GridH(g.winH))

	// reset draw counters for this pass
	g.lastDrawEdges, g.lastDrawNodes, g.lastDrawPulses = 0, 0, 0

	// Edge-style params (shared by edges + pulses).
	sigStyle := SignalUI
	sigStyle.Radius = float32(g.grid.SignalRadius(g.cam.Scale))
	edgeThick := g.grid.EdgeThickness(g.cam.Scale)
	arrow := g.grid.EdgeArrowSize()
	if RuntimeProf().DisableEdgeArrows {
		arrow = 0
	}
	// Skip tiny arrowheads at low zoom to save draw calls in web builds.
	apx := arrow * camScale
	if apx < 2 {
		arrow = 0
	}

	// Stash the remaining camera-derived locals for the drawGrid* sub-methods.
	ctx.unitPx = unitPx
	ctx.offX = offX
	ctx.offY = offY
	ctx.camScale = camScale
	ctx.cam = cam
	ctx.minX, ctx.maxX, ctx.minY, ctx.maxY = minX, maxX, minY, maxY
	ctx.sigStyle = sigStyle
	ctx.edgeThick = edgeThick
	ctx.arrow = arrow

	// g.frame only advances while playing, so it cannot key the memo when stopped.
	g.guideMemoFrame = -1

	if g.logDrawNodes {
		// Debug-only: run the background now (ctx is fully populated above) to
		// fill ctx.phase*/tile* for the FRAME trace. In production the tree's
		// grid-bg layer is the sole background draw — see the tree dispatch below.
		g.drawGridBackground(dst)
		phaseX, phaseY, tileW, tileH := ctx.phaseX, ctx.phaseY, ctx.tileW, ctx.tileH
		ds := 1.0
		g.logger.Tracef("[draw/cam] frame=%d scale=%.4f off=(%.0f,%.0f) unitPx=%.2f stepPx=%d camScale=%.6f splitY=%d dscale=%.2f", g.frame, g.cam.Scale, offX, offY, unitPx, g.grid.StepPixels(g.cam.Scale), camScale, g.split.Y, ds)
		// One-line frame state summary for real-time debugging, include modes.
		mode := ""
		if renderSafe {
			mode += " RENDER_SAFE"
		}
		if screenEdges && !renderSafe {
			mode += " SCREEN_EDGES"
		}
		if envNoPixelSnap {
			mode += " NO_PIXEL_SNAP"
		}
		g.logger.Tracef("[frame/scene] f=%d nodes=%d edges=%d pulses=%d tile=(%d,%d) phase=(%d,%d) cam=(%.3f,%.0f,%.0f)%s", g.frame, len(g.nodes), len(g.edges), len(g.activePulses), tileW, tileH, phaseX, phaseY, g.cam.Scale, offX, offY, mode)
	}

	// Ascending z, so the sidebar and cursor label composite over the coordinate badge;
	// clip=false layers get the unclipped screen.
	g.gridTree.SetBounds(gridRect)
	g.gridTree.Draw(screen)

	// Divider drawn after both panes
}

// Below this radius the desaturated body alone carries the Muted state.
const nodeStateSlashMinPx = 10

// Smaller chord nodes get three ticks instead.
const nodeChordGlyphMinPx = 9

// Mirrors the base loop's culling and colors so the overlay registers on the body.
func (g *Game) drawNodeStateOverlays(dst *ebiten.Image, minX, maxX, minY, maxY float64, gridW, gridH int) {
	nodeStyle := NodeUI
	for nodeIdx, n := range g.nodes {
		nodeInfo, ok := g.graph.Nodes[n.ID]
		if !ok {
			continue
		}
		nt := nodeInfo.Type
		if nt.Fires() && nt != model.NodeTypeChord {
			continue // a plain regular node has no state overlay
		}
		if nodeIdx < len(g.nodeRadiiCache) {
			rWorld := g.nodeRadiiCache[nodeIdx]
			if n.X+rWorld < minX || n.X-rWorld > maxX || n.Y+rWorld < minY || n.Y-rWorld > maxY {
				continue
			}
		}
		sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
		if sx2 < 0 || sx1 >= float64(gridW) || sy2 < 0 || sy1 >= float64(gridH) {
			continue
		}
		fillCol := nodeStyle.Fill
		borderCol := nodeStyle.Border
		if rowIdx, rowOK := g.nodeRows[n.ID]; rowOK && rowIdx >= 0 && rowIdx < len(g.drum.Rows) {
			base := g.drum.Rows[rowIdx].Color
			if n.Start {
				fillCol = adjustColor(base, 40)
			} else {
				fillCol = base
			}
			borderCol = adjustColor(base, 80)
		}
		g.drawNodeStateOverlay(dst, sx1, sy1, sx2, sy2, nt, fillCol, borderCol)
	}
}

// Drawn after the base body in screen space (the sprite key ignores the type), so it works on
// every draw path. Invisible nodes have no base draw: this is their only render.
func (g *Game) drawNodeStateOverlay(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64, nt model.NodeType, fillCol, borderCol color.Color) {
	rPx := int(math.Round((sx2 - sx1) * 0.5))
	if rPx < 1 {
		rPx = 1
	}
	cx := int(math.Round((sx1 + sx2) * 0.5))
	cy := int(math.Round((sy1 + sy2) * 0.5))
	rect := image.Rect(cx-rPx, cy-rPx, cx+rPx, cy+rPx)
	if rect.Empty() {
		return
	}
	switch nt {
	case model.NodeTypeChord:
		// The border tone reads on the row-colored fill.
		if rPx >= nodeChordGlyphMinPx {
			g := rPx * 3 / 2
			DrawIcon(dst, IconChord, image.Rect(cx-g/2, cy-g/2, cx+g-g/2, cy+g-g/2), borderCol)
		} else {
			// The same mark a chord cell carries in the drum view.
			w := rPx
			if w < 2 {
				w = 2
			}
			for _, dy := range [3]int{-2, 0, 2} {
				drawRect(dst, image.Rect(cx-w/2, cy+dy, cx-w/2+w, cy+dy+1), borderCol, true)
			}
		}
	case model.NodeTypeMute:
		// Desaturate the body with a translucent fill scrim, then re-edge.
		drawRect(dst, rect, WithAlphaFromColor(fillCol, AlphaMedium), true)
		drawRect(dst, rect, borderCol, false)
		if rPx >= nodeStateSlashMinPx {
			var idm ebiten.GeoM
			DrawLineCam(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y), &idm, colMuteCell, float64(genGeomHighlightBorderThickness))
		}
	case model.NodeTypeSilent:
		inner := image.Rect(rect.Min.X+1, rect.Min.Y+1, rect.Max.X-1, rect.Max.Y-1)
		if !inner.Empty() {
			drawRect(dst, inner, colBGTop, true)
		}
		drawRect(dst, rect, borderCol, false)
	case model.NodeTypeInvisible:
		// Ghosted: faint body + dashed/dotted subtle outline.
		drawRect(dst, rect, WithAlphaFromColor(fillCol, AlphaFaint), true)
		ghostBorder := WithAlphaFromColor(borderCol, AlphaSubtle)
		if rPx >= 4 {
			drawDashedRoundedBorder(dst, rect, ghostBorder, 0)
		} else {
			drawRect(dst, rect, ghostBorder, false)
		}
	}
}

// A node highlights in its own color: rowless nodes use the border, since their dark fill would
// not read as a halo.
func (g *Game) nodeOwnHighlightColor(rowIdx int, rowOK bool) color.Color {
	if rowOK && rowIdx >= 0 && rowIdx < len(g.drum.Rows) {
		return g.drum.Rows[rowIdx].Color
	}
	return genColorNodeBorder
}

func (g *Game) drawNodeSelectionHalo(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64) {
	rPx := int(math.Round((sx2 - sx1) * 0.5))
	if rPx < 1 {
		rPx = 1
	}
	cx := int(math.Round((sx1 + sx2) * 0.5))
	cy := int(math.Round((sy1 + sy2) * 0.5))
	halo := WithAlpha(colAccent, AlphaSubtle)
	// Four strips around the rect, so the body stays uncovered.
	const grow = 4
	inX0, inY0 := cx-rPx, cy-rPx
	inX1, inY1 := cx+rPx, cy+rPx
	outX0, outY0 := inX0-grow, inY0-grow
	outX1, outY1 := inX1+grow, inY1+grow
	// Top, bottom, left, right bands.
	drawRect(dst, image.Rect(outX0, outY0, outX1, inY0), halo, true)
	drawRect(dst, image.Rect(outX0, inY1, outX1, outY1), halo, true)
	drawRect(dst, image.Rect(outX0, inY0, inX0, inY1), halo, true)
	drawRect(dst, image.Rect(inX1, inY0, outX1, inY1), halo, true)
}

// Grown past the selection ring so both show without overlap; larger ringGrow values nest
// several group rings concentrically.
func (g *Game) drawNodeGroupRing(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64, ringCol color.Color, ringGrow int) {
	x1, y1 := sx1-float64(ringGrow), sy1-float64(ringGrow)
	x2, y2 := sx2+float64(ringGrow), sy2+float64(ringGrow)
	var idm ebiten.GeoM
	DrawLineCam(dst, x1, y1, x2, y1, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x2, y1, x2, y2, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x2, y2, x1, y2, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x1, y2, x1, y1, &idm, ringCol, float64(genGeomHighlightBorderThickness))
}

const (
	groupExternalRingStep1 = 3
	groupExternalRingStep2 = 6
)

// Always strictly outside the membership rings (outermostGrow is the last ring's grow).
func (g *Game) drawNodeExternalHighlightRing(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64, gid model.GroupID, outermostGrow int) {
	col := g.groupColor(gid)
	var idm ebiten.GeoM

	grow1 := outermostGrow + groupExternalRingStep1
	x1, y1 := sx1-float64(grow1), sy1-float64(grow1)
	x2, y2 := sx2+float64(grow1), sy2+float64(grow1)
	strong := WithAlpha(col, AlphaStrong)
	DrawLineCam(dst, x1, y1, x2, y1, &idm, strong, 2)
	DrawLineCam(dst, x2, y1, x2, y2, &idm, strong, 2)
	DrawLineCam(dst, x2, y2, x1, y2, &idm, strong, 2)
	DrawLineCam(dst, x1, y2, x1, y1, &idm, strong, 2)

	grow2 := outermostGrow + groupExternalRingStep2
	x1b, y1b := sx1-float64(grow2), sy1-float64(grow2)
	x2b, y2b := sx2+float64(grow2), sy2+float64(grow2)
	medium := WithAlpha(col, AlphaMedium)
	DrawLineCam(dst, x1b, y1b, x2b, y1b, &idm, medium, 1)
	DrawLineCam(dst, x2b, y1b, x2b, y2b, &idm, medium, 1)
	DrawLineCam(dst, x2b, y2b, x1b, y2b, &idm, medium, 1)
	DrawLineCam(dst, x1b, y2b, x1b, y1b, &idm, medium, 1)
}

func groupIDsContain(ids []model.GroupID, gid model.GroupID) bool {
	for _, id := range ids {
		if id == gid {
			return true
		}
	}
	return false
}

func (g *Game) computeNodeGraphSig() uint64 {
	s := uint64(len(g.nodes))
	for _, n := range g.nodes {
		ni, ok := g.graph.Nodes[n.ID]
		if !ok {
			continue
		}
		v := uint64(n.I)<<32 | uint64(uint16(n.J))<<16 | uint64(ni.Type)<<8
		if n.Start {
			v |= 1
		}
		s = (s*1469598103934665603 ^ v) * 1099511628211
		if row, rok := g.nodeRows[n.ID]; rok && row >= 0 && row < len(g.drum.Rows) {
			cr := color.RGBAModel.Convert(g.drum.Rows[row].Color).(color.RGBA)
			cv := uint64(cr.R)<<24 | uint64(cr.G)<<16 | uint64(cr.B)<<8 | uint64(cr.A)
			s = (s*1469598103934665603 ^ cv) * 1099511628211
		}
	}
	return s
}
