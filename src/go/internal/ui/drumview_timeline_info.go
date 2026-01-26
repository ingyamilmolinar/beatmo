package ui

import (
	"fmt"
	"math"
)

func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	totalBeats := math.Max(float64(dv.timelineBeats), elapsedBeats)

	// Convert to seconds and milliseconds with rounding, carrying overflows.
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	totMS := int(math.Round(totalBeats * dv.secPerBeat * 1000.0))
	curS, curMs := curMS/1000, curMS%1000
	totS, totMs := totMS/1000, totMS%1000

	return fmt.Sprintf("Beat %.3f/%.3f Time %ds %dms/%ds %dms", elapsedBeats, totalBeats, curS, curMs, totS, totMs)
}

// timelineInfoCached caches the last formatted timeline info string and only
// re-renders when milliseconds change. This avoids per-frame allocations.
func (dv *DrumView) timelineInfoCached(elapsedBeats float64) string {
	totalBeats := math.Max(float64(dv.timelineBeats), elapsedBeats)
	curMS := int(math.Round(elapsedBeats * dv.secPerBeat * 1000.0))
	totMS := int(math.Round(totalBeats * dv.secPerBeat * 1000.0))
	// Optional throttling on web builds to reduce per-frame text churn.
	if timelineInfoThrottleMS > 0 {
		thr := timelineInfoThrottleMS
		if (curMS/thr) == (dv.lastInfoCurMS/thr) && (totMS/thr) == (dv.lastInfoTotMS/thr) && dv.lastInfoText != "" {
			return dv.lastInfoText
		}
	} else {
		if curMS == dv.lastInfoCurMS && totMS == dv.lastInfoTotMS && dv.lastInfoText != "" {
			return dv.lastInfoText
		}
	}
	curS, curMs := curMS/1000, curMS%1000
	totS, totMs := totMS/1000, totMS%1000
	dv.lastInfoCurMS = curMS
	dv.lastInfoTotMS = totMS
	dv.lastInfoText = fmt.Sprintf("Beat %.3f/%.3f Time %ds %dms/%ds %dms", elapsedBeats, totalBeats, curS, curMs, totS, totMs)
	return dv.lastInfoText
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
