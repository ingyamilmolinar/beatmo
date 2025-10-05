//go:build test

package ui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
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

// buildDemo imports a demo config from TUNKUL_DEMO_CONFIG (or TUNKUL_CONFIG)
// during tests so we can validate the env-based demo path without building the
// full programmatic demo.
func (g *Game) buildDemo() {
	if g.demoBuilt || g.drum == nil || g.graph == nil {
		return
	}
	cfg := os.Getenv("TUNKUL_DEMO_CONFIG")
	if cfg == "" {
		cfg = os.Getenv("TUNKUL_CONFIG")
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
					g.drum.SetInstrument("kick")
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
