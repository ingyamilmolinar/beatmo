package audio

import "sync"

// loudnessAmp returns the loudness-normalized playback amplitude for an
// instrument id and whether a measured value exists. The values are generated
// offline by cmd/measure-loudness (instrument_loudness_gen.go) so that every
// instrument sits at a common perceived loudness. Build-tag-free: it is the
// single amplitude source of truth across desktop, test, and wasm Go builds.
//
// Runtime-registered instruments (clones / user Save-As) have no offline
// measurement, so a clone inherits its source's amp via runtimeLoudnessAmp —
// without it an exact clone would fall back to DefaultAmplitude and render
// perceptibly louder/quieter than the instrument it copies.
func loudnessAmp(id string) (float32, bool) {
	if v, ok := runtimeLoudnessAmp.Load(id); ok {
		return v.(float32), true
	}
	v, ok := instrumentLoudnessAmp[id]
	return v, ok
}

// runtimeLoudnessAmp overlays instrumentLoudnessAmp for ids created at runtime
// (clones). It takes precedence so a clone reports the same amplitude as its
// source, making an exact clone render byte-identically.
var runtimeLoudnessAmp sync.Map // id(string) -> float32

// setRuntimeLoudnessAmp records the loudness amp a runtime instrument should
// use (its source's measured amp). clearRuntimeLoudnessAmp drops it on delete.
func setRuntimeLoudnessAmp(id string, amp float32) { runtimeLoudnessAmp.Store(id, amp) }
func clearRuntimeLoudnessAmp(id string)            { runtimeLoudnessAmp.Delete(id) }
