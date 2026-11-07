//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Without it the desktop legacy-voice path keeps playing the chop, because the recipe path is skipped for an
// un-customised instrument.
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
