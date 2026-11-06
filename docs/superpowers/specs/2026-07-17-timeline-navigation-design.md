# Timeline Navigation — Design

**Date:** 2026-07-17
**Status:** Approved design, pre-implementation
**Branch context:** designed on `add-core-node-types`; implementation should start from its own branch.

## Problem

Beatmo has a full playhead-seek engine — `Game.Seek(beats)` (`src/go/internal/ui/game_seek.go:11`) re-anchors clocks, trims mutable history, and respawns sequencer state, working both during playback and stopped — but its only caller is undo/redo restore. Every user-facing "timeline" interaction (ribbon scrub, grid drag, two-finger pan, `clickTimelineAt` JS export) moves `dv.Offset`, the *editing view window*, never the playhead. Users cannot navigate playback to an arbitrary beat/subdivision.

Additionally, transport timing has drifted into duplication: the position formula `BeatBase + dt*bpm/60` is hand-computed at four sites (`game_beat_display.go:122`, `game_sequencer_schedule.go:105`, `game_sync_ui.go:51`, `game_update.go:968`), and `game_media_session_wasm.go:84-114` duplicates the resume-from-offset anchor dance. The UI playhead and the audio scheduler independently integrate the same clock and merely happen to agree.

## Goals

1. Navigate the playhead to any circuit position (beat + subdivision), forward or backward, while playing or stopped.
2. One source of truth for timing/progress, with named interfaces between the transport clock, the timeline service, the predictor, and the sequencer.
3. DAW-familiar UX: click-to-seek ruler, FF/RW transport buttons with hold-shuttle, click-the-counter-to-type position entry, keyboard nudges.

## Decisions (settled with user)

| Question | Decision |
|---|---|
| Ribbon click/drag | Seeks the playhead (DAW-standard). View-scroll remains via grid drag / wheel / two-finger pan. |
| Seek timing during playback | Instant jump (what `Game.Seek` already does). No quantize-to-beat mode. |
| Snapping | Ribbon interactions snap to whole beats (honest about ~4px/beat resolution). Numeric entry is subdivision-accurate. |
| Controls in scope | Beat-counter position entry, FF/RW buttons, desktop keyboard shortcuts. No dedicated RTZ button (Stop already returns to zero). |
| Replaying over recorded history | Re-simulate with current graph; new pass re-records from the seek point ("most recent pass wins"). Old immutable commits are archived, not deleted. |
| Implementation shape | Callback wiring through existing idioms (`TransportCallbacks`, hit adapters, `paramEditor`). `Game.SeekSteps` is the single engine chokepoint. No new NavController component. |

## Architecture

### The transport clock authority (`internal/gamestate`)

`gamestate.State` already owns every timing anchor (`beatBase`, `audioStart`, `playStart`, `pausedBeats`, applied BPM, playing/paused flags) but is a passive getter/setter bag. It becomes the **single authority for position**, absorbing the formula that is currently duplicated:

- `PosBeats(now time.Time, audioNow float64) float64` — the one clock integration. Playing: `beatBase + dt*bpm/60`, preferring the audio clock (`audioNow - audioStart`) and falling back to wall clock (`now - playStart`). Paused/stopped: derived from `pausedBeats`.
- `PosStep(div int, now, audioNow) int` — canonical absolute-subdivision position (`round(PosBeats*div)`); what `Game.playheadAbsSubdiv()` delegates to.
- `TargetStep(div int, now, audioNow) int` — the scheduler's catch-up target (`floor(PosBeats*div)`); replaces the inline formula at `game_sequencer_schedule.go:105`.
- `AudioTimeOfStep(idx, div int) float64` — inverse map (beat index → ideal audio-clock time); replaces the inline `beatIdealAudio` computation at `game_sequencer_schedule.go:362`.
- `SeekTo(steps, div int, now time.Time, audioNow float64)` — the **only** relocation mutation. Playing: sets `beatBase = steps/div`, re-anchors `playStart`/`audioStart`. Not playing: sets `pausedBeats = steps`. Replaces `Game.applySeekState`'s direct pokes; follows the `AnchorForBPMChange` precedent.

Raw `SetBeatBase` / `SetAudioStart` / `SetPausedBeats` setters are demoted to `...ForTest` names so production code cannot bypass the authority. The four duplicated formula sites and the media-session duplicate all become calls into these methods.

**Currency rule:** the absolute subdivision step (`abs`) is the single unit crossing every boundary. Per-row counters (`seqNextIdxs`, `nextBeatIdxs`) are *derived caches* of clock position — realigned at seek / BPM change / structural edits, never independently authoritative.

The sequencer consumes the clock through a read-only `TransportClock` interface (the methods above minus mutations), so scheduling code cannot move anchors.

### Boundary interfaces

**Seek → timeline service:** new method `timeline.RewindTo(row, abs)` in `internal/timeline`. Internally: demote immutable `Playback`/`Import` commits *after* `abs` into the cold archive tier (same mechanism as the existing noring-demote archive), trim mutable cells, lower the freeze watermark. Replaces `Game.Seek`'s separate pokes at `timelineTrimAfterMutable` + `frozenUpToByRow`; rewind semantics become the timeline package's business, tested there. History is never dropped (full-history-retention rule) — superseded passes remain in the archive; the live ribbon shows the latest reality.

**Seek → predictor:** new `Rewind(abs)` in `core/engine`, plus existing `Ensure(horizon)`. A backward seek below `WindowStart()` resets the window to start at the target and re-simulates forward with the current graph — which is exactly the chosen "re-simulate" semantics, so no old predictions need preserving. Forward seeks call `Ensure(target + horizon)`. This discharges the guard documented at `predictor_api.go:46` (evicted window must not read as frozen-empty past).

**Orchestration:** `Game.SeekSteps(steps int)` — extracted from today's `Seek(beats)` body; `Seek(beats)` becomes `SeekSteps(beats * MaxDiv())`. Ordered sequence under `seqMu`:

1. clamp `steps` to `[0, timelineBeats*MaxDiv()]`
2. `state.SeekTo(steps, div, now, audioNow)`
3. `timeline.RewindTo(row, steps)` per row
4. predictor `Rewind`/`Ensure`
5. realign `seqNextIdxs` / `nextBeatIdxs` / `muteUntilByRow`
6. reset highlights/pulses; spawn fresh pulses if playing
7. emit `EventSeek`

Every entry point in this feature (ribbon, FF/RW, counter, keyboard, JS) calls `SeekSteps`.

### Semantics kept as-is / explicit non-goals

- **Stop returns to zero** (existing `setPrimaryStep(0)`); Pause preserves position. Seek-while-stopped + Play works via the existing `resumeFromOffset` path (`game_update.go:787`) with no new plumbing.
- **Seek is not undoable** — transport navigation (`ScopeTransport`), like Play/Stop. It trims only speculative future cells, never document state.
- **Seek during recording** needs no special handling — Beatmo records master output; the jump is simply audible in the take.
- **No quantized-seek mode, no marker/locator system** (possible future layers; nothing here precludes them).

## UI/UX

### Ribbon = seek surface

`timelineScrubHitAdapter` (`timeline_zone.go:1363`) changes meaning:

- **Press:** snap pointer-x to nearest whole beat → `SeekSteps` immediately.
- **Drag:** ghost playhead line + beat-number chip (Spectrum-cursor-chip pattern); no seeks while dragging.
- **Release:** commit final seek. A drag gesture is therefore at most two seeks — never a per-frame storm of pulse respawns.

View-window scrolling is unchanged elsewhere: grid drag, wheel, and two-finger pan still move `dv.Offset`. After any seek the edit window centers once on the target; the existing follow toggle governs afterward. `clickTimelineAt(frac)` JS export is re-pointed to seek (matching the new ribbon meaning).

### FF / RW transport buttons

- Placement: `rwBtn` left of Play, `ffBtn` right of Stop; Record stays put (minimal-UX-change rule). Mobile bottom-bar variant follows the zone's existing layout machinery.
- Icons: new `IconRewind`, `IconFastForward` `IconID`s in `icons.go` (forbidden-glyphs rule — no text glyphs).
- Sizing: existing transport Density tokens; no new DESIGN.md knobs.
- Behavior: tap = ±1 beat (snap current position to whole beat first). Hold = existing Button auto-repeat, ±1 per repeat, escalating to ±4 per repeat after ~1 s. Clamped at 0 and timeline end.
- Telemetry: the button's own `[ui] tap` Layer-B line plus one `[seek]` domain line per gesture (accepted two-line bound). `EventSeek` emitted once, at gesture end.

### Beat counter → position entry

The beat counter above the ribbon becomes tappable, opening the same `paramEditor` numeric popup BPM uses, pre-filled with the current position. Grammar: `17` → beat 17, subdiv 0; `17.3` → beat 17, subdivision 3 (0-based, clamped to `MaxDiv-1`). The popup hint text states that the fractional part is a **subdivision index**, not a decimal (i18n entry added). This is the subdivision-precise navigation path.

### Keyboard (desktop)

New gated `"seek"` node in `keyboardShortcutRouter` (`keyboard_shortcuts.go`):

| Key | Action |
|---|---|
| `.` | +1 beat (held = repeat) |
| `,` | −1 beat (held = repeat) |
| `Shift+.` / `Shift+,` | ±4 beats |
| `Home` | seek to beat 0 |

Audit confirmed these keys are globally unclaimed (arrows/brackets/0 = grid pan/zoom; Space/±/numpad = transport; Z/Y/R/C/X/V = undo/clipboard; 1-7/? = audio panel; Home is used only inside the instrument-menu overlay, which gates separately). Router gating keeps the node dead while text inputs or overlays have focus; the router's key-claim table makes no-conflict testable.

### JS bridge

- `seekSteps(steps)` → `Game.SeekSteps`, origin `js`.
- `playheadStep()` → reads the clock (tests + console diagnostics).
- Both added to the `wasm_bridge_smoke.browser.test.js` catalogue; `js_exports_catalogue_drift_test.go` keeps them honest.

### Events

`SeekPayload` gains `Steps int` and `Origin string` (`ribbon` / `transport` / `counter` / `keyboard` / `js`) per the event-source-attribution convention; existing `Beats` field kept (= `Steps/div`). One `EventSeek` per gesture (shuttle-hold and ribbon-drag emit at release only) — satisfies the INFO anti-pollution rule with no coalescer changes. `EventSeek` already exists in the registry with tag `seek`, so `NumNonVerbose` is unchanged.

## Testing

Go owns logic; JS owns boundaries (JS Test Charter).

**`internal/gamestate`** — table-driven tests for `PosBeats` / `PosStep` / `TargetStep` / `AudioTimeOfStep` / `SeekTo` across playing/paused/stopped × BPM changes × audio-clock-vs-wall-clock fallback. An equivalence test pins that the UI-playhead derivation and the scheduler-target derivation agree at all combinations — the drift this refactor eliminates stays eliminated.

**`internal/timeline`** — `RewindTo`: archive-demote of post-seek immutables, mutable trim, watermark lowering, archived passes retrievable, nothing dropped.

**`core/engine`** — predictor `Rewind`: backward seek below `WindowStart` yields a fresh window at target (never frozen-empty past); forward `Ensure` beyond horizon.

**`internal/ui`** (functional, driving real `Game.Update` per convention):
- seek while playing / paused / stopped; play-after-seek via `resumeFromOffset`
- ribbon press/drag/release gesture (ghost shown, ≤2 seeks per gesture)
- FF/RW tap and hold-acceleration; clamps at both ends
- counter entry parse/clamp (`17`, `17.3`, out-of-range subdiv)
- `"seek"` router node behavior + extended router test proving no key-claim collisions
- parity checks green after seek (existing `parity_*` machinery)
- one-`EventSeek`-per-gesture with correct `Origin`
- existing discipline tests stay green (zone input isolation, alloc budgets — ghost-cursor draw checked against the timeline zone's budget if one exists)

**Browser:**
- `e2e_timeline_seek.browser.test.js` re-pointed at playhead movement (assert via `playheadStep()`, not `drumOffset()`)
- bridge catalogue entries for `seekSteps` / `playheadStep`
- a seek step added to `xplat_parity.browser.test.js` so desktop/WASM agree on post-seek scheduling

## Risks

- **Predictor `Rewind` is new surface** in `core/engine` — the sliding window was designed to move forward. Mitigation: re-simulate semantics mean a rewind is just "reset window start + Ensure", no state preservation.
- **Archive-demote on seek** touches the immutability machinery. Mitigation: reuses the existing noring-demote archive path; tested in `internal/timeline` in isolation.
- **Ribbon behavior change** breaks the old scrub habit (ribbon-drag no longer scrolls the view). Mitigation: grid drag / wheel / two-finger pan already provide view scrolling; the e2e test is updated deliberately, not silently.
- **Seek storms** (hold-shuttle at max repeat) each respawn pulses under `seqMu`. Mitigation: repeats are frame-paced by Button machinery (~single-digit Hz), and `Game.Seek`'s body is already exercised at this cost by undo.
