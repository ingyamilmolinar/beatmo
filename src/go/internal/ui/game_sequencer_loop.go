package ui

import (
	"math"
	"os"
	"strconv"
	"time"
)

// sequencerLoop listens to engine tick events and schedules audio playback
// strictly on the engine timeline, decoupled from UI rendering.
func (g *Game) sequencerLoop() {
	// Browser uses a longer tick (~4ms) to reduce main-thread pressure;
	// audio lookahead keeps scheduling safe. Desktop uses a tight 1ms tick.
	tick := time.Duration(RuntimeProf().SequencerTickMS) * time.Millisecond
	if tick <= 0 {
		tick = time.Millisecond
	}
	if v := os.Getenv("SEQ_TICK_MS"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms > 0 {
			tick = time.Duration(ms) * time.Millisecond
		}
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-g.seqQuit:
			return
		case <-ticker.C:
			if !g.Playing() {
				continue
			}
			g.seqScheduleTime()
		}
	}
}

// zoomAtScreen applies zoom centered at a given screen-space coordinate.
func (g *Game) zoomAtScreen(sx, sy float64, steps float64) {
	wx := (sx - g.cam.OffsetX) / g.cam.Scale
	// Account for the transport bar offset in screen space
	wy := (sy - float64(gridTopOffset()) - g.cam.OffsetY) / g.cam.Scale
	const zoomFactor = 1.05
	const zoomSensitivity = 0.1
	newScale := g.cam.Scale * math.Pow(zoomFactor, steps*zoomSensitivity)
	if newScale < 0.1 {
		newScale = 0.1
	} else if newScale > 10.0 {
		newScale = 10.0
	}
	g.cam.OffsetX = sx - wx*newScale
	g.cam.OffsetY = sy - float64(gridTopOffset()) - wy*newScale
	g.cam.Scale = newScale
	g.cam.Snap()
}
