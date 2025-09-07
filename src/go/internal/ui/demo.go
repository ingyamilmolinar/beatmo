//go:build !test

package ui

import (
    "os"
    "slices"
    "strings"
    "time"

    "github.com/ingyamilmolinar/tunkul/internal/audio"
)

// buildDemo constructs a multi-row, multi-length circuit showcasing the game.
// It places four instruments (kick, snare, two hi-hats) and creates distinct
// loops so the DrumView and grid highlight their traversal.
func (g *Game) buildDemo() {
    if g.demoBuilt || g.drum == nil || g.graph == nil {
        return
    }

    // Aim for a human-friendly four-on-the-floor groove at ~100 BPM.
    // The grid has 32 subdivisions per beat; set rectangle sides in multiples
    // of 16/32/64 to align musical timing with grid distances.
    g.drum.SetBPM(100)

    // Choose desired sample-backed instruments when available.
    ids := audio.Instruments()
    pick := func(sub string, fallback string) string {
        for _, id := range ids {
            if strings.Contains(strings.ToLower(id), sub) {
                return id
            }
        }
        return fallback
    }
    kick := pick("kick", "kick")           // sample-kick-drum or built-in kick
    snare := pick("snare", "snare")         // sample-snare or built-in snare
    hatClosed := pick("closed", "hihat")     // closed hi-hat sample else synth
    hatOpen := pick("open", "hihat")        // open hi-hat sample else synth
    clap := pick("clap", "clap")            // clap if available (fallback rendered)

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
    vols := []float64{1.0, 0.9, 0.6, 0.5, 0.55}
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

    // Sort instruments so sample-backed entries appear after the built-ins.
    // This keeps menus tidy while still exposing the new assets.
    opts := audio.Instruments()
    slices.SortFunc(opts, func(a, b string) int {
        as := strings.HasPrefix(a, "sample-")
        bs := strings.HasPrefix(b, "sample-")
        if as == bs {
            if a < b { return -1 } else if a > b { return 1 } else { return 0 }
        }
        if as { return 1 }
        return -1
    })
    g.demoBuilt = true
}

// RunDemo builds the demo circuit, starts playback, and exits after a short delay.
func (g *Game) RunDemo() {
    g.buildDemo()
    g.playing = true
    g.engine.Start()
    g.spawnPulseFrom(0)
    go func() {
        time.Sleep(2 * time.Second)
        g.logger.Infof("[DEMO] Finished demo run")
        os.Exit(0)
    }()
}
