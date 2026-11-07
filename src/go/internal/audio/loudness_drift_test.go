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

// render.amplitude is MEASURED from the audio, so a retune makes it stale, and only the game sees it: A/B and
// diagnose level-match. The measurement is deterministic, so the comparison can be tight.

// Above the 4-decimal rounding and host floating-point variation, far below any real drift.
const loudnessDriftTol = 0.005

var loudnessRowRe = regexp.MustCompile(`^(\S+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+([\d.]+)\s+([\d.]+)`)

// Shells out to the real command so the test fails exactly when `make measure-loudness` would write a different
// file; a reimplementation here could drift and check itself.
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
