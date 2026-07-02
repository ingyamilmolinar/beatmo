//go:build js && !test

package ui

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	softKeyboardInit()
	filePickerInit()
	g.initFilePickerActions()
	g.initJSPlaybackPerf()
	g.initJSEqWidgets()
	g.initJSTimelinePredictor()
	g.initJSHarness()
	g.initJSGraphUI()
	g.initJSTouchDebug()
	g.initJSInsertEffects()
	g.initJSSynthRecipe()
	g.initJSRecording()
	g.initMediaSessionExports()
	g.initJSScenes()
	g.initJSDiag()
	g.initJSSlimBarDiag()
}
