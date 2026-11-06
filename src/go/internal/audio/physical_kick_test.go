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

// TestPhysicalKicksDoNotWarble pins the "fuzz" fix. The first physical kicks
// were heard as "very noisy/fuzzy"; the cause was WARBLE — 2-30 Hz amplitude
// modulation of the body (fingerprint WarbleDepth 1.8-3.4) from a detuned
// resonant head drifting through the pitch glide, and a shell mode at 0.86·f0
// beating against the settled fundamental. Real recorded kicks measure
// 0.41-0.62; the shipped physical kicks 0.32-0.60. Anything that puts a
// partial near the fundamental without riding its pitch path lands far above
// the ceiling.
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

// TestPhysicalKicksAreKickPhysicalDocuments guards the 0-not-NaN trap: an
// unset kick knob reaches C as 0 (there is no per-variant literal), so a
// variant-8 kick missing env0 rings undamped. Each physical kick's document
// plays a kick.physical layer; document validation requires every value that
// module type declares, so no knob can be missing.
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
