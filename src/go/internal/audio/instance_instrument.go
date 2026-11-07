package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Makes an instance id ("organ--2") a playable instrument that renders IDENTICALLY to its base, or a template that
// repeats an instrument would fail to import and play silence. A synth base gets its own document copy (Origin =
// base); a sample base is aliased. Mutates the process-global registry: tests t.Cleanup with ResetInstruments.
func EnsureInstanceInstrument(id instrument.ID) {
	if instanceAlreadyRegistered(id) {
		return
	}
	base := id.Base()
	if base == id {
		return
	}
	if !instanceAlreadyRegistered(base) {
		return
	}
	// A synth instance gets its OWN document copy (spec §6) and plays it.
	if _, ok := EnsureInstrumentDoc(id); ok {
		registerDocInstrument(id)
		return
	}
	registerInstanceAlias(id, base)
	// A non-document instance has no render.amplitude of its own and would fall back to DefaultAmplitude, off the base's
	// level.
	setRuntimeLoudnessAmp(id, ConfigForInstrument(base).Amplitude)
}
