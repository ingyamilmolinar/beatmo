//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestUnregisterSamplePCMRestoresFactoryInstrument proves the desktop half of
// the factory-Reset fix: after the Sampler's Save registered a chopped Sample
// over a built-in (kick-snap), UnregisterSamplePCM must restore the as-shipped
// document instrument so the synth — not the leftover chop — is what plays.
// Without this, the desktop legacy-voice path keeps the reversed Sample because
// the as-shipped recipe path is skipped for an un-customised instrument.
func TestUnregisterSamplePCMRestoresFactoryInstrument(t *testing.T) {
	ResetInstruments() // snapshots factoryInstruments

	RegisterSamplePCM(roster.KickSnap, []float32{0.1, 0.2, 0.3}, SampleRate())
	instMu.RLock()
	_, isSample := instruments[roster.KickSnap].(Sample)
	instMu.RUnlock()
	if !isSample {
		t.Fatalf("precondition: kick-snap should be a Sample after RegisterSamplePCM")
	}

	UnregisterSamplePCM(roster.KickSnap)

	instMu.RLock()
	inst := instruments[roster.KickSnap]
	instMu.RUnlock()
	di, ok := inst.(docInstrument)
	if !ok || di.id != roster.KickSnap {
		t.Errorf("after UnregisterSamplePCM kick-snap = %T (%+v), want the factory docInstrument kick-snap", inst, inst)
	}
}
