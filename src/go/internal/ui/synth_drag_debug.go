package ui

// Off by default (zero cost); the JS export __setSynthDragDebug turns it on so a browser repro can capture
// the UI→audio cascade without a special build.
var synthDragDebug bool

// buildSynthTab runs every frame, so only signature changes are logged.
var lastSynthBuildSig string

// INFO surfaces in the WASM console by default.
func (dv *DrumView) sdbg(format string, a ...interface{}) {
	if !synthDragDebug || dv == nil || dv.logger == nil {
		return
	}
	dv.logger.Infof(format, a...)
}

func (dv *DrumView) sdbgBuildTab(sig string) {
	if !synthDragDebug || dv == nil || dv.logger == nil {
		return
	}
	if sig == lastSynthBuildSig {
		return
	}
	lastSynthBuildSig = sig
	dv.logger.Infof("[synthdrag] buildSynthTab %s", sig)
}
