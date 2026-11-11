package ui

import (
	"fmt"
	"strconv"
)

// Value models for the navigation menu's go-to wheel (beat and time modes).
// Both are NumberModels so the shared ValueWheel scrolls them exactly like
// any other value.

// formatGotoBeat renders an absolute subdivision step as the user reads it:
// "Beat N" with N 1-BASED (step 0 is Beat 1 — the same number the beat
// counter shows), plus the position inside the beat as a reduced fraction
// when it is off the beat ("Beat 3 · 3/8", "Beat 2 · 1/2").
func formatGotoBeat(steps, div int) string {
	if div <= 0 {
		div = 1
	}
	if steps < 0 {
		steps = 0
	}
	beat := steps/div + 1
	num := steps % div
	if num == 0 {
		return "Beat " + strconv.Itoa(beat)
	}
	g := gcdInt(num, div)
	return fmt.Sprintf("Beat %d · %d/%d", beat, num/g, div/g)
}

func gcdInt(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}

// gotoBeatStrides are the go-to wheel's beat-mode step sizes, in beats, coarse
// rungs only; the 1/div subdivision rung is prepended when div > 1.
var gotoBeatStrides = []int{1, 4, 16}

// newGotoBeatModel builds the beat-mode model over absolute subdivision steps
// [0, maxSteps], starting at startSteps, with the one-beat rung selected.
func newGotoBeatModel(div, maxSteps, startSteps int) *NumberModel {
	if div <= 0 {
		div = 1
	}
	var ladder []float64
	if div > 1 {
		ladder = append(ladder, 1)
	}
	for _, b := range gotoBeatStrides {
		ladder = append(ladder, float64(b*div))
	}
	rung := 0
	if div > 1 {
		rung = 1
	}
	m := &NumberModel{
		Min: 0, Max: float64(maxSteps), Ladder: ladder, Rung: rung,
		Format: func(v, _ float64) string { return formatGotoBeat(int(v), div) },
		LadderFormat: func(step float64) string {
			s := int(step)
			switch {
			case s < div:
				return fmt.Sprintf("%d/%d", s, div)
			case s == div:
				return "1 beat"
			default:
				return fmt.Sprintf("%d beats", s/div)
			}
		},
	}
	m.Value = m.clamp(float64(startSteps))
	return m
}

// gotoTimeStrides are the time-mode step sizes in seconds.
var gotoTimeStrides = []float64{1, 5, 15, 60}

// newGotoTimeModel builds the time-mode model over whole seconds [0, maxSec].
func newGotoTimeModel(maxSec, startSec int) *NumberModel {
	m := &NumberModel{
		Min: 0, Max: float64(maxSec), Ladder: gotoTimeStrides, Rung: 0,
		Format: func(v, _ float64) string { return formatClock(int(v)) },
		LadderFormat: func(step float64) string {
			if step >= 60 {
				return strconv.Itoa(int(step)/60) + "m"
			}
			return strconv.Itoa(int(step)) + "s"
		},
	}
	m.Value = m.clamp(float64(startSec))
	return m
}

// gotoStepsToSeconds converts an absolute subdivision step to whole seconds
// (rounded down) at the given tempo.
func gotoStepsToSeconds(steps, div, bpm int) int {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		bpm = 120
	}
	return steps * 60 / (div * bpm)
}
