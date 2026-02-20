//go:build js && !test

package ui

import "syscall/js"

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	softKeyboardInit()
	filePickerInit()
	g.initJSPlaybackPerf()
	g.initJSEqWidgets()
	g.initJSTimelinePredictor()
	g.initJSHarness()
	g.initJSGraphUI()
	g.initJSTouchDebug()
	g.initJSInsertEffects()
	g.initJSRecording()
	g.initMediaSessionExports()
}

// reportStateJS publishes the current beat for tests.
func (g *Game) reportStateJS() {
	js.Global().Set("__beat", js.ValueOf(g.currentBeat()))
}
