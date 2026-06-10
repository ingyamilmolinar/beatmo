package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
	top.Fill(colBGTop)
	// Always draw top‑pane content into the same subimage to avoid any
	// compositor/target disparities between cached and direct draws.
	dst := top
	// Expose tile/phase to the later [FRAME] log even if grid is disabled.
	var phaseX, phaseY, tileW, tileH int
	// Render mode flags
	renderSafe := envRenderSafe
	screenEdges := renderSafe || RuntimeProf().ScreenEdgesDefault || envScreenEdges || g.simpleDraw

	// Optionally disable grid drawing entirely for geometry debugging.
	if envNoGridDraw {
		if g.logDrawNodes {
			g.logger.Debugf("[DRAW-GRID] disabled via NO_GRID_DRAW")
		}
	} else {
		// Grid layer cache: build once per scale/subdiv, then blit with translation
		// for small pans. This reduces per-frame tiling draw calls dramatically.
		stepPx := g.grid.StepPixels(g.cam.Scale)
		if envNoGridTileCache {
			releaseImage(g.gridTile)
			g.gridTile = nil
		}
		// Ensure the base tile exists for this scale/subdiv.
		if g.gridTile == nil || g.gridTileStepPx != stepPx || g.gridTileSubSig != g.grid.subSig {
			if g.logDrawNodes {
				g.logger.Tracef("[DRAW/GRID] rebuild tile: stepPx=%d maxDiv=%d unitPx=%.2f subSig=%d", stepPx, g.grid.MaxDiv(), g.grid.UnitPixels(g.cam.Scale), g.grid.subSig)
			}
			releaseImage(g.gridTile)
			g.gridTile = g.buildGridTile(stepPx)
			g.gridTileStepPx = stepPx
			g.gridTileSubSig = g.grid.subSig
		}
		if g.gridTile != nil && stepPx > 0 {
			// Camera offset snapped to px for phase alignment.
			offXInt := int(math.Round(g.cam.OffsetX))
			offYInt := int(math.Round(g.cam.OffsetY))
			tileW = g.gridTile.Bounds().Dx()
			tileH = g.gridTile.Bounds().Dy()
			// Start positions so that the grid's tile origin aligns with the
			// camera’s world→screen origin within the cache (including pad).
			phaseX = ((offXInt % tileW) + tileW) % tileW
			phaseY = (((offYInt + gridTopOffset()) % tileH) + tileH) % tileH
			// Try to reuse an existing grid cache by shifting within pad.
			reuse := false
			if g.gridCache != nil && g.gridCacheW == g.split.GridW(g.winW)+2*g.gridCachePad && g.gridCacheH == g.split.GridH(g.winH)+2*g.gridCachePad && g.gridCacheScale == g.cam.Scale && g.gridCacheStepPx == stepPx && g.gridCacheSubSig == g.grid.subSig {
				dx := int(math.Round(g.cam.OffsetX - g.gridCacheOffX))
				dy := int(math.Round(g.cam.OffsetY - g.gridCacheOffY))
				if abs(dx) <= g.gridCachePad && abs(dy) <= g.gridCachePad {
					reuse = true
					blitCache(dst, g.gridCache, -g.gridCachePad+dx, -g.gridCachePad+dy)
					if g.logDrawNodes {
						g.logger.Tracef("[DRAW/GRID-CACHE] reuse dx=%d dy=%d pad=%d", dx, dy, g.gridCachePad)
					}
				}
			}
			if !reuse {
				// Adaptive grid pad: if reuse failed due to pad, enlarge pad.
				if RuntimeProf().AdaptivePanPad && g.gridCache != nil && g.gridCacheW == g.split.GridW(g.winW)+2*g.gridCachePad && g.gridCacheH == g.split.GridH(g.winH)+2*g.gridCachePad && g.gridCacheScale == g.cam.Scale && g.gridCacheStepPx == stepPx && g.gridCacheSubSig == g.grid.subSig {
					dx := int(math.Round(g.cam.OffsetX - g.gridCacheOffX))
					dy := int(math.Round(g.cam.OffsetY - g.gridCacheOffY))
					if abs(dx) > g.gridCachePad || abs(dy) > g.gridCachePad {
						// Double up to a modest cap to convert rebuilds into reuses next frame.
						if g.gridCachePad < 256 {
							g.gridCachePad *= 2
							if g.gridCachePad > 256 {
								g.gridCachePad = 256
							}
						}
					}
				}
				// Rebuild the full grid cache with an offscreen pad to sustain pans.
				w := g.split.GridW(g.winW) + 2*g.gridCachePad
				h := g.split.GridH(g.winH) + 2*g.gridCachePad
				releaseImage(g.gridCache)
				g.gridCache = newTrackedImage("gridCache", w, h)
				g.gridCacheW, g.gridCacheH = w, h
				g.gridCacheScale = g.cam.Scale
				g.gridCacheOffX = g.cam.OffsetX
				g.gridCacheOffY = g.cam.OffsetY
				g.gridCacheStepPx = stepPx
				g.gridCacheSubSig = g.grid.subSig
				// Tile the step tile into the cache, aligning the phase plus pad.
				// Use modulo so startX/Y ∈ [-tileW, -1], guaranteeing the first tile
				// covers cache pixel (0,0) regardless of how small tileW is vs the pad.
				startX := (g.gridCachePad+phaseX)%tileW - tileW
				startY := (g.gridCachePad+phaseY)%tileH - tileH
				var op ebiten.DrawImageOptions
				for y := startY; y < h; y += tileH {
					for x := startX; x < w; x += tileW {
						op.GeoM.Reset()
						op.GeoM.Translate(float64(x), float64(y))
						g.gridCache.DrawImage(g.gridTile, &op)
					}
				}
				blitCache(dst, g.gridCache, -g.gridCachePad, -g.gridCachePad)
				if g.logDrawNodes {
					g.logger.Tracef("[DRAW/GRID-CACHE] rebuild w=%d h=%d pad=%d phase=(%d,%d)", w, h, g.gridCachePad, phaseX, phaseY)
				}
			}
		}
	}

	// camera matrix for world drawings (shift down by bar height)
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	if envNoPixelSnap {
		offX = g.cam.OffsetX
		offY = g.cam.OffsetY
	}
	camScale := unitPx / g.grid.Unit()
	if g.logDrawNodes {
		ds := 1.0
		g.logger.Tracef("[DRAW/CAM] frame=%d scale=%.4f off=(%.0f,%.0f) unitPx=%.2f stepPx=%d camScale=%.6f splitY=%d dscale=%.2f", g.frame, g.cam.Scale, offX, offY, unitPx, g.grid.StepPixels(g.cam.Scale), camScale, g.split.Y, ds)
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
		g.logger.Tracef("[FRAME/SCENE] f=%d nodes=%d edges=%d pulses=%d tile=(%d,%d) phase=(%d,%d) cam=(%.3f,%.0f,%.0f)%s", g.frame, len(g.nodes), len(g.edges), len(g.activePulses), tileW, tileH, phaseX, phaseY, g.cam.Scale, offX, offY, mode)
	}
	var cam ebiten.GeoM
	cam.Scale(camScale, camScale)
	cam.Translate(offX, offY+float64(gridTopOffset()))
	// Snap world coordinates to the nearest screen pixel to keep nodes,
	// edges, and pulses phase-aligned with the tiled grid. This avoids the
	// half‑pixel drift that occurs when world→screen projects to fractional
	// pixels.
	snapWorld := func(v float64) float64 { return math.Round(v*camScale) / camScale }
	var id ebiten.GeoM
	_ = id

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

	// edges with connection animation
	// Detect row color changes and invalidate edge cache if needed so edge
	// tints track row colors even when the camera doesn't move.
	curColorSig := g.rowColorSig()
	if g.edgeCache != nil && curColorSig != g.edgeCacheColorSig {
		g.edgesDirty = true
	}
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
	// Render cached baseline edges unless disabled by env or in render-safe.
	disableCache := envNoEdgeCache || envRenderSafe || screenEdges
	reuseCache := false
	dx, dy := 0, 0
	if !disableCache {
		if g.edgeCache != nil && g.edgeCacheW == g.split.GridW(g.winW)+2*g.edgeCachePad && g.edgeCacheH == g.split.GridH(g.winH)+2*g.edgeCachePad && g.edgeCacheScale == camScale && !g.edgesDirty {
			dx = int(math.Round(offX - g.edgeCacheOffX))
			dy = int(math.Round(offY - g.edgeCacheOffY))
			if abs(dx) <= g.edgeCachePad && abs(dy) <= g.edgeCachePad {
				reuseCache = true
				blitCache(dst, g.edgeCache, -g.edgeCachePad+dx, -g.edgeCachePad+dy)
				g.lastDrawEdges = g.edgeCacheCount
				if g.logDrawNodes {
					g.logger.Tracef("[DRAW/EDGE-CACHE] state=reuse dx=%d dy=%d count=%d scale=%.6f off=(%.0f,%.0f) pad=%d", dx, dy, g.edgeCacheCount, camScale, offX, offY, g.edgeCachePad)
				}
			}
		}
		if !reuseCache {
			// Adaptive edge pad: if reuse failed due to pad, enlarge pad.
			if RuntimeProf().AdaptivePanPad && g.edgeCache != nil && g.edgeCacheW == g.split.GridW(g.winW)+2*g.edgeCachePad && g.edgeCacheH == g.split.GridH(g.winH)+2*g.edgeCachePad && g.edgeCacheScale == camScale && !g.edgesDirty {
				dx2 := int(math.Round(offX - g.edgeCacheOffX))
				dy2 := int(math.Round(offY - g.edgeCacheOffY))
				if abs(dx2) > g.edgeCachePad || abs(dy2) > g.edgeCachePad {
					if g.edgeCachePad < 256 {
						g.edgeCachePad *= 2
						if g.edgeCachePad > 256 {
							g.edgeCachePad = 256
						}
					}
				}
			}
			// Rebuild cache centered at current camera offset with pad margin.
			w := g.split.GridW(g.winW) + 2*g.edgeCachePad
			h := g.split.GridH(g.winH) + 2*g.edgeCachePad
			releaseImage(g.edgeCache)
			g.edgeCache = newTrackedImage("edgeCache", w, h)
			g.edgeCacheW, g.edgeCacheH = w, h
			g.edgeCacheScale, g.edgeCacheOffX, g.edgeCacheOffY = camScale, offX, offY
			g.edgeCacheColorSig = curColorSig
			// Culling rect extended by world distance equivalent to pad.
			minX2, maxX2, minY2, maxY2 := visibleWorldRect(g.cam, g.split.GridW(g.winW), g.split.GridH(g.winH))
			worldPad := float64(g.edgeCachePad) / camScale
			minX2 -= worldPad
			maxX2 += worldPad
			minY2 -= worldPad
			maxY2 += worldPad
			var cnt int
			for i := range g.edges {
				e := &g.edges[i]
				padw := g.grid.Unit() * 2
				ex1, ex2 := e.A.X, e.B.X
				if ex1 > ex2 {
					ex1, ex2 = ex2, ex1
				}
				ey1, ey2 := e.A.Y, e.B.Y
				if ey1 > ey2 {
					ey1, ey2 = ey2, ey1
				}
				ex1 -= padw
				ex2 += padw
				ey1 -= padw
				ey2 += padw
				if ex2 < minX2 || ex1 > maxX2 || ey2 < minY2 || ey1 > maxY2 {
					continue
				}
				edgeStyle := EdgeUI
				edgeStyle.Thickness = edgeThick
				edgeStyle.ArrowSize = arrow
				if row, ok := g.nodeRows[e.A.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
					base := g.drum.Rows[row].Color
					edgeStyle.Color = base
				}
				ax, ay := snapWorld(e.A.X), snapWorld(e.A.Y)
				bx, by := snapWorld(e.B.X), snapWorld(e.B.Y)
				// Draw into the cache with an additional +pad translation so the
				// cached content aligns when we later blit with -pad.
				camCache := cam
				camCache.Translate(float64(g.edgeCachePad), float64(g.edgeCachePad))
				edgeStyle.Draw(g.edgeCache, ax, ay, bx, by, &camCache)
				cnt++
			}
			g.edgeCacheCount = cnt
			g.edgesDirty = false
			blitCache(dst, g.edgeCache, -g.edgeCachePad, -g.edgeCachePad)
			g.lastDrawEdges = g.edgeCacheCount
			if g.logDrawNodes {
				g.logger.Tracef("[DRAW/EDGE-CACHE] state=rebuild count=%d scale=%.6f off=(%.0f,%.0f) pad=%d worldPad=%.3f", cnt, camScale, offX, offY, g.edgeCachePad, float64(g.edgeCachePad)/camScale)
			}
		}
	}
	for i := range g.edges {
		e := &g.edges[i]
		edgeStyle := EdgeUI
		edgeStyle.Thickness = edgeThick
		edgeStyle.ArrowSize = arrow
		if row, ok := g.nodeRows[e.A.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
			base := g.drum.Rows[row].Color
			edgeStyle.Color = base
		}
		// Optional debug path: draw edges in screen-space to bypass any
		// backend transform ambiguity (SCREEN_EDGES=1).
		if envScreenEdges {
			sx1 := e.A.X*camScale + offX
			sy1 := e.A.Y*camScale + offY + float64(gridTopOffset())
			sx2 := e.B.X*camScale + offX
			sy2 := e.B.Y*camScale + offY + float64(gridTopOffset())
			x0 := int(math.Round(sx1))
			y0 := int(math.Round(sy1))
			x1 := int(math.Round(sx2))
			y1 := int(math.Round(sy2))
			if y0 == y1 {
				if x0 > x1 {
					x0, x1 = x1, x0
				}
				r := image.Rect(x0, y0, x1, y0+1)
				drawRect(dst, r, edgeStyle.Color, true)
			} else if x0 == x1 {
				if y0 > y1 {
					y0, y1 = y1, y0
				}
				r := image.Rect(x0, y0, x0+1, y1)
				drawRect(dst, r, edgeStyle.Color, true)
			} else {
				// Fallback to world transform for non-orthogonal edges (shouldn't happen).
				ax, ay := snapWorld(e.A.X), snapWorld(e.A.Y)
				bx, by := snapWorld(e.B.X), snapWorld(e.B.Y)
				edgeStyle.DrawProgress(dst, ax, ay, bx, by, &cam, e.t)
			}
			// Draw arrowheads in screen space once the connection is complete.
			if e.t >= 1 {
				var idCam ebiten.GeoM // identity
				apx := float64(math.Round(arrow * camScale))
				// Skip tiny arrowheads at low zoom to save draw calls.
				if apx < 2 {
					g.lastDrawEdges++
					continue
				}
				fx0, fy0 := float64(x0), float64(y0)
				fx1, fy1 := float64(x1), float64(y1)
				angle := math.Atan2(fy1-fy0, fx1-fx0)
				lx := fx1 - apx*math.Cos(angle-math.Pi/6)
				ly := fy1 - apx*math.Sin(angle-math.Pi/6)
				rx := fx1 - apx*math.Cos(angle+math.Pi/6)
				ry := fy1 - apx*math.Sin(angle+math.Pi/6)
				drawEdgeLine(dst, fx1, fy1, lx, ly, &idCam, edgeStyle.Color, 1)
				drawEdgeLine(dst, fx1, fy1, rx, ry, &idCam, edgeStyle.Color, 1)
			}
			g.lastDrawEdges++
			continue
		}
		// Cull edges outside the visible world rect (expanded slightly).
		pad := g.grid.Unit() * 2
		ex1, ex2 := e.A.X, e.B.X
		if ex1 > ex2 {
			ex1, ex2 = ex2, ex1
		}
		ey1, ey2 := e.A.Y, e.B.Y
		if ey1 > ey2 {
			ey1, ey2 = ey2, ey1
		}
		ex1 -= pad
		ex2 += pad
		ey1 -= pad
		ey2 += pad
		if ex2 < minX || ex1 > maxX || ey2 < minY || ey1 > maxY {
			continue
		}
		// When cache is disabled (or in safe mode), draw the full baseline
		// edge directly so completed connections remain visible.
		if disableCache {
			ax, ay := snapWorld(e.A.X), snapWorld(e.A.Y)
			bx, by := snapWorld(e.B.X), snapWorld(e.B.Y)
			edgeStyle.Draw(dst, ax, ay, bx, by, &cam)
			g.lastDrawEdges++
		} else if e.t < 1 {
			ax, ay := snapWorld(e.A.X), snapWorld(e.A.Y)
			bx, by := snapWorld(e.B.X), snapWorld(e.B.Y)
			edgeStyle.DrawProgress(dst, ax, ay, bx, by, &cam, e.t)
			g.lastDrawEdges++
		}
		if g.logDrawNodes {
			ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
			acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
			bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
			bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
			ex0 := e.A.X*camScale + offX
			ey0 := e.A.Y*camScale + offY + float64(gridTopOffset())
			ex1 := e.B.X*camScale + offX
			ey1 := e.B.Y*camScale + offY + float64(gridTopOffset())
			// Rounded (pixel-snapped) centers used for sprites and edges
			dcxA, dcyA := math.Round(acx), math.Round(acy)
			dcxB, dcyB := math.Round(bcx), math.Round(bcy)
			rEx0, rEy0 := math.Round(ex0), math.Round(ey0)
			rEx1, rEy1 := math.Round(ex1), math.Round(ey1)
			g.logger.Tracef("[DRAW/EDGE] frame=%d A=%d gridA=(%d,%d) nodeA=(%.1f,%.1f) projA=(%.1f,%.1f) roundA=(%.0f,%.0f) B=%d gridB=(%d,%d) nodeB=(%.1f,%.1f) projB=(%.1f,%.1f) roundB=(%.0f,%.0f)",
				g.frame, e.A.ID, e.A.I, e.A.J, acx, acy, ex0, ey0, dcxA, dcyA, e.B.ID, e.B.I, e.B.J, bcx, bcy, ex1, ey1, dcxB, dcyB)
			if dcxA != rEx0 || dcyA != rEy0 || dcxB != rEx1 || dcyB != rEy1 {
				g.logger.Debugf("[EDGE-MISALIGN] A id=%d nodeRound=(%.0f,%.0f) projRound=(%.0f,%.0f) B id=%d nodeRound=(%.0f,%.0f) projRound=(%.0f,%.0f)", e.A.ID, dcxA, dcyA, rEx0, rEy0, e.B.ID, dcxB, dcyB, rEx1, rEy1)
			}
		}
		// Suppress transient edge pulses during origin selection and for a
		// couple frames after committing an origin/node to avoid flickers
		// in other circuits.
		if e.pulse >= 0 && g.pendingStartRow < 0 && g.quietFrames == 0 {
			px := e.A.X + (e.B.X-e.A.X)*e.pulse
			py := e.A.Y + (e.B.Y-e.A.Y)*e.pulse
			px, py = snapWorld(px), snapWorld(py)
			sigStyle.Color = edgeStyle.Color
			sigStyle.Draw(dst, px, py, &cam)
			g.lastDrawPulses++
		}
	}

	// Visual overlay: draw crosses at first edge endpoints and node centers in screen space
	if g.logDrawNodes && len(g.edges) > 0 {
		e := &g.edges[0]
		// Project world→screen endpoints (without cam) using the same math as logs
		ex0 := e.A.X*camScale + offX
		ey0 := e.A.Y*camScale + offY + float64(gridTopOffset())
		ex1 := e.B.X*camScale + offX
		ey1 := e.B.Y*camScale + offY + float64(gridTopOffset())
		ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
		acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
		bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
		bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
		// Draw screen-space crosses for easy visual verification
		drawCrossScreen(screen, int(math.Round(ex0)), int(math.Round(ey0)), 5, genColorVizDebugEdge) // yellow: edge A
		drawCrossScreen(screen, int(math.Round(ex1)), int(math.Round(ey1)), 5, genColorVizDebugEdge) // yellow: edge B
		drawCrossScreen(screen, int(math.Round(acx)), int(math.Round(acy)), 7, genColorVizDebugNode) // cyan: node A
		drawCrossScreen(screen, int(math.Round(bcx)), int(math.Round(bcy)), 7, genColorVizDebugNode) // cyan: node B
	}

	// Focused, human-readable check for the first edge vs its nodes.
	if g.logDrawNodes && len(g.edges) > 0 {
		e := &g.edges[0]
		ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
		acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
		bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
		bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
		ex0 := e.A.X*camScale + offX
		ey0 := e.A.Y*camScale + offY + float64(gridTopOffset())
		ex1 := e.B.X*camScale + offX
		ey1 := e.B.Y*camScale + offY + float64(gridTopOffset())
		da := math.Hypot(acx-ex0, acy-ey0)
		db := math.Hypot(bcx-ex1, bcy-ey1)
		g.logger.Tracef("[DRAW/EDGE-CHECK] first edge A=%d@(%d,%d) B=%d@(%d,%d) nodeA=(%.0f,%.0f) edgeA=(%.0f,%.0f) dA=%.2f nodeB=(%.0f,%.0f) edgeB=(%.0f,%.0f) dB=%.2f cache=%t dx=%d dy=%d",
			e.A.ID, e.A.I, e.A.J, e.B.ID, e.B.I, e.B.J,
			math.Round(acx), math.Round(acy), math.Round(ex0), math.Round(ey0), da,
			math.Round(bcx), math.Round(bcy), math.Round(ex1), math.Round(ey1), db,
			reuseCache, dx, dy)
	}

	// link preview
	if g.linkDrag.active {
		edgeStyle := EdgeUI
		edgeStyle.Thickness = edgeThick
		edgeStyle.ArrowSize = arrow
		if row, ok := g.nodeRows[g.linkDrag.from.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
			base := g.drum.Rows[row].Color
			edgeStyle.Color = adjustColor(base, 80)
		}
		edgeStyle.Draw(dst, g.linkDrag.from.X, g.linkDrag.from.Y,
			g.linkDrag.toX, g.linkDrag.toY, &cam)
	}

	// nodes
	// Pre-compute all node radii for this frame (avoids O(n²) neighbor checks in draw loop)
	if cap(g.nodeRadiiCache) < len(g.nodes) {
		g.nodeRadiiCache = make([]float64, len(g.nodes))
	} else {
		g.nodeRadiiCache = g.nodeRadiiCache[:len(g.nodes)]
	}
	for i, n := range g.nodes {
		g.nodeRadiiCache[i] = g.nodeRadius(n)
	}

	// reset last-frame node highlight map
	g.lastNodeHLReset()
	nodeStyle := NodeUI

	// Static node layer cache: during playback, most nodes stay in their
	// default (non-highlighted) state. Cache all base-state nodes into a
	// layer and only overdraw highlighted/selected nodes per frame,
	// reducing DrawImage calls from ~58 to ~2-9.
	useNodeLayer := !envRenderSafe && !envNoSpriteNodes && !g.logDrawNodes
	if useNodeLayer {
		nodeGraphSig := g.computeNodeGraphSig()
		gridW, gridH := g.split.GridW(g.winW), g.split.GridH(g.winH)
		needRebuild := g.nodeLayer == nil ||
			g.nodeLayer.Bounds().Dx() != gridW || g.nodeLayer.Bounds().Dy() != gridH ||
			g.nodeLayerCamScale != camScale ||
			g.nodeLayerCamOffX != offX || g.nodeLayerCamOffY != offY ||
			g.nodeLayerGraphSig != nodeGraphSig

		if needRebuild {
			if g.nodeLayer == nil || g.nodeLayer.Bounds().Dx() != gridW || g.nodeLayer.Bounds().Dy() != gridH {
				releaseImage(g.nodeLayer)
				g.nodeLayer = newTrackedImage("nodeLayer", gridW, gridH)
			} else {
				g.nodeLayer.Clear()
			}
			if g.nodeSpriteCache == nil {
				g.nodeSpriteCache = make(map[spriteKey]*ebiten.Image)
			}
			cnt := 0
			for nodeIdx, n := range g.nodes {
				nodeInfo, ok := g.graph.Nodes[n.ID]
				if !ok || nodeInfo.Type == model.NodeTypeInvisible {
					continue
				}
				rWorld := g.nodeRadiiCache[nodeIdx]
				if n.X+rWorld < minX || n.X-rWorld > maxX || n.Y+rWorld < minY || n.Y-rWorld > maxY {
					continue
				}
				sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
				if sx2 < 0 || sx1 >= float64(gridW) || sy2 < 0 || sy1 >= float64(gridH) {
					continue
				}
				rPx := int(math.Round((sx2 - sx1) * 0.5))
				if rPx < 1 {
					rPx = 1
				}
				rowIdx, rowOK := g.nodeRows[n.ID]
				if !rowOK || rowIdx < 0 || rowIdx >= len(g.drum.Rows) {
					rowOK = false
				}
				fillCol := nodeStyle.Fill
				borderCol := nodeStyle.Border
				if rowOK {
					base := g.drum.Rows[rowIdx].Color
					if n.Start {
						fillCol = adjustColor(base, 40)
					} else {
						fillCol = base
					}
					borderCol = adjustColor(base, 80)
				}
				fr, fg, fb, fa := rgba8(fillCol)
				br, bg, bb, ba := rgba8(borderCol)
				skey := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: br, bg: bg, bb: bb, ba: ba}
				spr := g.nodeSpriteCache[skey]
				if spr == nil {
					spr = buildNodeSprite(fillCol, borderCol, rPx)
					g.nodeSpriteCache[skey] = spr
				}
				var sop ebiten.DrawImageOptions
				nlcx := (sx1 + sx2) * 0.5
				nlcy := (sy1 + sy2) * 0.5
				sop.GeoM.Translate(math.Round(nlcx)-float64(rPx), math.Round(nlcy)-float64(rPx))
				g.nodeLayer.DrawImage(spr, &sop)
				cnt++
			}
			g.nodeLayerCamScale = camScale
			g.nodeLayerCamOffX = offX
			g.nodeLayerCamOffY = offY
			g.nodeLayerGraphSig = nodeGraphSig
			g.lastDrawNodes = cnt
		}

		// Blit static node layer.
		dst.DrawImage(g.nodeLayer, nil)

		// Per-frame overlay: highlighted nodes, glow, selection boxes.
		for nodeIdx, n := range g.nodes {
			nodeInfo, ok := g.graph.Nodes[n.ID]
			if !ok || nodeInfo.Type == model.NodeTypeInvisible {
				continue
			}
			isMute := nodeInfo.Type == model.NodeTypeMute
			isSilent := nodeInfo.Type == model.NodeTypeSilent
			rowIdx, rowOK := g.nodeRows[n.ID]
			if !rowOK || rowIdx < 0 || rowIdx >= len(g.drum.Rows) {
				rowOK = false
				rowIdx = -1
			}
			var highlightCol color.Color = colHighlight
			if isMute || isSilent {
				highlightCol = colMuteHighlight
			} else if rowOK {
				highlightCol = g.drum.Rows[rowIdx].Color
			}
			aLevel := 0.0
			if g.pendingStartRow < 0 && g.quietFrames == 0 {
				if a := g.nodeAnimGet(n.ID); a > 0 {
					if _, _, hasWindow := g.nodeHighlightUntil(n.ID); !hasWindow {
						aLevel = a
						g.lastNodeHLMark(n.ID)
					}
				}
			}
			if start, end, ok := g.nodeHighlightUntil(n.ID); ok {
				now := audio.Now()
				if now >= start && now < end {
					aLevel = 1
					g.lastNodeHLMark(n.ID)
				}
			}
			isSelected := g.sel == n && g.pendingStartRow < 0
			isNeighbor := g.pendingStartRow < 0 && g.selNeighbors != nil && g.selNeighbors[n]
			if aLevel <= 0 && !isSelected && !isNeighbor {
				continue
			}
			rWorld := g.nodeRadiiCache[nodeIdx]
			if n.X+rWorld < minX || n.X-rWorld > maxX || n.Y+rWorld < minY || n.Y-rWorld > maxY {
				continue
			}
			sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
			if sx2 < 0 || sx1 >= float64(gridW) || sy2 < 0 || sy1 >= float64(gridH) {
				continue
			}
			if aLevel > 0 {
				rPx := int(math.Round((sx2 - sx1) * 0.5))
				if rPx < 1 {
					rPx = 1
				}
				fillCol := nodeStyle.Fill
				borderCol := nodeStyle.Border
				if rowOK {
					base := g.drum.Rows[rowIdx].Color
					if n.Start {
						fillCol = adjustColor(base, 40)
					} else {
						fillCol = base
					}
					borderCol = highlightCol
				}
				// Glow overlay for animated nodes.
				if rPx >= 2 && !RuntimeProf().DisableNodeGlow {
					glowBase := 1.2
					glowAmp := float64(genAnimNodeTriggerGlow)
					rpScr := float64(rPx) * (glowBase + glowAmp*aLevel)
					maxGlow := float64(gridH) / 8
					if rpScr > maxGlow {
						rpScr = maxGlow
					}
					if rpScr < 2 {
						rpScr = 2
					}
					g.lastGlowScr = rpScr
					glow := SignalUI
					glow.Radius = float32(rpScr / g.cam.Scale)
					glow.Color = highlightCol
					nx, ny := snapWorld(n.X), snapWorld(n.Y)
					glow.Draw(screen, nx, ny, &cam)
				}
				// Low-overhead highlight overlay for simpleDraw.
				if g.simpleDraw || RuntimeProf().DisableNodeGlow {
					icx := int(math.Round((sx1 + sx2) * 0.5))
					icy := int(math.Round((sy1 + sy2) * 0.5))
					ringScale := 1.12 + 0.28*aLevel
					rp := int(math.Round(float64(rPx) * ringScale))
					if rp <= rPx {
						rp = rPx + 1
					}
					maxRP := g.winW / 10
					if gridH/10 < maxRP {
						maxRP = gridH / 10
					}
					if maxRP < 8 {
						maxRP = 8
					}
					if rp > maxRP {
						rp = maxRP
					}
					ix := icx - rp
					iy := icy - rp
					outer := genColorBorder
					drawRect(dst, image.Rect(ix-2, iy-2, ix+2*rp+2, iy+2*rp+2), outer, false)
					drawRect(dst, image.Rect(ix-1, iy-1, ix+2*rp+1, iy+2*rp+1), outer, false)
					drawRect(dst, image.Rect(ix, iy, ix+2*rp, iy+2*rp), highlightCol, false)
				}
				// Highlighted sprite (overdraws base in static layer).
				if g.nodeSpriteCache == nil {
					g.nodeSpriteCache = make(map[spriteKey]*ebiten.Image)
				}
				fr, fg, fb, fa := rgba8(fillCol)
				br, bg, bb, ba := rgba8(borderCol)
				skey := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: br, bg: bg, bb: bb, ba: ba}
				spr := g.nodeSpriteCache[skey]
				if spr == nil {
					spr = buildNodeSprite(fillCol, borderCol, rPx)
					g.nodeSpriteCache[skey] = spr
				}
				var sop ebiten.DrawImageOptions
				hlcx := (sx1 + sx2) * 0.5
				hlcy := (sy1 + sy2) * 0.5
				sop.GeoM.Translate(math.Round(hlcx)-float64(rPx), math.Round(hlcy)-float64(rPx))
				dst.DrawImage(spr, &sop)
			}
			// Selection boxes (cyan ring for selected, faded for neighbors).
			x1, y1, x2, y2 := sx1, sy1, sx2, sy2
			var idm ebiten.GeoM
			if isSelected {
				DrawLineCam(dst, x1, y1, x2, y1, &idm, colStep, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x2, y1, x2, y2, &idm, colStep, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x2, y2, x1, y2, &idm, colStep, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x1, y2, x1, y1, &idm, colStep, float64(genGeomHighlightBorderThickness))
			} else if isNeighbor {
				hl := fadeColor(colStep, float64(genAnimHighlightFaded))
				DrawLineCam(dst, x1, y1, x2, y1, &idm, hl, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x2, y1, x2, y2, &idm, hl, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x2, y2, x1, y2, &idm, hl, float64(genGeomHighlightBorderThickness))
				DrawLineCam(dst, x1, y2, x1, y1, &idm, hl, float64(genGeomHighlightBorderThickness))
			}
		}
	} else {
	// Fallback: original loop for debug modes (renderSafe, noSpriteNodes, logDrawNodes).
	for nodeIdx, n := range g.nodes {
		nodeInfo, ok := g.graph.Nodes[n.ID]
		if !ok || nodeInfo.Type == model.NodeTypeInvisible {
			continue
		}

		isMute := nodeInfo.Type == model.NodeTypeMute
		isSilent := nodeInfo.Type == model.NodeTypeSilent
		rowIdx, rowOK := g.nodeRows[n.ID]
		if !rowOK || rowIdx < 0 || rowIdx >= len(g.drum.Rows) {
			rowOK = false
			rowIdx = -1
		}

		var highlightCol color.Color = colHighlight
		if isMute || isSilent {
			highlightCol = colMuteHighlight
		} else if rowOK {
			highlightCol = g.drum.Rows[rowIdx].Color
		}

		// Cull nodes outside visible rect (include radius pad)
		rWorld := g.nodeRadiiCache[nodeIdx]
		if n.X+rWorld < minX || n.X-rWorld > maxX || n.Y+rWorld < minY || n.Y-rWorld > maxY {
			continue
		}

		style := nodeStyle
		// Highlight triggered nodes by tinting border; keep instrument-based fill.
		// Suppress during origin selection and shortly after committing it to
		// avoid any cross-circuit flickers.
		aLevel := 0.0
		if g.pendingStartRow < 0 && g.quietFrames == 0 {
			if a := g.nodeAnimGet(n.ID); a > 0 {
				// When a time-based highlight window exists, defer to it:
				// the sequencer sets nodeAnim=1 immediately but the window
				// may not have started yet (lookahead scheduling).
				if _, _, hasWindow := g.nodeHighlightUntil(n.ID); !hasWindow {
					style.Border = highlightCol
					aLevel = a
					g.lastNodeHLMark(n.ID)
				}
			}
		}
		if start, end, ok := g.nodeHighlightUntil(n.ID); ok {
			now := audio.Now()
			if now >= start && now < end {
				aLevel = 1
				g.lastNodeHLMark(n.ID)
			}
		}
		style.Radius = float32(g.nodeRadiiCache[nodeIdx])
		if rowOK {
			base := g.drum.Rows[rowIdx].Color
			if n.Start {
				style.Fill = adjustColor(base, 40)
			} else {
				style.Fill = base
			}
			style.Border = adjustColor(base, 80)
			if g.logDrawNodes {
				fb := color.RGBAModel.Convert(style.Fill).(color.RGBA)
				bb := color.RGBAModel.Convert(style.Border).(color.RGBA)
				x1, y1, x2, y2 := g.nodeScreenRect(n)
				g.logger.Tracef("[DRAW/NODE] frame=%d id=%d row=%d grid=(%d,%d) scr=(%.1f,%.1f)-(%.1f,%.1f) radius=%.2f start=%t fill=(%d,%d,%d,%d) border=(%d,%d,%d,%d) pendingStartRow=%d playing=%t",
					g.frame, n.ID, rowIdx, n.I, n.J, x1, y1, x2, y2, style.Radius, n.Start,
					fb.R, fb.G, fb.B, fb.A, bb.R, bb.G, bb.B, bb.A, g.pendingStartRow, g.Playing())
			}
		}

		// Compute screen-space rect once regardless of draw path (used for selection boxes)
		sx1, sy1, sx2, sy2 := g.nodeScreenRect(n)
		// Cull nodes entirely outside the grid pane to avoid unnecessary draws.
		if sx2 < 0 || sx1 >= float64(g.split.GridW(g.winW)) || sy2 < 0 || sy1 >= float64(g.split.GridH(g.winH)) {
			continue
		}

		// Render-safe path (screen-space rectangles for nodes)
		if envRenderSafe {
			rPx := int(math.Round((sx2 - sx1) * 0.5))
			if rPx < 1 {
				rPx = 1
			}
			fillCol := style.Fill
			borderCol := style.Border
			if rowOK {
				base := g.drum.Rows[rowIdx].Color
				if n.Start {
					fillCol = adjustColor(base, 40)
				} else {
					fillCol = base
				}
				borderCol = adjustColor(base, 80)
				if aLevel > 0 {
					borderCol = highlightCol
				}
			}
			cx := (sx1 + sx2) * 0.5
			cy := (sy1 + sy2) * 0.5
			ix := int(math.Round(cx)) - rPx
			iy := int(math.Round(cy)) - rPx
			rect := image.Rect(ix, iy, ix+2*rPx, iy+2*rPx)
			drawRect(dst, rect, fillCol, true)
			drawRect(dst, rect, borderCol, false)
			g.lastDrawNodes++
		} else if envNoSpriteNodes {
			// Draw via camera transform using world coords (no sprites).
			style.Draw(dst, snapWorld(n.X), snapWorld(n.Y), &cam)
			g.lastDrawNodes++
		} else {
			// Draw node via cached screen-space sprite to reduce draw calls.
			// Compute screen-space rect and radius in pixels.
			rPx := int(math.Round((sx2 - sx1) * 0.5))
			if rPx < 1 {
				rPx = 1
			}
			// Derive final fill/border colors
			fillCol := style.Fill
			borderCol := style.Border
			if rowOK {
				base := g.drum.Rows[rowIdx].Color
				if n.Start {
					fillCol = adjustColor(base, 40)
				} else {
					fillCol = base
				}
				borderCol = adjustColor(base, 80)
				if aLevel > 0 {
					// Keep highlight border color when animating
					borderCol = highlightCol
				}
			}
			// Optional glow overlay for animated nodes (drawn behind the sprite)
			// Skip when the node is too small on screen to be visible to save draw calls.
			if aLevel > 0 && rPx >= 2 && !RuntimeProf().DisableNodeGlow {
				// Compute screen-space glow radius and clamp to a reasonable bound.
				glowBase := 1.2
				glowAmp := float64(genAnimNodeTriggerGlow)
				rpScr := float64(rPx) * (glowBase + glowAmp*aLevel)
				maxGlow := float64(g.split.GridH(g.winH)) / 8
				if rpScr > maxGlow {
					rpScr = maxGlow
				}
				if rpScr < 2 {
					rpScr = 2
				}
				g.lastGlowScr = rpScr
				glow := SignalUI
				glow.Radius = float32(rpScr / g.cam.Scale)
				glow.Color = highlightCol
				nx, ny := snapWorld(n.X), snapWorld(n.Y)
				glow.Draw(screen, nx, ny, &cam)
			}
			// Extra low-overhead highlight overlay for simpleDraw: draw a 1px expanded border in highlight color.
			if aLevel > 0 && (g.simpleDraw || RuntimeProf().DisableNodeGlow) {
				// High-contrast, thicker outline: two white rings + inner row-colored ring
				cx := int(math.Round((sx1 + sx2) * 0.5))
				cy := int(math.Round((sy1 + sy2) * 0.5))
				ringScale := 1.12 + 0.28*aLevel
				rp := int(math.Round(float64(rPx) * ringScale))
				if rp <= rPx {
					rp = rPx + 1
				}
				// Safety clamp: prevent accidental huge overlays due to projection bugs
				maxRP := g.winW / 10
				if g.split.GridH(g.winH)/10 < maxRP {
					maxRP = g.split.GridH(g.winH) / 10
				}
				if maxRP < 8 {
					maxRP = 8
				}
				if rp > maxRP {
					rp = maxRP
				}
				ix := cx - rp
				iy := cy - rp
				outer := genColorBorder
				// Outer white ring (thickness 2 via two nested borders)
				drawRect(dst, image.Rect(ix-2, iy-2, ix+2*rp+2, iy+2*rp+2), outer, false)
				drawRect(dst, image.Rect(ix-1, iy-1, ix+2*rp+1, iy+2*rp+1), outer, false)
				// Inner ring using row/mute highlight color
				drawRect(dst, image.Rect(ix, iy, ix+2*rp, iy+2*rp), highlightCol, false)
			}
			if g.nodeSpriteCache == nil {
				g.nodeSpriteCache = make(map[spriteKey]*ebiten.Image)
			}
			fr, fg, fb, fa := rgba8(fillCol)
			br, bg, bb, ba := rgba8(borderCol)
			skey := spriteKey{rpx: rPx, fr: fr, fg: fg, fb: fb, fa: fa, br: br, bg: bg, bb: bb, ba: ba}
			spr := g.nodeSpriteCache[skey]
			if spr == nil {
				spr = buildNodeSprite(fillCol, borderCol, rPx)
				g.nodeSpriteCache[skey] = spr
			}
			var sop ebiten.DrawImageOptions
			cx := (sx1 + sx2) * 0.5
			cy := (sy1 + sy2) * 0.5
			dx := math.Round(cx) - float64(rPx)
			dy := math.Round(cy) - float64(rPx)
			sop.GeoM.Translate(dx, dy)
			dst.DrawImage(spr, &sop)
			g.lastDrawNodes++
			if g.logDrawNodes {
				dcx := math.Round(cx)
				dcy := math.Round(cy)
				ex := n.X*camScale + offX
				ey := n.Y*camScale + offY + float64(gridTopOffset())
				wpx := float64(rPx) * 2
				g.logger.Tracef("[DRAW/NODE-PLACED] id=%d grid=(%d,%d) drawnCenter=(%.0f,%.0f) rect=(%.0f,%.0f)-(%.0f,%.0f) proj=(%.2f,%.2f)", n.ID, n.I, n.J, dcx, dcy, dx, dy, dx+wpx, dy+wpx, ex, ey)
			}
		}

		x1, y1, x2, y2 := sx1, sy1, sx2, sy2
		var id ebiten.GeoM
		if g.sel == n && g.pendingStartRow < 0 {
			DrawLineCam(dst, x1, y1, x2, y1, &id, colHighlight, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x2, y1, x2, y2, &id, colHighlight, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x2, y2, x1, y2, &id, colHighlight, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x1, y2, x1, y1, &id, colHighlight, float64(genGeomHighlightBorderThickness))
		} else if g.pendingStartRow < 0 && g.selNeighbors != nil && g.selNeighbors[n] {
			hl := fadeColor(colHighlight, float64(genAnimHighlightFaded))
			DrawLineCam(dst, x1, y1, x2, y1, &id, hl, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x2, y1, x2, y2, &id, hl, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x2, y2, x1, y2, &id, hl, float64(genGeomHighlightBorderThickness))
			DrawLineCam(dst, x1, y2, x1, y1, &id, hl, float64(genGeomHighlightBorderThickness))
		}
	}
	} // end useNodeLayer else

	// Visual overlay: draw crosses after nodes so they are on top
	if g.logDrawNodes && len(g.edges) > 0 {
		e := &g.edges[0]
		ex0 := e.A.X*camScale + offX
		ey0 := e.A.Y*camScale + offY + float64(gridTopOffset())
		ex1 := e.B.X*camScale + offX
		ey1 := e.B.Y*camScale + offY + float64(gridTopOffset())
		ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
		acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
		bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
		bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
		drawCrossScreen(dst, int(math.Round(ex0)), int(math.Round(ey0)), 7, genColorVizDebugEdge)
		drawCrossScreen(dst, int(math.Round(ex1)), int(math.Round(ey1)), 7, genColorVizDebugEdge)
		drawCrossScreen(dst, int(math.Round(acx)), int(math.Round(acy)), 9, genColorVizDebugNode)
		drawCrossScreen(dst, int(math.Round(bcx)), int(math.Round(bcy)), 9, genColorVizDebugNode)
	}

	// Node sidebar (draw in screen space)
	if g.sidebar.IsOpen() {
		g.sidebar.Draw(screen)
	}

	// Grid zoom buttons disabled; rely on wheel/touch gestures.

	// pulses
	g.renderedPulsesCount = 0
	for _, p := range g.activePulses {
		// Suppress transient pulse visuals while selecting an origin or just
		// after committing one to guarantee other circuits never flicker.
		if g.pendingStartRow >= 0 || g.quietFrames > 0 {
			continue
		}
		px := p.x1 + (p.x2-p.x1)*p.t
		py := p.y1 + (p.y2-p.y1)*p.t
		col := SignalUI.Color
		if p.row >= 0 && p.row < len(g.drum.Rows) {
			base := g.drum.Rows[p.row].Color
			col = adjustColor(base, 80)
		}
		DrawLineCam(dst, p.x1, p.y1, px, py, &cam, fadeColor(col, float64(genAnimEdgeFaded)), edgeThick)
		sigStyle.Color = col
		sigStyle.Draw(dst, px, py, &cam)
		g.renderedPulsesCount++
		if g.logDrawNodes {
			sx := px*camScale + offX
			sy := py*camScale + offY + float64(gridTopOffset())
			g.logger.Tracef("[DRAW/PULSE] frame=%d row=%d t=%.2f world=(%.2f,%.2f) screen=(%.1f,%.1f)", g.frame, p.row, p.t, px, py, sx, sy)
		}
	}

	// Optional alignment summary across edges for quick scanning
	if g.logDrawNodes && len(g.edges) > 0 {
		var maxDA, maxDB float64
		var badA, badB *uiNode
		for i := range g.edges {
			e := &g.edges[i]
			ax1, ay1, ax2, ay2 := g.nodeScreenRect(e.A)
			acx, acy := (ax1+ax2)*0.5, (ay1+ay2)*0.5
			bx1, by1, bx2, by2 := g.nodeScreenRect(e.B)
			bcx, bcy := (bx1+bx2)*0.5, (by1+by2)*0.5
			ex0 := e.A.X*camScale + offX
			ey0 := e.A.Y*camScale + offY + float64(gridTopOffset())
			ex1 := e.B.X*camScale + offX
			ey1 := e.B.Y*camScale + offY + float64(gridTopOffset())
			da := math.Hypot(acx-ex0, acy-ey0)
			db := math.Hypot(bcx-ex1, bcy-ey1)
			if da > maxDA {
				maxDA, badA = da, e.A
			}
			if db > maxDB {
				maxDB, badB = db, e.B
			}
		}
		g.logger.Debugf("[ALIGN] edges=%d maxDA=%.2f (id=%v) maxDB=%.2f (id=%v)", len(g.edges), maxDA, idOrNil(badA), maxDB, idOrNil(badB))
	}

	// Coordinate badge above selected node
	if g.coordBadgeNode != nil {
		showBadge := false
		if Profile().IsMobile() {
			// Mobile: show while node is selected, but not when popup is open
			// (the badge renders on top of the popup since it draws after it).
			showBadge = (g.sel == g.coordBadgeNode) && !g.sidebar.IsOpen()
		} else {
			// Desktop: show for 180 frames (~3s at 60 TPS)
			showBadge = (g.frame-g.coordBadgeFrame < 180)
		}
		if !showBadge {
			g.coordBadgeNode = nil
		} else {
			bx1, by1, bx2, _ := g.nodeScreenRect(g.coordBadgeNode)
			bcx := int(math.Round((bx1 + bx2) * 0.5))
			bcy := int(math.Round(by1)) - 16
			if bcy < 2 {
				bcy = 2
			}
			badgeText := fmt.Sprintf("(%d, %d)", g.coordBadgeNode.I, g.coordBadgeNode.J)
			tw := len(badgeText)*7 + 8
			pillRect := image.Rect(bcx-tw/2, bcy-1, bcx+tw/2, bcy+13)
			drawRect(dst, pillRect, WithAlpha(genColorVizPillFill, genAlphaSidebarChip), true)
			drawRect(dst, pillRect, genColorVizPillBorder, false)
			DrawTextAt(dst, badgeText, pillRect.Min.X+4, pillRect.Min.Y+2)
		}
	}

	// Move mode visual feedback
	if g.moveMode && g.movingNode != nil {
		// Banner at top of grid pane
		bannerText := fmt.Sprintf("MOVING NODE (%d,%d) — CLICK TO PLACE", g.movingNode.I, g.movingNode.J)
		if Profile().ShowEscHint {
			bannerText += " (ESC TO CANCEL)"
		}
		bannerW := TextWidth(bannerText) + 16
		bannerX := (g.split.GridW(g.winW) - bannerW) / 2
		if bannerX < 0 {
			bannerX = 0
		}
		bannerRect := image.Rect(bannerX, 2, bannerX+bannerW, 22)
		drawRoundedRect(dst, bannerRect, colPanelBG, popupCornerRadius(), true)
		drawRoundedRect(dst, bannerRect, colPanelBorder, popupCornerRadius(), false)
		DrawTextAt(dst, bannerText, bannerX+8, 5)

		// Highlight moving node with pulsing yellow outline
		mx1, my1, mx2, my2 := g.nodeScreenRect(g.movingNode)
		hlCol := WithAlpha(genColorVizGlow, 255)
		var idm ebiten.GeoM
		DrawLineCam(dst, mx1-1, my1-1, mx2+1, my1-1, &idm, hlCol, 2)
		DrawLineCam(dst, mx2+1, my1-1, mx2+1, my2+1, &idm, hlCol, 2)
		DrawLineCam(dst, mx2+1, my2+1, mx1-1, my2+1, &idm, hlCol, 2)
		DrawLineCam(dst, mx1-1, my2+1, mx1-1, my1-1, &idm, hlCol, 2)

		// Ghost indicator at cursor position (snapped to grid)
		cmx, cmy := cursorPosition()
		if g.split.InGridPane(cmx, cmy) && cmy >= gridTopOffset() {
			gwx := (float64(cmx) - g.cam.OffsetX) / g.cam.Scale
			gwy := (float64(cmy-gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
			_, _, gi, gj := g.grid.Snap(gwx, gwy)
			gsx := g.cam.OffsetX + unitPx*float64(gi)
			gsy := g.cam.OffsetY + unitPx*float64(gj) + float64(gridTopOffset())
			gr := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
			ghostRect := image.Rect(int(gsx-gr), int(gsy-gr), int(gsx+gr), int(gsy+gr))
			drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaAccentOverlay), true)
			drawRect(dst, ghostRect, WithAlpha(genColorVizGlow, genAlphaStrong), false)
		}
	}

	// Move confirmation dialog
	if g.moveConfirm {
		dw, dh := 280, 60
		dx := (g.split.GridW(g.winW) - dw) / 2
		dy := (g.split.GridH(g.winH) - dh) / 2
		dialogRect := image.Rect(dx, dy, dx+dw, dy+dh)
		drawPanel(dst, dialogRect)
		msgText := fmt.Sprintf("Moving will remove %d edge(s). Continue?", g.moveEdgeLoss)
		DrawTextAt(dst, msgText, dx+10, dy+8)
		// Confirm and cancel buttons
		radius := popupCornerRadius()
		confirmRect := image.Rect(dx+40, dy+30, dx+120, dy+50)
		cancelRect := image.Rect(dx+160, dy+30, dx+240, dy+50)
		drawRoundedRect(dst, confirmRect, WithAlpha(genColorVizConfirmGreen, 255), radius, true)
		DrawTextAt(dst, "Move", confirmRect.Min.X+12, confirmRect.Min.Y+4)
		drawRoundedRect(dst, cancelRect, WithAlpha(genColorVizCancelRed, 255), radius, true)
		DrawTextAt(dst, "Cancel", cancelRect.Min.X+8, cancelRect.Min.Y+4)
	}

	// Long-press quick-action popup (mobile)
	g.drawLongPressPopup(dst)

	// Connect mode visual feedback
	g.drawConnectMode(dst)

	// cursor coordinate label (hidden on mobile)
	mx, my := cursorPosition()
	if Profile().ShowCursorLabel && g.split.InGridPane(mx, my) {
		camScale := unitPx / g.grid.Unit()
		wx := (float64(mx) - offX) / camScale
		wy := (float64(my) - offY - float64(gridTopOffset())) / camScale
		_, _, ix, iy := g.grid.Snap(wx, wy)
		bx, nx, dx := g.grid.BeatSubdivision(ix)
		by, ny, dy := g.grid.BeatSubdivision(iy)
		xs := "0"
		ys := "0"
		if nx != 0 {
			xs = fmt.Sprintf("%d/%d", nx, dx)
		}
		if ny != 0 {
			ys = fmt.Sprintf("%d/%d", ny, dy)
		}
		g.cursorLabel = fmt.Sprintf("(%d:%s, %d:%s)", bx, xs, by, ys)
		DrawTextAt(screen, g.cursorLabel, mx+8, my+16)
	} else {
		g.cursorLabel = ""
	}

	// Divider drawn after both panes
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
