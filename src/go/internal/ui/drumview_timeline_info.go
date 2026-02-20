package ui

import (
	"fmt"
	"math"
)

func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	curS := curMS / 1000

	return fmt.Sprintf("Beat %d · %d:%02d", int(elapsedBeats)+1, curS/60, curS%60)
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
