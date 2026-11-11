# Parity checker — read the sources of truth, stop shadowing them

Date: 2026-10-04 · Status: PROPOSED (the four defects below are FIXED in the tree; this plan is the follow-up)

## What happened (the 2026-10-04 log)

Three distinct things printed `[PARITY]` / `[parity]` lines in one session on the default circuit:

| Symptom | Root cause | Fixed by | Guard |
|---|---|---|---|
| `[parity] [highlight_vs_audio] … src=refresh` storm after every **rewind / timeline go-to** while playing | `SeekSteps` wiped the highlights (`resetHighlights`) but not the parity **audio-event log**; every cell between a backward seek's target and the old playhead read "audio fired, no highlight" until the playhead re-passed it | `parityOnSeek` — a seek advances `parityGen`, the generation every parity record is already filtered by (also retires in-flight `audioCh` events, which carry their schedule-time generation) | `TestSeekWhilePlayingLeavesNoStaleParity` (rewind / go-to back / FF / go-to forward × lane / legacy, real request queue + `Game.Update`) |
| (latent, same trigger + any edit) `timeline_vs_pred` + `timeline_vs_pred_immutable` + `seq_vs_view`, **and the drawn past froze the abandoned pattern** | `timeline.Service.RewindTo` moved the abandoned pass into the scroll-back **archive**, and `valueLocked` falls back to the archive — so not-yet-replayed cells answered with the abandoned pass's Playback commit. `preview.CrossingCell` prefers a Playback commit over the predictor, so the played snapshot froze it (3/39 replayed cells wrong in the test) | `RewindTo` moves the abandoned pass (hot sidecar **and** already-archived entries ≥ target) to a separate `superseded` store — retained, never consulted by the live lookup | `TestRewindToSupersededPassIsNotLiveHistory` (timeline), `TestBackwardSeekAfterEditDoesNotCompareAbandonedPass` (ui, incl. the played-snapshot assertion) |
| `audio_missing` for **abs 0** at every play start (lane mode) | The UI lane read the audio lane's ledger before the lane's first tick (`laneTick` is deferred to the END of the pass); the note *was* committed and played. Same miss skipped the first cell's trigger light-up | `observeLaneCell` / `observePendingLaneCells`: cells the lane hadn't walked yet are observed after the pass's `laneTick` | `TestLanePlayStartObservesTheFirstCell` |
| `[PARITY] scheduler vs DrumView mismatch … kind=timing_seqfire` **by itself** | Stage A ("seqfire" = how late the sequencer fires a cell) measures the **UI lane** once the audio lane owns dispatch; on WASM the sequencer shares the main thread, so one ~17 ms frame puts p99 over the 20 ms bound. Every audio stage in those dumps was clean (`overdueCount=0`) | Stage A is gated only while the sequencer dispatches audio (`!g.audioLaneOn()`); lane-mode audio timing stays on Stages C/D | `TestLaneModeSeqFireIsNotAnAudioTimingBreach` (with a legacy control) |

Each fix was proven causal by reverting it alone (`go -overlay`) and watching its test fail.

## Why the checker keeps producing false positives

The comparator does not read the systems it checks; it keeps **shadow copies** of them and then has to guess when the copies went stale.

* **Five shadow logs**: `parityAudio`, `paritySeqDecisions`, `highlightedBeats` (used as evidence), `hlDropped`, `timingTraces`. Filled from ~8 sites on three goroutines.
* **Invalidation is hand-placed**: `parityAudio = nil` at 5 sites, `clearParityState` from 5, `audioGen.Add` at 4, `bumpParityGen` from 9, `resetHighlights` from 5. Every new transport discontinuity must remember all of them. Seek remembered one of three (highlights), which is the whole first bug.
* **Wall-clock graces stand in for ordering facts**: 120 ms `RecordedAt` freshness (×2), `When − now > 0.5 s`, 80 ms post-mutation grace, the "just-scheduled beat" exemption, highlight frame deadlines. They are why a fast test harness can't see stale-state bugs (the whole run finishes inside 120 ms of wall time — `TestLaneBackwardSeekReplaysCells` passed with all four fixes reverted and parity armed) and why a slow real frame trips them.
* **Duplicate kinds**: `timeline_vs_pred` (`diffPredictorTimeline`) and `timeline_vs_pred_immutable` (in-scan) compare the same commit against the same predictor — the edit test reported 248 of each.
* **The timing dump reuses the logical dump** (`buildTimingFatalDump` → `buildParityFatalDump` with `row=-1 abs=-1`): every seqfire WARN printed a fake "scheduler vs DrumView mismatch" header, an empty predictor/timeline section and the whole logical mismatch ring (the 100-line `recent mismatches` block in the log). `bridge.avg=NaN` prints for a stage with no samples.

## Target model — compare sources of truth, at scan time

| Question | Source of truth (already exists) | Replaces |
|---|---|---|
| What *should* sound at (row, abs)? | `laneNoteAt(row, abs)` — the pure function the lane commits from (predictor + gating + logic + params) | `paritySeqDecisions` (a recorded copy of the same decision) |
| What was handed to the engine? | **Lane**: the lane ledger (`audioLane.committed`, incl. `vid==0` policy drops). **Legacy**: `parityAudio`, stamped with the transport epoch | `parityAudio` in lane mode; `Enqueued` flag |
| What did the grid show for a passed cell? | The played tier (`timeline/played.go`, first-write-wins) | `highlightedBeats` as parity evidence |
| What will the grid show for an unplayed cell? | `preview.BuildRowWindow` over the live timeline lookup | (unchanged) |
| Was the note on time? | Lane mode: Stage C (lead at commit) + Stage D (start vs requested) + `laneStats.Late/DroppedLate`; the playback probe on real hardware | Stage A/B as audio gates |

Rules that make this resilient to code elsewhere:

1. **One transport epoch, owned by `gamestate.State`.** Bump it inside `State` whenever the abs↔time map *jumps* (a `SeekTo` to a different step, `StartClock`, Stop) — not on a same-step `SeekTo` (the paused path calls `applySeekState` every frame) and not on a BPM re-anchor (continuous). Every parity record stamps `State.Epoch()`; the scan filters on it. A new seek/transport path then retires parity state without knowing parity exists. Guard: an AST discipline test that `SeekTo` / `StartClock` / `AnchorForBPMChange` are only called on `g.state` (no side door), plus a table test driving every discontinuity (seek ×4, stop/play, pause/resume, undo-while-playing, import-while-playing) through real input and asserting zero mismatches with the freshness clock pinned.
2. **Scan after the drain, never "within 120 ms".** `parityScan` runs from `refreshDrumRow` inside `Update`; order it after `drainAndDecayHighlights` and after the sequencer pass, and define "the UI must have shown it" as `abs < nextBeatIdxs[row]` (already applied) — then the `RecordedAt` and `When − now` graces can go. Keep the mutation grace only where an edit and the lane reconcile are genuinely asynchronous (desktop audioLoop), and express it in *cells*, not wall time.
3. **Read the live tier explicitly.** Parity, `BuildRowWindow` and `CrossingCell` want "the pass being played"; scroll-back wants history. Give the timeline two lookups (`Live` = immutables → ring → scroll-back archive below the last rewind point; `History` = everything incl. superseded) instead of one that silently tiers into whatever is there.
4. **Lane mode reads the lane.** `audio_missing` = `laneNoteAt` says sound ∧ the ledger holds no entry; a `vid==0` entry is a *policy drop* (late tolerance) — report it as its own kind (`audio_dropped_late`, WARN) with `laneStats.DroppedLate`, not as a scheduler bug. The ledger must retain entries for the scan window (today it forgets after 1 s; the scan looks back `lookahead` cells — check both bounds agree, or have the lane expose a bounded history).

## Phases

1. **Dumps** (small, no behaviour change). Separate `buildTimingDump` (stage table only, `n/a` for empty stages, plus `laneStats` when the lane is on); collapse `timeline_vs_pred` + `_immutable` into one kind; tag timing lines `[parity][timing]`. Test: snapshot of each dump shape.
2. **Transport epoch** in `gamestate.State` (rule 1), replace the ad-hoc `parityAudio = nil` / `clearParityState` sites with the epoch filter; keep `parityGen` for structural edits only. Delete `parityOnSeek` (subsumed). Tests: the discontinuity table + AST guard.
3. **Lane mode reads the ledger** (rule 4); delete the lane-mode `recordParityAudio` path and `observePendingLaneCells` (subsumed — the ledger is read after the tick by construction). Tests: existing lane suite with parity **armed**, plus `audio_dropped_late` reported once per drop.
4. **Highlight evidence = played tier** (rule 2/3); remove the 120 ms / 0.5 s graces; `highlightedBeats` returns to being a rendering aid only. Tests: `parity_highlight_*_test.go` re-expressed as played-tier assertions; the freshness-clock pin becomes unnecessary.
5. **Decisions = `laneNoteAt`** for both lanes; drop `paritySeqDecisions`. The legacy scheduler and the lane then provably agree with one function (`TestLaneHeardEqualsLegacyDispatched` already checks the audio half).
6. **Timeline `Live` / `History` lookups** (rule 3) — the superseded store added today is the first half.

Each phase is independently shippable and must leave `make test` green with **no test disarming parity** (`grep -n "parityWatch = parityWatchLog" *_test.go` should shrink every phase).

## Open items found on the way

* `TestLaneHeardEqualsLegacyDispatched` still runs with parity in log mode: re-armed, seed 3 trips `audio_vs_view` at the cell right after a BPM edit (`predictor visible=false triggered=true`). The harness's BPM edit re-anchors with `time.Now()` against a fake audio clock (20 s of audio in ~1 s wall), so this may be a harness artefact — or a real lane/legacy disagreement on a mute cell after a re-anchor. Needs its own investigation before phase 3.
* Lane `vid==0` drops currently surface as `audio_missing` (see rule 4).
* Browser Stage B (`bridge`) has no samples in lane mode and prints NaN; decide whether it should observe `flushLaneOps` queue wait instead.
