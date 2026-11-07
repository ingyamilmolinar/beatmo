//go:build !js

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// Device sample-pos → wall seconds via the clock anchor. The -tags test stub returns 0, so UI tests call
// closeTimingTrace directly.
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
