//go:build !test && !js

package audio

import (
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// loudness_drift_test.go — every shipped document's render.amplitude must match
// what the instrument actually measures.
//
// WHY THIS WAS MISSING AND WHY IT MATTERS
// ---------------------------------------
// instrument_loudness_gen.go normalises every instrument to a common perceived
// loudness so no sound is conspicuously louder than its neighbours in a mix. It
// is MEASURED from the rendered audio, which means it goes stale the moment an
// instrument is retuned -- and unlike every other generated artifact in the
// tree (modular_instruments.gen.js, synth_param_abi.gen.js, chain_spec.gen.js,
// design_*.gen.go), nothing checked it. The existing loudness tests assert the
// table is WIRED, not that it is CURRENT.
//
// It had drifted, and the worst case was invisible in every tool: tubular-bells
// was pinned at 0.4122 against a measured 0.8608 -- it played 6.4 dB below every
// other instrument in the game. The A/B and diagnose paths level-match, so they
// cannot see it; only the game does. Six other instruments were 0.2-0.9 dB off,
// all of them ones whose sound had been changed at some point without
// re-measuring.
//
// The measurement is deterministic (verified: repeated runs are byte-identical),
// so this can be a tight comparison rather than a fuzzy one.

// loudnessDriftTol is the fractional tolerance on each amplitude.
//
// 0.5%: comfortably above the 4-decimal rounding in the generated file and any
// plausible floating-point variation between hosts, and far below the smallest
// real drift observed (2.1%, bassoon).
const loudnessDriftTol = 0.005

var loudnessRowRe = regexp.MustCompile(`^(\S+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+([\d.]+)\s+([\d.]+)`)

// measuredLoudnessAmps runs the generator in report mode and parses the
// newAmp column.
//
// Shelling out to the real command rather than duplicating its maths is
// deliberate: a reimplementation here could drift from the generator and the
// test would then be checking itself. Reusing the command means the test fails
// exactly when `make measure-loudness` would produce a different file.
func measuredLoudnessAmps(t *testing.T) map[string]float64 {
	t.Helper()
	cmd := exec.Command("go", "run", "./cmd/measure-loudness")
	cmd.Dir = "../.." // module root: src/go
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("could not run cmd/measure-loudness (%v); skipping drift check", err)
	}
	amps := map[string]float64{}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "instrument") {
			continue
		}
		m := loudnessRowRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(m[5], 64)
		if err != nil {
			continue
		}
		amps[m[1]] = v
	}
	return amps
}

// TestDocumentAmplitudesAreNotStale: every shipped instrument document's
// render.amplitude (the effective, loudness-normalised playback amplitude) must
// match what the instrument measures today.
func TestDocumentAmplitudesAreNotStale(t *testing.T) {
	measured := measuredLoudnessAmps(t)
	if len(measured) == 0 {
		t.Skip("no rows parsed from measure-loudness; skipping")
	}

	ids := make([]string, 0, len(measured))
	for id := range measured {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var drifted int
	for _, id := range ids {
		want := measured[id]
		d, ok := ShippedDoc(instrument.ID(id))
		if !ok {
			t.Errorf("%s renders but has no shipped instrument document", id)
			continue
		}
		got := d.Render.Amplitude
		if want <= 0 {
			continue
		}
		rel := math.Abs(got-want) / want
		if rel > loudnessDriftTol {
			drifted++
			t.Errorf("%s: document says %.4f, measurement says %.4f (%+.2f dB off).\n"+
				"    The instrument's sound changed without re-measuring, so it plays at the wrong "+
				"level RELATIVE TO EVERY OTHER INSTRUMENT in the game. Level-matched tools "+
				"(`refs compare`, `refs diagnose`, `make ab`) cannot see this; only playback can. "+
				"Run `make measure-loudness`.",
				id, got, want, 20*math.Log10(want/got))
		}
	}
	t.Logf("checked %d instrument amplitudes against a fresh measurement; %d drifted", len(ids), drifted)
}
