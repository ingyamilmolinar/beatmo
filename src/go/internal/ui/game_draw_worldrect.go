package ui

import (
	"image/color"
)

func visibleWorldRect(cam *Camera, screenW, screenH int) (minX, maxX, minY, maxY float64) {
	minX = (-cam.OffsetX) / cam.Scale
	maxX = (float64(screenW) - cam.OffsetX) / cam.Scale
	minY = (-cam.OffsetY - float64(gridTopOffset())) / cam.Scale
	maxY = (float64(screenH) - cam.OffsetY - float64(gridTopOffset())) / cam.Scale
	return
}

// rowColorSig returns a signature of current row colors to track cache invalidation.
func (g *Game) rowColorSig() uint64 {
	if g.drum == nil || len(g.drum.Rows) == 0 {
		return 0
	}
	s := uint64(len(g.drum.Rows))
	for i := range g.drum.Rows {
		r, g1, b, a := color.RGBAModel.Convert(g.drum.Rows[i].Color).(color.RGBA).RGBA()
		v := (uint64(r>>8) << 24) | (uint64(g1>>8) << 16) | (uint64(b>>8) << 8) | uint64(a>>8)
		s = (s*1469598103934665603 ^ v) * 1099511628211 // FNV-like mix
	}
	return s
}
