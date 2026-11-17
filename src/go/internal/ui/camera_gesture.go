package ui

// cameraGesture collapses the per-frame camera mutations from
// Camera.HandleMouse into one INFO line per finished gesture:
//
//   - pan  → emitted once on press→release, carrying the cumulative offset delta
//     (measured from the camera position on the frame before the gesture started)
//   - zoom → emitted each frame Scale changes (the eventlogger coalescer
//     debounce-trails a continuous wheel/pinch burst into one trailing line)
//
// It holds no Ebiten state; game_update.go feeds it the post-HandleMouse
// snapshot each frame.
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

// observe is called once per frame with the camera's drag flag and current
// offset/scale (already mutated by HandleMouse this frame).
func (g *cameraGesture) observe(panning bool, offX, offY, scale float64) {
	// Zoom: any scale change this frame is a zoom tick.
	if g.scaleInit && scale != g.lastScale && g.emitZoom != nil {
		g.emitZoom(scale / g.lastScale)
	}
	g.lastScale = scale
	g.scaleInit = true

	// Pan: track press→release, emit cumulative delta on release.
	// startOffX/Y is captured from the idle frame before the gesture starts,
	// so the delta measures the full camera travel of the gesture.
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
