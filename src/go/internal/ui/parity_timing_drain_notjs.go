//go:build !js

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// drainEmissionReports pulls the mixer's lock-free emission reports, maps each
// device sample-pos to wall-seconds via the device-clock anchor, and closes the
// matching timing trace (feeding Stage-D drift). Under -tags test the audio
// stub returns 0, so UI tests drive Stage-D via closeTimingTrace directly.
func (g *Game) drainEmissionReports() {
	n := audio.DrainEmitReports(g.emissionReportBuf)
	if n == 0 {
		return
	}
	pos, wall, ok := audio.DeviceClockSnapshot()
	if !ok {
		return
	}
	sr := float64(audio.SampleRate())
	for i := 0; i < n; i++ {
		r := g.emissionReportBuf[i]
		g.closeTimingTrace(r.Row, r.Abs, wall+float64(r.DevPos-pos)/sr)
	}
}
