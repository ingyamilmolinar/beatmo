//go:build !test && !js

package audio

import (
	"os"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// drumRattleCeiling is the most late-tail rattle (fingerprint.TailRattle,
// ticks/s over 2-16 kHz) a shipped drum may have. Calibrated 2026-09-30 at
// 44.1 kHz: three real snare recordings and a real closed hat score 0; the
// clean shipped drums 0-23 (hats 21.7, snare-acoustic 22.7 — a fused buzz, heard
// as fine); every take the listener called rattly scored 150-720 (snare-deep
// 608 and tom-high-acoustic 663 as first shipped). The ceiling sits between.
const drumRattleCeiling = 50.0

// knownRattlyDrums are shipped drums still over the ceiling, with the reason.
// A ratchet: an entry may only be removed (when the instrument is fixed), and a
// NEW rattly drum fails.
var knownRattlyDrums = map[string]string{}

// TestShippedDrumsDoNotRattle renders every table drum (gen sources 5-9: kick,
// tom, snare, clap, cymbal) at its own length and fails when its late tail
// rattles. BEATMO_RATTLE_REPORT=1 prints every score (for recalibrating).
func TestShippedDrumsDoNotRattle(t *testing.T) {
	const sr = 44100
	type score struct {
		id string
		v  float64
	}
	var all []score
	for _, id := range roster.All {
		doc, _ := ShippedDoc(id)
		src := modularBlock(instrument.Flatten(doc, BuiltinModuleRegistry())).get("gen1_source")
		if src < 5 || src > 9 {
			continue
		}
		dur := doc.Render.Seconds
		if dur < 0.3 {
			dur = 0.3
		}
		n := int(dur * sr)
		buf := renderFieldsForTest(t, id, sr, n, nil)
		s := make([]float64, n)
		for i, x := range buf {
			s[i] = float64(x)
		}
		v := fingerprint.TailRattle(s, sr)
		all = append(all, score{string(id), v})
		if v > drumRattleCeiling {
			if why, known := knownRattlyDrums[string(id)]; known {
				t.Logf("known rattly drum %s: %.0f ticks/s (%s)", id, v, why)
				continue
			}
			t.Errorf("%s rattles: %.0f late-tail ticks/s above 2 kHz (ceiling %.0f). "+
				"A wire/rattle layer whose collision RATE decays with its amplitude thins "+
				"out into audible ticks — set gen2_cym_rattle_hold (and a dense "+
				"gen2_cym_rattle_rate), or drop the wire layer on a tom/conga.",
				id, v, drumRattleCeiling)
		}
	}
	if len(all) == 0 {
		t.Fatal("no drums measured — the table filter is wrong, so a pass proves nothing")
	}
	if os.Getenv("BEATMO_RATTLE_REPORT") != "" {
		sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
		for _, s := range all {
			t.Logf("RATTLE %-22s %6.1f", s.id, s.v)
		}
	}
}
