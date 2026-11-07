package ui

// Here, not in uistate/, so they can touch unexported Game fields.

// SetCameraOffsetX sets the camera horizontal offset in screen pixels.
func (g *Game) SetCameraOffsetX(x float64) {
	if g.cam != nil {
		g.cam.OffsetX = x
		g.cam.Snap()
	}
}

// SetCameraOffsetY sets the camera vertical offset in screen pixels.
func (g *Game) SetCameraOffsetY(y float64) {
	if g.cam != nil {
		g.cam.OffsetY = y
		g.cam.Snap()
	}
}

// SetCameraScale sets the camera zoom scale (1.0 = no zoom).
func (g *Game) SetCameraScale(s float64) {
	if g.cam != nil && s > 0 {
		g.cam.Scale = s
	}
}

// Deterministic framing across captures.
func (g *Game) CenterCamera() {
	if g.cam == nil {
		return
	}
	g.centered = false
}

// Marks userSet so layout keeps it.
func (g *Game) SetSplitterFrac(frac float64) {
	if g.split == nil {
		return
	}
	if frac < 0.05 {
		frac = 0.05
	}
	if frac > 0.95 {
		frac = 0.95
	}
	if g.split.Horizontal() {
		// The next UpdateResize refines this from the ratio.
		g.split.Y = int(frac * 720)
	} else {
		g.split.X = int(frac * 1280)
	}
	// UpdateResize re-derives Y from the ratio, so the ratio is the persisted value.
	g.split.ratio = frac
	g.split.userSet = true
}

// Through setViewMode, never currentViewMode directly.
func (g *Game) SetViewMode(audio bool) {
	if g.drum == nil {
		return
	}
	g.drum.SetMobileEQMode(audio)
}

// SetMobileEQCollapsed wraps DrumView.SetMobileEQCollapsed.
func (g *Game) SetMobileEQCollapsed(b bool) {
	if g.drum != nil {
		g.drum.SetMobileEQCollapsed(b)
	}
}

// (0, 0) before the camera exists.
func (g *Game) CameraOffsets() (float64, float64) {
	if g.cam == nil {
		return 0, 0
	}
	return g.cam.OffsetX, g.cam.OffsetY
}

// 0 before the camera exists.
func (g *Game) CameraScale() float64 {
	if g.cam == nil {
		return 0
	}
	return g.cam.Scale
}

func (g *Game) SidebarOpen() bool {
	return g.sidebar != nil && g.sidebar.IsOpen()
}

// No-op if the node is not in the graph.
func (g *Game) OpenSidebarForNodeID(id int) {
	if g.sidebar == nil {
		return
	}
	for _, n := range g.nodes {
		if int(n.ID) == id {
			g.sidebar.Open(n)
			return
		}
	}
}
