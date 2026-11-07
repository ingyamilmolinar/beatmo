package ui

import "fmt"

// Through playheadPosAt, like the navigation menu's title, so the two readouts cannot drift.
func (dv *DrumView) timelineInfo(elapsedBeats float64) string {
	p := playheadPosAt(elapsedBeats, dv.secPerBeat, dv.timelineUnitsPerBeat)
	m := readoutMemo{beat: p.DisplayBeat, seconds: p.Seconds, mobile: Profile().IsMobile()}
	if m.beat != dv.readout.beat || m.seconds != dv.readout.seconds || m.mobile != dv.readout.mobile || dv.readout.text == "" {
		m.text = formatBeatReadout(m.beat, m.seconds)
		dv.readout = m
	}
	return dv.readout.text
}

type readoutMemo struct {
	beat, seconds int
	mobile        bool
	text          string
}

// Mobile drops the "Beat " label: its band is ~90 px between the chips. The drawn text and
// beatCounterSlotWidth share this branch so they cannot disagree.
func formatBeatReadout(beat, totalSeconds int) string {
	if Profile().IsMobile() {
		return fmt.Sprintf("%d · %s", beat, formatElapsedTime(totalSeconds))
	}
	return fmt.Sprintf("Beat %d · %s", beat, formatElapsedTime(totalSeconds))
}

// M:SS under an hour, H:MM:SS from one hour, so minutes never run past 60.
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
