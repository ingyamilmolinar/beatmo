package ui

// closeAllPopups closes the node sidebar and all drum view popups.
//
//nolint:unused // called from js_exports_graph_ui.go (WASM build tag)
func (g *Game) closeAllPopups() {
	g.sidebar.Close()
	g.dismissLongPressPopup("close-popups")
	g.modes.Cancel("close-popups")
	g.moveSkipRelease = false
	g.drum.CloseAllPopups()
}
