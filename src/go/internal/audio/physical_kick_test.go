//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// physicalKickIDs are the shipped variant-8 (Physical) kicks.
var physicalKickIDs = []instrument.ID{
	roster.KickBoomBap, roster.KickBrutal, roster.KickRock, roster.KickWood, roster.KickAiry,
	roster.KickOrganicTight, roster.KickMuted, roster.KickMutedRoom,
}

// A kick heard as "fuzzy" is WARBLE: 2-30 Hz amplitude modulation from a partial near the fundamental that does not
// ride its pitch glide. Real kicks measure 0.41-0.62.
func TestPhysicalKicksDoNotWarble(t *testing.T) {
	const ceiling = 0.70
	for _, id := range physicalKickIDs {
		buf, sr := RenderInstrumentOneShotRaw(id)
		if len(buf) == 0 {
			t.Fatalf("%s rendered nothing", id)
		}
		fp := fingerprint.KickAnalyze(wave.Wave{Samples: f32toF64(buf), SampleRate: sr})
		if fp.WarbleDepth > ceiling {
			t.Errorf("%s WarbleDepth = %.2f > %.2f — the body is fluttering (heard as fuzz); recorded kicks measure 0.41-0.62", id, fp.WarbleDepth, ceiling)
		}
	}
}

// An unset kick knob reaches C as 0, and a variant-8 kick missing env0 rings undamped; validation requires every
// value a kick.physical layer declares.
func TestPhysicalKicksAreKickPhysicalDocuments(t *testing.T) {
	for _, id := range physicalKickIDs {
		d, ok := ShippedDoc(id)
		if !ok {
			t.Fatalf("no shipped document %q", id)
		}
		if len(d.Layers) == 0 || d.Layers[0] == nil || d.Layers[0].Type != "kick.physical" {
			t.Errorf("%s: first layer is not a kick.physical module", id)
			continue
		}
		if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
}
