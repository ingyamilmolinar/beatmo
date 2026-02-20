package ui

import (
	"math"
	"os"
)

/* ───────────────── helper: node’s screen rect ───────────────── */

// Rectangle in *screen* pixels (y already includes the transport offset).
func (g *Game) nodeScreenRect(n *uiNode) (x1, y1, x2, y2 float64) {
	unitPx := g.grid.UnitPixels(g.cam.Scale) // px per smallest subdivision
	// Allow disabling pixel snapping for diagnostics.
	var offX, offY float64
	if os.Getenv("NO_PIXEL_SNAP") == "1" {
		offX = g.cam.OffsetX
		offY = g.cam.OffsetY
	} else {
		offX = math.Round(g.cam.OffsetX)
		offY = math.Round(g.cam.OffsetY)
	}

	sx := offX + unitPx*float64(n.I)
	sy := offY + unitPx*float64(n.J) + float64(gridTopOffset())
	// Use baseline grid radius for screen-rect math to keep alignment tests
	// stable; draw path applies animation/hover scaling visually.
	r := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
	return sx - r, sy - r, sx + r, sy + r
}
