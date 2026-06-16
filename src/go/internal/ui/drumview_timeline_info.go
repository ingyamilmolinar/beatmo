package ui

import (
	"fmt"
	"math"
)

func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	curS := curMS / 1000

	return formatBeatReadout(int(elapsedBeats)+1, curS)
}

// formatBeatReadout renders the beat-counter readout, position + elapsed time
// on both platforms. The desktop band has room for the labelled form
// ("Beat 12 · 1:04"); the mobile band above the step grid is bracketed by the
// track/lock chip (left) and the row-zoom chips (right) and is only ~90 px
// wide — too narrow for the labelled form plus the dedicated notification
// slot. On mobile we drop the "Beat " label and keep the compact "12 · 1:04"
// form, which fits position + time AND leaves the notification area its slot.
// Both the drawn text (timelineInfoCached) and the stable slot width
// (beatCounterSlotWidth) route through the same mobile branch so they can
// never disagree (the regression behind the clipped "Beat 1 · 0:(" garbage).
func formatBeatReadout(beat, totalSeconds int) string {
	if Profile().IsMobile() {
		return fmt.Sprintf("%d · %s", beat, formatElapsedTime(totalSeconds))
	}
	return fmt.Sprintf("Beat %d · %s", beat, formatElapsedTime(totalSeconds))
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
