package ui

// One INFO line per gesture: pan on release with the total delta; zoom per frame, debounced by the
// eventlogger coalescer.
type cameraGesture struct {
	panning   bool
	startOffX float64
	startOffY float64
	prevOffX  float64
	prevOffY  float64
	lastScale float64
	scaleInit bool
	emitPan   func(dx, dy float64)
	emitZoom  func(factor float64)
}

func (g *cameraGesture) observe(panning bool, offX, offY, scale float64) {
	// Zoom: any scale change this frame is a zoom tick.
	if g.scaleInit && scale != g.lastScale && g.emitZoom != nil {
		g.emitZoom(scale / g.lastScale)
	}
	g.lastScale = scale
	g.scaleInit = true

	// The start offset is captured on the idle frame before the gesture, so the delta is the full travel.
	switch {
	case panning && !g.panning: // gesture start — snapshot the pre-drag position
		g.startOffX, g.startOffY = g.prevOffX, g.prevOffY
	case !panning && g.panning: // gesture end
		dx, dy := offX-g.startOffX, offY-g.startOffY
		if (dx != 0 || dy != 0) && g.emitPan != nil {
			g.emitPan(dx, dy)
		}
	}
	g.panning = panning
	g.prevOffX, g.prevOffY = offX, offY
}
