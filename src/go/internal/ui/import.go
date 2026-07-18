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
	Version int `json:"version"`
	Subdiv  int `json:"subdiv"`
	BPM     int `json:"bpm"`
	// Pointer so an explicit 0 (muted master) is distinguishable from an
	// absent field (legacy files, which import at the historical 1.0).
	MasterVolume      *float64           `json:"master_volume,omitempty"`
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
	// Groups mirrors exportFile.Groups (node-group batch rules). Additive —
	// absent on files with no groups.
	Groups []exportGroup `json:"groups,omitempty"`
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

	// Undo handling for a document swap. A user-initiated load (template row /
	// file picker, flagged via g.importAsUndoStep) records the pre-load document
	// as ONE undo step so a mistap can't silently destroy the current circuit;
	// every other load (demo/startup/JS) drops history — you cannot undo across
	// those swaps. Deferred so it runs after all in-import mutations (which fire
	// hooks that would otherwise push undo steps); the pre-load document is still
	// the committed baseline because g.importing suppresses in-import captures.
	// Both paths no-op while an undo-restore is re-importing (manager's restoring
	// flag). The flag is consumed here so a later direct Import defaults to clear.
	undoable := g.importAsUndoStep
	g.importAsUndoStep = false
	if g.undoManager != nil {
		if undoable {
			defer g.undoManager.RecordExternalLoad("load template")
		} else {
			defer g.undoManager.OnExternalLoad()
		}
	}

	// Stop sequencer during import to prevent seqMu contention. The background
	// sequencerLoop checks Playing() before calling seqScheduleTime(), so setting
	// playing=false will cause it to skip scheduling until import completes.
	wasPlaying := g.Playing()
	if wasPlaying {
		g.logger.Debugf("[import] stopping playback during import")
		g.SetPlaying(false)
	}
	defer func() {
		if wasPlaying {
			g.logger.Debugf("[import] restoring playback after import")
			g.SetPlaying(true)
		}
	}()

	// Clear seqPathSnap so any in-flight seqScheduleTime() call returns early.
	// This provides an additional safety net against lock contention.
	g.seqPathSnap.Store(nil)

	g.dismissLongPressPopup("cancel")
	g.modes.Cancel("import")
	g.moveSkipRelease = false
	g.marquee = marqueeDrag{}
	if g.groupMenu != nil {
		g.groupMenu.Close()
	}
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

	g.logger.Debugf("[import] parity cleared elapsed=%v", time.Since(importStart))
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
	// Replace-not-merge for node groups: the graph object is reused (not
	// reallocated) across imports, so a previous project's groups map would
	// otherwise survive into a file that carries none. ResetGroups also
	// resets the GroupID counter (unlike a bare map reset) so re-importing
	// the same document always mints the same GroupIDs, keeping the exported
	// bytes deterministic across undo/redo's reimport-based restore.
	g.graph.ResetGroups()
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
		g.logger.Warnf("[game] Import: cannot apply subdiv %d now: %v; scaling nodes to current grid", f.Subdiv, err)
	}
	g.logger.Debugf("[import] SetSubdivisions elapsed=%v", time.Since(subdivStart))
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
	} else if g.drum != nil {
		// Import is replace-not-merge: a document with NO master EQ must RESET any
		// in-session master EQ to defaults, otherwise stale gains/mute/filters
		// persist (this broke undo of an EQ change — undoing to a no-EQ snapshot
		// left the EQ applied). Zero the shared gain/mute slices in place and
		// disable the filters.
		for i := range g.drum.eqBandGainsDB() {
			g.drum.eqBandGainsDB()[i] = 0
		}
		for i := range g.drum.eqBandMuted() {
			g.drum.eqBandMuted()[i] = false
		}
		g.drum.hpfEnabled = false
		g.drum.hpfCutoffHz = 20
		g.drum.lpfEnabled = false
		g.drum.lpfCutoffHz = 20000
		g.drum.applyEQ()
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
				// Per-node volume is an unbounded gain — it may boost above unity
				// arbitrarily. Only reject negatives (which would phase-invert);
				// there is intentionally no upper clamp.
				v := n.Volume
				if v < 0 {
					v = 0
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
	// Node groups. Group node_ids reference the JSON file's node ids, which
	// MUST be remapped through idToNode (imported node IDs are reissued).
	// Validation: unknown node ids are dropped; a group left with zero known
	// members is dropped entirely; every_n floors to 1; a bad rule is
	// dropped without rejecting the whole group (SetGroupRules is
	// all-or-nothing, so retry rule-by-rule on rejection).
	for _, eg := range f.Groups {
		ids := make([]model.NodeID, 0, len(eg.NodeIDs))
		for _, jsonID := range eg.NodeIDs {
			if un := idToNode[jsonID]; un != nil {
				ids = append(ids, un.ID)
			}
		}
		if len(ids) == 0 {
			continue // all members unknown -> drop group
		}
		gid, err := g.graph.CreateGroup(eg.Name, ids)
		if err != nil {
			g.logger.Debugf("[import] dropping group %d: %v", eg.ID, err)
			continue
		}
		rules := make([]model.GroupRule, 0, len(eg.Rules))
		for _, r := range eg.Rules {
			n := r.EveryN
			if n < 1 {
				n = 1
			}
			anchor := r.AnchorIdx
			if anchor < 0 {
				anchor = 0
			}
			rules = append(rules, model.GroupRule{
				Param:     model.GroupParam(strings.ToLower(strings.TrimSpace(r.Param))),
				Delta:     clampF64(r.Delta, -24, 24),
				EveryN:    n,
				Min:       r.Min,
				Max:       r.Max,
				AnchorIdx: anchor,
			})
		}
		// Per-rule rejection: SetGroupRules is all-or-nothing, so on error
		// retry each rule individually against the growing kept-set and only
		// commit the ones that validate.
		if err := g.graph.SetGroupRules(gid, rules); err != nil {
			kept := make([]model.GroupRule, 0, len(rules))
			for _, r := range rules {
				candidate := append(append([]model.GroupRule{}, kept...), r)
				if e := g.graph.SetGroupRules(gid, candidate); e == nil {
					kept = candidate
				}
			}
			_ = g.graph.SetGroupRules(gid, kept)
		}
		// RulesEnabled (task-enabled-status brief, Part C): the explicit flag
		// wins when the file carries it (a non-nil pointer — every document
		// this build exports always sets one, see exportGroup's doc comment).
		// A nil pointer means the key was genuinely absent — a legacy file
		// written before this field existed — so it falls back to the
		// backward-compat rule: such a file's rules were active by their mere
		// existence, so RulesEnabled = len(rules) > 0 keeps old files behaving
		// exactly as they did before this task.
		enabled := len(rules) > 0
		if eg.RulesEnabled != nil {
			enabled = *eg.RulesEnabled
		}
		_ = g.graph.SetGroupRulesEnabled(gid, enabled)
	}
	g.refreshGroupIndex()
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
		if inst.Name != "" {
			audio.SetInstrumentDisplayName(inst.ID, inst.Name)
		} else {
			audio.ClearInstrumentDisplayName(inst.ID)
		}
		row.Name = g.drum.computeInstLabel(inst.ID)
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
		// Promote gen-showcase instance variants ("organ-2") to first-class,
		// available, recipe-bound, voiced instruments that render identically to
		// their base ("organ"). Must run BEFORE availability bookkeeping below so
		// EnsureInstrumentKnown / refreshInstruments see the registered variant.
		audio.EnsureInstanceInstrument(inst.ID)
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
			g.logger.Debugf("[game] Import row %d: name=%q inst=%q origin(json)=%d -> nodeID=%d", i, inst.Name, inst.ID, inst.Origin, ui.ID)
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
	if f.MasterVolume != nil {
		// Explicit value — including 0 (muted): the old float64 field
		// dropped 0 via omitempty on export and blasted the re-import at
		// 1.0. See export.go exportFile.MasterVolume.
		mv := clampF64(*f.MasterVolume, 0, 1)
		audio.SetMainVolume(mv)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = mv
		}
	} else {
		// Legacy file without the field: those were exported when 1.0 was
		// the implicit default — restore that, not the newer 0.5
		// fresh-session headroom default.
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
		// Import is replace-not-merge, so reset the audio-panel channel
		// selector back to Master. A per-instrument selection carried over from
		// the previous circuit would otherwise leave every audio tab (Wave,
		// Spectrum, Chain, …) pointed at a channel the new circuit may not even
		// contain. selectAudioChannel is the cross-tab chokepoint: it re-syncs
		// the EQ mirror, the analyzer detail channel, and the scope instrument.
		// Unconditional on purpose — the pre-existing EQ re-syncs above only
		// fire when the already-selected channel still matches.
		g.drum.selectAudioChannel(audioPanelMasterChannelID)
		// Re-wire per-instrument analyzers for the freshly imported rows.
		// On WASM each instrument's AnalyserNode is enabled on demand;
		// a template that ships no per-instrument EQ (so applyRowEQ never
		// ran) would otherwise leave the Levels/per-instrument views blank
		// while audio plays. No tab transition fires on import, so do it
		// here. Idempotent + harmless on desktop. See
		// audio_panel_dispatcher.go (AnalyzerInstrumentTapsForTab).
		if g.drum.eqPanelZone != nil {
			g.drum.eqPanelZone.reapplyAnalyzerTaps()
		}
		// Flush stale analyzer data so the audio-panel tabs (Wave, Spectrum,
		// Levels, Chain) read flat until the newly loaded circuit is played.
		// When playback is stopped no fresh samples flow, and the per-channel
		// Analyzer instances are REUSED across imports (wireChannelAnalyzer) —
		// so without this they keep returning the previous circuit's retained
		// waveform/spectrum/peak. Run after reapplyAnalyzerTaps so every wired
		// analyzer (including ones just re-enabled for the current tab) is
		// zeroed. See internal/audio/analyzer.go ResetAllChannelAnalyzers.
		audio.ResetAllChannelAnalyzers()
		// Desktop reads Wave/Spectrum/Levels from the analyzer service, which
		// carries the last capture forward across ticks; flush it too (nil
		// under -tags test / WASM, where the snapshot path above covers it).
		if svc := audio.AnalyzerService(); svc != nil {
			svc.Reset()
		}
		// Clear the Levels-tab peak/clip latches and the rolling clip window so
		// held meters + "!" clip markers from the previous circuit disappear.
		audio.ResetClipsWindow()
		if g.drum.eqPanelZone != nil {
			g.drum.eqPanelZone.levelsLatch.Reset()
			if g.drum.eqPanelZone.levelsLatches != nil {
				g.drum.eqPanelZone.levelsLatches.Reset()
			}
		}
	}
	// The graph was rebuilt wholesale above; drop any transient UI that still
	// points at nodes from the previous graph (coordinate badge, open node menu)
	// so an undone/replaced node never leaves its coordinates or menu on screen.
	g.pruneDanglingNodeRefs()
	// Re-seed the Sampler tab's editing state from the restored document so an
	// undo/redo/load that changed an instrument's sample_edit moves the sampler
	// knobs/trim/reverse back (otherwise the tab keeps the pre-restore edit and
	// the next gesture re-applies it).
	g.drum.resyncSamplerEditFromDocument()
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
