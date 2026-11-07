package ui

import (
	"fmt"
	"strconv"
)

// NumberModels, so the shared ValueWheel scrolls them like any other value.

// 1-BASED like the beat counter, plus the reduced fraction inside the beat ("Beat 3 · 3/8").
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

// In beats; the 1/div rung is prepended when div > 1.
var gotoBeatStrides = []int{1, 4, 16}

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

// Rounded down.
func gotoStepsToSeconds(steps, div, bpm int) int {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		bpm = 120
	}
	return steps * 60 / (div * bpm)
}
