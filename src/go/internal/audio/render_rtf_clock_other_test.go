//go:build !linux && !test && !js

package audio

import "time"

var rtfClockStart = time.Now()

// renderCPUSeconds falls back to wall time where no per-thread CPU clock is
// at hand.
func renderCPUSeconds() float64 { return time.Since(rtfClockStart).Seconds() }
