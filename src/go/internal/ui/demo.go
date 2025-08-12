package ui

import (
	"os"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// RunDemo builds a simple circuit and starts playback, then exits after a short delay.
func (g *Game) RunDemo() {
	a := g.tryAddNode(4, 1, model.NodeTypeRegular)
	b := g.tryAddNode(5, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.playing = true
	g.engine.Start()
	g.spawnPulse()
	go func() {
		time.Sleep(2 * time.Second)
		g.logger.Infof("[DEMO] Finished demo run")
		os.Exit(0)
	}()
}
