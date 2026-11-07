//go:build !test

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// expandHome expands a leading ~ in a path to the user's home directory.
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

func (g *Game) buildDemo() {
	if g.demoBuilt || g.drum == nil || g.graph == nil {
		return
	}

	// The startup circuit is not a user action: undo recording is suppressed for the whole build and the result
	// becomes the empty-history baseline, so Undo is not lit at launch.
	if g.undoManager != nil {
		g.undoManager.restoring = true
		defer func() {
			g.undoManager.restoring = false
			g.undoManager.OnExternalLoad()
		}()
	}

	cfg := os.Getenv("BEATMO_DEMO_CONFIG")
	if cfg == "" {
		cfg = os.Getenv("BEATMO_CONFIG")
	}
	if cfg != "" {
		path := expandHome(cfg)
		g.logger.Infof("[demo] Loading env config: %s", path)
		if data, err := os.ReadFile(path); err == nil {
			if err := g.Import(data); err == nil {
				g.logger.Infof("[demo] Imported env config: rows=%d nodes=%d start=%v", len(g.drum.Rows), len(g.graph.Nodes), g.graph.StartNodeID)
				// A config without instruments gets one default row and a start node, so playback works out of the box.
				if len(g.drum.Rows) == 0 {
					// Pick the lowest non-invisible node id as origin.
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
						g.logger.Infof("[demo] Added default row and start from node %d", minID)
					}
				} else if g.graph.StartNodeID == model.InvalidNodeID && g.drum.Rows[0].Origin != model.InvalidNodeID {
					// Ensure graph has a valid start matching row 0.
					id := g.drum.Rows[0].Origin
					g.graph.StartNodeID = id
					g.start = g.nodeByID(id)
					g.logger.Infof("[demo] Set start from row 0 origin: %d", id)
				}
				// Recompute beat paths so drum view and playback are ready.
				g.updateBeatInfos()
				g.logger.Infof("[demo] After import: beatPath[0]=%d drumLen=%d rows=%d", len(g.beatInfosByRow[0]), g.drum.Length, len(g.drum.Rows))
				g.demoBuilt = true
				return
			} else {
				g.logger.Infof("[demo] Failed to import %s: %v (falling back)", cfg, err)
			}
		} else {
			g.logger.Infof("[demo] Cannot read %s: %v (falling back)", cfg, err)
		}
	}

	// Nothing is built or tuned on top of the startup JSON. demoBuilt is set even on failure, so a broken asset is
	// reported once instead of every frame.
	g.demoBuilt = true
	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		g.logger.Errorf("[demo] Failed to import the embedded startup demo: %v", err)
		return
	}
	g.logger.Infof("[demo] Using embedded startup demo (rows=%d)", len(g.drum.Rows))
}

// RunDemo builds the demo circuit, starts playback, and exits after a short delay.
func (g *Game) RunDemo() {
	g.buildDemo()
	g.SetPlaying(true)
	g.engine.Start()
	g.spawnPulseFromRow(0, 0)
	go func() {
		time.Sleep(2 * time.Second)
		g.logger.Infof("[demo] Finished demo run")
		os.Exit(0)
	}()
}

// Exits via ebiten.Termination. benchStormOff + "" leaves the bench unchanged.
func (g *Game) RunBenchmark(bpm int, dur time.Duration, storm benchStormScenario, stormInst instrument.ID) {
	g.benchBPM = bpm
	g.benchDuration = dur
	g.benchStorm = storm
	g.benchStormInst = stormInst
	g.demoScheduled = true
}

// outDir must exist.
func (g *Game) RunRecordBenchmark(bpm int, dur time.Duration, outDir string) {
	g.benchBPM = bpm
	g.benchDuration = dur
	g.benchRecord = true
	g.benchOutDir = outDir
	g.demoScheduled = true
}
