package ui

import (
	"math"
	"time"
)

// gen/parityGen pin the generation at open, so a report after a live edit is dropped.
type timingTrace struct {
	row, abs  int
	gen       uint64
	parityGen uint64
	idealWhen float64
	openAt    time.Time
}

// WarnOnly lets emitTimingFatal downgrade the breach even when the gate is fatal.
type timingBreach struct {
	Stage    string
	Stat     string
	Value    float64
	Limit    float64
	Count    int64
	WarnOnly bool
}

// absmax first (fires without MinSamples), then the statistical checks; a bound of 0 disables a
// statistic; mean compares by absolute value so bias in either direction is caught.
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
