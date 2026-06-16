//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// undoCase is one row of the per-action undo sweep. kind is the user-action
// hooks.Kind the case exercises; TestEveryUndoableKindHasActionCase cross-checks
// the set of kinds against the registry so no undoable action goes uncovered.
type undoCase struct {
	name   string
	kind   hooks.Kind
	setup  func(t *testing.T, g *Game)
	mutate func(t *testing.T, g *Game)
}

// pickOtherInstrumentFor returns an instrument id different from the row-0 one.
func pickOtherInstrumentFor(g *Game) (string, bool) {
	cur := g.drum.Rows[0].Instrument
	for _, id := range g.drum.instOptions {
		if id != cur && id != "" {
			return id, true
		}
	}
	return "", false
}

// undoActionCases is the single source of truth for the per-action undo sweep:
// both TestUndoAllActionsRoundTripAndAtomic (which proves the 3 round-trip
// properties) and TestEveryUndoableKindHasActionCase (which proves the registry
// coverage) read this one list. Adding a new undoable kind to the registry MUST
// add a case here or the coverage test fails.
func undoActionCases() []undoCase {
	return []undoCase{
		{
			name:   "add-node",
			kind:   hooks.EventNodeAdded,
			mutate: func(t *testing.T, g *Game) { g.tryAddNode(7, 7, model.NodeTypeRegular) },
		},
		{
			name: "add-node-with-autostitch-atomic",
			kind: hooks.EventNodeAdded,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(3, 3, model.NodeTypeRegular)
				b := g.tryAddNode(6, 3, model.NodeTypeRegular)
				g.addEdge(a, b)
			},
			// Placing a node on the existing A-B edge splits it (edge-delete +
			// two edge-adds + node-add) — must be ONE step.
			mutate: func(t *testing.T, g *Game) { g.tryAddNode(4, 3, model.NodeTypeRegular) },
		},
		{
			name: "delete-node-leaf",
			kind: hooks.EventNodeDeleted,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(8, 8, model.NodeTypeRegular)
			},
			mutate: func(t *testing.T, g *Game) { g.deleteNode(g.nodeAt(8, 8)) },
		},
		{
			name: "delete-node-mid-circuit-atomic",
			kind: hooks.EventNodeDeleted,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(3, 6, model.NodeTypeRegular)
				b := g.tryAddNode(4, 6, model.NodeTypeRegular)
				c := g.tryAddNode(5, 6, model.NodeTypeRegular)
				g.addEdge(a, b)
				g.addEdge(b, c)
			},
			mutate: func(t *testing.T, g *Game) { g.deleteNode(g.nodeAt(4, 6)) },
		},
		{
			name: "move-node-atomic",
			kind: hooks.EventNodeMoved,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(3, 9, model.NodeTypeRegular)
				b := g.tryAddNode(5, 9, model.NodeTypeRegular)
				g.addEdge(a, b)
			},
			mutate: func(t *testing.T, g *Game) { g.moveNode(g.nodeAt(5, 9), 5, 10) },
		},
		{
			name: "add-edge",
			kind: hooks.EventEdgeAdded,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(3, 11, model.NodeTypeRegular)
				g.tryAddNode(6, 11, model.NodeTypeRegular)
			},
			mutate: func(t *testing.T, g *Game) { g.addEdge(g.nodeAt(3, 11), g.nodeAt(6, 11)) },
		},
		{
			name: "delete-edge",
			kind: hooks.EventEdgeDeleted,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(3, 12, model.NodeTypeRegular)
				b := g.tryAddNode(6, 12, model.NodeTypeRegular)
				g.addEdge(a, b)
			},
			mutate: func(t *testing.T, g *Game) { g.deleteEdge(g.nodeAt(3, 12), g.nodeAt(6, 12)) },
		},
		{
			name: "node-logic-params",
			kind: hooks.EventNodeParamsChanged,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(9, 9, model.NodeTypeRegular)
			},
			mutate: func(t *testing.T, g *Game) {
				n := g.nodeAt(9, 9)
				mn, ok := g.graph.GetNodeByID(n.ID)
				if !ok {
					t.Fatal("node missing")
				}
				ps := mn.Params
				ps.LogicKind = "probability"
				ps.LogicP = 0.5
				g.graph.SetNodeParams(n.ID, ps)
				emitNodeParamsChanged(n.ID, ps)
			},
		},
		{
			// Mirrors the node-type change commit path (event_helpers.go
			// emitNodeTypeChanged → recordUndo). Add a Regular node, then flip it
			// to Silent on the graph and emit — the node's exported "type" changes.
			name: "node-type-changed",
			kind: hooks.EventNodeTypeChanged,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(10, 10, model.NodeTypeRegular)
			},
			mutate: func(t *testing.T, g *Game) {
				n := g.nodeAt(10, 10)
				mn, ok := g.graph.GetNodeByID(n.ID)
				if !ok {
					t.Fatal("node missing")
				}
				old := mn.Type
				mn.Type = model.NodeTypeSilent
				g.graph.Nodes[n.ID] = mn
				g.cacheNode(n.ID)
				g.notifyPredictorNode(n.ID)
				g.updateBeatInfos()
				emitNodeTypeChanged(n.ID, old, model.NodeTypeSilent)
			},
		},
		{
			// Mirrors js_exports_graph_ui.go setStartNodeGrid: assign a row's origin
			// to a node, then emitStartNodeChanged. Use a non-zero row so we don't
			// also rewrite g.start / graph.StartNodeID. The instrument's exported
			// Origin changes.
			name: "start-node-changed",
			kind: hooks.EventStartNodeChanged,
			setup: func(t *testing.T, g *Game) {
				g.drum.AddRow()
				if other, ok := pickOtherInstrumentFor(g); ok {
					g.drum.selRow = len(g.drum.Rows) - 1
					g.drum.SetInstrument(other)
				}
				g.tryAddNode(11, 13, model.NodeTypeRegular)
				g.updateBeatInfos()
			},
			mutate: func(t *testing.T, g *Game) {
				row := len(g.drum.Rows) - 1
				n := g.nodeAt(11, 13)
				if n == nil {
					t.Fatal("origin node missing")
				}
				g.drum.Rows[row].Origin = n.ID
				g.drum.Rows[row].Node = n
				g.updateBeatInfos()
				emitStartNodeChanged(row, n.ID)
			},
		},
		{
			name:   "add-row",
			kind:   hooks.EventRowAdded,
			mutate: func(t *testing.T, g *Game) { g.drum.AddRow() },
		},
		{
			name: "delete-row",
			kind: hooks.EventRowDeleted,
			setup: func(t *testing.T, g *Game) {
				g.drum.AddRow()
				// Give the added row a distinct instrument. Two rows sharing one
				// instrument trip a SEPARATE import-fidelity issue (import's
				// EnsureUniqueRowColors reshuffles colliding same-instrument row
				// colors instead of trusting the stored hex), which is orthogonal
				// to undo correctness and tracked separately.
				if other, ok := pickOtherInstrumentFor(g); ok {
					g.drum.selRow = len(g.drum.Rows) - 1
					g.drum.SetInstrument(other)
				}
			},
			mutate: func(t *testing.T, g *Game) { g.drum.DeleteRow(len(g.drum.Rows) - 1) },
		},
		{
			name: "change-instrument",
			kind: hooks.EventRowInstrumentChange,
			mutate: func(t *testing.T, g *Game) {
				other, ok := pickOtherInstrumentFor(g)
				if !ok {
					t.Skip("no alternate instrument available")
				}
				g.drum.selRow = 0
				g.drum.SetInstrument(other)
			},
		},
		{
			// Mirrors the DrumView rename closure (drumview_ctor.go): rename the
			// row-0 instrument id, emitInstrumentRenamed. The exported instrument
			// id (and the node-origin binding via the row) changes.
			name: "instrument-renamed",
			kind: hooks.EventInstrumentRenamed,
			setup: func(t *testing.T, g *Game) {
				// RenameInstrument mutates PROCESS-GLOBAL audio state (the instrument
				// list + recipe binding). Restore it after this subtest so a later
				// case keyed on the original id (e.g. the synth-param cases on the
				// row-0 instrument) still resolves its recipe.
				oldID := g.drum.Rows[0].Instrument
				recipe := audio.RecipeForInstrument(oldID)
				t.Cleanup(func() {
					audio.RenameInstrument("renamed-inst-x", oldID)
					if recipe != "" {
						audio.BindInstrumentToRecipe(oldID, recipe)
					}
				})
			},
			mutate: func(t *testing.T, g *Game) {
				oldID := g.drum.Rows[0].Instrument
				newID := "renamed-inst-x"
				// Faithful to the production closure (drumview_ctor.go:787-795): it
				// emits — which taps recordUndo — BEFORE updating the row's Instrument
				// id, even though the exported doc reads from row.Instrument. The
				// immediate record is therefore a no-op (doc still == baseline); rename
				// undo works ONLY because Game.Update wraps every frame in
				// beginGroup/endGroup (game_update.go:76-77), deferring the snapshot to
				// frame end after the row is updated. Mirror that bracket here so the
				// case tests the REAL mechanism, not a contrived ordering.
				beginUndoGroup("rename instrument")
				audio.RenameInstrument(oldID, newID)
				emitInstrumentRenamed(oldID, newID)
				g.drum.Rows[0].Instrument = newID
				g.drum.Rows[0].Name = newID
				endUndoGroup()
			},
		},
		{
			name: "recolor-row",
			kind: hooks.EventRowColorChanged,
			mutate: func(t *testing.T, g *Game) {
				g.drum.SetRowColor(0, color.RGBA{R: 11, G: 99, B: 200, A: 255})
			},
		},
		{
			name: "row-volume",
			kind: hooks.EventRowVolume,
			mutate: func(t *testing.T, g *Game) {
				g.drum.Rows[0].Volume = 0.42
				g.drum.commitRowVolume(0)
			},
		},
		{
			name:   "bpm",
			kind:   hooks.EventBPMChange,
			mutate: func(t *testing.T, g *Game) { g.drum.SetBPM(143) },
		},
		{
			name:   "subdivision",
			kind:   hooks.EventSubdivChange,
			mutate: func(t *testing.T, g *Game) { _ = g.SetSubdivisions(16) },
		},
		{
			name: "master-volume",
			kind: hooks.EventMasterVolumeChange,
			mutate: func(t *testing.T, g *Game) {
				audio.SetMainVolume(0.55)
				g.drum.mainVolPending, g.drum.mainVolPendingDirty = 0.55, true
				g.drum.commitMainVolume()
			},
		},
		{
			// Mirrors the EQ-tab HPF/LPF toggle (toggleHPF → commitEQFilter →
			// emitEQFilterToggled + recordUndoStep). Enabling the master HPF
			// makes the exported master-EQ block appear.
			name: "eq-filter-toggle",
			kind: hooks.EventEQFilterToggled,
			setup: func(t *testing.T, g *Game) {
				g.drum.eqPanelZone.SetActiveChannel("main")
			},
			mutate: func(t *testing.T, g *Game) {
				g.drum.toggleHPF()
			},
		},
		{
			name: "eq-band",
			kind: hooks.EventEQBandChange,
			mutate: func(t *testing.T, g *Game) {
				z := g.drum.eqPanelZone
				if len(z.bandGainsDB) <= 3 {
					t.Skip("eq band gains not initialised")
				}
				z.SetActiveChannel("main")
				z.bandGainsDB[3] = 6.0 // the exported value
				g.drum.eqPendingChannel = "main"
				g.drum.eqPendingBand = 3
				g.drum.eqPendingGainDB = 6.0
				g.drum.eqPendingDirty = true
				g.drum.commitEQBand()
			},
		},
		{
			// Mirrors drumview_fx_panel.go FX add menu (audio.AddInsertEffect +
			// emitInsertEffectAdded). export.go reads audio.GetInsertEffects, so the
			// added slot changes the exported instrument; Undo re-imports the
			// effect-free baseline (ClearAllInsertEffectsAndNotify) to clear it.
			name: "insert-effect-added",
			kind: hooks.EventInsertEffectAdded,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				slot := audio.AddInsertEffect(id, audio.EffectDelay, nil)
				emitInsertEffectAdded(id, slot, string(audio.EffectDelay))
			},
		},
		{
			// Mirrors js_exports_insert_effects.go removeInsertEffect.
			name: "insert-effect-removed",
			kind: hooks.EventInsertEffectRemoved,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
				id := g.drum.Rows[0].Instrument
				audio.AddInsertEffect(id, audio.EffectDelay, nil)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.RemoveInsertEffect(id, 0)
				emitInsertEffectRemoved(id, 0)
			},
		},
		{
			// Mirrors drumview_fx_panel.go commitFXSlider (audio.SetInsertEffectParam
			// + emitInsertEffectParam + recordUndoStep). emitInsertEffectParam does
			// NOT tap recordUndo on its own, so the commit site records explicitly.
			name: "insert-effect-param",
			kind: hooks.EventInsertEffectParam,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
				id := g.drum.Rows[0].Instrument
				audio.AddInsertEffect(id, audio.EffectDelay, nil)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				slots := audio.GetInsertEffects(id)
				if len(slots) == 0 {
					t.Fatal("no effect slot to edit")
				}
				// Pick a param and move it clearly off its current value.
				var pname string
				var pval float64
				for k, v := range slots[0].Params {
					pname, pval = k, v
					break
				}
				if pname == "" {
					t.Skip("effect has no params")
				}
				newVal := pval + 0.137
				audio.SetInsertEffectParam(id, 0, pname, newVal)
				emitInsertEffectParam(id, 0, pname, newVal)
				g.drum.recordUndoStep(hooks.EventInsertEffectParam)
			},
		},
		{
			// Mirrors js_exports_insert_effects.go toggleInsertEffect (audio.Toggle
			// + emit + recordUndoStep).
			name: "insert-effect-toggled",
			kind: hooks.EventInsertEffectToggled,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
				id := g.drum.Rows[0].Instrument
				audio.AddInsertEffect(id, audio.EffectDelay, nil)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.ToggleInsertEffect(id, 0, false)
				emitInsertEffectToggled(id, 0, false)
				g.drum.recordUndoStep(hooks.EventInsertEffectToggled)
			},
		},
		{
			// Mirrors js_exports_insert_effects.go moveInsertEffect (audio.Move +
			// emit + recordUndoStep). Two distinct effect types so the reorder is
			// observable in the exported chain.
			name: "insert-effect-moved",
			kind: hooks.EventInsertEffectMoved,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
				id := g.drum.Rows[0].Instrument
				audio.AddInsertEffect(id, audio.EffectDelay, nil)
				audio.AddInsertEffect(id, audio.EffectReverb, nil)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.MoveInsertEffect(id, 0, 1)
				emitInsertEffectMoved(id, 0, 1)
				g.drum.recordUndoStep(hooks.EventInsertEffectMoved)
			},
		},
		{
			// Mirrors synth_panel_zone.go knob release (audio.SetInstrumentParam +
			// emitInstrumentParamsCommitted + recordUndoStep). export.go folds the
			// effective-vs-shipped delta into synth_params, so a changed param value
			// changes the exported instrument.
			name: "instrument-params-committed",
			kind: hooks.EventInstrumentParamsCommitted,
			setup: func(t *testing.T, g *Game) {
				audio.ResetInstrumentParams(g.drum.Rows[0].Instrument)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				recipe := audio.RecipeForInstrument(id)
				if recipe == "" {
					t.Skip("row-0 instrument has no recipe binding")
				}
				if _, ok := pickSynthParam(g, id, recipe); !ok {
					t.Skip("no editable synth param produced a document change")
				}
				emitInstrumentParamsCommitted(id, recipe)
				g.drum.recordUndoStep(hooks.EventInstrumentParamsCommitted)
			},
		},
		{
			// Mirrors recipe_save_sink.go resetRecipeForInstrument (ResetRecipe +
			// ResetInstrumentParams + emitInstrumentParamsReset + recordUndo). Setup
			// commits a synth param change so the reset has something to revert.
			name: "instrument-params-reset",
			kind: hooks.EventInstrumentParamsReset,
			setup: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.ResetInstrumentParams(id)
				recipe := audio.RecipeForInstrument(id)
				if recipe == "" {
					t.Skip("row-0 instrument has no recipe binding")
				}
				if _, ok := pickSynthParam(g, id, recipe); !ok {
					t.Skip("no editable synth param produced a document change")
				}
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				recipe := audio.RecipeForInstrument(id)
				audio.ResetInstrumentParams(id)
				emitInstrumentParamsReset(id, recipe)
				g.drum.recordUndoStep(hooks.EventInstrumentParamsReset)
			},
		},
		{
			// Mirrors sampler_panel_zone.go samplerSave (audio.SetSampleEdit +
			// recordUndoStep(EventSampleEditChanged)). export.go reads
			// audio.SampleEditFor; Undo re-imports the edit-free baseline
			// (ClearSampleEdit) to revert it.
			name: "sample-edit-changed",
			kind: hooks.EventSampleEditChanged,
			setup: func(t *testing.T, g *Game) {
				audio.ClearSampleEdit(g.drum.Rows[0].Instrument)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				edit := audio.SampleEdit{
					StartFrac:      0,
					EndFrac:        1,
					GainDB:         -3.0,
					TransposeSemis: 2,
				}
				audio.SetSampleEdit(id, edit)
				g.drum.recordUndoStep(hooks.EventSampleEditChanged)
			},
		},
	}
}

// pickSynthParam finds a recipe param for instrument id that, when written via
// audio.SetInstrumentParam, actually changes the exported document (the value
// differs from the shipped default after sanitize/clamp). It writes the change
// as a side effect and returns the param name. ok=false means no param stuck.
func pickSynthParam(g *Game, id, recipe string) (string, bool) {
	shipped := audio.RecipeShippedDefaults(recipe)
	for name, cur := range shipped {
		// Try a few candidate values; SetInstrumentParam clamps to the param's
		// range, so some candidates may collapse back to the current value.
		for _, cand := range []float64{cur + 0.5, cur - 0.5, 0.75, 0.25, cur + 1, cur - 1} {
			audio.ResetInstrumentParams(id)
			// Re-apply any baseline already established by the caller would be
			// lost here; callers reset first and rely on a single change.
			audio.SetInstrumentParam(id, name, cand)
			eff := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(id))
			if s, ok := shipped[name]; !ok || (eff[name]-s > 1e-9 || s-eff[name] > 1e-9) {
				return name, true
			}
		}
	}
	audio.ResetInstrumentParams(id)
	return "", false
}

// TestUndoAllActionsRoundTripAndAtomic is the thorough per-action sweep the
// undo feature is judged against. For EVERY undoable user action it proves three
// properties through the real export serializer:
//
//  1. the action changes the exported document (it actually does something),
//  2. it records EXACTLY ONE undo step (compound gestures are atomic — no
//     partial intermediate steps), and
//  3. one Undo restores the pre-action document byte-for-byte, and one Redo
//     re-applies the post-action document byte-for-byte.
//
// setup builds any prerequisite state; the undo baseline is taken AFTER setup so
// only the action under test is measured.
func TestUndoAllActionsRoundTripAndAtomic(t *testing.T) {
	for _, tc := range undoActionCases() {
		t.Run(tc.name, func(t *testing.T) {
			g := newTestGameForUndo(t)
			if tc.setup != nil {
				tc.setup(t, g)
			}
			// Baseline = the document right before the action under test.
			g.undoManager.OnExternalLoad()
			before := g.undoCapture()
			depth0 := len(g.undoManager.undo)

			tc.mutate(t, g)

			after := g.undoCapture()
			if string(after) == string(before) {
				t.Fatalf("%s did not change the exported document", tc.name)
			}
			if steps := len(g.undoManager.undo) - depth0; steps != 1 {
				t.Fatalf("%s recorded %d undo steps, want exactly 1 (atomic)", tc.name, steps)
			}

			// Undo is the strong check: it must restore the pre-action document
			// byte-for-byte. (This is what caught the master-EQ import bug — undo
			// to a no-EQ snapshot must actually clear the EQ.)
			g.undoManager.Undo()
			if got := g.undoCapture(); string(got) != string(before) {
				t.Fatalf("%s: Undo not byte-identical to pre-action document", tc.name)
			}

			// Redo must restore the post-action document. exportBytes is not
			// byte-canonical across a node delete — a removed node leaves a gap in
			// the raw node-id sequence that the import path (which restore rides)
			// compacts. That renumbering is invisible to the user, so compare Redo
			// against the import-normalised form of `after` rather than the raw
			// in-memory bytes. Restoring is set so the normalisation neither
			// records a step nor clears history.
			normalize := func(b []byte) string {
				g.undoManager.restoring = true
				_ = g.Import(b)
				out := g.undoCapture()
				g.undoManager.restoring = false
				return string(out)
			}
			wantRedo := normalize(after)
			g.undoManager.Redo()
			if got := g.undoCapture(); string(got) != wantRedo {
				t.Fatalf("%s: Redo did not restore the post-action document", tc.name)
			}
		})
	}
}

// TestEveryUndoableKindHasActionCase machine-checks that the per-action undo
// sweep in undoActionCases() covers EVERY undoable kind the action registry
// declares — and that every kind a case claims is genuinely undoable. This is
// the guard that turns the round-trip sweep from a hand-maintained list into a
// registry-backed contract: add a new undoable kind to hooks.ActionRegistry and
// this test fails until a real case (exercising the production commit path)
// lands in undoActionCases().
func TestEveryUndoableKindHasActionCase(t *testing.T) {
	covered := map[hooks.Kind]bool{}
	for _, c := range undoActionCases() {
		covered[c.kind] = true
	}
	for _, a := range hooks.AllActions() {
		if !hooks.Undoable(a.Kind) {
			continue
		}
		if !covered[a.Kind] {
			t.Errorf("undoable kind %q has no case in undo_all_actions_test.go — add one", a.Kind)
		}
	}
	for k := range covered {
		if k != "" && !hooks.Undoable(k) {
			t.Errorf("case references %q which the registry says is not undoable", k)
		}
	}
}
