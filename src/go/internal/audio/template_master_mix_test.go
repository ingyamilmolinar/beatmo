//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// The mix must keep headroom below the soft-clip knee (0.9), so the clip is a safety net, not a constantly slammed
// distortion stage; the browser's half is chain_spec_parity / master_headroom.

func TestTemplateMasterMix(t *testing.T) {
	// Worst case: every instrument on one downbeat, applied as on import.
	for _, tp := range assets.Templates() {
		d := parseTemplateDoc(t, tp.Genre, tp.Bytes)
		peak := renderTemplateAllHitsPeak(t, d)
		headroomDB := 20 * math.Log10(0.9/math.Max(peak, 1e-9))
		t.Logf("%-9s all-hits-downbeat pre-limiter peak=%.3f  (%.1f dB below 0.90)", tp.Genre, peak, headroomDB)
		if peak > 0.9 {
			t.Errorf("%s: aligned-downbeat peak %.3f > 0.9 — clips the master when everything hits together (raise per-voice headroom / lower volumes)", tp.Genre, peak)
		}
	}
}
