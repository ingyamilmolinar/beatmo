package ui

import (
	"math"
	"time"
)

// timingTrace records the ideal-when for a scheduled beat, opened at
// scheduleSound time and closed when the mixer/WebAudio reports the voice's
// actual emission (see openTimingTrace / closeTimingTrace in
// game_parity_timing.go). gen/parityGen pin the generation at open time so a
// report that lands after a live edit is dropped by closeTimingTrace's
// gen filter rather than polluting the Stage-D drift window.
type timingTrace struct {
	row, abs  int
	gen       uint64
	parityGen uint64
	idealWhen float64
	openAt    time.Time
}

// timingBreach describes the first statistic of a stage that exceeded its bound.
// WarnOnly is copied from the stage bounds so emitTimingFatal can downgrade a
// WarnOnly stage's breach to a log line even when the global gate is fatal.
type timingBreach struct {
	Stage    string
	Stat     string
	Value    float64
	Limit    float64
	Count    int64
	WarnOnly bool
}

// evalTimingStage applies the multi-statistic gate to one stage's window.
// Order of checks: absmax (single-sample dropout, fires regardless of
// MinSamples), then mean/p99/stddev (statistical, require MinSamples). A bound
// of 0 disables that statistic. mean is compared by absolute value so a signed
// systematic bias in either direction is caught.
func evalTimingStage(stage string, b timingStageBounds, count int64, mean, p99, stddev, absMax float64, minSamples int) *timingBreach {
	// Dropout-class: one reading so large it is an audible glitch, not noise.
	if b.AbsMax > 0 && absMax > b.AbsMax {
		return &timingBreach{Stage: stage, Stat: "absmax", Value: absMax, Limit: b.AbsMax, Count: count, WarnOnly: b.WarnOnly}
	}
	if count < int64(minSamples) {
		return nil
	}
	if b.MeanMax > 0 && math.Abs(mean) > b.MeanMax {
		return &timingBreach{Stage: stage, Stat: "mean", Value: mean, Limit: b.MeanMax, Count: count, WarnOnly: b.WarnOnly}
	}
	if b.P99Max > 0 && p99 > b.P99Max {
		return &timingBreach{Stage: stage, Stat: "p99", Value: p99, Limit: b.P99Max, Count: count, WarnOnly: b.WarnOnly}
	}
	if b.StdDevMax > 0 && !math.IsNaN(stddev) && stddev > b.StdDevMax {
		return &timingBreach{Stage: stage, Stat: "stddev", Value: stddev, Limit: b.StdDevMax, Count: count, WarnOnly: b.WarnOnly}
	}
	return nil
}
