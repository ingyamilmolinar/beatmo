//go:build !test

package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

// buildDemo constructs a multi-row, multi-length circuit showcasing the game.
// It places four instruments (kick, snare, two hi-hats) and creates distinct
// loops so the DrumView and grid highlight their traversal.
func (g *Game) buildDemo() {
	if g.demoBuilt || g.drum == nil || g.graph == nil {
		return
	}

	// Optional override: when BEATMO_DEMO_CONFIG (or BEATMO_CONFIG) is set,
	// load that JSON as the initial circuit.
	cfg := os.Getenv("BEATMO_DEMO_CONFIG")
	if cfg == "" {
		cfg = os.Getenv("BEATMO_CONFIG")
	}
	if cfg != "" {
		path := expandHome(cfg)
		g.logger.Infof("[DEMO] Loading env config: %s", path)
		if data, err := os.ReadFile(path); err == nil {
			if err := g.Import(data); err == nil {
				g.logger.Infof("[DEMO] Imported env config: rows=%d nodes=%d start=%v", len(g.drum.Rows), len(g.graph.Nodes), g.graph.StartNodeID)
				// If the imported config lacks instruments, create a sensible
				// default drum row and set a start node so playback and the
				// drum view work out of the box.
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
						g.drum.SetInstrument("kick")
						if len(g.drum.Rows) > 0 {
							g.drum.Rows[0].Name = "Row"
							g.drum.Rows[0].Origin = minID
							g.drum.Rows[0].Node = g.nodeByID(minID)
						}
						g.start = g.nodeByID(minID)
						g.graph.StartNodeID = minID
						g.logger.Infof("[DEMO] Added default row and start from node %d", minID)
					}
				} else if g.graph.StartNodeID == model.InvalidNodeID && g.drum.Rows[0].Origin != model.InvalidNodeID {
					// Ensure graph has a valid start matching row 0.
					id := g.drum.Rows[0].Origin
					g.graph.StartNodeID = id
					g.start = g.nodeByID(id)
					g.logger.Infof("[DEMO] Set start from row 0 origin: %d", id)
				}
				// Recompute beat paths so drum view and playback are ready.
				g.updateBeatInfos()
				g.logger.Infof("[DEMO] After import: beatPath[0]=%d drumLen=%d rows=%d", len(g.beatInfosByRow[0]), g.drum.Length, len(g.drum.Rows))
				g.demoBuilt = true
				return
			} else {
				g.logger.Infof("[DEMO] Failed to import %s: %v (falling back)", cfg, err)
			}
		} else {
			g.logger.Infof("[DEMO] Cannot read %s: %v (falling back)", cfg, err)
		}
	}

	// Primary startup demo: rock kit built from the embedded startup_demo.json.
	// If this fails, fall back to the programmatic construction below.
	if len(assets_pkg.StartupDemoJSON) > 0 {
		if err := g.Import(assets_pkg.StartupDemoJSON); err == nil {
			g.logger.Infof("[DEMO] Using embedded startup rock demo (rows=%d)", len(g.drum.Rows))
			g.demoBuilt = true
			return
		} else {
			g.logger.Infof("[DEMO] Failed to import startup demo JSON: %v (falling back)", err)
		}
	}

	// Aim for a human-friendly four-on-the-floor groove at ~100 BPM.
	// The grid has 32 subdivisions per beat; set rectangle sides in multiples
	// of 16/32/64 to align musical timing with grid distances.
	g.drum.SetBPM(100)

	// Prefer the built-in Miniaudio-synth instruments for the demo by default
	// to ensure a consistent, musical sound across platforms. Users can still
	// switch to the autoloaded sample instruments from the UI.
	kick := "kick"
	snare := "snare"
	hatClosed := "hihat"
	hatOpen := "hihat" // use synth hat for both; different volume below
	clap := "clap"

	// Ensure 5 rows exist (Kick, Snare, Closed HH, Open HH, Clap).
	for len(g.drum.Rows) < 5 {
		g.drum.AddRow()
	}
	// Assign instruments and friendly names.
	insts := []string{kick, snare, hatClosed, hatOpen, clap}
	names := []string{"Kick", "Snare", "Closed Hat", "Open Hat", "Clap"}
	for i := 0; i < 5; i++ {
		g.drum.selRow = i
		g.drum.SetInstrument(insts[i])
		if i < len(g.drum.Rows) {
			g.drum.Rows[i].Name = names[i]
		}
	}
	// Basic row volumes (mix-friendly)
	// Kick solid, snare strong, closed hat lower, open hat a touch louder,
	// clap subtle.
	vols := []float64{1.0, 0.85, 0.35, 0.45, 0.40}
	for i := 0; i < 5; i++ {
		if i < len(g.drum.Rows) {
			g.drum.Rows[i].Volume = vols[i]
		}
	}

	// Helper to create or reuse a regular node at (i,j)
	// (kept for possible future demo helpers)
	// node := func(i, j int) *uiNode { return g.tryAddNode(i, j, model.NodeTypeRegular) }

	// ── Musical rectangles using beat-aligned side lengths ──
	// Define a helper to translate beats to grid units (32 per beat).
	beat := func(b float64) int { return int(b*32 + 0.5) }

	toSub := func(f float64) int { return ToSub(g.grid, f) }

	// Kick: 1x1 beats rectangle (closed loop)
	_ = g.BuildPath(0, [][2]int{{toSub(-2), toSub(-1)}, {toSub(-1), toSub(-1)}, {toSub(-1), toSub(0)}, {toSub(-2), toSub(0)}, {toSub(-2), toSub(-1)}})

	// Snare: 2x2 beats rectangle (closed loop)
	_ = g.BuildPath(1, [][2]int{{toSub(0), toSub(-2)}, {toSub(2), toSub(-2)}, {toSub(2), toSub(0)}, {toSub(0), toSub(0)}, {toSub(0), toSub(-2)}})

	// Closed hi-hat: 0.5x0.5 beats rectangle (closed loop)
	_ = g.BuildPath(2, [][2]int{{toSub(1), toSub(1)}, {toSub(1.5), toSub(1)}, {toSub(1.5), toSub(1.5)}, {toSub(1), toSub(1.5)}, {toSub(1), toSub(1)}})

	// Open hi-hat: 2x2 beats rectangle (closed loop)
	_ = g.BuildPath(3, [][2]int{{toSub(-3), toSub(1)}, {toSub(-1), toSub(1)}, {toSub(-1), toSub(3)}, {toSub(-3), toSub(3)}, {toSub(-3), toSub(1)}})

	// Clap: 2x2 beats rectangle (closed loop)
	_ = g.BuildPath(4, [][2]int{{toSub(3), toSub(0)}, {toSub(5), toSub(0)}, {toSub(5), toSub(2)}, {toSub(3), toSub(2)}, {toSub(3), toSub(0)}})

	// Ensure row 0 has the official start node.
	if g.drum.Rows[0].Node != nil {
		g.start = g.drum.Rows[0].Node
		g.graph.StartNodeID = g.start.ID
	}

	// Fit drum machine length to the largest loop length encountered.
	// updateBeatInfos computes per-row paths and shrinks beat length to traversal.
	g.updateBeatInfos()

	// Nudge the timeline to start at the beginning.
	g.drum.Offset = 0

	// Subtle phase offsets so rows don’t all strike on beat 1.
	// Values wrap to each row’s loop length.
	if len(g.nextBeatIdxs) >= 5 {
		// Phasing at subdivision indices (32 per beat):
		// Snare on 2 & 4 → offset 1 beat.
		g.nextBeatIdxs[1] = beat(1)
		// Closed hat straight 8ths → start on 1 (no offset).
		g.nextBeatIdxs[2] = 0
		// Open hat occasional off-beats over 8-beat loop → start at 1.5 beats.
		g.nextBeatIdxs[3] = beat(1.5)
		// Clap align with snare but quieter.
		g.nextBeatIdxs[4] = beat(1)
	}

	// Keep menus tidy: ensure built-ins appear before sample-backed entries.
	opts := audio.Instruments()
	slices.SortFunc(opts, func(a, b string) int {
		as := strings.HasPrefix(a, "sample-")
		bs := strings.HasPrefix(b, "sample-")
		if as == bs {
			if a < b {
				return -1
			} else if a > b {
				return 1
			} else {
				return 0
			}
		}
		if as {
			return 1
		}
		return -1
	})
	g.demoBuilt = true
}

// RunDemo builds the demo circuit, starts playback, and exits after a short delay.
func (g *Game) RunDemo() {
	g.buildDemo()
	g.SetPlaying(true)
	g.engine.Start()
	g.spawnPulseFromRow(0, 0)
	go func() {
		time.Sleep(2 * time.Second)
		g.logger.Infof("[DEMO] Finished demo run")
		os.Exit(0)
	}()
}

// RunBenchmark configures the game to auto-play the demo circuit at the given
// BPM for the specified duration, then exit cleanly via ebiten.Termination.
func (g *Game) RunBenchmark(bpm int, dur time.Duration) {
	g.benchBPM = bpm
	g.benchDuration = dur
	g.demoScheduled = true
}
