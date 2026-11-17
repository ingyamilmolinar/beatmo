//go:build js && !test

package ui

// initJS exposes helper functions for browser-based tests.
func (g *Game) initJS() {
	afterJSExport = g.kickLane // bridge edits reach the audio lane before control returns to JS
	softKeyboardInit()
	filePickerInit()
	g.initFilePickerActions()
	g.initJSPlaybackPerf()
	g.initJSAudioTap()
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
