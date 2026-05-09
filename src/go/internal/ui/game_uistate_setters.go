package ui

// game_uistate_setters.go exposes thin wrappers used by the uistate
// subpackage to apply declarative -ui-state config. They live here (not
// in uistate/) so they can touch the unexported Game fields directly.

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

// CenterCamera resets the camera to the centered default. Used by
// scene boot to ensure deterministic framing across captures.
func (g *Game) CenterCamera() {
	if g.cam == nil {
		return
	}
	g.centered = false
}

// SetSplitterFrac sets the splitter divider as a fraction of the relevant
// dimension (0..1). Marks userSet so the layout pass preserves it.
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
		// Y/winH; use a sensible default totalH that the next UpdateResize
		// pass will refine. The userSet flag preserves the ratio.
		g.split.Y = int(frac * 720)
	} else {
		g.split.X = int(frac * 1280)
	}
	g.split.userSet = true
}

// SetViewMode toggles between Rows and Audio (mobile EQ/Wave) view.
// audio=true switches to the audio view.
func (g *Game) SetViewMode(audio bool) {
	if g.drum == nil {
		return
	}
	if audio {
		g.drum.SetMobileEQMode(true)
		g.drum.currentViewMode = viewModeEQ
	} else {
		g.drum.SetMobileEQMode(false)
		g.drum.currentViewMode = viewModeRows
	}
	g.drum.refreshWidgetLayout()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	g.drum.markAllRowsDirty()
	g.drum.rowsLayerDirty = true
}

// SetMobileEQCollapsed wraps DrumView.SetMobileEQCollapsed.
func (g *Game) SetMobileEQCollapsed(b bool) {
	if g.drum != nil {
		g.drum.SetMobileEQCollapsed(b)
	}
}

// CameraOffsets returns the current camera offset as (x, y). Read
// counterpart to SetCameraOffsetX/Y; returns (0, 0) when the camera is
// not yet initialized.
func (g *Game) CameraOffsets() (float64, float64) {
	if g.cam == nil {
		return 0, 0
	}
	return g.cam.OffsetX, g.cam.OffsetY
}

// CameraScale returns the current camera zoom scale. Read counterpart
// to SetCameraScale; returns 0 when the camera is not yet initialized.
func (g *Game) CameraScale() float64 {
	if g.cam == nil {
		return 0
	}
	return g.cam.Scale
}

// SidebarOpen reports whether the node parameter sidebar is currently
// visible. Read counterpart to OpenSidebarForNodeID.
func (g *Game) SidebarOpen() bool {
	return g.sidebar != nil && g.sidebar.IsOpen()
}

// OpenSidebarForNodeID opens the node parameter sidebar for the node with
// the given ID (the same numeric ID exposed via JS exports / import JSON).
// No-op if the node is not in the current graph.
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
