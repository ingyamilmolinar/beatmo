//go:build js && !test

package ui

import "syscall/js"

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	g.initJSPlaybackPerf()
	g.initJSEqWidgets()
	g.initJSTimelinePredictor()
	g.initJSHarness()
	g.initJSGraphUI()
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
