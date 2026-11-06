package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"runtime/debug"
	"strings"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/theme"

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

// importedRowColorTransform is a TEST seam (nil in production): the
// theme-provenance guard (TestNoColorRendersOutsideTheTheme) shifts every
// instrument color at the point it enters the app, so identity colors differ
// between its two renders before any surface can capture them.
var importedRowColorTransform func(color.Color) color.Color

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

	// Consume the one-shot "record as an undo step" flag first, so a rejected
	// load cannot leak it into the next import.
	undoable := g.importAsUndoStep
	g.importAsUndoStep = false

	// Parse and validate EVERYTHING before touching the session: a rejected
	// file must leave it exactly as it was — no stopped/silenced playback (the
	// path snapshot below is only rebuilt by a successful import), no wiped
	// undo history, no cleared parity state.
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	// Reject documents that parse as JSON but are not beatmo project files,
	// and documents whose schema this build cannot read. A doc with neither
	// marker (an arbitrary {} or some other app's JSON picked in the file
	// dialog) used to sail through as an all-zero project and silently
	// REPLACE the current one — import is replace-not-merge, so that was a
	// data-wipe with a success return. Runs BEFORE any reset, so a rejected
	// file leaves the current circuit untouched.
	if err := f.validateProjectVersion(); err != nil {
		return err
	}
	// Every instrument document must migrate and validate (and a recipe-era
	// file is refused) before anything is reset, so a rejected file leaves the
	// current circuit untouched.
	if err := f.validateProjectDocs(data); err != nil {
		return err
	}
	// A node's chord block must name a known quality with an in-range
	// voicing — checked before anything is reset, like the documents above.
	if err := f.validateNodeChords(); err != nil {
		return err
	}
	// Every row names a valid instrument, says what kind it is, and a
	// built-in is never replaced by a WAV — before anything is reset.
	if err := f.validateInstrumentRows(); err != nil {
		return err
	}
	// Size guards to avoid pathological inputs.
	if len(f.Instruments) > maxImportInstruments {
		return fmt.Errorf("too many instruments in import: %d > %d", len(f.Instruments), maxImportInstruments)
	}
	if len(f.Nodes) > maxImportNodes {
		return fmt.Errorf("too many nodes in import: %d > %d", len(f.Nodes), maxImportNodes)
	}
	// Undo handling for a document swap. A user-initiated load (template row /
	// file picker, flagged via g.importAsUndoStep) records the pre-load document
	// as ONE undo step so a mistap can't silently destroy the current circuit;
	// every other load (demo/startup/JS) drops history — you cannot undo across
	// those swaps. Deferred so it runs after all in-import mutations (which fire
	// hooks that would otherwise push undo steps); the pre-load document is still
	// the committed baseline because g.importing suppresses in-import captures.
	// Both paths no-op while an undo-restore is re-importing (manager's restoring
	// flag). The flag is consumed here so a later direct Import defaults to clear.
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
	// Re-register any kits the project carried so authored kit definitions
	// survive the round-trip (the exporter writes them; before export and
	// import shared one struct, the import side lacked the field and
	// dropped them). Additive registry write.
	for _, k := range f.Kits {
		audio.RegisterKit(k)
	}
	// The file's instrument documents replace the session's copies; every
	// instrument the file does not carry returns to its library copy (replace,
	// not merge). Validated above, so this cannot fail on the file's account.
	// Opening a circuit lets the user's SAVED version of an instrument trump
	// the file's (and remembers the file's for Reset); an undo/redo re-import
	// restores its snapshot verbatim — the snapshot already holds the
	// effective documents, and the undo journal swaps the circuit's.
	if g.restoringUndo() {
		if err := audio.ReplaceInstrumentDocs(f.fullDocs()); err != nil {
			g.logger.Warnf("[import] instrument documents: %v", err)
		}
	} else if err := audio.OpenCircuitInstrumentDocs(f.fullDocs()); err != nil {
		g.logger.Warnf("[import] instrument documents: %v", err)
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
		g.drum.SetProjectPins(append([]instrument.ID(nil), f.PinnedInstruments...))
	}

	// Circuit theme: replace-not-merge. An absent block clears any previous
	// circuit theme so an import never inherits colors from the last project.
	var ct theme.Doc
	if f.Theme != nil {
		ct = f.Theme.Clone()
	}
	g.SetCircuitTheme(ct)

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
	g.playedGraveyard = nil
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
		// Import band mute state
		muted := make([]bool, len(eqBandDefs))
		for i := range muted {
			if i < len(f.EQ.BandMuted) {
				muted[i] = f.EQ.BandMuted[i]
			}
		}
		// setMasterEQBandState, NOT a copy into eqBandGainsDB: the latter is the
		// audio panel's working copy of whatever channel is on screen, so with an
		// instrument selected this wrote the file's MASTER curve into that
		// INSTRUMENT's display buffer and left the master untouched — the master
		// EQ never came back, and re-exporting then dropped it from the file for
		// good. Writes the master store, and mirrors into the zone only when
		// Master is the channel being shown.
		g.drum.setMasterEQBandState(gains, muted)
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
		// applyMasterEQ, not applyEQ: applyEQ routes to the SELECTED channel, so
		// with an instrument selected the master block was pushed onto that
		// instrument's chain instead of the master bus.
		g.drum.applyMasterEQ()
		// Sync UI sliders if master EQ is the active channel
		if g.drum.activeEQChannel() == channel.Master {
			g.drum.setEQActiveChannel(channel.Master)
		}
	} else if g.drum != nil {
		// Import is replace-not-merge: a document with NO master EQ must RESET any
		// in-session master EQ to defaults, otherwise stale gains/mute/filters
		// persist (this broke undo of an EQ change — undoing to a no-EQ snapshot
		// left the EQ applied). Zero the shared gain/mute slices in place and
		// disable the filters.
		g.drum.setMasterEQBandState(make([]float64, len(eqBandDefs)), make([]bool, len(eqBandDefs)))
		g.drum.hpfEnabled = false
		g.drum.hpfCutoffHz = 20
		g.drum.lpfEnabled = false
		g.drum.lpfCutoffHz = 20000
		g.drum.applyMasterEQ()
		if g.drum.activeEQChannel() == channel.Master {
			g.drum.setEQActiveChannel(channel.Master)
		}
	}

	// Create nodes (regular/silent/mute) and remember mapping id->uiNode.
	nodesStart := time.Now()
	idToNode := map[int]*uiNode{}
	for _, n := range f.Nodes {
		if n.Type == model.NodeTypeInvisible {
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
		// The type decoded strictly (an unknown spelling rejected the file).
		nodeType := n.Type
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
			// Groove and logic decoded strictly (an unknown spelling rejected
			// the file); only the ranges are clamped here.
			if n.GrooveKind != model.GrooveNone {
				p.GrooveKind = n.GrooveKind
				p.GroovePct = math.Max(0, math.Min(1, n.GroovePct))
			}
			if n.LogicKind != model.LogicNone {
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
			// Chord spec (validated above). A chord node without one is the
			// default triad — what the type cycle seeds.
			p.Chord = n.Chord.spec()
			if nodeType == model.NodeTypeChord && p.Chord.IsZero() {
				p.Chord = model.ChordSpec{Quality: model.ChordDefaultQuality}
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
		if n.Type == model.NodeTypeInvisible {
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
				Param:     r.Param,
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
		// RulesEnabled: every document this build exports sets the flag (see
		// exportGroup); an absent flag means disabled.
		enabled := eg.RulesEnabled != nil && *eg.RulesEnabled
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
	// Every instrument's EQ settings go with them; the file's own are applied
	// per row below.
	audio.ClearAllInsertEffectsAndNotify()
	audio.ResetEQSettings()
	// A document swap is a playback discontinuity: the new circuit must not
	// inherit the old one's filter delay lines, insert-effect LFO phase,
	// compressor envelope or reverb/delay tails. Without this the same project
	// sounds different depending on what was loaded before it — the reason an
	// export re-imported into a live session did not sound like the session it
	// came from. Configuration is untouched; only remembered state is cleared.
	audio.ResetDSPState()
	for i, inst := range f.Instruments {
		g.drum.AddRow()
		idx := len(g.drum.Rows) - 1
		row := g.drum.Rows[idx]
		if inst.Name != "" {
			audio.SetInstrumentDisplayName(inst.ID, inst.Name)
		} else {
			audio.ClearInstrumentDisplayName(inst.ID)
		}
		if !g.restoringUndo() {
			// The circuit's name for the instrument: what Reset restores
			// (a Save As renames the copy).
			audio.SetCircuitInstrumentName(inst.ID, inst.Name)
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
		if importedRowColorTransform != nil {
			row.Color = importedRowColorTransform(row.Color)
		}
		// Promote gen-showcase instance variants ("organ--2") to first-class,
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
		} else if audio.DropBuiltinSampleOverride(inst.ID) {
			// Replace-not-merge: a previous project (or an undone step) turned
			// this built-in into a sample; this file says it is the synth.
			g.drum.refreshInstruments()
		}

		// Strategy 1: Try catalog lookup by ID (handles WAV instruments regardless of prefix)
		if !embeddedPCM {
			if err := audio.EnsureInstrumentLoaded(inst.ID); err == nil {
				// Instrument loaded from catalog - update samplePath if catalog has path
				if meta, ok := audio.CatalogLookup(inst.ID); ok && meta.Path != "" {
					if g.drum.samplePath == nil {
						g.drum.samplePath = map[instrument.ID]string{}
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
					g.drum.samplePath = map[instrument.ID]string{}
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
			eq := audio.NewEQSettings()
			for j := range eq.GainsDB {
				if j < len(inst.EQ.GainsDB) {
					eq.GainsDB[j] = math.Max(-24, math.Min(24, inst.EQ.GainsDB[j]))
				}
				eq.BandMuted[j] = j < len(inst.EQ.BandMuted) && inst.EQ.BandMuted[j]
			}
			eq.HPF, eq.LPF = inst.EQ.HPFEnabled, inst.EQ.LPFEnabled
			if inst.EQ.HPFCutoffHz > 0 {
				eq.HPFHz = inst.EQ.HPFCutoffHz
			}
			if inst.EQ.LPFCutoffHz > 0 {
				eq.LPFHz = inst.EQ.LPFCutoffHz
			}
			*rowEQ(row) = *eq
			g.drum.applyRowEQ(idx)
			// Sync UI sliders if this instrument is the active EQ channel
			if g.drum.activeEQChannel() == channel.Of(row.Instrument) {
				g.drum.setEQActiveChannel(channel.Of(row.Instrument))
			}
		} else {
			// Import is replace-not-merge, and that has to reach the AUDIO
			// layer, not just the row. A document whose instrument carries no
			// `eq` block previously left the PREVIOUS project's curve on that
			// instrument's channel: the rebuilt row read flat in the UI while
			// the mixer kept applying the old EQ. Flatten it and push it
			// (no EQ processor at all — a unity chain would differ from an
			// absent one in the last bits), exactly as the master-EQ branch
			// below does for f.EQ == nil.
			g.drum.clearRowEQ(idx)
			if g.drum.activeEQChannel() == channel.Of(row.Instrument) {
				g.drum.setEQActiveChannel(channel.Of(row.Instrument))
			}
		}
		// Apply per-instrument insert effects if present
		if len(inst.Effects) > 0 {
			audio.SetInsertEffects(inst.ID, inst.Effects)
		}
		// A row the file carries no document for plays its library copy
		// (ReplaceInstrumentDocs above already reset every library id; this
		// covers an instance id first seen in this file).
		audio.EnsureInstrumentDoc(inst.ID)
		// Non-destructive Sampler edit descriptor: replace-not-merge, so a
		// previous project's edit can't leak.
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
		// 1.0. See project_file.go projectFile.MasterVolume.
		mv := clampF64(*f.MasterVolume, 0, 1)
		audio.SetMainVolume(mv)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = mv
		}
	} else {
		// No master_volume in the file: the fresh-session default, so the
		// previous project's master level never survives an import.
		audio.SetMainVolume(audio.FreshSessionMasterVolume)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = audio.FreshSessionMasterVolume
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
	// Browser: channel + insert-FX node construction is deferred into
	// pendingChannelOps while no AudioContext exists, so a pre-unlock import
	// would build the whole graph in a burst at the first note. Force the
	// context now, BEFORE beginWarmup below, where a stall is invisible.
	//
	// Order matters: this MUST run before beginWarmup("import"), not after.
	// EnsureAudioGraphReady is what makes the real AudioContext (and its
	// real sampleRate) exist. If beginWarmup ran first, the feeder would
	// start warming the voice cache at whatever sampleRate was assumed
	// pre-context; creating the context here-after could then observe a
	// DIFFERENT rate on the very next frame, tripping
	// checkWarmupSampleRate's "sample-rate" gate and re-warming the entire
	// working set a second time — roughly doubling the user's wait for
	// every browser import.
	audio.EnsureAudioGraphReady()
	// Trip the playback readiness gate: import is replace-not-merge, so the
	// voice cache is cold for every instrument/pitch this circuit introduces.
	// Runs AFTER updateBeatInfos so beatInfosByRow is populated for the full-
	// loop enumeration buildWarmSet walks, and AFTER EnsureAudioGraphReady
	// (see above) so the warm set is built against the real sample rate.
	g.beginWarmup("import")
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
		g.drum.selectAudioChannel(channel.Master)
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

// validateInstrumentRows checks each instrument row on its own: a valid id;
// a row that carries a sample (embedded PCM or a WAV path) says it is one;
// and a built-in is a sample only through embedded PCM (the Sampler's edit of
// it, which travels in the file) — never through a WAV path, which would
// replace the shipped instrument with whatever file it names.
func (f *projectFile) validateInstrumentRows() error {
	for i, inst := range f.Instruments {
		if _, err := instrument.ParseID(string(inst.ID)); err != nil {
			return fmt.Errorf("instrument row %d: %w", i, err)
		}
		if inst.carriesSample() && inst.Kind != kindSample {
			return fmt.Errorf("instrument row %d (%s): carries a sample but says kind %q", i, inst.ID, inst.Kind)
		}
		if roster.Has(inst.ID.Base()) && inst.Kind == kindSample && (inst.PCM == nil || inst.Path != "") {
			return fmt.Errorf("instrument row %d: the built-in %q is a sample only through embedded PCM, never a WAV path", i, inst.ID)
		}
	}
	return nil
}
