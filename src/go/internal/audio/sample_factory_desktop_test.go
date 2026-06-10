//go:build !test && !js

package audio

import "testing"

// TestUnregisterSamplePCMRestoresFactoryInstrument proves the desktop half of
// the factory-Reset fix: after the Sampler's Save registered a chopped Sample
// over a built-in (kick-1), UnregisterSamplePCM must restore the as-shipped
// CVariantInstrument so the synth — not the leftover chop — is what plays.
// Without this, the desktop legacy-voice path keeps the reversed Sample because
// the as-shipped recipe path is skipped for an un-customised instrument.
func TestUnregisterSamplePCMRestoresFactoryInstrument(t *testing.T) {
	ResetInstruments() // snapshots factoryInstruments

	RegisterSamplePCM("kick-1", []float32{0.1, 0.2, 0.3}, SampleRate())
	instMu.RLock()
	_, isSample := instruments["kick-1"].(Sample)
	instMu.RUnlock()
	if !isSample {
		t.Fatalf("precondition: kick-1 should be a Sample after RegisterSamplePCM")
	}

	UnregisterSamplePCM("kick-1")

	instMu.RLock()
	inst := instruments["kick-1"]
	instMu.RUnlock()
	cv, ok := inst.(CVariantInstrument)
	if !ok || cv.Name != "kick-1" {
		t.Errorf("after UnregisterSamplePCM kick-1 = %T (%+v), want factory CVariantInstrument kick-1", inst, inst)
	}
}
