package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"runtime/debug"
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
	// Kits mirrors exportFile.Kits so a project that carried authored kits
	// re-registers them on import instead of silently dropping the field
	// (kit-bus audio routing is still future work, but the definitions survive
	// the round-trip). Additive — absent on files with no kits.
	Kits []audio.Kit `json:"kits,omitempty"`
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
func (g *Game) Import(data []byte) (retErr error) {
	importStart := time.Now()
	g.logger.Debugf("[import] started (%d bytes)", len(data))
	defer func() { g.logger.Debugf("[import] total elapsed=%v", time.Since(importStart)) }()
	// Convert any panic into a returned error. On WASM an unrecovered panic
	// kills the Update() goroutine and freezes the canvas with no message; a
	// returned error surfaces a toast (game_update.go) and keeps the app alive.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("import panic: %v", r)
			g.logger.Errorf("[import] PANIC recovered: %v\n%s", r, debug.Stack())
		}
	}()

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
	// Reject documents that parse as JSON but are not beatmo project files.
	// Every exporter since v1 writes "version": 1; a doc without it (e.g. an
	// arbitrary {} or some other app's JSON picked in the file dialog) used to
	// sail through as an all-zero importFile and silently REPLACE the current
	// project with an empty one — import is replace-not-merge, so this was a
	// data-wipe with a success return.
	if f.Version < 1 {
		return fmt.Errorf("not a beatmo project file: missing or unsupported \"version\" (got %d, want >= 1)", f.Version)
	}
	// Size guards to avoid pathological inputs.
	if len(f.Instruments) > maxImportInstruments {
		return fmt.Errorf("too many instruments in import: %d > %d", len(f.Instruments), maxImportInstruments)
	}
	if len(f.Nodes) > maxImportNodes {
		return fmt.Errorf("too many nodes in import: %d > %d", len(f.Nodes), maxImportNodes)
	}
	// Re-register any kits the project carried so authored kit definitions
	// survive the round-trip (exportFile writes them; previously the import
	// struct lacked the field and dropped them). Additive registry write.
	for _, k := range f.Kits {
		audio.RegisterKit(k)
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
			// Logic fields.
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
			}
			// Per-node effect overrides (model-only, no audio wiring in V1)
			if len(n.EffectOverrides) > 0 {
				p.EffectOverrides = make([]model.EffectOverride, len(n.EffectOverrides))
				copy(p.EffectOverrides, n.EffectOverrides)
			}
			// Per-node synth parameters. The canonical v1+synth shape is the
			// synth_overrides map; legacy v1 files carry 7 individual synth_*
			// fields. Both fold into the same NodeParams slots, with the
			// canonical map winning on key conflict.
			over := n.legacySynthOverrides()
			if len(n.SynthOverrides) > 0 {
				if over == nil {
					over = make(map[string]float64, len(n.SynthOverrides))
				}
				for k, v := range n.SynthOverrides {
					over[k] = v
				}
			}
			if len(over) > 0 {
				applySynthOverridesToNodeParams(&p, over)
				p.SynthTone = clampF64(p.SynthTone, -10, 10)
				p.SynthDrive = clampF64(p.SynthDrive, 0, 1)
				p.SynthBody = clampF64(p.SynthBody, 0, 1)
				p.SynthColor = clampF64(p.SynthColor, -1, 1)
				p.SynthBrightness = clampF64(p.SynthBrightness, 0, 1)
			}
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
	// Replace-not-merge for insert effect chains: drop every chain the
	// previous project installed (Go mixer + JS WebAudio via the per-id
	// notify) before applying the file's chains below. Without this, a
	// project without effects kept the old project's chain processing audio
	// — invisible in the UI because rows are rebuilt with empty Effects.
	audio.ClearAllInsertEffectsAndNotify()
	for i, inst := range f.Instruments {
		g.drum.AddRow()
		idx := len(g.drum.Rows) - 1
		row := g.drum.Rows[idx]
		row.Name = inst.Name
		row.Instrument = inst.ID
		// Explicit kit-role tag (export.go writes it; previously never applied).
		row.Role = inst.Role
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

		// Strategy 0: embedded PCM (Sampler-created user sample) supersedes the
		// catalog/path strategies — the sound travels inside the project file.
		embeddedPCM := false
		if pcm, sr, ok := decodeSamplePCM(inst.PCM); ok {
			audio.PutUserSample(inst.ID, pcm, sr)
			g.drum.refreshInstruments()
			embeddedPCM = true
		}

		// Strategy 1: Try catalog lookup by ID (handles WAV instruments regardless of prefix)
		if !embeddedPCM {
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
		// Phase 3: apply per-instrument synth-recipe metadata. Recipe is
		// optional — when absent we keep whatever binding was already
		// established by ResetInstruments. SynthParams replaces whatever
		// the manager held (bulk set, not merge) so projects load to a
		// clean state.
		// gen_type-era migration: projects saved before the native-engine
		// deprecation could persist a re-voiced generator (gen_type >= 1).
		// MigrateGenType rebinds those to the Modular recipe with the
		// matching osc_type and drops the dead key (Native/absent is a
		// no-op). See internal/audio/migrate_gen_type.go.
		recipeID, synthParams := audio.MigrateGenType(inst.Recipe, audio.RecipeParams(inst.SynthParams))
		if recipeID != "" {
			audio.BindInstrumentToRecipe(inst.ID, recipeID)
			// The file's synth_params is a delta vs the recipe's SHIPPED
			// defaults (export.go), but the trigger render merges the overlay
			// over the CURRENT registered defaults — which a Synth-tab Save
			// (or a userprefs RecipeOverrides reload) may have customized in
			// this session. Pin the file's full effective tone (shipped ⊕
			// delta) as the overlay so every key elided at export resolves to
			// SHIPPED, never to the session's customized default. Re-export
			// stays byte-identical: the delta recomputed vs shipped is the
			// original delta. When the file carries no delta AND the session's
			// defaults are as-shipped, reset instead — that keeps the cheap
			// legacy dispatch path (and parity goldens) for default projects.
			if len(synthParams) > 0 || audio.RecipeDefaultsCustomized(recipeID) {
				pinned := audio.RecipeShippedDefaults(recipeID)
				for k, v := range synthParams {
					pinned[k] = v
				}
				audio.SetInstrumentParams(inst.ID, pinned)
			} else {
				audio.ResetInstrumentParams(inst.ID)
			}
		} else if len(synthParams) > 0 {
			// Legacy file without a recipe binding: the params are a raw
			// overlay (export's no-recipe fallback), apply them as-is.
			audio.SetInstrumentParams(inst.ID, synthParams)
		} else {
			// No user params in the JSON → make sure no stale values from
			// a previous project are still in the manager.
			audio.ResetInstrumentParams(inst.ID)
		}
		// Non-destructive Sampler edit descriptor: same replace-not-merge
		// semantics as SynthParams so a previous project's edit can't leak.
		if len(inst.SampleEdit) > 0 {
			audio.SetSampleEdit(inst.ID, audio.SampleEditFromFields(inst.SampleEdit))
		} else {
			audio.ClearSampleEdit(inst.ID)
		}
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
