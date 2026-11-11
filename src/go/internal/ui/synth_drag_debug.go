package ui

// synthDragDebug gates verbose, per-frame Synth-tab drag/schema-swap logging.
// It is OFF by default (zero cost in production) and flipped on at runtime by
// the JS export __setSynthDragDebug(true) (see js_exports_synth_recipe.go) so a
// browser repro can capture the full UI→audio cascade in the console without a
// special build. Desktop never sets it. Added while chasing the "changing the
// generator silences the soloed instrument" report — see
// project_synth_generator_drag_schema_swap memory.
var synthDragDebug bool

// lastSynthBuildSig dedupes the per-frame buildSynthTab log: buildSynthTab runs
// every frame (~30/s), so logging unconditionally floods the console and buries
// the meaningful transitions (gen_type / deferred / capturing changes). We only
// emit when the signature changes.
var lastSynthBuildSig string

// sdbg emits a gated INFO line (INFO surfaces in the WASM console by default).
// No-op unless synthDragDebug is set and the view has a logger.
func (dv *DrumView) sdbg(format string, a ...interface{}) {
	if !synthDragDebug || dv == nil || dv.logger == nil {
		return
	}
	dv.logger.Infof(format, a...)
}

// sdbgBuildTab logs the buildSynthTab state ONLY when it changes vs the previous
// frame, so the console shows transitions (open → drag → re-voice → release)
// instead of hundreds of identical lines.
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
