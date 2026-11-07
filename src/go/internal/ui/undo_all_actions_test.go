//go:build test

package ui

import (
	"image/color"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// kind is cross-checked against the registry so no undoable action goes uncovered.
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
			return string(id), true
		}
	}
	return "", false
}

// The one list both the round-trip sweep and the registry-coverage test read: a new undoable kind needs a case here.
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
			// Placing a node on the A-B edge splits it (edge delete, two edge adds, node add): ONE step.
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
			name: "paste-node",
			kind: hooks.EventNodePasted,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(9, 9, model.NodeTypeRegular) // source, auto-selected
				g.copySelectedNode()
			},
			mutate: func(t *testing.T, g *Game) { g.pasteNodeFrom(11, 11, g.clipboard) },
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
				ps.LogicKind = model.LogicProbability
				ps.LogicP = 0.5
				g.graph.SetNodeParams(n.ID, ps)
				emitNodeParamsChanged(n.ID, ps)
			},
		},
		{
			// The node-type commit path: flipping Regular to Silent changes the exported "type".
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
			// The setStartNodeGrid path, on a non-zero row so g.start is not rewritten too.
			name: "start-node-changed",
			kind: hooks.EventStartNodeChanged,
			setup: func(t *testing.T, g *Game) {
				g.drum.AddRow()
				if other, ok := pickOtherInstrumentFor(g); ok {
					g.drum.selRow = len(g.drum.Rows) - 1
					g.drum.SetInstrument(instrument.ID(other))
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
				// A distinct instrument: two rows sharing one hit an unrelated import colour reshuffle.
				if other, ok := pickOtherInstrumentFor(g); ok {
					g.drum.selRow = len(g.drum.Rows) - 1
					g.drum.SetInstrument(instrument.ID(other))
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
				g.drum.SetInstrument(instrument.ID(other))
			},
		},
		{
			// Rename is metadata only: the id never changes, only the display-name override and the row's Name.
			name: "instrument-renamed",
			kind: hooks.EventInstrumentRenamed,
			setup: func(t *testing.T, g *Game) {
				// The display-name override is PROCESS-GLOBAL; cleared so later cases render the canonical name.
				id := g.drum.Rows[0].Instrument
				t.Cleanup(func() { audio.ClearInstrumentDisplayName(id) })
			},
			mutate: func(t *testing.T, g *Game) {
				// Rename undo works only because Update wraps each frame in beginGroup/endGroup, deferring the snapshot until the
				// row's Name is updated; the bracket is mirrored so the real mechanism is tested.
				beginUndoGroup("rename instrument")
				g.drum.renameInstrumentTo(0, "Renamed Display")
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
			name: "circuit-theme",
			kind: hooks.EventCircuitThemeChanged,
			mutate: func(t *testing.T, g *Game) {
				t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
				g.setCircuitThemeCategory("accent", "orchid-300")
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
			// Enabling the master HPF makes the exported master-EQ block appear.
			name: "eq-filter-toggle",
			kind: hooks.EventEQFilterToggled,
			setup: func(t *testing.T, g *Game) {
				g.drum.eqPanelZone.SetActiveChannel(channel.Master)
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
				z.SetActiveChannel(channel.Master)
				z.bandGainsDB[3] = 6.0 // the exported value
				g.drum.eqPendingChannel = channel.Master
				g.drum.eqPendingBand = 3
				g.drum.eqPendingGainDB = 6.0
				g.drum.eqPendingDirty = true
				g.drum.commitEQBand()
			},
		},
		{
			// The FX add path; Undo re-imports the effect-free baseline to clear the slot.
			name: "insert-effect-added",
			kind: hooks.EventInsertEffectAdded,
			setup: func(t *testing.T, g *Game) {
				audio.ClearAllInsertEffects()
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				slot := audio.AddInsertEffect(id, audio.EffectDelay, nil)
				emitInsertEffectAdded(channel.Of(id), slot, string(audio.EffectDelay))
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
				emitInsertEffectRemoved(channel.Of(id), 0)
			},
		},
		{
			// emitInsertEffectParam does not record on its own, so the commit site records explicitly.
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
				emitInsertEffectParam(channel.Of(id), 0, pname, newVal)
				g.drum.recordUndoStep(hooks.EventInsertEffectParam)
			},
		},
		{
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
				emitInsertEffectToggled(channel.Of(id), 0, false)
				g.drum.recordUndoStep(hooks.EventInsertEffectToggled)
			},
		},
		{
			// Two distinct effect types so the reorder shows in the exported chain.
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
				emitInsertEffectMoved(channel.Of(id), 0, 1)
				g.drum.recordUndoStep(hooks.EventInsertEffectMoved)
			},
		},
		{
			// A knob release: the changed value changes the exported document.
			name: "instrument-params-committed",
			kind: hooks.EventInstrumentParamsCommitted,
			setup: func(t *testing.T, g *Game) {
				audio.ResetInstrumentParams(g.drum.Rows[0].Instrument)
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				if !audio.HasInstrumentDoc(id) {
					t.Skip("row-0 instrument has no instrument document")
				}
				if _, ok := pickSynthParam(g, string(id)); !ok {
					t.Skip("no editable synth param produced a document change")
				}
				emitInstrumentParamsCommitted(id, instrument.ID(instrumentOriginForTest(string(id))))
				g.drum.recordUndoStep(hooks.EventInstrumentParamsCommitted)
			},
		},
		{
			// Setup commits a synth change so the reset has something to revert.
			name: "instrument-params-reset",
			kind: hooks.EventInstrumentParamsReset,
			setup: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.ResetInstrumentParams(id)
				if !audio.HasInstrumentDoc(id) {
					t.Skip("row-0 instrument has no instrument document")
				}
				if _, ok := pickSynthParam(g, string(id)); !ok {
					t.Skip("no editable synth param produced a document change")
				}
			},
			mutate: func(t *testing.T, g *Game) {
				id := g.drum.Rows[0].Instrument
				audio.ResetInstrumentParams(id)
				emitInstrumentParamsReset(id, instrument.ID(instrumentOriginForTest(string(id))))
				g.drum.recordUndoStep(hooks.EventInstrumentParamsReset)
			},
		},
		{
			// Undo re-imports the edit-free baseline to revert it.
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
		{
			// emitGroupCreated records synchronously; no Update needed.
			name: "group-created",
			kind: hooks.EventGroupCreated,
			setup: func(t *testing.T, g *Game) {
				g.tryAddNode(20, 20, model.NodeTypeRegular)
				g.tryAddNode(21, 20, model.NodeTypeRegular)
			},
			mutate: func(t *testing.T, g *Game) {
				a := g.nodeAt(20, 20)
				b := g.nodeAt(21, 20)
				gid, err := g.graph.CreateGroup("", []model.NodeID{a.ID, b.ID})
				if err != nil {
					t.Fatalf("CreateGroup: %v", err)
				}
				grp, _ := g.graph.Group(gid)
				emitGroupCreated(gid, grp.Name, grp.NodeIDs)
			},
		},
		{
			// Directly, not through the deferred GroupMenu method, so undoCapture sees the mutation at once.
			name: "group-rule-changed",
			kind: hooks.EventGroupChanged,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(22, 20, model.NodeTypeRegular)
				b := g.tryAddNode(23, 20, model.NodeTypeRegular)
				if _, err := g.graph.CreateGroup("rule-group", []model.NodeID{a.ID, b.ID}); err != nil {
					t.Fatalf("CreateGroup: %v", err)
				}
			},
			mutate: func(t *testing.T, g *Game) {
				groups := g.graph.AllGroups()
				var gid model.GroupID
				for _, gr := range groups {
					if gr.Name == "rule-group" {
						gid = gr.ID
					}
				}
				if gid == 0 {
					t.Fatal("rule-group not found")
				}
				if err := g.graph.SetGroupRules(gid, []model.GroupRule{
					{Param: model.GroupParamPitch, Delta: 2, EveryN: 1},
				}); err != nil {
					t.Fatalf("SetGroupRules: %v", err)
				}
				emitGroupChanged(gid)
			},
		},
		{
			name: "group-deleted",
			kind: hooks.EventGroupDeleted,
			setup: func(t *testing.T, g *Game) {
				a := g.tryAddNode(24, 20, model.NodeTypeRegular)
				b := g.tryAddNode(25, 20, model.NodeTypeRegular)
				if _, err := g.graph.CreateGroup("doomed-group", []model.NodeID{a.ID, b.ID}); err != nil {
					t.Fatalf("CreateGroup: %v", err)
				}
			},
			mutate: func(t *testing.T, g *Game) {
				groups := g.graph.AllGroups()
				var gid model.GroupID
				for _, gr := range groups {
					if gr.Name == "doomed-group" {
						gid = gr.ID
					}
				}
				if gid == 0 {
					t.Fatal("doomed-group not found")
				}
				if err := g.graph.DeleteGroup(gid); err != nil {
					t.Fatalf("DeleteGroup: %v", err)
				}
				emitGroupDeleted(gid)
			},
		},
	}
}

// instrumentOriginForTest is the library entry id's project copy came from.
func instrumentOriginForTest(id string) string {
	d, _ := audio.InstrumentDoc(instrument.ID(id))
	return string(d.Origin)
}

// Writes the change as a side effect; ok=false means no candidate value survived the clamp.
func pickSynthParam(g *Game, id string) (string, bool) {
	audio.ResetInstrumentParams(instrument.ID(id))
	shipped := map[string]float64{}
	names := []string{}
	for name, v := range audio.MergedInstrumentParamsRO(instrument.ID(id)) {
		shipped[name] = v
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cur := shipped[name]
		// The clamp can collapse a candidate back to the current value.
		for _, cand := range []float64{cur + 0.5, cur - 0.5, 0.75, 0.25, cur + 1, cur - 1} {
			audio.ResetInstrumentParams(instrument.ID(id))
			// Callers reset first and rely on a single change.
			audio.SetInstrumentParam(instrument.ID(id), name, cand)
			eff := audio.MergedInstrumentParamsRO(instrument.ID(id))
			if s, ok := shipped[name]; !ok || (eff[name]-s > 1e-9 || s-eff[name] > 1e-9) {
				return name, true
			}
		}
	}
	audio.ResetInstrumentParams(instrument.ID(id))
	return "", false
}

// For EVERY undoable action, through the real export serializer: it changes the document, records EXACTLY ONE
// step (compound gestures are atomic), and one Undo / one Redo restore the before / after documents byte for byte.
// The baseline is taken after setup.
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

			// Byte-for-byte: undo to a no-EQ snapshot must actually clear the EQ.
			g.undoManager.Undo()
			if got := g.undoCapture(); string(got) != string(before) {
				t.Fatalf("%s: Undo not byte-identical to pre-action document", tc.name)
			}

			// A node delete leaves a gap in the raw id sequence that import compacts, invisibly to the user, so Redo is compared
			// against the import-normalised `after`; both restore flags are set, as undoRestore sets them, so it records nothing
			// and re-imports verbatim rather than as a circuit open.
			normalize := func(b []byte) string {
				g.undoManager.restoring, g.undoRestoring = true, true
				_ = g.Import(b)
				out := g.undoCapture()
				g.undoManager.restoring, g.undoRestoring = false, false
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

// Every kind the registry declares undoable has a case on its production commit path, and every claimed kind is
// undoable.
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
