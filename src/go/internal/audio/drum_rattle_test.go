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

// Between the clean drums (real recordings score 0) and every take the listener called rattly; measured at 44.1 kHz.
const drumRattleCeiling = 50.0

// A ratchet: entries may only be removed when fixed, and a new rattly drum fails.
var knownRattlyDrums = map[string]string{}

// Each at its own length; BEATMO_RATTLE_REPORT=1 prints every score for recalibration.
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
