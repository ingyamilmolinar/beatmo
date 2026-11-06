# PlaybackSlate Extraction (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract Game's scheduling/beat-path/row-refresh/parity engine into a standalone `PlaybackSlate` collaborator with a narrow `SlateHost` interface, reducing Game's method surface and making the scheduling core independently testable — with zero behavior change.

**Architecture:** A 3-rung safety ladder. (1) Group the slate-owned fields into a value-embedded `playbackSlate` struct (field promotion → zero call-site churn). (2) Relocate the three method families onto `*playbackSlate` receivers using a verbatim `g := s.g` prologue so bodies are unchanged, leaving one-line forwarding shims on `*Game`. (3) Replace the `*Game` back-reference with a narrow `SlateHost` interface, add Game-side accessors for fields the slate reads through the host, and add unit tests that construct `PlaybackSlate` against a fake host. Every rung compiles and passes the full suite before the next.

**Tech Stack:** Go 1.23 (bundled at `.tools/go/bin/go`); Ebiten (stubbed under `-tags test`); the existing `internal/ui` + `internal/timeline` test suites; `make test-parity` cross-platform golden.

## Global Constraints

*Every task's requirements implicitly include this section. Copied verbatim from the design spec (`docs/superpowers/specs/2026-07-09-internal-ui-decomposition-design.md` §6).*

- **Use the bundled Go only:** `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`. Never system `go`.
- **Fast gate command (run from `src/go`):** `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline`
- **seqMu discipline:** exactly one `seqMu` critical section per frame in `Game.Update`; deferred `pendingImportData`/`pendingActions`/`pendingNotifs` draining stays AFTER `seqMu.Unlock()`; the sequencer side (`seqScheduleTime`) uses `TryLock` and skips the tick if held; `mutateStructural` (`game_mutation_gate.go:59`) stays the single structural-mutation choke-point. Lock order is `seqMu → predictor lock`.
- **Atomics stay atomic & lock-free from the audio thread:** `audioGen`, `seqPathSnap`. Never guard them with a mutex.
- **updateBeatInfos invalidate→SetPaths→RebaseAt stays ONE atomic critical section** (`game_graph_update_beat_infos.go:444-524`).
- **`Draw` / `DrumView.Draw` stay dispatch-only** (AST-guarded: `TestRenderPipelineDiscipline`, `TestGridTreeDrawHasSingleDispatchSite`). This phase does not touch draw dispatch.
- **copylocks:** a struct containing a `sync.Mutex`/atomics must NEVER be copied. Value-embed it and initialize its fields with statements after construction — never with a composite literal. `go vet` must stay clean.
- **DO NOT touch** `core/engine/Predictor.Ensure` or the 6 existing field-bag embeds (`gridRenderCache`/`parityTracker`/`inputGestureState`/`rowSpriteCache`/`rowsLayerCache`/`rowsWinCache`) — extend, don't re-extract.
- **No commits by the agent unless the human explicitly asks.** The repo convention is to leave work uncommitted (`feedback_never_stage_or_commit`). Where a step below says "Commit", treat it as a *checkpoint gate* — run the gate, confirm green, and PAUSE for human review; only `git commit` if the human directs it. Each "Commit" step's git command is shown for completeness but is gated on that instruction.

### Coordination hazard (read before starting)

`updateBeatInfos` (`game_graph_update_beat_infos.go`) is the subject of separate in-flight work — the "circuit-edit audio disruption" fix targets `updateBeatInfos:446-491` (removing `audio.Stop`, scoping resets per-row+future). **Before starting Task 3, check `git log`/working tree for changes to that file and rebase this task onto the current body.** Task 3 relocates the method verbatim, so it composes with any body edit, but the line refs below may shift.

---

## File Structure

**New files:**
- `internal/ui/playback_slate.go` — the `playbackSlate` struct definition + the `SlateHost` interface (Rung 3). One responsibility: the field cluster + host contract.
- `internal/ui/playback_slate_host.go` — `*Game`'s implementation of `SlateHost` (accessor methods) + the fake host used by tests (Rung 3). *(Fake host may instead live in `playback_slate_test.go` — see Task 5.)*
- `internal/ui/playback_slate_test.go` — unit tests constructing `PlaybackSlate` against the fake host (Rung 3 payoff).

**Modified files (relocations — the method bodies move OUT of these onto `*playbackSlate`, leaving shims):**
- `game_struct.go` — remove the slate-owned field declarations; add the embedded `playbackSlate`.
- `game_refresh_drum_row.go`, `game_row_window_builder.go`, `game_row_helpers.go`, `game_timeline.go` — Task 2 family.
- `game_graph_update_beat_infos.go`, `game_graph_beat_index.go` — Task 3 family.
- `game_sequencer_schedule.go`, `game_audio_schedule.go`, `game_sequencer_highlight.go`, `game_trigger_logic.go`, `game_tick_highlight.go`, `game_trigger_cache.go` — Task 4 family.
- `game_new.go` — wire the slate's back-ref / host and statement-init the moved atomic/channel fields.

**Method families (verified inventory — these are the shims to create):**
- **Row-refresh family (Task 2):** `refreshDrumRow`, `buildRowWindow`, `rowIndexForNode`, `resetLogicStateForRow`, `clampRowFreeze`, `invalidateReconciledRow`, `invalidateReconciledAll`, plus the `timeline*` accessors in `game_timeline.go` (`timelineWindowLen`, `timelineService`, `recordTimelineCommit`, `recordTimelineCommitKind`, `timelineTrimAfter`, `timelineTrimAfterMutable`, `timelineClearRow`, `timelineCommitted`, `timelineCommittedWithKind`, `timelineReplaceCommit`, `TimelineSegments`, `rowWindow`, `timelineCommittedRange`).
- **Beat-path family (Task 3):** `updateBeatInfos`, `beatInfoAtRow`, `wrapBeatIndexRow`, `rowIsAudible`, `playheadFloor`, `globalPastExclusive`, `rowPastExclusive`.
- **Scheduling family (Task 4):** `seqScheduleTime`, `queueSoundParams`, `rowStructGenLoad`, `bumpRowStructGen`, `queueSoundAtParams`, `audioChNearFull`, `runtimeAudioLookahead`, `scheduleSound`, `expectedHighlightSeconds`, `applySequencerHighlight`, `incrementLogicTriggerCount`, `seqPrevTriggered`, `seqShouldTriggerNode`, `evalNodeParamsOnly`, `nodeTriggered`, `nodeTriggeredState`, `deterministicRoll`, `onTick`, `highlightBeat`, `setLastTriggered`, `cacheNode`, `removeNodeCache`, `rebuildNodeCache`, `nodeSnapshot`, `lastTriggered`, `setLastTriggeredForTest`, `lastTriggeredForTest`.

---

## The verbatim-relocation recipe (used in Tasks 2–4)

This is the mechanical move applied to every method in a family. It keeps the body byte-for-byte identical.

**Before** (in e.g. `game_refresh_drum_row.go`):
```go
func (g *Game) refreshDrumRow() {
    // ... 422 lines referencing g.field, g.method() ...
}
```

**After** — body unchanged, receiver swapped, one prologue line, plus a shim:
```go
// relocated onto *playbackSlate; body verbatim
func (s *playbackSlate) refreshDrumRow() {
    g := s.g // back-ref; keeps the body's g.field / g.method() references valid via field promotion (Rung 2)
    // ... 422 lines UNCHANGED ...
}

// shim retained on *Game so all call sites and tests are unchanged
func (g *Game) refreshDrumRow() { g.playbackSlate.refreshDrumRow() }
```

Because `playbackSlate` is value-embedded in `Game` and every slate-owned field is *promoted*, `g.nextBeatIdxs` etc. still resolve from within the relocated body. `s.g` is the back-reference set once in `New`. At Rung 2 the bodies do not change; only the receiver and the added `g := s.g` line. Rung 3 (Task 5) is where `s.g` becomes `s.host` and the churn is concentrated.

> **Exception — same-family calls:** when method A (relocated) calls method B (also relocated in the same task), the shim on `*Game` still makes `g.B()` resolve, so A's body needs no edit. Do not "optimize" intra-slate calls to `s.B()` during Rung 2 — that adds churn and defeats the verbatim guarantee. Leave it for an optional Rung 3 cleanup.

---

### Task 0: Establish the green baseline + metric snapshot

**Files:** none (measurement only).

- [ ] **Step 1: Confirm the suite is green before touching anything**

Run (from `src/go`):
```
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
```
Expected: PASS. If anything is red, STOP — per `feedback_no_preexisting_failure_baselines`, a red test is a real regression to resolve first, never a baseline to accept.

- [ ] **Step 2: Record the baseline metrics** (for the success-criteria diff at the end)

Run (from `src/go`):
```
echo "Game methods: $(grep -rhcE '^func \(g \*Game\)' internal/ui/*.go | paste -sd+ | bc)"
echo "game_struct.go LOC: $(wc -l < internal/ui/game_struct.go)"
```
Record both numbers in the task's notes. (Baseline expectation from the spec: Game ≈ 342 methods, `game_struct.go` = 418 LOC.)

- [ ] **Step 3: Confirm `go vet` copylocks is clean** (so a later regression is attributable to this work)

Run (from `src/go`):
```
../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui
```
Expected: no output (clean).

---

### Task 1: Rung 1 — value-embed the `playbackSlate` field cluster

**Files:**
- Create: `internal/ui/playback_slate.go`
- Modify: `internal/ui/game_struct.go` (remove the slate-owned field lines; add the embed), `internal/ui/game_new.go` (statement-init the moved atomic/channel/map fields)

**Interfaces:**
- Produces: `type playbackSlate struct { ... }` value-embedded in `Game`; a `g *Game` field on it (set in `New`, used from Task 2 onward). All moved fields are promoted, so `g.<field>` continues to resolve everywhere.

**Field cluster to move** (from `game_struct.go`, verified line refs — move these declarations into `playbackSlate`, EXCLUDING the coarse `seqMu` which stays on `Game` as the cross-cutting spine):
`nextBeatIdxs`, `seqNextIdxs`, `nextIdxSticky`, `elapsedBeats`, `startNextBuf`, `lastHLIdxByRow` · `beatInfos`, `drumBeatInfos`, `beatInfosByRow`, `isLoop`, `loopStartIndex`, `isLoopByRow`, `loopStartByRow`, `loopLenByRow`, `originIdxsByRow`, `nextOriginIdxByRow` · `seqCh`, `seqQuit`, `seqPathSnap` (atomic), `hlCh` (chan) · `timeline`, `frozenUpToByRow`, `reconciledUpToByRow`, `freezeBackfillIters` · `pathSigByRow`, `rowsPathChanged`, `pathChangeBeatByRow`, `pathsDirty`, `rowRenderSig` · `rowStepsScratch`, `rowTypesScratch`, `rowSnapshotMode`, `lastStepsOffset`, `lastCellTypesOffset` · `audioGen` (atomic) · `nodeTriggerCountsByRow`, `lastEvalIdxByRowNode`, `nodeLogicTriggerCountsByRow`, `lastTriggeredByRow`, `lastFiredNodeByRow`, `muteUntilByRow`, `loopCountByRow`.

> Keep on `Game` for now (cross-cutting, resolved in Rung 3): `seqMu`, `triggerMu`, `engine`, `graph`, `drum`, `nodeCache`, `nodeCacheMu`, `bpm`, `prevBPM`, `state`.

- [ ] **Step 1: Create `playback_slate.go` with the struct (fields only, no methods yet)**

Copy each listed field's declaration verbatim (with its doc comment) out of `game_struct.go` into:
```go
package ui

// playbackSlate owns the row-indexed scheduling, beat-path, timeline/freeze,
// and trigger-logic state that Game.Update, the background sequencer, and the
// drum-row refresh share. Value-embedded in Game (see game_struct.go) so every
// field stays promoted (g.nextBeatIdxs, …) — zero call-site churn. Methods are
// relocated onto *playbackSlate in Tasks 2–4; the host interface arrives in Task 5.
//
// Contains atomics (seqPathSnap, audioGen) → NEVER construct a playbackSlate{}
// composite literal and never copy it (copylocks). Value-embed + statement-init.
type playbackSlate struct {
    g *Game // back-reference, set once in New (replaced by a SlateHost interface in Task 5)

    // <-- paste the field declarations here, verbatim from game_struct.go -->
}
```

- [ ] **Step 2: Remove those field declarations from `game_struct.go` and add the embed**

Delete the moved lines. Add, in the sequencer region of the struct (near the old `seqMu`):
```go
    // playbackSlate owns the scheduling/beat-path/timeline/trigger field cluster
    // (see playback_slate.go). Anonymous value-embed so g.nextBeatIdxs / g.timeline /
    // g.beatInfosByRow / … keep resolving via field promotion.
    playbackSlate
```

- [ ] **Step 3: Statement-init the atomic/channel/map fields in `New`**

In `game_new.go`, wherever these were previously set via the `&Game{...}` composite literal, move them to statement form AFTER construction (composite literals cannot key promoted fields, and the atomics must not be copied). Set the back-ref here too:
```go
    g.playbackSlate.g = g
    // any prior literal inits for hlCh / seqPathSnap / the trigger maps / timeline
    // become statements: g.hlCh = make(chan struct{ ... }, N); g.timeline = ...
```
Search the codebase for `&Game{` and confirm none of the moved fields are keyed there; convert any that are.

- [ ] **Step 4: Build all three targets**

Run (from `src/go`):
```
../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui
../../.tools/go/bin/go build ./internal/ui
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui
```
Expected: all succeed. (The `js` build catches promoted-field references in `js_exports_*.go`.)

- [ ] **Step 5: `go vet` copylocks + full gate**

Run (from `src/go`):
```
../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
```
Expected: vet clean; tests PASS. If copylocks fires, a `playbackSlate` is being copied — find the `playbackSlate{...}` literal or by-value pass and convert to statement-init / pointer.

- [ ] **Step 6: Checkpoint** (gate green → pause for review; commit only if human directs)

```bash
git add internal/ui/playback_slate.go internal/ui/game_struct.go internal/ui/game_new.go
git commit -m "refactor(ui): group scheduling field cluster into embedded playbackSlate"
```

---

### Task 2: Rung 2 — relocate the row-refresh family onto `*playbackSlate`

**Files:**
- Modify: `internal/ui/game_refresh_drum_row.go`, `internal/ui/game_row_window_builder.go`, `internal/ui/game_row_helpers.go`, `internal/ui/game_timeline.go`

**Interfaces:**
- Consumes: `playbackSlate` (Task 1), its `g *Game` back-ref.
- Produces: the row-refresh methods on `*playbackSlate`; identical `*Game` shims retained for every one.

Apply the verbatim-relocation recipe to every method in the row-refresh family (listed in File Structure). Do them one file at a time; gate after each file.

- [ ] **Step 1: Relocate `game_refresh_drum_row.go` (`refreshDrumRow`)**

Change the receiver to `func (s *playbackSlate) refreshDrumRow()`, add `g := s.g` as the first body line, leave the remaining ~422 lines unchanged. Append the shim `func (g *Game) refreshDrumRow() { g.playbackSlate.refreshDrumRow() }` (place shims together in the same file).

- [ ] **Step 2: Gate**

Run (from `src/go`): `../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui` then the fast gate command. Expected: PASS.

- [ ] **Step 3: Relocate `game_row_window_builder.go` (`buildRowWindow`) + gate** — same recipe, then rerun build + fast gate. Expected: PASS.

- [ ] **Step 4: Relocate `game_row_helpers.go` (`rowIndexForNode`, `resetLogicStateForRow`, `clampRowFreeze`, `invalidateReconciledRow`, `invalidateReconciledAll`) + gate.** Expected: PASS.

- [ ] **Step 5: Relocate `game_timeline.go` (all 13 `timeline*`/`rowWindow`/`TimelineSegments`/`timelineCommittedRange` methods) + gate.**

Note: `TimelineSegments` and `rowWindow` are exported/public — the shim MUST keep the exported name on `*Game` (JS exports and tests call `g.TimelineSegments`). Expected: PASS.

- [ ] **Step 6: Full gate + js build**

Run (from `src/go`):
```
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
```
Expected: PASS.

- [ ] **Step 7: Checkpoint** (gate green → pause for review)

```bash
git add internal/ui/game_refresh_drum_row.go internal/ui/game_row_window_builder.go internal/ui/game_row_helpers.go internal/ui/game_timeline.go
git commit -m "refactor(ui): relocate row-refresh family onto playbackSlate (shims retained)"
```

---

### Task 3: Rung 2 — relocate the beat-path family onto `*playbackSlate`

> **Read the Coordination hazard note first** — reconcile with any in-flight `updateBeatInfos` edit before starting.

**Files:**
- Modify: `internal/ui/game_graph_update_beat_infos.go`, `internal/ui/game_graph_beat_index.go`

**Interfaces:**
- Consumes: `playbackSlate` + its `g` back-ref.
- Produces: `updateBeatInfos`, `beatInfoAtRow`, `wrapBeatIndexRow`, `rowIsAudible`, `playheadFloor`, `globalPastExclusive`, `rowPastExclusive` on `*playbackSlate`; `*Game` shims retained.

- [ ] **Step 1: Relocate `game_graph_beat_index.go` (the 6 helper methods) + gate.** Verbatim recipe. Run build + fast gate. Expected: PASS.

- [ ] **Step 2: Relocate `updateBeatInfos` in `game_graph_update_beat_infos.go`**

Verbatim recipe (`func (s *playbackSlate) updateBeatInfos()`, `g := s.g` prologue). **Do not restructure the single seqMu critical section** (`~:444-524`) — it moves as-is inside the verbatim body. Add the `*Game` shim.

- [ ] **Step 3: Gate — including the parity guard**

Run (from `src/go`):
```
../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
```
Expected: PASS. The seqMu/parity tests are the specific gate here.

- [ ] **Step 4: `-race` on the concurrency tests** (updateBeatInfos publishes the path snapshot the sequencer reads)

Run (from `src/go`):
```
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race -run 'Parity|SeqPath|Snapshot' ./internal/ui
```
Expected: PASS, no race reports. (If the `-run` filter matches nothing, widen it; the goal is to exercise the parityGen/seqPathSnap concurrency.)

- [ ] **Step 5: Checkpoint** (gate green → pause for review)

```bash
git add internal/ui/game_graph_update_beat_infos.go internal/ui/game_graph_beat_index.go
git commit -m "refactor(ui): relocate beat-path family onto playbackSlate (shims retained)"
```

---

### Task 4: Rung 2 — relocate the scheduling family onto `*playbackSlate`

**Files:**
- Modify: `internal/ui/game_sequencer_schedule.go`, `internal/ui/game_audio_schedule.go`, `internal/ui/game_sequencer_highlight.go`, `internal/ui/game_trigger_logic.go`, `internal/ui/game_tick_highlight.go`, `internal/ui/game_trigger_cache.go`

**Interfaces:**
- Consumes: `playbackSlate` + its `g` back-ref.
- Produces: the 27 scheduling-family methods (listed in File Structure) on `*playbackSlate`; `*Game` shims retained. `seqScheduleTime` runs on the background sequencer goroutine — its `seqMu.TryLock` (`game_sequencer_schedule.go:18`) moves verbatim; `s.g.seqMu` is the same lock (seqMu stayed on Game).

- [ ] **Step 1: Relocate the leaf helpers first** — `game_trigger_cache.go`, `game_trigger_logic.go`, `game_audio_schedule.go`, `game_sequencer_highlight.go`, `game_tick_highlight.go` (verbatim recipe, shims retained). Preserve exported/test-only names on the shim (`setLastTriggeredForTest`, `lastTriggeredForTest`). Gate build + fast test after each file. Expected: PASS.

- [ ] **Step 2: Relocate `seqScheduleTime` in `game_sequencer_schedule.go`**

Verbatim recipe. The `TryLock`-skip-if-held preamble and the burst=32 catch-up loop move unchanged. Add the `*Game` shim.

- [ ] **Step 3: Gate + js build + `-race`**

Run (from `src/go`):
```
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race -run 'Sched|Parity|Seq|Highlight' ./internal/ui
```
Expected: PASS, no races.

- [ ] **Step 4: Real-Ebiten + cross-platform parity gate** (the definitive behavioral guard on the scheduling core)

Run (from `src/go`):
```
xvfb-run -a ../../.tools/go/bin/go test -run 'Mixer|Schedul' ./internal/ui ./internal/audio/...
```
Then from repo root:
```
make test-parity
```
Expected: PASS. `make test-parity` proves the WASM-compiled scheduler still produces byte-identical output to native — the strongest signal that the relocation changed nothing.

- [ ] **Step 5: Checkpoint** (gate green → pause for review)

```bash
git add internal/ui/game_sequencer_schedule.go internal/ui/game_audio_schedule.go internal/ui/game_sequencer_highlight.go internal/ui/game_trigger_logic.go internal/ui/game_tick_highlight.go internal/ui/game_trigger_cache.go
git commit -m "refactor(ui): relocate scheduling family onto playbackSlate (shims retained)"
```

---

### Task 5: Rung 3 — introduce `SlateHost`, cut the back-ref, add fake-host unit tests

This is the payoff and the largest design step: the slate stops depending on `*Game` and depends on a narrow interface, making it independently testable.

**Files:**
- Modify: `internal/ui/playback_slate.go` (add `SlateHost`; change `g *Game` → `host SlateHost`)
- Create: `internal/ui/playback_slate_host.go` (`*Game` implements `SlateHost` via accessor methods)
- Create: `internal/ui/playback_slate_test.go` (fake host + unit tests)

**Interfaces:**
- Consumes: everything from Tasks 1–4.
- Produces:
```go
type SlateHost interface {
    Predictor() *engine.Predictor            // was g.engine.Predictor()
    Graph() *model.Graph                      // was g.graph
    Rows() []*DrumRow                         // was g.drum.Rows
    NodeSnapshotFor(id model.NodeID) (model.Node, bool) // was g.nodeCache read under nodeCacheMu
    BPM() int                                 // was g.bpm
    SeqMu() *sync.Mutex                        // the coarse spine lock (stays Game-owned)
    EnqueueHighlight(row, idx int, info model.BeatInfo) // hlCh producer
    PlaySound(req soundReq)                    // audioCh / scheduleSound sink
    MutateStructural(fn func(), holdSeqMu bool) // choke-point (game_mutation_gate.go)
    // extend ONLY as the compiler proves a method needs it — keep this minimal
}
```
- `func NewPlaybackSlate(host SlateHost) *playbackSlate` constructor.

- [ ] **Step 1: Enumerate the exact host surface**

For the relocated bodies, grep each `s.g.<X>` reference (they are the only coupling to Game now):
```
cd /home/ymolinar/Repos/beatmo/src/go && grep -noE 's\.g\.[A-Za-z_]+' internal/ui/game_*.go internal/ui/playback_slate.go | sort -u
```
This produces the definitive list. Every distinct `s.g.<X>` must map to exactly one `SlateHost` method (or an accessor of identical name). Record the list; it defines the interface precisely (the sketch above is the expected shape — reconcile against reality).

- [ ] **Step 2: Write the failing test first (TDD for the new surface)**

Create `playback_slate_test.go` with a `fakeSlateHost` implementing `SlateHost` (in-memory rows/graph/predictor stub) and a test that exercises a pure slate operation without a full Game — start with the simplest deterministic one, e.g. `deterministicRoll` or `wrapBeatIndexRow`:
```go
func TestPlaybackSlate_WrapBeatIndexRow_FakeHost(t *testing.T) {
    h := newFakeSlateHost(/* 1 row, known path len */)
    s := NewPlaybackSlate(h)
    // seed the minimal slate state the method reads (beatInfosByRow / loopLenByRow)
    got := s.wrapBeatIndexRow(0, /* idx past loop end */)
    if got != /* expected wrapped idx */ {
        t.Fatalf("wrapBeatIndexRow = %d, want %d", got, /* expected */)
    }
}
```
Pick the concrete method + expected value by reading the current `wrapBeatIndexRow` body (`game_graph_beat_index.go:32`) so the assertion is exact.

- [ ] **Step 3: Run it — expect a COMPILE failure** (`NewPlaybackSlate`/`SlateHost` don't exist yet)

Run (from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPlaybackSlate ./internal/ui`
Expected: FAIL to compile (`undefined: NewPlaybackSlate`).

- [ ] **Step 4: Add `SlateHost` + `NewPlaybackSlate`; change the back-ref**

In `playback_slate.go`: replace the `g *Game` field with `host SlateHost`; add `func NewPlaybackSlate(host SlateHost) *playbackSlate { return &playbackSlate{host: host} }`. Replace every `g := s.g` prologue with `h := s.host` and mechanically rewrite the body's `g.<X>` Game-couplings to `h.<X>()` per the Step-1 map. (Slate-owned fields stay `s.field` / promoted — only the Game-couplings change.) This is the concentrated churn the shims deferred.

- [ ] **Step 5: Make `*Game` implement `SlateHost`**

In `playback_slate_host.go`, add the accessor methods on `*Game` (`func (g *Game) Predictor() *engine.Predictor { return g.engine.Predictor() }`, `func (g *Game) SeqMu() *sync.Mutex { return &g.seqMu }`, `EnqueueHighlight` wrapping the `hlCh` send, etc.). In `game_new.go` set `g.playbackSlate = *NewPlaybackSlate(g)` via statement-init (or set `g.playbackSlate.host = g`). The `*Game` shims from Tasks 2–4 stay unchanged (`g.refreshDrumRow()` → `g.playbackSlate.refreshDrumRow()`).

- [ ] **Step 6: Run the new test — expect PASS**

Run (from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPlaybackSlate ./internal/ui`
Expected: PASS.

- [ ] **Step 7: Full gate + vet + js build + parity**

Run (from `src/go`):
```
../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline
```
Then from repo root: `make test-parity`
Expected: all PASS.

- [ ] **Step 8: Add 2–3 more fake-host unit tests** covering a scheduling decision (`seqShouldTriggerNode` / `nodeTriggeredState`) and a freeze/reconcile path — the operations that were previously untestable without a full Game. These are the concrete evidence that the extraction delivered testability. Gate green.

- [ ] **Step 9: Metric diff + checkpoint** (gate green → pause for review)

Re-run the Task 0 Step 2 measurement. Confirm Game method count dropped by ~60 (the scheduling family now lives on `*playbackSlate`; shims count toward Game but the bodies don't) and record the delta. Then:
```bash
git add internal/ui/playback_slate.go internal/ui/playback_slate_host.go internal/ui/playback_slate_test.go internal/ui/game_new.go internal/ui/game_*.go
git commit -m "refactor(ui): PlaybackSlate depends on narrow SlateHost; add fake-host unit tests"
```

---

## Self-Review

**Spec coverage** (against design spec §5 Phase 1):
- Field cluster extraction → Task 1. ✔
- Relocate the four method families (refresh, beat-path, scheduling; parity kept as the existing embedded `parityTracker` per spec open-question #2 leaning) → Tasks 2–4. ✔ *Note: parity methods (`parityScan` et al.) are intentionally NOT relocated in this plan — the spec's leaning is to keep `parityTracker` embedded and have the slate emit to it; that wiring is folded into the `SlateHost` surface (a parity-emit hook) in Task 5 rather than a separate task, since the parity fields already live in their own embed.*
- Narrow `SlateHost` interface + fake-host tests → Task 5. ✔
- Migration order (refresh → beat-path → scheduling, scheduling last as highest-hazard) → Tasks 2→3→4 matches spec §5 exactly. ✔
- Verification gates (fast suite, `-race` parityGen, `xvfb` Mixer, `make test-parity`) → present in Tasks 3, 4, 5. ✔
- Invariants (seqMu single region, TryLock, atomic lock-free, copylocks, single critical section) → Global Constraints + called out at each relocation. ✔

**Placeholder scan:** The relocation-body steps say "verbatim, unchanged" rather than reprinting 1900 LOC — this is intentional and complete for a mechanical move (the recipe block shows the exact transform). The one genuinely-new-code step (Task 5 Step 2 test) uses `/* … */` markers where the implementer must read a specific cited line to get the exact value — these are *instructions to compute an exact value from a cited source*, not open-ended TODOs. Acceptable for a refactor plan; flagged so the executor knows to fill them from the named source.

**Type consistency:** `playbackSlate` (lowercase, unexported) throughout; embedded as `playbackSlate` (promotes as `g.playbackSlate.<method>`). `SlateHost` / `NewPlaybackSlate` consistent between the File Structure, Task 5 interface block, and steps. Shim signatures match the verified inventory names exactly (exported names `TimelineSegments`/`rowWindow` preserved).

**Gap accepted:** the exact `SlateHost` method set is finalized empirically in Task 5 Step 1 (grep of `s.g.<X>`), not guessed — the sketch is labeled as the expected shape to reconcile. This is deliberate: the interface is *discovered* from the relocated bodies, which is more reliable than pre-declaring it.
