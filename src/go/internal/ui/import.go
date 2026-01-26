package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// Import safety limits to avoid pathological inputs.
const (
	maxImportInstruments = 128
	maxImportNodes       = 20000
	// Coordinates are clamped to ±(MaxDiv * importCoordLimitFactor)
	importCoordLimitFactor = 4096
	maxLogicN              = 4096
)

type importFile struct {
	Version     int                `json:"version"`
	Subdiv      int                `json:"subdiv"`
	BPM         int                `json:"bpm"`
	Instruments []exportInstrument `json:"instruments"`
	Nodes       []exportNode       `json:"nodes"`
	EQ          *exportEQ          `json:"eq,omitempty"`
}

func parseHexColor(s string) color.Color {
	// Expect #RRGGBBAA (8 hex digits). If invalid, default to opaque white.
	s = strings.TrimSpace(s)
	if s == "" {
		return color.RGBA{255, 255, 255, 255}
	}
	s = strings.TrimPrefix(s, "#")
	if len(s) != 8 {
		return color.RGBA{255, 255, 255, 255}
	}
	for i := 0; i < 8; i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return color.RGBA{255, 255, 255, 255}
		}
	}
	var r, g, b, a uint8
	_, err := fmt.Sscanf(s, "%02X%02X%02X%02X", &r, &g, &b, &a)
	if err != nil {
		return color.RGBA{255, 255, 255, 255}
	}
	return color.RGBA{r, g, b, a}
}

// Import rebuilds the graph and drum rows from exported JSON data. It accepts
// coordinates in the JSON's subdivision units and scales them to the current
// grid's MaxDiv. Missing subdiv defaults to 32.
func (g *Game) Import(data []byte) error {
	importStart := time.Now()
	g.logger.Infof("[IMPORT] started (%d bytes)", len(data))
	defer func() { g.logger.Infof("[IMPORT] total elapsed=%v", time.Since(importStart)) }()

	// Stop sequencer during import to prevent seqMu contention. The background
	// sequencerLoop checks Playing() before calling seqScheduleTime(), so setting
	// playing=false will cause it to skip scheduling until import completes.
	wasPlaying := g.Playing()
	if wasPlaying {
		g.logger.Debugf("[IMPORT] stopping playback during import")
		g.SetPlaying(false)
	}
	defer func() {
		if wasPlaying {
			g.logger.Debugf("[IMPORT] restoring playback after import")
			g.SetPlaying(true)
		}
	}()

	// Clear seqPathSnap so any in-flight seqScheduleTime() call returns early.
	// This provides an additional safety net against lock contention.
	g.seqPathSnap.Store(nil)

	g.importing = true
	defer func() { g.importing = false }()
	g.renderReady = false
	defer func() { g.renderReady = true }()
	g.ClearParityMismatches()
	g.parityMu.Lock()
	g.parityAudio = nil
	g.parityAudioMaxIdx = nil
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.parityMu.Unlock()

	g.logger.Debugf("[IMPORT] parity cleared elapsed=%v", time.Since(importStart))
	var f importFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	// Size guards to avoid pathological inputs.
	if len(f.Instruments) > maxImportInstruments {
		return fmt.Errorf("too many instruments in import: %d > %d", len(f.Instruments), maxImportInstruments)
	}
	if len(f.Nodes) > maxImportNodes {
		return fmt.Errorf("too many nodes in import: %d > %d", len(f.Nodes), maxImportNodes)
	}
	// Normalize and apply imported subdivision before constructing nodes so
	// coordinates in the JSON map 1:1 to the new grid.
	if f.Subdiv == 0 {
		f.Subdiv = 32
	}
	switch f.Subdiv {
	case 4, 8, 16, 32:
	default:
		f.Subdiv = 32
	}

	// Reset graph and UI state without replacing the graph pointer. The engine
	// predictor and other subsystems hold references to g.graph and must remain
	// in sync. Replacing the pointer would desynchronize predictions and break
	// precomputed drum view state.
	g.graph.Nodes = map[model.NodeID]model.Node{}
	g.graph.Edges = map[[2]model.NodeID]struct{}{}
	g.graph.Next = 0
	g.graph.Row = make([]bool, 4)
	g.nodes = nil
	g.nodesByID = make(map[model.NodeID]*uiNode)
	g.edges = nil
	g.drum.Graph = g.graph
	g.start = nil
	g.graph.StartNodeID = model.InvalidNodeID
	// Reset timeline/history state so stale immutable commits from a previous
	// session do not mask freshly imported beats or prevent re-added nodes from
	// becoming visible. A new service will be lazily created on next use.
	g.timeline = nil
	g.frozenUpToByRow = nil
	g.nextBeatIdxs = nil
	g.seqNextIdxs = nil
	g.muteUntilByRow = nil
	// Clear drum rows BEFORE SetSubdivisions to prevent updateBeatInfos from
	// processing stale rows with invalid origins against an empty graph.
	g.drum.Rows = nil

	// Apply incoming subdivision now that graph/UI state are cleared.
	subdivStart := time.Now()
	if err := g.SetSubdivisions(f.Subdiv); err != nil {
		// If change is denied (e.g., playing), keep current grid and scale coords.
		g.logger.Warnf("[GAME] Import: cannot apply subdiv %d now: %v; scaling nodes to current grid", f.Subdiv, err)
	}
	g.logger.Debugf("[IMPORT] SetSubdivisions elapsed=%v", time.Since(subdivStart))
	cur := g.grid.MaxDiv()
	scale := float64(cur) / float64(f.Subdiv)

	// Apply master EQ if present.
	if f.EQ != nil && g.drum != nil {
		// Clamp/resize to our band count.
		gains := make([]float64, len(eqBandDefs))
		for i := range gains {
			if i < len(f.EQ.GainsDB) {
				v := f.EQ.GainsDB[i]
				if v < -24 {
					v = -24
				}
				if v > 24 {
					v = 24
				}
				gains[i] = v
			}
		}
		g.drum.eqBandGainsDB = gains
		// Import band mute state
		muted := make([]bool, len(eqBandDefs))
		for i := range muted {
			if i < len(f.EQ.BandMuted) {
				muted[i] = f.EQ.BandMuted[i]
			}
		}
		g.drum.eqBandMuted = muted
		g.drum.applyEQ()
		// Sync UI sliders if master EQ is the active channel
		if g.drum.activeEQChannel() == "main" {
			g.drum.setEQActiveChannel("main")
		}
	}

	// Create nodes (regular/silent/mute) and remember mapping id->uiNode.
	nodesStart := time.Now()
	idToNode := map[int]*uiNode{}
	for _, n := range f.Nodes {
		if n.Type == "invisible" {
			continue
		}
		// Sanitize ids and coordinates.
		i := int(math.Round(float64(n.ID)))
		sx := int(math.Round(float64(n.I) * scale))
		sy := int(math.Round(float64(n.J) * scale))
		// Clamp coordinates to a reasonable bound to prevent extreme values.
		bound := g.grid.MaxDiv() * importCoordLimitFactor
		if sx > bound {
			sx = bound
		} else if sx < -bound {
			sx = -bound
		}
		if sy > bound {
			sy = bound
		} else if sy < -bound {
			sy = -bound
		}
		nodeType := model.NodeTypeRegular
		switch strings.ToLower(n.Type) {
		case "silent":
			nodeType = model.NodeTypeSilent
		case "mute":
			nodeType = model.NodeTypeMute
		}
		ui := g.tryAddNode(sx, sy, nodeType)
		// Apply parameters if present.
		if mn, ok := g.graph.GetNodeByID(ui.ID); ok {
			p := mn.Params
			if n.Volume != 0 {
				// Clamp imported gain to a safe range (0..4) matching web gain limits.
				v := n.Volume
				if v < 0 {
					v = 0
				} else if v > 4 {
					v = 4
				}
				p.Volume = v
			}
			if n.Pitch != 0 {
				p.Pitch = n.Pitch
			}
			if n.Duration > 0 {
				p.Duration = n.Duration
			}
			// Groove (new)
			if n.GrooveKind != "" {
				gk := strings.ToLower(strings.TrimSpace(n.GrooveKind))
				switch gk {
				case "delay", "rush":
					p.GrooveKind = gk
					gp := n.GroovePct
					if gp < 0 {
						gp = 0
					} else if gp > 1 {
						gp = 1
					}
					p.GroovePct = gp
				default:
					// ignore invalid groove kinds
				}
			}
			// Logic fields (new). If present, take precedence over SkipEvery.
			if n.LogicKind != "" {
				if n.LogicKind == "prev_fired" {
					n.LogicKind = "trigger_if_prev_triggered"
				}
				if n.LogicKind == "every_n_loops" {
					n.LogicKind = "every_n_triggers"
				}
				// Map removed redundant rules to canonical forms.
				if n.LogicKind == "skip_if_prev_skipped" {
					n.LogicKind = "trigger_if_prev_triggered"
				}
				if n.LogicKind == "skip_if_prev_triggered" {
					n.LogicKind = "trigger_if_prev_skipped"
				}
				// Clamp ranges and apply.
				p.LogicKind = n.LogicKind
				if n.LogicN > 0 {
					if n.LogicN > maxLogicN {
						n.LogicN = maxLogicN
					}
					p.LogicN = n.LogicN
				}
				if n.LogicP > 0 {
					if n.LogicP < 0 {
						n.LogicP = 0
					}
					if n.LogicP > 1 {
						n.LogicP = 1
					}
					p.LogicP = n.LogicP
				}
				// Do not copy SkipEvery into params when logic is explicit to avoid double gating.
			} else if n.SkipEvery > 0 {
				// Back-compat import upgrade: map legacy SkipEvery to logic_kind=skip_every_n so
				// the UI reflects the rule and parameter controls work via the logic dropdown.
				p.LogicKind = "skip_every_n"
				if n.SkipEvery > maxLogicN {
					p.LogicN = maxLogicN
				} else if n.SkipEvery > 0 {
					p.LogicN = n.SkipEvery
				}
				p.SkipEveryN = 0
			}
			g.graph.SetNodeParams(ui.ID, p)
		}
		idToNode[i] = ui
	}
	g.logger.Infof("[IMPORT] node creation elapsed=%v nodes=%d", time.Since(nodesStart), len(idToNode))
	// Create edges from all non-invisible nodes. Silent nodes are valid
	// routing points and must retain their connections.
	edgesStart := time.Now()
	for _, n := range f.Nodes {
		if strings.ToLower(n.Type) == "invisible" {
			continue
		}
		from := idToNode[int(n.ID)]
		if from == nil {
			continue
		}
		for _, out := range n.Outputs {
			to := idToNode[int(out)]
			if to != nil {
				g.addEdge(from, to)
			}
		}
	}
	g.logger.Infof("[IMPORT] edge creation elapsed=%v", time.Since(edgesStart))
	// Instruments -> rows
	rowsStart := time.Now()
	g.drum.Rows = nil
	for i, inst := range f.Instruments {
		g.drum.AddRow()
		idx := len(g.drum.Rows) - 1
		row := g.drum.Rows[idx]
		row.Name = inst.Name
		row.Instrument = inst.ID
		// Clamp row volume for consistency across platforms (0..4)
		rv := inst.Volume
		if rv < 0 {
			rv = 0
		} else if rv > 4 {
			rv = 4
		}
		row.Volume = rv
		row.Color = parseHexColor(inst.Color)
		// Remember instrument id even if missing so users can switch back or
		// load it later by the same name.
		g.drum.EnsureInstrumentKnown(inst.ID)

		// Strategy 1: Try catalog lookup by ID (handles WAV instruments regardless of prefix)
		if err := audio.EnsureInstrumentLoaded(inst.ID); err == nil {
			// Instrument loaded from catalog - update samplePath if catalog has path
			if meta, ok := audio.CatalogLookup(inst.ID); ok && meta.Path != "" {
				if g.drum.samplePath == nil {
					g.drum.samplePath = map[string]string{}
				}
				g.drum.samplePath[inst.ID] = meta.Path
			}
			g.drum.refreshInstruments()
		}

		// Strategy 2: Try provided path or legacy "sample-" prefix resolution
		instPath := inst.Path
		if instPath == "" && strings.HasPrefix(inst.ID, "sample-") {
			instPath = resolveSamplePath(inst.ID)
		}

		// Strategy 3: Use provided or resolved path
		if instPath != "" {
			if g.drum.samplePath == nil {
				g.drum.samplePath = map[string]string{}
			}
			g.drum.samplePath[inst.ID] = instPath
			// Attempt to register; on stub/wasm this makes it available immediately.
			_ = audio.RegisterWAV(inst.ID, instPath)
			g.drum.refreshInstruments()
		}
		if ui := idToNode[int(inst.Origin)]; ui != nil {
			row.Origin = ui.ID
			row.Node = ui
			if idx == 0 {
				g.start = ui
				g.graph.StartNodeID = ui.ID
			}
			g.logger.Debugf("[GAME] Import row %d: name=%q inst=%q origin(json)=%d -> nodeID=%d", i, inst.Name, inst.ID, inst.Origin, ui.ID)
		} else {
			g.logger.Infof("[GAME] Import row %d: name=%q inst=%q origin(json)=%d not found; leaving origin unset", i, inst.Name, inst.ID, inst.Origin)
		}
		// Apply per-instrument EQ if present
		if inst.EQ != nil && (len(inst.EQ.GainsDB) > 0 || len(inst.EQ.BandMuted) > 0) {
			gains := make([]float64, len(eqBandDefs))
			for j := range gains {
				if j < len(inst.EQ.GainsDB) {
					v := inst.EQ.GainsDB[j]
					if v < -24 {
						v = -24
					}
					if v > 24 {
						v = 24
					}
					gains[j] = v
				}
			}
			row.EQGainsDB = gains
			// Import per-instrument band mute state
			muted := make([]bool, len(eqBandDefs))
			for j := range muted {
				if j < len(inst.EQ.BandMuted) {
					muted[j] = inst.EQ.BandMuted[j]
				}
			}
			row.EQBandMuted = muted
			g.drum.applyRowEQ(idx)
			// Sync UI sliders if this instrument is the active EQ channel
			if g.drum.activeEQChannel() == row.Instrument {
				g.drum.setEQActiveChannel(row.Instrument)
			}
		}
	}
	g.logger.Infof("[IMPORT] row creation elapsed=%v rows=%d", time.Since(rowsStart), len(g.drum.Rows))
	// Ensure imported colors are unique across rows.
	g.drum.EnsureUniqueRowColors()
	if f.BPM > 0 {
		g.drum.SetBPM(f.BPM)
	}
	// Clear any pending UI-added rows state so origin selection does not remain
	// armed after an import that already set each row's origin.
	g.drum.added = nil
	g.pendingStartRow = -1
	g.logger.Infof("[IMPORT] before updateBeatInfos elapsed=%v nodes=%d rows=%d offset=%d length=%d",
		time.Since(importStart), len(g.graph.Nodes), len(g.drum.Rows), g.drum.Offset, g.drum.Length)
	beatInfoStart := time.Now()
	g.updateBeatInfos()
	g.logger.Infof("[IMPORT] updateBeatInfos elapsed=%v", time.Since(beatInfoStart))
	g.paramsDirty = false
	g.perfMode.ForceRefresh()
	if g.drum != nil {
		g.drum.markAllRowsDirty()
	}
	g.logger.Infof("[GAME] Import completed: bpm=%d nodes=%d rows=%d subdiv=%d", f.BPM, len(f.Nodes), len(g.drum.Rows), f.Subdiv)
	return nil
}

// resolveSamplePath attempts to locate a WAV file matching a sample-backed
// instrument ID (e.g., "sample-kick-9-wonder") within the assets tree. It
// returns an empty string when no candidate is found.
func resolveSamplePath(id string) string {
	// First, check catalog directly by ID (handles non-"sample-" prefixed WAVs)
	if meta, ok := audio.CatalogLookup(id); ok && meta.Path != "" {
		return meta.Path
	}

	base := strings.TrimPrefix(id, "sample-")
	if base == "" {
		return ""
	}
	// Search env override first.
	if env := os.Getenv("TUNKUL_ASSETS"); env != "" {
		for _, cand := range []string{
			filepath.Join(env, base+".wav"),
			filepath.Join(env, "wav", base+".wav"),
			filepath.Join(env, "assets", "wav", base+".wav"),
		} {
			if info, err := os.Stat(cand); err == nil && !info.IsDir() {
				abs, _ := filepath.Abs(cand)
				return abs
			}
		}
	}
	// Walk a few parent levels to cover running from repo root or src/go.
	for up := 0; up <= 5; up++ {
		parts := make([]string, 0, up+4)
		for i := 0; i < up; i++ {
			parts = append(parts, "..")
		}
		parts = append(parts, "assets", "wav", base+".wav")
		cand := filepath.Join(parts...)
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			abs, _ := filepath.Abs(cand)
			return abs
		}
	}
	// Fall back to catalog metadata (if already initialized) to reuse known paths.
	for _, meta := range audio.Catalog() {
		if strings.ToLower(strings.TrimPrefix(meta.ID, "sample-")) == strings.ToLower(base) {
			if meta.Path != "" {
				return meta.Path
			}
		}
		if meta.Path != "" {
			bn := strings.TrimSuffix(filepath.Base(meta.Path), filepath.Ext(meta.Path))
			if strings.EqualFold(bn, base) {
				return meta.Path
			}
		}
	}
	return ""
}
