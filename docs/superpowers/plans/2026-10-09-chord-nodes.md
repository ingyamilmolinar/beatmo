# Chord Nodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `chord` node type that fires N pitches from one grid cell, a chord builder in the node sidebar, and a Root × Quality chord matrix picker.

**Architecture:** The node stores a `ChordSpec{Quality, Inversion, Spread}`; one pure function `model.NodeLayers` derives the voice stack (the "z-axis"). The timeline sees a chord cell as one firing cell (`NodeType.Fires()`); the fan-out to N voices happens only where a note becomes sound (audio lane sink, legacy dispatch, songrender). The chord matrix is a second model behind the existing note-matrix grid picker.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten (ebitestub under `-tags test`), the existing audio lane / PlayBatch.

**Spec:** `docs/superpowers/specs/2026-10-09-chord-nodes-design.md`

## Global Constraints

- **Never `git add` / `git commit`** in this repo (user rule). Every "Commit" step in the template is replaced by a test checkpoint.
- Go tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`; the ui suite needs `-timeout 40m` when run whole.
- A single-layer (non-chord) note must be **bit-identical** to today everywhere (vol, pitch, dur, voice id sequence).
- `schema_version` stays `0.0.1`; `chord` is an additive `omitempty` node field, written whenever the spec is non-empty, for any type.
- Pitch range of the root stays `model.NodePitchMin/Max` (±24).
- No forbidden glyphs; the chord icon is an `IconID`. Names use `°` (U+00B0) and `ø` (U+00F8) only.
- No new `hooks.Kind` (reuse `emitNodeTypeChanged` / `emitNodeParamsChanged`).
- UI is not done until a captured PNG has been opened (CLAUDE.md loop); desktop + mobile.
- Other sessions may edit this tree: before editing a file with unstaged changes (`git status` `MM`), re-read it; never stash.

## Review Focus

1. **A chord node on an unpitched row** (hi-hat): cycle skips Chord; an existing chord node keeps playing. Test in Task 6.
2. **Changing a chord while playing** must never sound old + new together, nor drop the new chord when only a phantom voice was refused. Test in Task 4 (`TestLaneChordReplaceRefusedDropsWholeChord`).
3. **Undo / redo across a type cycle** restores the dormant spec (undo replays the export). Test in Task 3.
4. **Picking a chord root near the ±24 edge** clamps rather than producing an unpickable/out-of-range root. Test in Task 8.
5. **A file with `"type":"chord"` and no `chord` block** imports as a major triad (seeded like the cycle), not a silent node. Test in Task 3.

---

### Task 1: Model — chord spec, quality table, layers, names

**Files:**
- Create: `src/go/core/model/chord.go`, `src/go/core/model/chord_test.go`
- Modify: `src/go/core/model/node_types.go` (NodeTypeChord, Fires, NodeParams.Chord), `src/go/core/model/graph_params.go` (copy Chord)

**Interfaces:**
- Produces:
  - `const NodeTypeChord NodeType` (after `NodeTypeMute`, value 4 — fits the played tier's 6 type bits)
  - `func (t NodeType) Fires() bool` — Regular || Chord
  - `type ChordSpec struct{ Quality string; Inversion, Spread int }`, `func (c ChordSpec) IsZero() bool`
  - `const ChordSpreadClose, ChordSpreadDrop2, ChordSpreadWide = 0, 1, 2`; `ChordSpreadCount = 3`
  - `type ChordQuality struct{ ID, Suffix, Page string; Intervals []int }`
  - `var ChordQualities []ChordQuality`; `var ChordPages = []string{"Triads","7ths","Ext","Sus"}`
  - `func ChordQualityByID(id string) (ChordQuality, bool)`
  - `func ChordQualitiesOnPage(page string) []ChordQuality`
  - `func NormalizeChord(c ChordSpec) ChordSpec` — empty quality stays empty; unknown quality → zero spec; inversion clamped to [0, n-1]; spread clamped to [0,2]
  - `type Layer struct{ Semis, Gain float64 }`
  - `func NodeLayers(t NodeType, p NodeParams) []Layer` — returns nil for single-voice nodes (callers treat nil as one `{0,1}` voice)
  - `func ChordName(rootMIDI int, c ChordSpec) string`, `func PitchClassName(pc int) string`
  - `func NodeTypeName(t NodeType) string`, `func ParseNodeType(s string) (NodeType, bool)`

- [ ] **Step 1: Write failing tests** (`chord_test.go`)

```go
func TestChordQualityTableShape(t *testing.T) {
	if len(ChordQualities) != 20 { t.Fatalf("want 20 qualities, got %d", len(ChordQualities)) }
	seen := map[string]bool{}
	for _, p := range ChordPages {
		if n := len(ChordQualitiesOnPage(p)); n != 5 { t.Fatalf("page %s has %d", p, n) }
	}
	for _, q := range ChordQualities {
		if seen[q.ID] { t.Fatalf("dup %s", q.ID) }
		seen[q.ID] = true
		if q.Intervals[0] != 0 { t.Fatalf("%s must start at the root", q.ID) }
		for i := 1; i < len(q.Intervals); i++ {
			if q.Intervals[i] <= q.Intervals[i-1] { t.Fatalf("%s not ascending", q.ID) }
		}
	}
}

func semis(ls []Layer) []float64 { out := make([]float64, len(ls)); for i, l := range ls { out[i] = l.Semis }; return out }

func TestNodeLayersSingleForNonChord(t *testing.T) {
	p := NodeParams{Chord: ChordSpec{Quality: "maj7"}}
	for _, ty := range []NodeType{NodeTypeRegular, NodeTypeSilent, NodeTypeMute, NodeTypeInvisible} {
		if NodeLayers(ty, p) != nil { t.Fatalf("type %d must be single-voice", ty) }
	}
	if NodeLayers(NodeTypeChord, NodeParams{}) == nil { t.Fatal("chord with empty spec is a major triad") }
}

func TestNodeLayersVoicings(t *testing.T) {
	cases := []struct{ spec ChordSpec; want []float64 }{
		{ChordSpec{Quality: "maj"}, []float64{0, 4, 7}},
		{ChordSpec{Quality: "maj", Inversion: 1}, []float64{4, 7, 12}},
		{ChordSpec{Quality: "maj", Inversion: 2}, []float64{7, 12, 16}},
		{ChordSpec{Quality: "maj", Spread: ChordSpreadDrop2}, []float64{-8, 0, 7}},
		{ChordSpec{Quality: "maj", Spread: ChordSpreadWide}, []float64{-12, -8, 7}},
		{ChordSpec{Quality: "maj7", Inversion: 3}, []float64{11, 12, 16, 19}},
		{ChordSpec{Quality: "5", Spread: ChordSpreadDrop2}, []float64{-12, 7}},
		{ChordSpec{Quality: "5", Spread: ChordSpreadWide}, []float64{-12, 7}},
	}
	for _, c := range cases {
		got := semis(NodeLayers(NodeTypeChord, NodeParams{Chord: c.spec}))
		if !reflect.DeepEqual(got, c.want) { t.Errorf("%+v: got %v want %v", c.spec, got, c.want) }
	}
	ls := NodeLayers(NodeTypeChord, NodeParams{Chord: ChordSpec{Quality: "maj7"}})
	if g := ls[0].Gain; math.Abs(g-0.5) > 1e-12 { t.Fatalf("gain = 1/sqrt(4), got %v", g) }
}

func TestNormalizeChordClamps(t *testing.T) {
	if got := NormalizeChord(ChordSpec{Quality: "maj", Inversion: 3, Spread: 9}); got.Inversion != 2 || got.Spread != 2 { t.Fatal(got) }
	if !NormalizeChord(ChordSpec{Quality: "nope"}).IsZero() { t.Fatal("unknown quality must zero") }
}

func TestChordName(t *testing.T) {
	cases := []struct{ midi int; spec ChordSpec; want string }{
		{60, ChordSpec{Quality: "maj"}, "C"},
		{61, ChordSpec{Quality: "m7"}, "C#m7"},
		{60, ChordSpec{Quality: "maj7", Inversion: 1}, "Cmaj7/E"},
		{60, ChordSpec{Quality: "dim"}, "C°"},
		{60, ChordSpec{Quality: "m7b5"}, "Cø7"},
		{60, ChordSpec{Quality: "maj", Spread: ChordSpreadWide}, "C"},
	}
	for _, c := range cases {
		if got := ChordName(c.midi, c.spec); got != c.want { t.Errorf("got %q want %q", got, c.want) }
	}
}

func TestNodeTypeNameRoundTrip(t *testing.T) {
	for _, ty := range []NodeType{NodeTypeRegular, NodeTypeSilent, NodeTypeMute, NodeTypeChord} {
		got, ok := ParseNodeType(NodeTypeName(ty))
		if !ok || got != ty { t.Fatalf("%d round-trip", ty) }
	}
	if !NodeTypeChord.Fires() || !NodeTypeRegular.Fires() || NodeTypeMute.Fires() { t.Fatal("Fires") }
}

func TestSetNodeParamsCopiesChord(t *testing.T) {
	g := NewGraph()
	id := g.AddNode(0, 0, NodeTypeChord)
	g.SetNodeParams(id, NodeParams{Volume: 1, Duration: 1, Chord: ChordSpec{Quality: "m7", Inversion: 1}})
	if n := g.Nodes[id]; n.Params.Chord.Quality != "m7" || n.Params.Chord.Inversion != 1 { t.Fatal(n.Params.Chord) }
}
```
(Use the graph constructor/add helpers `graph_test.go` already uses if they differ from `NewGraph`/`AddNode`.)

- [ ] **Step 2: Run, verify FAIL** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/model/ -run 'Chord|NodeLayers|NodeTypeName'` → undefined symbols.

- [ ] **Step 3: Implement** `chord.go` (quality table exactly as the spec's table: IDs `maj m dim aug 5 | 7 maj7 m7 m7b5 dim7 | 6 m6 9 maj9 m9 | sus2 sus4 7sus4 add9 madd9`, suffixes `"" m ° + 5 | 7 maj7 m7 ø7 °7 | 6 m6 9 maj9 m9 | sus2 sus4 7sus4 add9 m(add9)`). `NodeLayers`:

```go
func NodeLayers(t NodeType, p NodeParams) []Layer {
	if t != NodeTypeChord { return nil }
	c := NormalizeChord(p.Chord)
	if c.Quality == "" { c.Quality = "maj" }
	q, _ := ChordQualityByID(c.Quality)
	v := make([]int, len(q.Intervals))
	copy(v, q.Intervals)
	for k := 0; k < c.Inversion; k++ { v[k] += 12 }
	sort.Ints(v)
	if c.Spread >= ChordSpreadDrop2 {
		low := v[0]
		if len(v) == 2 {
			v[0] -= 12
		} else {
			v[len(v)-2] -= 12
			if c.Spread == ChordSpreadWide {
				for i := range v { if v[i] == low { v[i] -= 12; break } }
			}
		}
		sort.Ints(v)
	}
	g := 1 / math.Sqrt(float64(len(v)))
	out := make([]Layer, len(v))
	for i, s := range v { out[i] = Layer{Semis: float64(s), Gain: g} }
	return out
}
```
`ChordName`: root pc = `((rootMIDI%12)+12)%12`; name = `PitchClassName(pc)+q.Suffix`; when `Inversion>0`, bass = lowest layer of the CLOSE voicing at the same inversion → `"/"+PitchClassName(pc+bassSemis)`. `PitchClassName` uses the same sharps table as `ui.midiNoteName` (`C C# D D# E F F# G G# A A# B`). Add `NodeTypeChord`, `Fires`, `Chord ChordSpec` field, the `SetNodeParams` copy line `n.Params.Chord = p.Chord`, and the name table (`regular silent mute invisible chord`).

- [ ] **Step 4: Run, verify PASS** (same command) and `go test ./core/...`.

---

### Task 2: Timeline treats chord as a firing node (`Fires()` migration + guard)

**Files:**
- Modify: every non-test site listed by `grep -rn "NodeTypeRegular" --include=*.go src/go/core src/go/internal | grep -v _test | grep -v scene_catalog` whose meaning is "this node sounds/triggers/shows as on" (`core/engine/predictor_compute.go`, `predictor_logic.go`, `internal/ui/game_trigger_logic.go`, `game_tick_highlight.go`, `game_highlight_state.go`, `game_sequencer_highlight.go`, `game_sequencer_schedule.go`, `game_parity_*.go`, `game_refresh_drum_row.go`, `game_graph_update_beat_infos.go`, `game_subdivisions.go`, `warm_set.go`, `voice_prewarmer.go`, `timeline_zone.go`, `drumview_*` cell code, `js_exports_timeline_predictor.go`, `audio_lane_note.go`, `game_graph_nodes.go`, `game_played_snapshot.go`)
- Create: `src/go/internal/ui/node_type_fires_discipline_test.go`, `src/go/core/engine/predictor_chord_test.go`

**Interfaces:** Consumes `NodeType.Fires()`. Produces: a guard that fails on any new `== / != model.NodeTypeRegular` comparison, and on any `case model.NodeTypeRegular` clause that does not also list `model.NodeTypeChord`.

- [ ] **Step 1: Write the predictor equivalence test** — build the same 4-node loop twice (one node Regular vs Chord, same params incl. a LogicKind on another node), run `Predictor.Ensure(64)`, assert `AudibleAt/VisibleAt/TriggeredAt/DecisionAt` identical for every (row, abs). Use the setup pattern of an existing `core/engine/predictor_*_test.go`.
- [ ] **Step 2: Write the AST guard** (go/parser over `core/` and `internal/` non-test files, excluding `scene_catalog.go`): flag `BinaryExpr` with `==`/`!=` where either side is selector `model.NodeTypeRegular` (or bare `NodeTypeRegular` inside package model, except `node_types.go`), and `CaseClause` lists containing NodeTypeRegular without NodeTypeChord. Report `file:line`.
- [ ] **Step 3: Run both, verify FAIL** (equivalence fails because the predictor ignores type 4; guard lists every site).
- [ ] **Step 4: Migrate each site.** Rules: `x == NodeTypeRegular` → `x.Fires()`; `x != NodeTypeRegular` → `!x.Fires()`; `x != Regular && x != Mute` → `!x.Fires() && x != NodeTypeMute`; `case NodeTypeRegular:` → `case NodeTypeRegular, NodeTypeChord:`. Constructor arguments (`tryAddNode(..., NodeTypeRegular)`) stay. In `game_graph_nodes.go` keep the creation-time branches meaning "a brand-new regular node" unchanged if they are constructor logic; read each before editing.
- [ ] **Step 5: Run** `go test -tags test ./core/... ./internal/timeline/` and `-run 'NodeTypeFires|Predictor|Parity|Highlight' ./internal/ui/` → PASS.

---

### Task 3: Format, single name table, undo

**Files:**
- Modify: `internal/ui/project_file.go` (or wherever `exportNode` lives — `export.go:141`), `internal/ui/export.go:262-313`, `internal/ui/import.go:318-420`, `internal/ui/event_helpers.go:446`, `internal/ui/js_exports_graph_ui.go:29`, `internal/ui/js_exports_timeline_predictor.go:256`, `internal/audio/songrender/graph.go:20`, `internal/eventlogger/format.go:91`, `cmd/gen-showcase/main.go:188`
- Create: `internal/ui/chord_node_format_test.go`

**Interfaces:**
- Produces: `type exportChord struct{ Quality string \`json:"quality"\`; Inversion int \`json:"inversion,omitempty"\`; Spread int \`json:"spread,omitempty"\` }`; `exportNode.Chord *exportChord \`json:"chord,omitempty"\``.
- Consumes: `model.NodeTypeName/ParseNodeType/NormalizeChord/ChordQualityByID`.

- [ ] **Step 1: Failing tests** in `chord_node_format_test.go` (use the test-Game helpers other import/export tests use, e.g. `newTestGame(t)` + `t.Cleanup(g.CloseForTest)`):
  - `TestChordNodeExportImportRoundTrip`: chord node `{maj7, inv 1, spread 2}` → `exportBytes` contains `"type":"chord"` and `"chord":{"quality":"maj7","inversion":1,"spread":2}` → import into a fresh game → node type Chord, spec equal; re-export byte-identical.
  - `TestDormantChordSpecSurvivesExport`: Regular node with spec `{m7}` exports the `chord` block with `"type":"regular"`; import restores it.
  - `TestImportRejectsUnknownChordQuality`: `"chord":{"quality":"bogus"}` → import error and the current circuit unchanged (node count before == after); same for `inversion` ≥ voice count and `spread` > 2.
  - `TestImportChordTypeWithoutSpecSeedsMajor` (Review Focus 5): `"type":"chord"` without a block → spec `{maj}`.
  - `TestUndoRestoresChordAcrossTypeCycle`: set chord {m7}, cycle the type to Silent via the sidebar path (or `setNodeType` helper the "aud" button uses), `Undo()` → type Chord, spec {m7}.
- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement.** Export: `Type: model.NodeTypeName(n.Type)`; `if !p.Chord.IsZero() { en.Chord = &exportChord{...} }`. Import: `ParseNodeType` (unknown → regular, as today); validate the chord block in the pre-reset validation pass (find where `validateProjectDocs` / node validation runs before reset; add a `validateNodeChords(pf)` returning `fmt.Errorf("node %d: unknown chord quality %q", ...)`); apply via `SetNodeParams` with `Chord: model.NormalizeChord(...)`; when type is Chord and the block is absent, `Chord{Quality: "maj"}`. Replace the 9 duplicated type↔string switches with the model table.
- [ ] **Step 4: Run** `-run 'Chord|Import|Export|Undo' ./internal/ui/` + `./internal/audio/songrender/ ./internal/eventlogger/ ./cmd/gen-showcase/` → PASS.

---

### Task 4: Fan-out — lane, sink, legacy dispatch, prewarm

**Files:**
- Modify: `internal/ui/audio_lane_note.go` (laneNote.Layers, hash, `voiceCount`, `voiceAt`), `internal/ui/audio_lane.go` (vid blocks), `internal/ui/audio_lane_sink.go` (expand voices, Replace signature), `internal/ui/game_audio_schedule.go` (legacy loop), `internal/ui/game_trigger_logic.go` (`nodeLayersAt`), test fakes in `audio_lane_test.go` / `audio_lane_sink_test.go`
- Create: `internal/ui/audio_lane_chord_test.go`

**Interfaces:**
- Produces:
  - `laneNote.Layers []model.Layer` (nil = single voice)
  - `func (n *laneNote) voiceCount() int` (max(1, len(Layers)))
  - `func (n *laneNote) voiceAt(k int) (pitch, vol float64)` — k-th voice: `Pitch+Layers[k].Semis`, `Vol*Layers[k].Gain`; nil Layers → `(Pitch, Vol)` exactly
  - `func laneVoiceIDs(base uint64, n *laneNote) []uint64` → `base .. base+voiceCount-1`
  - `audioSink.Replace(old uint64, oldVoices int, vid uint64, n laneNote)`
  - `func (s *playbackSlate) nodeLayersAt(info model.BeatInfo) []model.Layer`
- Lane minting: `func (l *audioLane) mint(n *laneNote) uint64 { base := l.nextVID + 1; l.nextVID += uint64(n.voiceCount()); return base }` — for single voices identical to `nextVID++`.

- [ ] **Step 1: Failing tests** (`audio_lane_chord_test.go`, reuse `fakeSink` + scripted source of `audio_lane_test.go`):
  - `TestLaneChordCommitsOneBlockPerCell`: a source cell with 3 layers → one committed entry, fake sink sees Schedule(base) with `voiceCount()==3`; next cell's base = base+3.
  - `TestLaneChordEditReplacesWholeChord`: change layers → `Replace(old, 3, new, n)`.
  - `TestLaneChordCancelCancelsEveryVoice`: remove the cell → cancels `[b, b+1, b+2]`.
  - `TestFlushLaneOpsExpandsChordVoices` (sink, real `flushLaneOps` with `audio` test hooks as in `audio_lane_sink_test.go`): one Schedule of a 3-layer note → PlayBatch gets 3 params with VoiceIDs b..b+2, pitches root+semis, vols vol·gain.
  - `TestLaneChordReplaceRefusedDropsWholeChord` (Review Focus 2): engine reports only `b+1` started → none of the replacement voices are added; and when it reports nothing started → all 3 added.
  - `TestLegacyDispatchPlaysEveryChordVoice`: legacy path (`enableAudioLaneForTest` NOT called) with a chord node → `playFn` called 3× with the expected pitches; a regular node → 1× with today's exact args.
  - `TestLaneSingleVoiceUnchanged`: regular note → vids 1,2,3… and BatchParam fields identical to before (compare against a hand-built param).
- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement.**
  - `nodeLayersAt`: `if !info.NodeType.Fires() {return nil}; n, ok := g.nodeSnapshot(info.NodeID); if !ok {return nil}; return model.NodeLayers(n.Type, n.Params)`.
  - `laneNoteAt`: after computing vol/pitch/dur: `n.Layers = s.nodeLayersAt(info)`; hash writes `len(Layers)` and each `Semis`/`Gain` bits.
  - Lane: `commit` and the replace branch use `mint`; `cancel` appends `laneVoiceIDs(e.vid, &e.note)...`; replace calls `l.sink.Replace(e.vid, e.note.voiceCount(), vid, n)`.
  - Sink: `laneOp.oldN int`; `add(vid, n)` loops `k < n.voiceCount()`: `p, v := n.voiceAt(k)`, warmPitch(inst, p), append BatchParam with `VoiceID: vid+uint64(k)`; open the timing trace once (k==0). Replace run: `olds` gets every `old..old+oldN-1`; a replacement is added only if none of its old ids is in `started`.
  - Legacy `scheduleSound`: `layers := s.nodeLayersAt(info)`; if nil → today's code path untouched; else loop: voice 0 keeps parityRow/parityAbs and the timing trace, voices ≥1 pass `-1,-1` to `queueSoundAtParams`; warmPitch per voice.
- [ ] **Step 4: Run** `-run 'Lane|Legacy|Chord|FlushLaneOps' ./internal/ui/` → PASS, then the whole lane family incl. `TestLaneHeardEqualsLegacyDispatched` and `TestLaneKeysCoverEveryEditClass` (add chord quality/inversion/spread/type to its edit classes).

---

### Task 5: Offline render (songrender)

**Files:** Modify `internal/audio/songrender/arrangement.go:101-130`; test in `internal/audio/songrender/arrangement_test.go` (or the package's existing test file).

- [ ] **Step 1: Failing test** `TestArrangementChordEmitsOneNoteEventPerLayer`: template JSON with one chord node `{m7}` → 4 `NoteEvent`s per firing step, `PitchSemis` = pitch + {0,3,7,10}, `Volume` = vol·0.5; a regular node → 1 event, unchanged.
- [ ] **Step 2: FAIL.** **Step 3:** after computing vol/dur: `layers := model.NodeLayers(node.Type, node.Params)`; nil → one event as today; else one event per layer. (`songrender/graph.go` must parse `"chord"` + the block — done in Task 3 via the shared table; add block parsing here if its parser is separate.) **Step 4: PASS.**

---

### Task 6: Sidebar — type cycle, chord section, Single note

**Files:**
- Modify: `internal/ui/game_node_sidebar.go` (layout `:300-335`, sections, `wireButtons :679`, aud cycle `:973-990`, `buttonOrder :1569`, `sectionBadgeText :2134`, the `:1698/:1890/:2178` type switches), `internal/ui/js_exports_graph_ui.go :385/:491` cycles
- Create: `internal/ui/node_sidebar_chord_test.go`, `internal/ui/node_type_cycle.go`

**Interfaces:**
- Produces: `func nextNodeType(cur model.NodeType, pitched bool) model.NodeType` (Regular→Chord→Silent→Mute→Regular; skips Chord when !pitched); `func (g *Game) setNodeChord(id model.NodeID, c model.ChordSpec)` (SetNodeParams → notifyPredictorNode → emitNodeParamsChanged); sidebar button keys `"chinv-"`, `"chinv+"`, `"chspr-"`, `"chspr+"`, `"chsingle"`.

- [ ] **Step 1: Failing tests** (real input through `Game.Update`, mirroring the existing node-sidebar tests' click helpers):
  - cycle order on a pitched row; on an unpitched row (`hihat`) Chord is skipped (Review Focus 1); an existing chord node on a row switched to `hihat` keeps type Chord and still fires (lane/legacy `playFn` count > 0).
  - entering Chord with an empty spec seeds `{maj}`; spec kept through Chord→Silent→Mute→Regular→Chord.
  - chord section visible only on Chord; Inv ± range 0..2 on a triad, 0..3 on maj7; Spread ± 0..2; each press = one undo step.
  - Single note → Regular, spec kept.
- [ ] **Step 2: FAIL.** **Step 3: Implement** using the existing `layoutSection` stepper row for Inv and Spread (labels: `root/1st/2nd/3rd`, `close/drop2/wide`), a full-width keycap for Single note; `sectionBadgeText` for the chord section = `ChordName`. **Step 4: PASS** + existing sidebar tests (`-run NodeSidebar`).

---

### Task 7: Extract the generic grid picker (behaviour-neutral)

**Files:** Modify `internal/ui/note_matrix_comp.go`, `internal/ui/note_matrix_model.go`, `internal/ui/game_node_sidebar_note_matrix.go`; Create `internal/ui/grid_picker_model.go`.

**Interfaces:** Produces `type gridPickerModel interface { Cols() int; Rows() int; RowLabel(row int) string; ColLabel(col int) string; CellLabel(col, row int) string; Selected() (col, row int, ok bool) }`; the component holds a `gridPickerModel` instead of `noteMatrixModel`; optional `pages *SegmentedControl` header slot (nil for the note matrix).

- [ ] **Step 1:** Run the note-matrix tests to record green: `-run 'NoteMatrix' ./internal/ui/`.
- [ ] **Step 2:** Refactor (noteMatrixModel implements the interface; the component reads only the interface; picking stays routed through the host's apply func).
- [ ] **Step 3:** Same tests → PASS unchanged; capture scene `node_note_matrix` (or the existing note-matrix scene) before/after and compare the PNGs.

---

### Task 8: Chord matrix

**Files:** Create `internal/ui/chord_matrix_model.go`, `internal/ui/game_node_sidebar_chord_matrix.go`, `internal/ui/chord_matrix_test.go`; Modify the sidebar's pitch-chip handler (`"pitnote"`) to open the chord matrix on chord nodes.

**Interfaces:**
- `type chordMatrixModel struct{ page int; rootPitch float64; instOffset float64; spec model.ChordSpec }` implementing `gridPickerModel` (rows = 12 pitch classes C..B, cols = qualities on `page`).
- `func (m chordMatrixModel) PickAt(col, row int) (pitch float64, spec model.ChordSpec)` — pitch = same octave as the current root: `base := rootPitch - float64(((pcOf(rootPitch)) ))` then `+row`, clamped into [NodePitchMin, NodePitchMax] by ±12 (Review Focus 4); spec quality = column's, inversion re-clamped, spread kept.
- `func (g *Game) applyChordMatrixPick(col, row int)` — selecting applies; re-selecting the current chord closes.

- [ ] **Step 1: Failing tests:** model (labels `C#m7`, selected cell, root-octave rule incl. root at +23 picking `B`→ stays ≤ +24, at −24 picking `C` ok); real input: chip on a chord node opens the chord matrix (on regular, the note matrix); page switch by tap and Tab/Shift+Tab; pick applies + stays open; re-pick closes; scroll drag never picks; Esc / outside tap / × close (add a row to `TestEveryMenuClosesTheSameWay` if the sidebar pickers are listed there); arrows don't pan while open, pan when closed.
- [ ] **Step 2: FAIL.** **Step 3: Implement** by mirroring `game_node_sidebar_note_matrix.go` (open/close state, `noteMatrixBounds`-style clamp, `overPicker` guard, `MenuKeyNav` grid host, `ClaimsKeyboard`), with the `SegmentedControl` (`ChordPages`) in the header. **Step 4: PASS.**

---

### Task 9: Rendering — icon, node overlay, badge, drum-view mark, log

**Files:** Modify `internal/ui/icons.go` (IconChord), `internal/ui/game_draw_grid_pane.go:178-240` (`drawNodeStateOverlay`), `internal/ui/grid_pane_draw.go:1201` (`coordBadgeText`), drum-view cell drawing (`drumview_cache_row_sprite.go`, `drumview_draw.go`, `drumview_cache_rows_window.go`), `internal/eventlogger/format.go`; tests in `internal/ui/chord_node_render_test.go`.

- [ ] **Step 1: Failing tests:** `coordBadgeText` for a chord node starts with `ChordName`; `drawNodeStateOverlay` draws `IconChord` for chord nodes (intercept `DrawIcon` or pixel-diff vs a regular node using the ebitestub); a played chord cell keeps its chord mark after the node is changed to Regular (played-tier byte carries type 4); the event formatter renders `chord C#m7`.
- [ ] **Step 2: FAIL.** **Step 3: Implement** (icon = three stacked horizontal bars drawn in the icon mask style; colour from an existing token, e.g. the node-state overlay colour; drum cell mark = three 1px ticks in the cell's top-right, same token). **Step 4: PASS** + `TestNoColorRendersOutsideTheTheme`, `TestTokenDiscipline`, forbidden-glyph tests.

---

### Task 10: Scenes, screenshots, real-build identity

**Files:** Modify `internal/ui/scene_catalog.go`; `export_import_master_audio_harness_test.go` headline circuit + `TestHeadlineEditsAreAudible`.

- [ ] **Step 1:** Scenes `node_chord_sidebar`, `chord_matrix`, `crop_chord_matrix` (Subject = picker rect), `crop_chord_node` (node + badge + drum row) — one-liner Setup style.
- [ ] **Step 2:** Build + capture desktop and mobile (`CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd`; `xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`; mobile `-mobile -scene-pass mobile -win-w 390 -win-h 844`; `XDG_CONFIG_HOME=<scratch>`). Open every PNG; fix and re-capture until right.
- [ ] **Step 3:** Add a chord node to the identity headline circuit and a "change the chord quality" audible edit; run `make test-real` scoped: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/ui/ -run 'ExportImportMasterMix|HeadlineEdits' -timeout 40m`.
- [ ] **Step 4:** Full fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... -timeout 40m`; report any failure with output (prove causation before calling anything pre-existing).
