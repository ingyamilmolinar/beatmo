# Chord nodes — a per-node voice stack ("z-axis") + chord builder + chord matrix

Date: 2026-10-09 · Status: approved in conversation (§1–§3), written spec pending review

## Intent

- A **chord** node type: one node fires several pitches at once on its row's
  instrument, so a chord no longer needs one circuit (row) per voice.
- Built on a general **voice stack** — the "z-axis": a node fans out into N
  simultaneous voices from one grid cell. The graph stays 2D; the stack lives
  inside the node. The chord builder is the stack's first (and in v1 only) producer.
- A **chord builder** in the node sidebar, shown only on chord nodes, and a
  **chord matrix** menu showing every chord (12 roots × 20 qualities = 240).
- Reuse what exists: node sidebar sections + steppers, the note-matrix grid picker
  (`solveSwatchGrid`, `MenuKeyNav` grid mode), `SegmentedControl`, the audio lane's
  batch dispatch, the export-replay undo journal.

User decisions (conversation, 2026-10-09):

| Question | Decision |
|---|---|
| How literal is the z-axis? | **Internal voice stack** — not user-navigable, no 3D graph |
| Matrix axes | **Root × Quality**: 12 root rows × quality columns, paged by family |
| Voicing controls in v1 | **Inversion** + **Spread** (strum and custom notes deferred) |
| Entry point | **Type cycle**: Regular → Chord → Silent → Mute → Regular |
| Leaving Chord | **Keeps the chord spec** — collapse plays the root; cycling back restores the last chord. Plus a direct **Single note** action. |

Assumptions (stated, not contested): a chord plays on the row's single
instrument; the node's existing `Pitch` is the chord **root**; chords are only
offered on pitched instruments.

Non-goals (v1): strum / per-layer timing, custom note clusters, per-layer
instrument, a project key / diatonic view, converting `gen-showcase` score chords
into chord nodes (all 83 templates would change — follow-up).

## Why this shape (what the survey found)

- One-note-per-cell is assumed from the predictor down to the lane:
  predictor bools per `[row][abs]`; `laneNoteAt` returns one note; lane
  `committed map[{row,abs}]laneEntry` holds one `vid`; `evalNodeParamsOnly`
  returns one (vol, pitch, dur); `songrender` emits one `NoteEvent` per node-step.
- **The engine does not care**: voice ids are a lane counter, `PlayBatch` already
  takes a batch on both platforms, and nothing chokes a second voice of the same
  instrument. Fan-out therefore belongs in the note derivation, not the engine.
- The predictor gates on `== NodeTypeRegular` in ~20 places, and the
  type↔string mapping is duplicated in 9 files.

Approaches considered: **(A) store the chord spec, derive the layers** — chosen;
(B) store explicit layers (two sources of truth or a chord-recognition engine,
YAGNI until custom notes exist); (C) expand chords into hidden rows (pollutes
solo/mute, mixer, drum view, row cap — rejected).

## §1 Data model + predictor

### Node type

- `core/model/node_types.go`: add `NodeTypeChord`.
- Add `func (t NodeType) Fires() bool` — true for `Regular` and `Chord`. Every
  `== NodeTypeRegular` check that means "this node sounds / triggers"
  (`core/engine/predictor_compute.go`, `predictor_logic.go`, and any UI site with
  the same meaning) switches to `Fires()`. The predictor cannot tell a chord from
  a regular node: same decisions, same timeline, same played-tier entries.
- **One name table**: `model.NodeTypeName(t) string` / `model.ParseNodeType(s)
  (NodeType, bool)` replace the duplicated mappings in `internal/ui/export.go`,
  `import.go`, `event_helpers.go` (`nodeTypeName`), `js_exports_graph_ui.go` (incl.
  the two cycling switches), `js_exports_timeline_predictor.go`,
  `internal/audio/songrender/graph.go`, `internal/eventlogger/format.go`,
  `cmd/gen-showcase/main.go`. String: `"chord"`.

### Chord spec

```go
// core/model/chord.go
type ChordSpec struct {
    Quality   string // key into ChordQualities; "" = no spec
    Inversion int    // 0 = root position … len(intervals)-1
    Spread    int    // ChordSpreadClose | ChordSpreadDrop2 | ChordSpreadWide
}
```

- `NodeParams` gains `Chord ChordSpec`; `SetNodeParams` (`graph_params.go`) copies
  it (it copies field-by-field — a missed field is silently dropped).
- **The spec is independent of the node type.** It is read only while the type is
  `Chord`, but it is kept on every type, so collapsing to Regular (or cycling
  through Silent/Mute) and back restores the last chord. The root is the node's
  `Pitch`, so transposing a collapsed node and re-entering Chord gives the same
  chord on the new root.

### Quality table — one table, four pages of five

`model.ChordQualities` (ordered; `Page`, `ID`, `Suffix`, `Intervals` in semitones
from the root):

| Page | Qualities (suffix: intervals) |
|---|---|
| Triads | maj `""`: 0 4 7 · m `m`: 0 3 7 · dim `°`: 0 3 6 · aug `+`: 0 4 8 · power `5`: 0 7 |
| 7ths | 7 `7`: 0 4 7 10 · maj7 `maj7`: 0 4 7 11 · m7 `m7`: 0 3 7 10 · half-dim `ø7`: 0 3 6 10 · dim7 `°7`: 0 3 6 9 |
| Ext | 6 `6`: 0 4 7 9 · m6 `m6`: 0 3 7 9 · 9 `9`: 0 4 7 10 14 · maj9 `maj9`: 0 4 7 11 14 · m9 `m9`: 0 3 7 10 14 |
| Sus | sus2 `sus2`: 0 2 7 · sus4 `sus4`: 0 5 7 · 7sus4 `7sus4`: 0 5 7 10 · add9 `add9`: 0 4 7 14 · m(add9) `m(add9)`: 0 3 7 14 |

Glyphs `°` (U+00B0) and `ø` (U+00F8) are in the shipped Latin-1 font subset
(`scripts/subset_fonts.mjs`); no `♭` is used. Names use sharps, like
`midiNoteName`.

### Layers — the z-axis

```go
type Layer struct{ Semis, Gain float64 } // Semis relative to node Pitch
func NodeLayers(t NodeType, p NodeParams) []Layer
```

- Non-chord type, or empty/unknown spec → one layer `{0, 1}` (identical to today).
- Chord: start from the quality's intervals (root position, ascending), then
  - **Inversion k**: the lowest k voices move up an octave (+12), re-sorted.
    `Pitch` stays the root-position root, so ± transposes the whole chord.
  - **Spread**: `Close` = as above; `Drop2` = the second-highest voice −12;
    `Wide` = Drop2, plus the voice that was lowest **before** the drop also −12
    (C4 E4 G4 → Drop2 E3 C4 G4 → Wide C3 E3 G4). Power chords (2 voices): Drop2
    and Wide both mean "lowest voice −12".
  - **Gain** = `1/√N` per layer (equal power), so a chord reads as loud as one note.
- Pure, allocation-light, and the only place chord math lives.
- `ChordName(rootMIDI int, spec ChordSpec) string` → `"C#m7"`, with `/<bass>`
  when the inversion makes the lowest voice not the root (`"Cmaj7/E"`). Spread does
  not change the name.

### Clamping rules

- Inversion is clamped to `[0, len(intervals)-1]` whenever the quality changes.
- Root pitch keeps its existing `NodePitchMin/Max` (±24). Layer pitches may go
  beyond it (max +24 + 14 + 12); the engine already handles those rates.

### Unpitched instruments

- The type cycle **skips** Chord when the row's instrument is unpitched
  (`nodeNoteLabel(...) == ""` — the 12 noise instruments).
- A chord node whose row is later switched to an unpitched instrument keeps its
  data and keeps playing (resampled layers); nothing is silently rewritten.

### Format

- `exportNode` gains `Chord *exportChord \`json:"chord,omitempty"\`` =
  `{"quality":"maj7","inversion":1,"spread":1}`, written **whenever the spec is
  non-empty, for any type** (the dormant spec on a collapsed node must survive
  save/load and undo — undo replays `exportBytes`).
- `type: "chord"` via `ParseNodeType`.
- Import: an unknown `quality`, or an out-of-range `inversion` / `spread`, is a
  **hard error raised before anything resets** (same class as the other
  `validateProjectDocs` rejections). `schema_version` stays `0.0.1` (additive,
  pre-release).
- Templates and `songrender.ParseTemplate` read the same shape.

## §2 Fan-out: one cell → N voices

### One helper for both scheduling paths

```go
// internal/ui: what one firing cell sounds like — 1 voice for Regular, N for Chord.
type cellVoice struct{ Pitch, Vol, Dur float64 }
func cellVoices(t model.NodeType, p model.NodeParams, rowVol, groupPitch float64) []cellVoice
```

Wraps `model.NodeLayers` and folds in exactly what `evalNodeParamsOnly` applies
today (row volume × node volume, node pitch + group pitch delta, duration). The
single-layer case must produce the same (vol, pitch, dur) as today, bit for bit.

### Audio lane

- `laneNote` stays **one per cell** — the `{row, abs}` key, `Committed()`, seek and
  played-history semantics are unchanged. Its scalar `Pitch`/`Vol` become
  `Voices []laneVoice{Pitch, Vol}`; `Hash` covers every voice.
- `laneEntry.vid` → `vids []uint64` (one per layer, minted from `nextVID`).
- `laneSource.NoteAt` derives voices through `cellVoices`.
- **Replace is per cell and all-or-nothing**: `audioSink.Replace(old, new
  []uint64, n laneNote)`. `flushLaneOps` cancels the old vids in its existing single
  `audio.CancelVoices` call; if the engine refuses **any** of them (started), all
  of that cell's replacements are dropped. A chord's voices share one start time,
  so they start — and are refused — together; the never-old-and-new-together
  guarantee holds at chord level. An edit reaches the next chord that has not
  started (unchanged `CancelGuardSec` semantics).
- Cancel / stop / seek paths iterate `vids`.
- `flushLaneOps` emits one `audio.BatchParam` per voice (same `When`, `Row`,
  `Abs`, `Grid`, `Ideal`; own `VoiceID`). No engine change on either platform.

### Legacy dispatch

`scheduleSound` (lane off; all `go test` paths unless `enableAudioLaneForTest`)
loops `cellVoices` and calls `playFn` once per voice.

### Prewarm

`voicePrewarmer.warmPitch(inst, pitch)` is called for every voice pitch (lane
sink and the gated import warm-up). The voice cache's 0.5-semitone rounding means
repeated chords hit the cache.

### Unchanged on purpose

Predictor, timeline, played tier, highlights: a chord cell is one trigger.
Parity (`parityCheck` / `parityScan` / `parityAudio`) compares *whether a cell
fired* — still a per-cell bool. Timing traces stay per cell (first voice).

### Offline render

`songrender/arrangement.go` emits one `NoteEvent` per layer (pitch = node pitch +
layer semis, volume × layer gain), so `audio.RenderMasterMixOffline` and the
export→import identity tests hear chords.

## §3 UI

### Type cycle

- `game_node_sidebar.go` "aud" button and the JS cycling switches:
  **Regular → Chord → Silent → Mute → Regular**, skipping Chord on unpitched
  instruments. Reuses `emitNodeTypeChanged` (no new `hooks.Kind`, so
  `NumNonVerbose` is untouched).
- Entering Chord with an empty spec seeds `{Quality: "maj"}`; with a dormant spec,
  restores it.

### Sidebar on a chord node

```
Pitch   [Cmaj7/E ▾]  [−][ 0 ][+]     ← chip opens the CHORD matrix; ± transposes the root
Chord   [      Cmaj7/E       ]        ← chord chip: opens the chord matrix too
        Inv    [−][ 1st ][+]
        Spread [−][close][+]
        [Single note]                 ← collapse to Regular (spec kept)
```

Usability fixes (2026-10-10, found by driving the card the way a user does —
sections collapsed, nothing pre-scrolled; guards in
`node_sidebar_chord_usability_test.go`):
- The section headers come from ONE list (`nodeSidebarSections`); the Chord
  header was missing from the two hand-kept copies, so it never toggled.
- The Chord section leads with its own chip (`chname`): the Pitch chip alone hid
  the matrix inside a separately collapsed section.
- The type button keeps itself under the pointer (`keepUnderPointer`: the card
  holds its position, the content scrolls by however far the button moved) —
  Regular → Chord used to push it off the card's bottom.
- A picker cell that confirms (re-select) closes the picker mid-gesture; the
  release is still the sidebar's (`PressOwner`), not a tap outside the card.
- On a phone the pickers start below the top-right button row, and the
  outside-tap rule uses `NodeSidebar.Hit` (card or picker), not the bounding
  box of the two — so the strip beside those buttons is a real "outside".
- A page switch re-seats the keyboard cursor on the current chord, or clears it.

- The Pitch row's chip reads `ChordName(...)` and opens the chord matrix instead
  of the note matrix (on Regular it still opens the note matrix).
- New "chord" section built from the existing `layoutSection` stepper rows;
  `buttonOrder` / `wireButtons` entries; `sectionBadgeText` shows the chord name
  when collapsed.
- **Single note**: sets type Regular via the same path as the cycle; spec kept.
- Every step applies via `SetNodeParams` → `notifyPredictorNode` →
  `emitNodeParamsChanged` — one undo step, one log line, per press.
- The chord section exists only on Chord nodes (Regular / Silent / Mute never
  show it; Silent / Mute also hide the Pitch row, as today).

### Generic grid picker (targeted refactor)

`NoteMatrixComponent` hard-codes `noteMatrixModel`. Extract:

```go
type gridPickerModel interface {
    Cols() int; Rows() int
    RowLabel(row int) string; ColLabel(col int) string
    CellLabel(col, row int) string
    CellFor() (col, row int, ok bool) // current selection
    Pick(col, row int)                 // apply (selecting applies, re-selecting confirms)
}
```

- `NoteMatrixComponent` becomes `GridPickerComponent` + an optional header
  `SegmentedControl` (pages). The note matrix is one instance; its existing tests
  (real input, pixel highlight, keyboard, scroll-vs-pick) prove the extraction.
- `chordMatrixModel` is the second instance.

### Chord matrix

```
┌ Chords · [Triads] 7ths  Ext  Sus       × ┐
│      maj    m     °     +     5          │
│ C   [ C ] [Cm ] [C° ] [C+ ] [C5 ]        │
│ C#  [C# ] [C#m] ▓C#°▓ [C#+] [C#5]  ← current (tint)
│ D   [ D ] [Dm ] [D° ] [D+ ] [D5 ]        │
│ …  rows scroll, touch-min cells          │
└──────────────────────────────────────────┘
```

- 12 root rows (gutter C…B, instrument-offset aware like the note matrix) × 5
  quality columns per page; geometry is `solveSwatchGrid` unchanged, so 5 columns
  fit a 390 px phone at touch-min. Rows scroll; pick on RELEASE (scroll slop wins).
- **Root mapping**: picking row *pc* sets `Pitch` to that pitch class **in the
  current root's octave**, clamped to ±24 — the voicing never jumps two octaves.
  Picking also sets the quality and re-clamps the inversion; spread is kept.
- Opens on the current quality's page, current root row scrolled into view,
  keyboard cursor on the current chord.
- **Selecting applies, re-selecting confirms** (positional, no timers) — audition
  several chords without reopening.
- Keyboard: `MenuKeyNav` grid mode (arrows move, Enter picks); **Tab / Shift+Tab**
  switch pages. `ClaimsKeyboard()` so arrows never pan the camera while open
  (paired tests: no pan while open, pans when closed). Esc / outside tap / × close
  as every menu does (`TestEveryMenuClosesTheSameWay` row added).
- Hosted exactly like the note matrix: driven by the node sidebar on `GridTree`
  (not the portal), clamped to the grid pane, first refusal in
  `NodeSidebar.HandleInput` with the `overPicker` guard.

### Rendering

- **Grid node**: new `IconChord` (three stacked bars, icon enum + `DrawIcon`,
  no text glyph) in `drawNodeStateOverlay`, theme-token colour (no literal; theme
  drift guards apply).
- **Coordinate badge** (`coordBadgeText`): `Cmaj7/E · (0:1/2)`.
- **Drum-view row cell**: a stacked-tick mark on chord cells. Requires a chord
  cell-type value through `preview.BuildRowWindow` → `CellTypes` → row sprite, and
  in the played-tier byte so a chord stays a chord in the frozen past.
- **Event log**: the node-params formatter renders the chord (`[node] chord C#m7/E`);
  `nodeTypeName` gains `chord`.

## Testing

Pure / model (`core/model`):
- Quality table: 20 entries, 4 pages × 5, unique ids, ascending intervals.
- `NodeLayers`: single-layer identity for non-chord types; every quality ×
  inversion × spread against hand-computed expectations; gain = 1/√N.
- `ChordName`: slash names, sharps, every quality suffix.
- `SetNodeParams` copies `Chord`.

Predictor (`core/engine`): a circuit with a Chord node yields decisions identical
to the same circuit with that node Regular (`Fires()` migration is behaviour-neutral).

Lane (`internal/ui`):
- `audio_lane_test.go`: a chord cell commits N vids; an edit replaces all N; an
  engine refusal of any old vid drops all replacements (no doubling); seek/cancel
  iterate every vid.
- `TestLaneKeysCoverEveryEditClass`: chord quality / inversion / spread / type
  change each move a key.
- `TestLaneHeardEqualsLegacyDispatched`: seeded edits include chord nodes.
- Legacy path: `playFn` called N times with the expected pitches/vols.
- `cellVoices` single-layer case equals today's `evalNodeParamsOnly` output.

UI, real input through `Game.Update`:
- Type cycle order, Chord skipped on an unpitched instrument, spec kept across
  Chord → Silent → Mute → Regular → Chord, Single note collapses and keeps the spec.
- Chip opens the chord matrix on a chord node, the note matrix on a regular node.
- Page switch (tap + Tab), pick applies + stays open, re-pick closes, root keeps
  octave, inversion re-clamped, scroll gesture never picks, Esc / outside tap / ×.
- Keyboard: arrows move the cursor and do not pan; pan works when closed.
- Steppers: inversion / spread ranges, one undo step per press.
- Note matrix regression tests stay green after the `GridPickerComponent` extraction.

Format / undo:
- Export → import round-trip of type + spec (including a dormant spec on a Regular
  node); unknown quality / out-of-range fields rejected before reset; undo / redo
  restores a chord edit.

Audio identity (`make test-real`, `!test`):
- `TestExportImportMasterMixIsByteIdentical`'s headline circuit gains a chord node;
  `TestHeadlineEditsAreAudible` gains "change the chord quality" (an identity test
  is only as good as the proof its edits are audible).

Visual (CLAUDE.md mandatory loop, desktop + mobile, PNGs opened):
- Scenes `node_chord_sidebar`, `chord_matrix`, `crop_chord_matrix`,
  `crop_chord_node` (glyph + badge + drum-view tick).

## Rollout order (for the plan)

1. Model: `NodeTypeChord`, `Fires()`, name table, `ChordSpec`, quality table,
   `NodeLayers`, `ChordName`, `SetNodeParams` (+ tests).
2. Format: export / import / validation / undo round-trip.
3. Fan-out: `cellVoices`, legacy dispatch, lane vids + atomic replace, sink,
   prewarm, songrender (+ lane and oracle tests).
4. Sidebar: type cycle, chord section, Single note.
5. `GridPickerComponent` extraction (note matrix tests green), then the chord matrix.
6. Rendering: icon, badge, drum-view cell type, event log.
7. Scenes + screenshots; `make test-real` identity.
