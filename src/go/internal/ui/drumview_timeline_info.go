package ui

import (
	"fmt"
	"math"
)

func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	curS := curMS / 1000

	return fmt.Sprintf("Beat %d · %s", int(elapsedBeats)+1, formatElapsedTime(curS))
}

// formatElapsedTime renders totalSeconds as M:SS while under an hour and as
// H:MM:SS once it reaches one hour. Switching forms (rather than always
// emitting H:MM:SS) keeps the readout compact for the common case while
// preventing minute counts from running past 60.
func formatElapsedTime(totalSeconds int) string {
	if totalSeconds < 0 {
		totalSeconds = 0
	}
	if totalSeconds >= 3600 {
		h := totalSeconds / 3600
		m := (totalSeconds % 3600) / 60
		s := totalSeconds % 60
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", totalSeconds/60, totalSeconds%60)
}

// max1 returns at least 1 to avoid division by zero for unit conversions.
func max1(n int) int {
	if n <= 0 {
		return 1
	}
	return n
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}
