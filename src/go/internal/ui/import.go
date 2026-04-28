package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
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
	Version           int                `json:"version"`
	Subdiv            int                `json:"subdiv"`
	BPM               int                `json:"bpm"`
	MasterVolume      float64            `json:"master_volume,omitempty"`
	Instruments       []exportInstrument `json:"instruments"`
	Nodes             []exportNode       `json:"nodes"`
	EQ                *exportEQ          `json:"eq,omitempty"`
	SendEffects       *SendEffectsConfig `json:"send_effects,omitempty"`
	PinnedInstruments []string           `json:"pinned_instruments,omitempty"` // per-project pin tier; absent on legacy files
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
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
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
	g.logger.Debugf("[import] started (%d bytes)", len(data))
	defer func() { g.logger.Debugf("[import] total elapsed=%v", time.Since(importStart)) }()

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

	g.dismissLongPressPopup()
	g.cancelConnectMode()
	g.cancelMoveMode()
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

	// Apply per-project instrument pins. Always written, even when the field
	// is absent from the JSON: an absent or empty list resets the set so an
	// import never silently inherits pins from a previous project.
	if g.drum != nil {
		g.drum.SetProjectPins(f.PinnedInstruments)
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
		// Copy into existing slice to preserve the shared alias with EQPanelZone.bandGainsDB.
		// See drumview_ctor.go:305 — dv.eqBandGainsDB and z.bandGainsDB share the same backing array.
		copy(g.drum.eqBandGainsDB(), gains)
		// Import band mute state
		muted := make([]bool, len(eqBandDefs))
		for i := range muted {
			if i < len(f.EQ.BandMuted) {
				muted[i] = f.EQ.BandMuted[i]
			}
		}
		copy(g.drum.eqBandMuted(), muted)
		// Import HPF/LPF for master channel
		g.drum.hpfEnabled = f.EQ.HPFEnabled
		if f.EQ.HPFCutoffHz > 0 {
			g.drum.hpfCutoffHz = f.EQ.HPFCutoffHz
		} else {
			g.drum.hpfCutoffHz = 20
		}
		g.drum.lpfEnabled = f.EQ.LPFEnabled
		if f.EQ.LPFCutoffHz > 0 {
			g.drum.lpfCutoffHz = f.EQ.LPFCutoffHz
		} else {
			g.drum.lpfCutoffHz = 20000
		}
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
			// Per-node effect overrides (model-only, no audio wiring in V1)
			if len(n.EffectOverrides) > 0 {
				p.EffectOverrides = make([]model.EffectOverride, len(n.EffectOverrides))
				copy(p.EffectOverrides, n.EffectOverrides)
			}
			// Per-node synth parameters
			p.SynthDecay = n.SynthDecay
			p.SynthTone = clampF64(n.SynthTone, -10, 10)
			p.SynthAttack = n.SynthAttack
			p.SynthDrive = clampF64(n.SynthDrive, 0, 1)
			p.SynthBody = clampF64(n.SynthBody, 0, 1)
			p.SynthColor = clampF64(n.SynthColor, -1, 1)
			p.SynthBrightness = clampF64(n.SynthBrightness, 0, 1)
			g.graph.SetNodeParams(ui.ID, p)
		}
		idToNode[i] = ui
	}
	g.logger.Debugf("[import] node creation elapsed=%v nodes=%d", time.Since(nodesStart), len(idToNode))
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
	g.logger.Debugf("[import] edge creation elapsed=%v", time.Since(edgesStart))
	// Instruments -> rows
	rowsStart := time.Now()
	g.drum.Rows = nil
	g.drum.SuppressLayout()
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

		// Strategy 2: use the explicit path provided in the import payload, if any.
		// External references (filesystem, DB, future remote pack) are resolved
		// upstream by whoever produced the JSON.
		instPath := inst.Path
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
			g.logger.Warnf("[import] row %d: name=%q inst=%q origin(json)=%d not found; leaving origin unset", i, inst.Name, inst.ID, inst.Origin)
		}
		// Apply per-instrument EQ if present
		if inst.EQ != nil {
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
			// Import per-instrument HPF/LPF
			row.HPFEnabled = inst.EQ.HPFEnabled
			if inst.EQ.HPFCutoffHz > 0 {
				row.HPFCutoffHz = inst.EQ.HPFCutoffHz
			} else {
				row.HPFCutoffHz = 20
			}
			row.LPFEnabled = inst.EQ.LPFEnabled
			if inst.EQ.LPFCutoffHz > 0 {
				row.LPFCutoffHz = inst.EQ.LPFCutoffHz
			} else {
				row.LPFCutoffHz = 20000
			}
			g.drum.applyRowEQ(idx)
			// Sync UI sliders if this instrument is the active EQ channel
			if g.drum.activeEQChannel() == row.Instrument {
				g.drum.setEQActiveChannel(row.Instrument)
			}
		}
		// Apply per-instrument insert effects if present
		if len(inst.Effects) > 0 {
			row.Effects = make([]audio.EffectSlot, len(inst.Effects))
			copy(row.Effects, inst.Effects)
			audio.SetInsertEffects(inst.ID, inst.Effects)
		}
		// Apply per-instrument pan and send levels
		row.Pan = clampF64(inst.Pan, -1, 1)
		row.DelaySend = clampF64(inst.DelaySend, 0, 1)
		row.ReverbSend = clampF64(inst.ReverbSend, 0, 1)
		audio.SetChannelPan(inst.ID, row.Pan)
		audio.SetDelaySend(inst.ID, row.DelaySend)
		audio.SetReverbSend(inst.ID, row.ReverbSend)
	}
	g.drum.ResumeLayout()
	g.logger.Debugf("[import] row creation elapsed=%v rows=%d", time.Since(rowsStart), len(g.drum.Rows))
	// Ensure imported colors are unique across rows.
	g.drum.EnsureUniqueRowColors()
	if f.BPM > 0 {
		g.drum.SetBPM(f.BPM)
	}
	// Restore master volume. Default to 1.0 for legacy files that omit it.
	if f.MasterVolume > 0 {
		mv := clampF64(f.MasterVolume, 0, 1)
		audio.SetMainVolume(mv)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = mv
		}
	} else {
		audio.SetMainVolume(1)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = 1
		}
	}
	// Apply send effects configuration if present.
	if f.SendEffects != nil {
		if d := f.SendEffects.Delay; d != nil {
			audio.ConfigureSendDelay(d.TimeMs, d.Feedback, d.DampingHz)
		}
		if r := f.SendEffects.Reverb; r != nil {
			audio.ConfigureSendReverb(r.Room, r.Damping, r.Wet)
		}
	}
	// Clear any pending UI-added rows state so origin selection does not remain
	// armed after an import that already set each row's origin.
	g.drum.added = nil
	g.pendingStartRow = -1
	g.logger.Debugf("[import] before updateBeatInfos elapsed=%v nodes=%d rows=%d offset=%d length=%d",
		time.Since(importStart), len(g.graph.Nodes), len(g.drum.Rows), g.drum.Offset, g.drum.Length)
	beatInfoStart := time.Now()
	g.updateBeatInfos()
	g.logger.Debugf("[import] updateBeatInfos elapsed=%v", time.Since(beatInfoStart))
	g.paramsDirty = false
	g.perfMode.ForceRefresh()
	if g.drum != nil {
		g.drum.markAllRowsDirty()
	}
	g.logger.Debugf("[import] completed: bpm=%d nodes=%d rows=%d subdiv=%d", f.BPM, len(f.Nodes), len(g.drum.Rows), f.Subdiv)
	return nil
}

// clampF64 constrains v to the range [lo, hi].
func clampF64(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
