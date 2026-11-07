//go:build test

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func expandHome(p string) string {
	if p == "" {
		return p
	}
	if p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Under tests only the env-config path is built.
func (g *Game) buildDemo() {
	if g.demoBuilt || g.drum == nil || g.graph == nil {
		return
	}
	cfg := os.Getenv("BEATMO_DEMO_CONFIG")
	if cfg == "" {
		cfg = os.Getenv("BEATMO_CONFIG")
	}
	if cfg == "" {
		return
	}
	if data, err := os.ReadFile(expandHome(cfg)); err == nil {
		if err := g.Import(data); err == nil {
			// Fallbacks when the file has no instruments or no start set.
			if len(g.drum.Rows) == 0 {
				minID := model.InvalidNodeID
				for id, n := range g.graph.Nodes {
					if n.Type == model.NodeTypeInvisible {
						continue
					}
					if minID == model.InvalidNodeID || id < minID {
						minID = id
					}
				}
				if minID != model.InvalidNodeID {
					g.drum.AddRow()
					g.drum.selRow = 0
					g.drum.SetInstrument(roster.Kick)
					if len(g.drum.Rows) > 0 {
						g.drum.Rows[0].Name = "Row"
						g.drum.Rows[0].Origin = minID
						g.drum.Rows[0].Node = g.nodeByID(minID)
					}
					g.start = g.nodeByID(minID)
					g.graph.StartNodeID = minID
				}
			} else if g.graph.StartNodeID == model.InvalidNodeID && g.drum.Rows[0].Origin != model.InvalidNodeID {
				id := g.drum.Rows[0].Origin
				g.graph.StartNodeID = id
				g.start = g.nodeByID(id)
			}
			g.updateBeatInfos()
			g.demoBuilt = true
		}
	}
}

// RunDemo is a no-op stub in test builds.
func (g *Game) RunDemo() {}

// Functional under tests so the lifecycle runs through direct Update() calls.
func (g *Game) RunBenchmark(bpm int, dur time.Duration, storm benchStormScenario, stormInst instrument.ID) {
	g.benchBPM = bpm
	g.benchDuration = dur
	g.benchStorm = storm
	g.benchStormInst = stormInst
}

// RunRecordBenchmark is the test-build stub matching demo.go's signature.
func (g *Game) RunRecordBenchmark(bpm int, dur time.Duration, outDir string) {
	g.benchBPM = bpm
	g.benchDuration = dur
	g.benchRecord = true
	g.benchOutDir = outDir
}
