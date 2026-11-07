package ui

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Stored linear, stepped and shown in dB: loudness is logarithmic, and a linear step near the top
// was inaudible. The stored value's meaning is unchanged.
const nodeVolStepDb = 1.0 // perceptual change per +/- click

// Below this (≈ −80 dB) stepping snaps to 0, so true mute is reachable; no upper cap.
const nodeVolMuteFloor = 1e-4

var nodeVolStepFactor = math.Pow(10, nodeVolStepDb/20) // ~1.122 (=+1 dB)

// Stepping up from 0 lands on the floor, so both directions are reversible.
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

// "dB" stays literal in every locale.
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
