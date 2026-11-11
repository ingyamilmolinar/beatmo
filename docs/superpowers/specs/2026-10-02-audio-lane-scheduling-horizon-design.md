# Audio lane: a reconciling scheduling horizon

Status: proposed (2026-10-02). Option B of the 2026-09-30 audio-isolation
investigation (`bench-results/audio_isolation_findings_2026-09-30.md`,
CLAUDE.md § Browser audio isolation).

## 1. Goal and success criteria

Smooth audio is the most critical property of Beatmo and must hold in the
browser, where Go, Ebiten, the GC and WebGL share one main thread.

The user-facing goals:

1. **A main-thread stall shorter than the horizon H is inaudible.** No late,
   rushed or missing notes for any UI stall shorter than H (browser H = 1.0 s).
2. **Real-time edits.** Any edit is audible on the next note that has **not
   started yet**. Already-sounding voices are left alone (decision recorded
   2026-10-01, option (a)). Edit latency must not grow with H; target ≤ one
   frame + the cancel guard (~20 ms).
3. **No regressions.** Same sound, same AV relationship as today in steady
   state, same notes (parity), both platforms on one code path.

Measured success criteria (§7): in the isolation bench, with the same build and
the same machine, interleaved lane-vs-legacy runs:

| Metric (probe) | Legacy today (headless) | Lane target |
|---|---|---|
| `arrivedLate` (notes reaching audio already late), stall ≤ H | 14–163 / run | 0 |
| notes > 20 ms late, stall ≤ H | 3–68 / run | 0 |
| IOI error p95 | 75–370 ms | ≤ 5 ms |
| edit-to-audio latency (mute, volume, pitch, synth knob) | ≈ lookahead + tick | ≤ 50 ms p95 |
| silent / dropped / spurious notes, clicks | 0 (synthetic) | 0 |
| Update / Draw cost | baseline | ≤ +5 % |

## 2. The problem (recap)

Today one loop, `seqScheduleTime`, decides a beat **when the wall clock reaches
its grid time T** and dispatches it with `when = T + lookahead` (60–150 ms). The
deadline to reach `BufferSource.start` is therefore the lookahead. Any
main-thread stall longer than it makes the note late and then rushes the backlog.
A bigger lookahead only moves audio further behind the playhead.

Deciding a beat and committing its audio must be separated in time. The UI keeps
deciding at grid time (highlights, timeline commits, parity); audio is committed
H ahead and kept correct under edits.

## 3. Architecture

```
            UI lane (unchanged role, no audio)          Audio lane (new)
seqScheduleTime @ grid time                  AudioLane.Tick @ every drive point
  decisions, highlights, timeline,             desired := NoteAt(row, abs) for abs in
  parity, node-logic counters                          [commitFrom, step(now+H)]
  └─ (lane mode) no audio dispatch             diff(desired, committed) →
     parity reads the lane's commits              schedule / cancel / stopAt
                                                       │
                                                  AudioSink (platform)
                                     desktop mixer  ◄──┴──►  audio.js lane registry
```

### 3.1 `NoteAt` — the pure note function

`NoteAt(row, abs) → Note` is side-effect free, a function of a read-only
snapshot of current state:

- `info := seqBeatInfoAtRow(snap, row, abs)`.
- Regular cell:
  - `Audible = Predictor.AudibleAt(row, abs) && rowAudible(row) && instrumentAvailable`.
  - `vol, pitch, dur := evalNodeParamsOnly(row, abs, info)` (row volume, node
    params, group rules).
  - For custom `Params.Logic` nodes (test-harness only), apply the decision
    multipliers the predictor recorded for that cell (new
    `Predictor.DecisionAt`, a sparse map populated only for custom-logic cells).
- Mute cell: `Predictor.TriggeredAt(row, abs)` yields a `StopAt` note for the
  row's instrument.
- `Time = AudioTimeOfStep(abs) + nodeNudgeSecondsAt(...) + AudioOffsetSec`.
- `Voice = audio.VoiceRevision(instrument)` (§3.4).
- `Hash = hash(instrument, nodeID, kind, audible, vol, pitch, dur, Time, Voice)`.

`AudioOffsetSec` is a constant (RuntimeProfile; default = today's base lookahead,
0.06 s), so the AV relationship is unchanged in steady state. The adaptive +30 ms
extras disappear: their job (absorbing stalls) is now the horizon's.

### 3.2 `AudioLane` — reconcile, don't invalidate

The lane keeps `committed: map[(row, abs)] → {hash, voiceID, time, kind}` for
every note in `[now − tail, now + H]`.

`Tick(now)`:

1. Compute a cheap **state key** from the inputs `NoteAt` reads:
   - path snapshot pointer, predictor epoch, graph/group/params versions;
   - per-row (instrument, volume, muted, solo);
   - applied BPM and clock anchor (`beatBase`, `audioStart`);
   - `audioGen`, transport state;
   - each instrument's `VoiceRevision`.
2. If the key is unchanged, only extend the window: `NoteAt` for the abs values
   that newly entered `now + H` → schedule.
3. If the key changed, **reconcile** every abs in `[commitFrom, step(now+H)]`.
   The cancel guard is `commitFrom = now + CancelGuardSec` (JS 8 ms, desktop one
   block). Notes before it are effectively started and are left alone.
   - Same hash → keep.
   - Different hash → `Cancel(old)`, then schedule the new note (if audible).
   - No longer present → `Cancel`.
4. Drop committed entries older than `now − tail`.

Edits therefore need **no complete event funnel**. Any change to anything
`NoteAt` reads changes the key and is reconciled on the next tick. This covers
the mutations that today bypass `bumpParityGen` (row volume, mute/solo, seek,
import, undo, BPM, instrument). An optional `Lane.Poke()` from known edit sites
just runs a tick immediately.

**Drive points:** the existing ones (4 ms sequencer goroutine, frame-end drive,
audioLoop), under `seqMu` exactly like `seqScheduleTime` (same locks, same thread
discipline).

**Transport:**
- **Stop / Pause / media pause:** `CancelAllFuture()` and clear `committed`.
- **Play / resume / seek / BPM re-anchor:** the anchor changes the key, and
  reconcile recommits.

**Late commit:** after a stall longer than H, or at play start, some notes are
due before `commitFrom`. Policy:
- within `LateToleranceSec` (25 ms) → play now and count `late`;
- otherwise → skip and count `dropped_late`.

This replaces today's "rush the backlog", which is the audible stumble.

### 3.3 `AudioSink` — platform API (both platforms)

```go
type AudioSink interface {
    Schedule(vid uint64, n Note)            // lane voice, start at n.Time
    Cancel(vid uint64) (started bool)       // silent if not started
    StopAt(inst string, t float64)          // stop lane voices of inst with start < t, at t
    CancelAllFuture()                       // all lane voices with start > now + guard
}
```

Lane voices carry `vid > 0`. Previews and UI sounds keep `vid = 0` and are
untouched by `Cancel`/`StopAt`/`CancelAllFuture`.

**Desktop mixer** (`engine_mixer.go`, `engine_stop.go`):
- Add `vid` to `voiceState`.
- `Cancel` locks `pendingMu` then `m.mu` (the order `Read` uses). It removes the
  voice from `pendingAdd` or `voices` only if `start > m.pos`; removal is
  silent. It replaces today's `RequestStop` ghost.
- `StopAt` is a stop-event queue applied by `processBlock` at the block that
  contains `t` (via the device clock), only to voices with `start < t`.
- Fix the unlocked `m.pos` read in `Schedule`.
- Move analyzer/scope/export `NotifyTrigger` from schedule time to first-render
  time, so cancelled or early-committed notes don't light the UI early.

**Browser** (`audio.js`, `batch_wasm.go`):
- `playSoundsBatchFlat` gains a `vids` array.
- JS keeps `laneVoices: Map<vid, {src, gain, inst, when}>`.
- `vid` is carried through `sanitizeAudioEvent`, `carryTiming` and every replay
  (stale race, cold-miss defer, sample load).
- `cancelLaneVoice(vid)`:
  - adds a **tombstone**, so pending or deferred replays are dropped in
    `processAudioEvent`;
  - if `when > currentTime + 8 ms`, calls `src.stop(0)` + disconnect (silent);
  - otherwise reports started.
- `stopLaneAt(inst, t)`, only for sources with `when < t`:
  - set the gain to 0.18 at t, ramp to 0 by t + 20 ms, then `stop(t + 21 ms)`;
  - no `cancelAndHoldAtTime` (Firefox doesn't support it).
- `cancelAllLaneFuture()`: bumps the lane generation, silently stops future lane
  sources, and resets `lastScheduledPlayWhen`.
- `timingTraces`, `recordSamples` and `RecordVoiceTrigger` move to actual-start
  time, or skip cancelled voices.
- The playback probe keys requests by `vid` and counts cancelled voices
  separately.

### 3.4 `VoiceRevision` — synth edits are real-time too

Desktop binds the rendered buffer at commit time; JS looks it up at
`processAudioEvent` time. Either way, a note committed before a synth-knob edit
carries the old sound for up to H.

`audio.VoiceRevision(inst)` combines:
- `InstrumentParamsRev`,
- `RecipeDefaultsRev`,
- sample-edit revision,
- recipe binding revision,
- WAV load revision.

It is part of the note hash, so an edit recommits that instrument's not-started
notes. In JS the existing stale-while-revalidate race then plays the fresh render
when it lands in time, and H gives it far more time than today.

During a knob drag, recommits for one instrument are rate-limited to one per
`VoiceRecommitMinSec` (30 ms).

Channel-level controls (EQ, insert FX, sends, pan, channel volume) act on the
live graph at play time and need nothing.

### 3.5 The UI lane in lane mode

`seqScheduleTime` keeps everything except audio:
- decisions, `recordSeqDecision`, highlights, `lastFiredNodeByRow`, node-logic
  counters, hooks;
- `scheduleSound`'s queue step and the mute `audio.Stop(inst)` are skipped;
- `markSeqAudioEnqueued` / `recordParityAudio` are fed from the lane's committed
  note for (row, abs), with its `Time`.

Parity therefore compares two independent computations (the UI lane's stateful
walk vs `NoteAt`), which is a stronger check than today. `audio_missing` treats a
lane note committed before the UI decision as enqueued.

## 4. Configuration

| RuntimeProfile field | Browser | Desktop | Meaning |
|---|---|---|---|
| `AudioLaneEnabled` | true | true | lane on; false = legacy path (kept for A/B) |
| `AudioHorizonSec` | 1.0 | 0.25 | commit window H. Browser sized above headless software-GL frames (0.4–0.6 s, up to 1.1 s under load) + an injected 150–300 ms stall; edit latency does not depend on H, only cancel churn does |
| `AudioOffsetSec` | 0.06 | 0.06 | constant audio offset vs grid (today's base lookahead) |
| `CancelGuardSec` | 0.008 | 0.003 | notes this close to start are treated as started |
| `LateToleranceSec` | 0.025 | 0.025 | late-commit play-now window |

Under `go test`, `AudioLaneEnabled` defaults to **false**. The ~70 legacy files
that assert synchronous `playFn` at schedule time keep running unchanged. Lane
tests opt in explicitly (§6). Browser override: `window.__beatmoProfileOverride`
(existing mechanism), used by the benches for same-build A/B.

## 5. Phases

1. **Measurement first.**
   - Isolation bench gets `SITE_DIR` (serve the frozen pre-refactor build in
     `scratchpad/baseline_site`) and a profile override for lane on/off.
   - Probe gets edit marks (`audioProbeMark(label)`) and an **edit-latency**
     metric: edit → first onset reflecting it (mute/unmute, row volume step,
     node pitch, synth knob).
   - Record the BEFORE numbers.
2. **AudioSink on both platforms.** vid, Cancel, StopAt, CancelAllFuture,
   tombstones, `NotifyTrigger` relocation. Tests below.
3. **NoteAt + AudioLane** (platform-agnostic Go, fake sink).
4. **Integration.** UI lane in lane mode, transport hooks, parity adaptation,
   `VoiceRevision`, predictor `DecisionAt`.
5. **Measure AFTER** (interleaved, same build + baseline_site), tune H and
   guards, turn on by default.

## 6. Testing

**Go, fake `AudioSink`, deterministic:**
- **Oracle equivalence (property test):** for random circuits and random edit
  sequences at random times, the set of notes **heard** (committed and not
  cancelled before start) equals the legacy sequencer's dispatched notes for the
  same timeline, i.e. `NoteAt` evaluated at grid time against the state current
  at that time.
- **Edit latency:** every mutation class (§2 of the inventory) changes the next
  not-started note and no started note.
- **Stall tolerance:** with no ticks for S < H, every note in the stall is
  already committed. For S > H the late policy is applied and counted.
- **Transport:** stop / pause / seek / BPM cancel or recommit correctly; no
  orphan voices.

**Desktop mixer:** cancel removes pending and future voices silently (sample
output identical to never scheduling); `StopAt` spares voices starting at or
after t; lock order is race-detector clean.

**Browser (Playwright, probe-verified):**
- Cancel before start → no onset, no click.
- Tombstoned deferred replay doesn't play.
- `StopAt` fades the right sources only.
- Lane end-to-end on the startup demo: 0 late under `stall-150`.
- Edit latency ≤ target for each edit kind.
- `xplat` parity still byte-identical.

**Existing suites:** parity, timing traces, three-stage (re-baselined: Stage A
becomes the UI lane; audio lateness comes from the probe).

## 7. Measurement protocol (apples to apples)

- Same machine, same circuit (startup demo, plus a synthetic 8-row circuit),
  same scenarios (idle, stall-50, stall-150, stall-300, tab-synth, tab-chain,
  zoom-storm, pan-storm, edit storms).
- **Interleaved** A/B runs (legacy, lane, legacy, lane, …), ≥ 3 each, load
  recorded per run; runs with load > 15 on 8 cores are discarded.
- Three arms:
  - `baseline_site`: frozen pre-refactor build;
  - new build with the lane off: proves the legacy path is unchanged;
  - new build with the lane on.
- Report medians and spread for every probe metric plus Update/Draw/audioCall,
  in `bench-results/audio_lane_ab_<date>.md`.

## 8. Risks

- **Reconcile cost**: rows × steps in H per changed tick (8 rows × 20 steps ≈
  160 `NoteAt`). Mitigated by the state-key fast path; budget measured in phase 5.
- **Cancel churn during drags**: rate limit (§3.4).
- **Two computations disagree** (UI lane counters vs predictor): surfaced by
  parity, which is the point.
- **Committed notes outlive a Go crash**: `CancelAllFuture` on `Game.Close`.
- **Notes committed before an edit and started inside the cancel guard play
  the old version.** That is the agreed semantics, (a).
