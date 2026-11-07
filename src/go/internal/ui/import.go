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

// TEST seam, nil in production: the theme guard shifts every instrument colour where it enters the app.
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

// Coordinates scale from the file's subdivision to the grid's MaxDiv; a missing subdiv means 32.
func (g *Game) Import(data []byte) (retErr error) {
	importStart := time.Now()
	g.logger.Debugf("[import] started (%d bytes)", len(data))
	defer func() { g.logger.Debugf("[import] total elapsed=%v", time.Since(importStart)) }()
	// On WASM an unrecovered panic kills Update and freezes the canvas silently; an error surfaces a toast.
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("import panic: %v", r)
			g.logger.Errorf("[import] PANIC recovered: %v\n%s", r, debug.Stack())
		}
	}()

	// Consumed first so a rejected load cannot leak it into the next import.
	undoable := g.importAsUndoStep
	g.importAsUndoStep = false

	// Validate EVERYTHING before touching the session: a rejected file leaves playback, undo history and parity
	// state exactly as they were.
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	// Also the foreign-JSON sniffer: import is replace-not-merge, so an unrecognised {} would otherwise replace the
	// current project with an empty one and report success.
	if err := f.validateProjectVersion(); err != nil {
		return err
	}
	// A recipe-era file is refused here too.
	if err := f.validateProjectDocs(data); err != nil {
		return err
	}
	if err := f.validateNodeChords(); err != nil {
		return err
	}
	// A built-in is never replaced by a WAV.
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
	// A user-initiated load records the old document as ONE undo step so a mistap cannot destroy the circuit; other
	// loads drop history. Deferred past every in-import mutation; a no-op during an undo re-import.
	if g.undoManager != nil {
		if undoable {
			defer g.undoManager.RecordExternalLoad("load template")
		} else {
			defer g.undoManager.OnExternalLoad()
		}
	}

	// The sequencer loop skips scheduling while not playing, so stopping avoids seqMu contention during import.
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

	// A second guard: any in-flight seqScheduleTime returns early.
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
	// Kits the project carries are re-registered so authored kits survive the round-trip.
	for _, k := range f.Kits {
		audio.RegisterKit(k)
	}
	// The file's documents replace the session's (replace, not merge); opening lets the user's SAVED version trump
	// the file's. An undo/redo re-import restores its snapshot verbatim instead.
	if g.restoringUndo() {
		if err := audio.ReplaceInstrumentDocs(f.fullDocs()); err != nil {
			g.logger.Warnf("[import] instrument documents: %v", err)
		}
	} else if err := audio.OpenCircuitInstrumentDocs(f.fullDocs()); err != nil {
		g.logger.Warnf("[import] instrument documents: %v", err)
	}

	// Before constructing nodes, so file coordinates map 1:1 to the grid.
	if f.Subdiv == 0 {
		f.Subdiv = 32
	}
	switch f.Subdiv {
	case 4, 8, 16, 32:
	default:
		f.Subdiv = 32
	}

	// Always written: an absent list resets the pins, so an import never inherits a previous project's.
	if g.drum != nil {
		g.drum.SetProjectPins(append([]instrument.ID(nil), f.PinnedInstruments...))
	}

	// An absent block clears the previous circuit's theme.
	var ct theme.Doc
	if f.Theme != nil {
		ct = f.Theme.Clone()
	}
	g.SetCircuitTheme(ct)

	// Reset in place, never a new pointer: the predictor and other subsystems hold g.graph.
	g.graph.Nodes = map[model.NodeID]model.Node{}
	g.graph.Edges = map[[2]model.NodeID]struct{}{}
	g.graph.Next = 0
	g.graph.Row = make([]bool, 4)
	// The graph is reused, so old groups would survive; ResetGroups also resets the id counter so re-importing the
	// same file mints the same ids and undo/redo exports stay byte-identical.
	g.graph.ResetGroups()
	g.nodes = nil
	g.nodesByID = make(map[model.NodeID]*uiNode)
	g.edges = nil
	g.drum.Graph = g.graph
	g.start = nil
	g.graph.StartNodeID = model.InvalidNodeID
	// Old immutable commits would mask the imported beats; a new service is created lazily.
	g.timeline = nil
	g.playedGraveyard = nil
	g.frozenUpToByRow = nil
	g.nextBeatIdxs = nil
	g.seqNextIdxs = nil
	g.muteUntilByRow = nil
	// Before SetSubdivisions, or updateBeatInfos walks stale rows against an empty graph.
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
		// The master store, NOT eqBandGainsDB (the panel's copy of the channel on screen): writing that put the master
		// curve on the selected instrument and lost the master for good on re-export.
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
		// applyEQ routes to the SELECTED channel, which would push the master block onto an instrument.
		g.drum.applyMasterEQ()
		// Sync UI sliders if master EQ is the active channel
		if g.drum.activeEQChannel() == channel.Master {
			g.drum.setEQActiveChannel(channel.Master)
		}
	} else if g.drum != nil {
		// No master EQ in the file resets it to defaults; stale gains otherwise survived (undoing an EQ change kept it).
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
				// An unbounded gain: only negatives (a phase inversion) are rejected.
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
			// Decoded strictly (an unknown spelling rejected the file); only ranges are clamped here.
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
			// A chord node without a spec is the default triad, what the type cycle seeds.
			p.Chord = n.Chord.spec()
			if nodeType == model.NodeTypeChord && p.Chord.IsZero() {
				p.Chord = model.ChordSpec{Quality: model.ChordDefaultQuality}
			}
			g.graph.SetNodeParams(ui.ID, p)
		}
		idToNode[i] = ui
	}
	g.logger.Debugf("[import] node creation elapsed=%v nodes=%d", time.Since(nodesStart), len(idToNode))
	// Silent nodes are routing points and keep their edges.
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
	// Node ids are reissued, so members remap through idToNode; unknown ids drop, an empty group drops, and a bad
	// rule drops alone (SetGroupRules is all-or-nothing, so rules retry one by one).
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
		// Every export sets the flag; absent means disabled.
		enabled := eg.RulesEnabled != nil && *eg.RulesEnabled
		_ = g.graph.SetGroupRulesEnabled(gid, enabled)
	}
	g.refreshGroupIndex()
	// Instruments -> rows
	rowsStart := time.Now()
	g.drum.Rows = nil
	g.drum.SuppressLayout()
	// Every old chain goes (Go mixer and WebAudio), or a file without effects kept processing audio through chains
	// invisible in the rebuilt rows. Each instrument's EQ goes with them.
	audio.ClearAllInsertEffectsAndNotify()
	audio.ResetEQSettings()
	// A discontinuity: the new circuit must not inherit filter memory, LFO phase or tails, or the same file sounds
	// different depending on what was loaded before. Configuration is untouched.
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
			// What Reset restores; a Save As renames the copy.
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
		// Instance variants ("organ--2") become real instruments sounding like their base; before the availability
		// bookkeeping, which must see them.
		audio.EnsureInstanceInstrument(inst.ID)
		// Remembered even when missing, so the user can switch back to it.
		g.drum.EnsureInstrumentKnown(inst.ID)

		// Embedded PCM supersedes the catalog/path strategies: the sound travels inside the file.
		embeddedPCM := false
		if pcm, sr, ok := decodeSamplePCM(inst.PCM); ok {
			audio.PutUserSample(inst.ID, pcm, sr)
			g.drum.refreshInstruments()
			embeddedPCM = true
		} else if audio.DropBuiltinSampleOverride(inst.ID) {
			// Replace-not-merge: a previous project may have made this built-in a sample.
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

			// External references are resolved by whoever produced the file.
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
			// Replace-not-merge must reach the AUDIO layer: a row without an eq block kept the previous project's curve while
			// reading flat. Cleared, not set to unity: a unity chain differs from none in the last bits.
			g.drum.clearRowEQ(idx)
			if g.drum.activeEQChannel() == channel.Of(row.Instrument) {
				g.drum.setEQActiveChannel(channel.Of(row.Instrument))
			}
		}
		// Apply per-instrument insert effects if present
		if len(inst.Effects) > 0 {
			audio.SetInsertEffects(inst.ID, inst.Effects)
		}
		// Covers an instance id first seen in this file; library ids were reset above.
		audio.EnsureInstrumentDoc(inst.ID)
		// Replace-not-merge, so a previous project's edit cannot leak.
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
		// Explicit, including 0 (muted), which an omitempty float once dropped and re-imported at 1.0.
		mv := clampF64(*f.MasterVolume, 0, 1)
		audio.SetMainVolume(mv)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = mv
		}
	} else {
		// No master_volume: the fresh-session default, never the previous project's.
		audio.SetMainVolume(audio.FreshSessionMasterVolume)
		if g.drum.mainVolSlider() != nil {
			g.drum.mainVolSlider().Value = audio.FreshSessionMasterVolume
		}
	}
	// So origin selection does not stay armed after an import that set every row's origin.
	g.drum.added = nil
	g.pendingStartRow = -1
	g.logger.Debugf("[import] before updateBeatInfos elapsed=%v nodes=%d rows=%d offset=%d length=%d",
		time.Since(importStart), len(g.graph.Nodes), len(g.drum.Rows), g.drum.Offset, g.drum.Length)
	beatInfoStart := time.Now()
	g.updateBeatInfos()
	g.logger.Debugf("[import] updateBeatInfos elapsed=%v", time.Since(beatInfoStart))
	g.paramsDirty = false
	// Before beginWarmup: the browser defers graph building until an AudioContext exists, and warming at an assumed
	// rate would see the real rate next frame, trip the sample-rate gate and warm everything twice.
	audio.EnsureAudioGraphReady()
	// After updateBeatInfos (the warm set walks the full loop) and after the AudioContext exists.
	g.beginWarmup("import")
	g.perfMode.ForceRefresh()
	if g.drum != nil {
		g.drum.markAllRowsDirty()
		// Back to Master, unconditionally: a carried-over channel may not exist in the new circuit.
		// selectAudioChannel re-syncs every tab.
		g.drum.selectAudioChannel(channel.Master)
		// No tab transition fires on import, and a template with no per-instrument EQ would leave the WASM analysers off.
		if g.drum.eqPanelZone != nil {
			g.drum.eqPanelZone.reapplyAnalyzerTaps()
		}
		// Analysers are REUSED across imports and keep returning the old circuit's data while stopped; after
		// reapplyAnalyzerTaps so every wired one is zeroed.
		audio.ResetAllChannelAnalyzers()
		// Desktop's analyzer service carries its last capture forward; nil under -tags test / WASM.
		if svc := audio.AnalyzerService(); svc != nil {
			svc.Reset()
		}
		// Held meters and "!" markers from the previous circuit go.
		audio.ResetClipsWindow()
		if g.drum.eqPanelZone != nil {
			g.drum.eqPanelZone.levelsLatch.Reset()
			if g.drum.eqPanelZone.levelsLatches != nil {
				g.drum.eqPanelZone.levelsLatches.Reset()
			}
		}
	}
	// The graph was rebuilt, so UI pointing at old nodes (coordinate badge, node menu) goes too.
	g.pruneDanglingNodeRefs()
	// Otherwise the Sampler keeps its pre-restore edit and the next gesture re-applies it.
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

// A built-in is a sample only through embedded PCM (its Sampler edit), never a WAV path, which would replace the
// shipped instrument with whatever file it names.
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
