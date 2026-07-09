package ui

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Per-node volume is stored as a LINEAR multiplier (see model.NodeParams.Volume)
// and folded into the trigger gain as row×node (game_trigger_logic.go). The UI,
// however, steps and displays it in dB, because loudness perception is
// logarithmic: the previous ±0.10 linear step was only −0.9 dB at the top of the
// range (100%→90%) — inaudible — so users "could barely hear any difference".
// Stepping and reading in dB makes each click a fixed, clearly-audible change
// without altering the stored value's meaning (JSON import/export and songrender
// keep seeing a plain linear multiplier).
const nodeVolStepDb = 1.0 // perceptual change per +/- click

// nodeVolMuteFloor: stepping down below this linear level (≈ −80 dB, far below
// audibility) snaps to 0 so the control can reach true mute. There is NO upper
// cap and no lower wall above the floor — each +/- click is one fixed dB step,
// so per-node volume can go arbitrarily high or, via the floor, all the way to 0.
const nodeVolMuteFloor = 1e-4

var nodeVolStepFactor = math.Pow(10, nodeVolStepDb/20) // ~1.122 (=+1 dB)

// stepNodeVolume returns the node volume after one perceptual (±nodeVolStepDb)
// step. Volume stays a linear multiplier and is unbounded above; stepping down
// past nodeVolMuteFloor snaps to 0 (mute), and stepping up from 0 lands at that
// same floor so the two directions are reversible.
func stepNodeVolume(cur float64, up bool) float64 {
	if up {
		if cur <= 0 {
			return nodeVolMuteFloor
		}
		return cur * nodeVolStepFactor
	}
	if cur <= 0 {
		return 0
	}
	if v := cur / nodeVolStepFactor; v >= nodeVolMuteFloor {
		return v
	}
	return 0
}

// formatNodeVolumeDb renders a node volume (linear multiplier) as the dB readout
// shown in the sidebar. 1.0 → "0 dB", 0 → localized "Muted". The "dB" unit stays
// literal across locales per the i18n unit convention.
func formatNodeVolumeDb(v float64) string {
	if v <= 0 {
		return i18n.T(i18n.KeyAudMuted)
	}
	db := int(math.Round(20 * math.Log10(v)))
	if db == 0 {
		return "0 dB"
	}
	return fmt.Sprintf("%+d dB", db)
}
