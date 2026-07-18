package ui

import (
	"image"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Cached environment variables read once at init to avoid per-frame syscalls.
// Tests in the same package can override these directly.
var (
	envRenderSafe      = os.Getenv("RENDER_SAFE") == "1"
	envScreenEdges     = os.Getenv("SCREEN_EDGES") == "1"
	envNoGridDraw      = os.Getenv("NO_GRID_DRAW") == "1"
	envNoGridTileCache = os.Getenv("NO_GRID_TILE_CACHE") == "1"
	envNoPixelSnap     = os.Getenv("NO_PIXEL_SNAP") == "1"
	envNoEdgeCache     = os.Getenv("NO_EDGE_CACHE") == "1"
	envNoSpriteNodes   = os.Getenv("NO_SPRITE_NODES") == "1"
)

// drawGridPane is dispatch-only: it computes the per-frame side-effecting setup
// exactly once (the grid-pane subimage, camera matrix, culling rect, edge-style
// params, FastPanDetect counter mutation, draw-stat reset, FRAME trace log),
// stashes the derived values into g.gridDrawCtx + g.gridDrawScreen, then calls
// the focused drawGrid* sub-methods (grid_pane_draw.go) IN THE ORIGINAL ORDER so
// rendered pixels stay byte-identical to the pre-split inlined version.
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
	// Always draw top‑pane content into the same subimage to avoid any
	// compositor/target disparities between cached and direct draws.
	dst := top
	// Render mode flags
	renderSafe := envRenderSafe
	screenEdges := renderSafe || RuntimeProf().ScreenEdgesDefault || envScreenEdges || g.simpleDraw

	// Stash the transient per-frame draw context for the drawGrid* sub-methods.
	// gridRect / render-mode flags first; the camera-derived locals are filled
	// in below. The whole ctx is populated BEFORE the debug-trace branch and
	// the tree dispatch, so any drawGrid* method (including the debug-path
	// drawGridBackground) always sees a complete context — no partial-ctx trap
	// if a future change makes the background read a camera-derived field.
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
	// Snap world coordinates to the nearest screen pixel to keep nodes,
	// edges, and pulses phase-aligned with the tiled grid. This avoids the
	// half‑pixel drift that occurs when world→screen projects to fractional
	// pixels.

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

	// Invalidate the guide-target memo for this draw pass: g.frame only
	// advances during playback, so it cannot serve as the memo key while
	// stopped (stale guide after an edit with a still cursor).
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

	// Dispatch every grid-pane layer through the GridTree, which walks the
	// registered Layers in ascending z-order (registerGridTree). Drawing in
	// z-order is the structural fix for the coordinate-badge-over-sidebar bug:
	// the node sidebar (z=60) and cursor label (z=70) now composite ON TOP of
	// the coordinate badge (z=40) / pulses / move-mode, instead of the old
	// inlined order that drew the sidebar right after the nodes (so the badge
	// could overpaint it). clip=true layers receive the grid-pane-clipped
	// subimage (== the original `top`); clip=false layers (sidebar, cursor
	// label) receive the unclipped screen. The drawGrid* methods still read the
	// per-frame g.gridDrawCtx + draw node-glow to g.gridDrawScreen, both set up
	// above before this call.
	g.gridTree.SetBounds(gridRect)
	g.gridTree.Draw(screen)

	// Divider drawn after both panes
}

// nodeStateSlashMinPx is the minimum on-screen node radius (px) at which the
// Muted diagonal slash detail is drawn. Below this the desaturated body alpha
// alone carries the state to keep tiny nodes legible (nodeMinPx is 8 desktop /
// 12 mobile).
const nodeStateSlashMinPx = 10

// drawNodeStateOverlays iterates every node and paints the state-distinct
// treatment for Muted / Silent / Invisible nodes (regular nodes are left
// untouched). It mirrors the base draw loop's culling + fill/border derivation
// so the overlay registers correctly against the row-colored body. Drawn in
// screen space onto dst.
func (g *Game) drawNodeStateOverlays(dst *ebiten.Image, minX, maxX, minY, maxY float64, gridW, gridH int) {
	nodeStyle := NodeUI
	for nodeIdx, n := range g.nodes {
		nodeInfo, ok := g.graph.Nodes[n.ID]
		if !ok {
			continue
		}
		nt := nodeInfo.Type
		if nt == model.NodeTypeRegular {
			continue
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

// drawNodeStateOverlay paints a distinct, token-driven treatment over the base
// node body so Muted / Silent / Invisible nodes are visually distinguishable
// from a normal node. It is drawn AFTER the base sprite/rect (the sprite cache
// key does not encode node type), in screen space, so it works on both the
// cached node-layer path and the RENDER_SAFE / NO_SPRITE_NODES fallback path.
//
//   - Muted:     body desaturated (fill @ AlphaMedium) + a diagonal slash in the
//     drum-mute gray family (gated on rPx >= nodeStateSlashMinPx).
//   - Silent:    hollow — interior cleared to the grid background, border-only
//     silhouette re-stroked.
//   - Invisible: ghosted — body @ AlphaFaint + a dashed outline @ AlphaSubtle.
//
// Invisible nodes are skipped by the base draw loops, so this is their ONLY
// render; the other two overlay on top of an already-drawn body.
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
	case model.NodeTypeMute:
		// Desaturate the body with a translucent fill scrim, then re-edge.
		drawRect(dst, rect, WithAlphaFromColor(fillCol, AlphaMedium), true)
		drawRect(dst, rect, borderCol, false)
		// Diagonal slash in the drum-mute gray family (legible detail only at
		// larger radii; tiny nodes rely on the desaturated body alone).
		if rPx >= nodeStateSlashMinPx {
			var idm ebiten.GeoM
			DrawLineCam(dst, float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y), &idm, colMuteCell, float64(genGeomHighlightBorderThickness))
		}
	case model.NodeTypeSilent:
		// Hollow: clear the interior to the grid background (no/low fill), then
		// re-stroke the border so only the silhouette remains.
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

// nodeOwnHighlightColor returns the color a node highlights/hovers/fires in:
// its OWN displayed color. Row-instrument nodes use their row color; every
// other node (structural / mute / silent, which has no row) uses the node
// border color — the node's visible ring — since the node fill is a dark,
// near-background purple that would not read as a halo. This keeps every
// highlight the same color as the thing it highlights, never a flat white.
func (g *Game) nodeOwnHighlightColor(rowIdx int, rowOK bool) color.Color {
	if rowOK && rowIdx >= 0 && rowIdx < len(g.drum.Rows) {
		return g.drum.Rows[rowIdx].Color
	}
	return genColorNodeBorder
}

// drawNodeSelectionHalo paints a soft glow around the selected node's selection
// ring, reusing the EQ-handle halo language (WithAlpha(primary, AlphaSubtle)).
// The crisp 2px selection ring itself is drawn separately by the caller in
// colStep (= primary); this adds the surrounding glow the ring previously
// lacked. Drawn in screen space so it works on every node-draw path.
func (g *Game) drawNodeSelectionHalo(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64) {
	rPx := int(math.Round((sx2 - sx1) * 0.5))
	if rPx < 1 {
		rPx = 1
	}
	cx := int(math.Round((sx1 + sx2) * 0.5))
	cy := int(math.Round((sy1 + sy2) * 0.5))
	halo := WithAlpha(colAccent, AlphaSubtle)
	// Soft glow = a translucent band ringing the selection box. Painted as four
	// filled strips between the node rect and an expanded rect so the node body
	// stays uncovered while the surround glows (EQ-handle halo language).
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

// drawNodeGroupRing draws a square outline ring around a node's screen-space
// rect, grown outward past the node body by ringGrow px — one radius step
// larger than the selection ring (see drawNodeSelectionHalo's inner box)
// so a node that is both selected and a group member shows both rings
// without overlap. ringGrow also lets multiple concurrent group-membership
// rings nest concentrically (see groupRingBaseGrow/groupRingStepGrow,
// game_group_badges.go) — the Nth ring on a multi-group node passes a larger
// ringGrow than the (N-1)th. Reuses the exact DrawLineCam primitive the
// selection ring uses; only the color and offset differ.
func (g *Game) drawNodeGroupRing(dst *ebiten.Image, sx1, sy1, sx2, sy2 float64, ringCol color.Color, ringGrow int) {
	x1, y1 := sx1-float64(ringGrow), sy1-float64(ringGrow)
	x2, y2 := sx2+float64(ringGrow), sy2+float64(ringGrow)
	var idm ebiten.GeoM
	DrawLineCam(dst, x1, y1, x2, y1, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x2, y1, x2, y2, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x2, y2, x1, y2, &idm, ringCol, float64(genGeomHighlightBorderThickness))
	DrawLineCam(dst, x1, y2, x1, y1, &idm, ringCol, float64(genGeomHighlightBorderThickness))
}

// groupExternalRingStep1 / groupExternalRingStep2 are the outward px steps
// (past the outermost drawn membership ring) for the Part-B external
// highlight (task-enabled-status brief): a stronger 2px stroke immediately
// outside the ring stack, plus a softer 1px stroke one more step further out
// (glow feel). Additive to the existing faint/strong ring behavior.
const (
	groupExternalRingStep1 = 3
	groupExternalRingStep2 = 6
)

// drawNodeExternalHighlightRing draws the Part-B external highlight for a
// node whose group is currently hovered (via its badge) or has its GroupMenu
// open: two extra square rings OUTSIDE whatever concentric membership rings
// drawNodeGroupRing already drew — outermostGrow is the grow value of the
// last (or would-be) membership ring, so this always sits strictly further
// out. First ring: 2px stroke at AlphaStrong; second ring
// (groupExternalRingStep2 further out): 1px stroke at AlphaMedium.
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

// groupIDsContain reports whether gid is present in ids — used to test
// membership of the current frame's external-highlight target group against
// a node's (already up-to-groupRingMaxCount-capped) membership list.
func groupIDsContain(ids []model.GroupID, gid model.GroupID) bool {
	for _, id := range ids {
		if id == gid {
			return true
		}
	}
	return false
}

// computeNodeGraphSig returns a hash of node positions, types, start flags,
// and row colors. Used to invalidate the static node layer cache.
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
