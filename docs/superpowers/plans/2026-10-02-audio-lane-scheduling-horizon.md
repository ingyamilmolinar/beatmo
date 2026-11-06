# Audio Lane (Reconciling Scheduling Horizon) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Commit audio up to H seconds ahead of the playhead, and keep it correct under real-time edits by reconciling instead of invalidating. A main-thread stall shorter than H becomes inaudible in the browser, while every edit still reaches the next note that has not started.

**Architecture:**
- **`laneNoteAt(row, abs)`** is a pure note function over current state (predictor + node params + row state + clock).
- **`audioLane`** keeps a committed set of notes for `[now, now+H]`. Each tick it compares cheap per-row fingerprints. For changed rows it recomputes notes and diffs them against what is committed (cancel / replace / schedule) through a platform `audioSink`.
- **The engines** (desktop mixer, audio.js) gain voice IDs, a silent cancel of not-yet-started voices, `StopLaneAt` and `CancelAllLaneFuture`.
- **`seqScheduleTime`** keeps deciding at grid time for the UI (highlights, timeline, parity) but no longer dispatches audio when the lane is on.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten, WebAudio / AudioWorklet (src/js/audio.js), Playwright browser tests, the playback probe (`audio_probe_*.js`).

**Spec:** `docs/superpowers/specs/2026-10-02-audio-lane-scheduling-horizon-design.md`

## Global Constraints

- Never `git add` / `git stage` / `git commit` (user rule). Every "finish" step leaves the changes in the working tree.
- Always use the bundled Go: `.tools/go/bin/go`.
  - Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ...`
  - Real-build tests (`//go:build !test`): `cd src/go && CGO_ENABLED=1 xvfb-run -a ../../.tools/go/bin/go test ...`
- Browser tests: `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/<test>.browser.test.js`.
- The machine is shared: check `cut -d' ' -f1 /proc/loadavg` before any timing run, and discard runs with load > 15.
- Never `pkill -f` broad patterns. Kill by PID.
- Other sessions rebuild `src/js/main.wasm`. Timing runs serve a private build via `WASM_FILE=` / `SITE_DIR=`.
- RuntimeProfile values (spec §4):

  | Field | Browser | Desktop |
  |---|---|---|
  | `AudioLaneEnabled` | true | true (false until Task 12 flips it) |
  | `AudioHorizonSec` | 1.0 | 0.25 |
  | `AudioOffsetSec` | 0.06 | 0.06 |
  | `CancelGuardSec` | 0.008 | 0.003 |
  | `LateToleranceSec` | 0.025 | 0.025 |
  | `LaneFullReconcileSec` | 0.25 | 0.25 |
  | `LaneVoiceRecommitMinSec` | 0.03 | 0.03 |

- Under `go test`, the lane is off unless a test calls `enableAudioLaneForTest`. Legacy tests must not change.
- Lane voices carry `VoiceID > 0`. Previews / UI sounds keep `VoiceID == 0` and must never be touched by lane cancel/stop operations.
- Edit semantics (decision (a)): an edit affects every note that has not started yet. Voices already sounding are never cut or retuned, except by a mute node's `StopLaneAt`, which keeps today's mute semantics.
- `AudioOffsetSec` stays 0.06, so the AV relationship is unchanged. The adaptive lookahead extras do not apply in lane mode.
- Every new `window.*` function called from Go goes into `jsNativeCatalogueEntries` (`src/go/internal/ui/js_exports_catalogue_drift_test.go`) and the smoke catalogue (`src/js/wasm_bridge_smoke.browser.test.js`) as `skipCall: true`.
- Every new runtime JS file goes into `scripts/deploy-wasm-gcs.sh` `ASSET_FILES`.

## Review Focus

1. **Row deleted / added mid-window.** Indices shift, so every committed note on the shifted rows must be cancelled or replaced and none may be orphaned. Test: Task 8, `TestLaneRowDeleteShiftsCommittedNotes`.
2. **Edit landing inside the cancel guard.** The note starts unchanged (it is "started"). Its replacement must NOT also play: no doubled hit. Test: Task 7, `TestLaneChangeInsideGuardDoesNotDouble`.
3. **Stop pressed during a 1 s horizon.** Nothing may sound after Stop beyond the voices already playing; previews still play. Tests: Task 4 (desktop), Task 5 (browser), Task 9 (`TestLaneStopCancelsFuture`).
4. **BPM change and seek while playing.** Every committed future note moves to the new grid, with no notes at the old times. Test: Task 9, `TestLaneBPMChangeRecommitsAtNewTimes`.
5. **Knob drag on a synth instrument.** Recommits are rate-limited, no voice is lost, and the probe sees no gaps or doubles. Tests: Task 7, `TestLaneVoiceRecommitRateLimited`; Task 11, the browser `edit-storm` scenario.

---

## Phase 1 — Measure BEFORE

### Task 1: Probe — cancelled notes and edit-latency metric

The probe must count lane-cancelled notes as intentionally silent (not "missed") and measure **edit staleness**: notes that start after an edit but still reflect the pre-edit state. Edit marks are recorded on the **Node side** (the ctx time read in the same `page.evaluate` that performs the edit), so the frozen baseline build, which has no mark API, is measured the same way.

**Files:**
- Modify: `src/js/audio_probe_analysis.js` (new `editMarks` input, `cancelled` handling, `edits` output)
- Modify: `src/js/audio.js` (probe `noteCancelled(pk)`; `cancels[]` in the raw log)
- Modify: `src/js/audio_probe_bench_helpers.js` (`markEdit(page, mark, editFn)`, `stopPlaybackProbe(..., { editMarks })`)
- Test: `src/js/audio_probe_analysis.browser.test.js` (pure Node)

**Interfaces:**
- Produces:
  - `analyzeAudioProbe(raw, { editMarks })` where `editMarks: [{ t, label, kind: 'mute'|'unmute'|'vol', channel, value? }]`;
  - report field `edits: { count, staleNotes, staleMs: stats, perKind: { [kind]: { count, staleNotes, maxStaleMs } } }`;
  - `notes.cancelled`.
  - JS: `audioProbe.noteCancelled(pk)` (internal); raw log gains `cancels: [{ pk, ctx }]`.
  - Node helper: `async markEdit(page, { label, kind, channel, value }, editFnSource)` returns the mark with `t` = ctx time at the edit.

- [ ] **Step 1: Write failing analysis tests** (append to `audio_probe_analysis.browser.test.js` before the final `if (failures)`):

```js
// 9. Cancelled notes are not "missed", and edit staleness is measured.
{
  const raw = log({ notes, onsetFor: (i, s) => (i === 6 ? null : s + 0.001) });
  raw.cancels = [{ pk: 7, ctx: 2.0 }]; // note index 6 → pk 7, cancelled before its start
  const r = analyzeAudioProbe(raw);
  check(r.notes.cancelled === 1 && r.notes.missed === 0, `cancelled note excluded from missed (${JSON.stringify(r.notes)})`);
}
{
  // Mute edit at t=2.1: notes at 2.25 and 2.5 (indices 5 and 6) still sound → stale.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 2.1, label: 'mute row0', kind: 'mute', channel: 'kick' }] });
  check(r.edits.count === 1, 'one edit counted');
  // Stale = heard notes on the muted channel starting after t + guard (8 ms).
  check(r.edits.staleNotes === 12, `stale notes after a mute = every later heard note (got ${r.edits.staleNotes})`);
  check(near(r.edits.perKind.mute.maxStaleMs, (notes[15] - 2.1) * 1000, 1), `max stale ≈ last note − edit (got ${r.edits.perKind.mute.maxStaleMs})`);
}
{
  // Volume edit: notes after the edit carry the new vol except one straggler.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  for (const q of raw.requests) if (q.when > 2.1) q.vol = 0.5;
  raw.requests[5].vol = 1; // note at 2.25 kept the old volume → stale
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 2.1, label: 'vol', kind: 'vol', channel: 'kick', value: 0.5 }] });
  check(r.edits.staleNotes === 1 && near(r.edits.perKind.vol.maxStaleMs, 150, 1), `one stale volume note 150 ms after the edit (got ${JSON.stringify(r.edits.perKind.vol)})`);
}
```

- [ ] **Step 2: Run it to see it fail.** Run `node src/js/audio_probe_analysis.browser.test.js`. Expected: FAIL (`r.notes.cancelled` undefined, `r.edits` undefined).

- [ ] **Step 3: Implement in `audio_probe_analysis.js`.**
  - Add `editGuardSec: 0.008` to `DEFAULTS`.
  - After building `startByPk`, build `const cancelledPks = new Set((raw.cancels || []).map((c) => c.pk));`.
  - When building `notes`, skip `s` whose pk is in `cancelledPks` **and** whose `startWhen > c.ctx` (cancelled before start); count them in `cancelledCount`.
  - Exclude cancelled pks from `dropped`.
  - Add the `edits` block before `return`:

```js
  // ── Edit staleness ── notes that START after an edit (+ guard) yet still
  // reflect the pre-edit state. Per kind:
  //   mute   — a heard note on the muted channel
  //   unmute — nothing is stale (missing notes would show as `missed`)
  //   vol    — a started note whose requested vol != the edit's target value
  //            (compared after normalising by the row's node volume: the
  //            mark's `value` is the expected per-note vol)
  const marks = (o.editMarks || []).slice().sort((a, b) => a.t - b.t);
  const perKind = {};
  let staleTotal = 0;
  const staleMsAll = [];
  marks.forEach((m, mi) => {
    const until = mi + 1 < marks.length ? marks[mi + 1].t : Infinity;
    const after = judged.filter((n) => n.id === m.channel && n.startWhen > m.t + o.editGuardSec && n.startWhen < until);
    let stale = [];
    if (m.kind === 'mute') stale = after.filter((n) => n.onset != null || n.masked);
    else if (m.kind === 'vol') stale = after.filter((n) => Math.abs((n.vol ?? 1) - m.value) > 1e-6);
    const k = perKind[m.kind] = perKind[m.kind] || { count: 0, staleNotes: 0, maxStaleMs: 0 };
    k.count++;
    k.staleNotes += stale.length;
    for (const n of stale) {
      const ms = (n.startWhen - m.t) * 1000;
      staleMsAll.push(ms);
      if (ms > k.maxStaleMs) k.maxStaleMs = Math.round(ms * 10) / 10;
    }
    staleTotal += stale.length;
  });
```

  - Return `edits: { count: marks.length, staleNotes: staleTotal, staleMs: stats(staleMsAll), perKind }` and `notes.cancelled: cancelledCount`.
  - Add to `formatAudioProbeReport`: `` `edits ${r.edits.count} stale ${r.edits.staleNotes} (max ${f(r.edits.staleMs.max)} ms)` ``.

- [ ] **Step 4: Probe hook in `audio.js`.** In `createAudioProbeState`, add `cancels: []` and:

```js
    noteCancelled(pk) {
      if (!pk || this.cancels.length >= AUDIO_PROBE_LOG_MAX) return;
      this.cancels.push({ pk, ctx: hasCtx() ? ctx.currentTime : 0 });
    },
```

  Add `cancels: st.cancels.slice()` to `audioProbeRawOf`. Task 5 calls `noteCancelled`.

- [ ] **Step 5: Node helpers in `audio_probe_bench_helpers.js`.**

```js
// markEdit performs an edit in the page and returns its mark, stamped with the
// AudioContext time read in the SAME evaluate — so the mark works against any
// build, including the frozen pre-lane baseline.
export async function markEdit(page, mark, editFnSource, arg) {
  const t = await page.evaluate(({ src, arg }) => {
    const t = window.__audioCtx ? window.__audioCtx.currentTime : 0;
    // eslint-disable-next-line no-new-func
    new Function('arg', src)(arg);
    return t;
  }, { src: editFnSource, arg });
  return { ...mark, t };
}
```

  Extend `stopPlaybackProbe(page, label, analysisOpts)` (existing signature) so callers pass `{ editMarks }` inside `analysisOpts`. It already forwards `analysisOpts` to `analyzeAudioProbe`. Add `probeEditStaleNotes: r.edits.staleNotes`, `probeEditStaleMaxMS: r.edits.staleMs.max` and `probeNotesCancelled: r.notes.cancelled` to `probeSummaryFields`.

- [ ] **Step 6: Run the tests.** Run `node src/js/audio_probe_analysis.browser.test.js`. Expected: `PASS audio_probe_analysis`.

- [ ] **Step 7: Finish.** Run `node --check src/js/audio.js`. No commit; leave the changes in the working tree.

### Task 2: A/B harness (three arms, interleaved) and the BEFORE record

**Files:**
- Modify: `src/js/webaudio_audio_isolation_bench.browser.test.js`
  - `SITE_DIR` env: serve every non-wasm file from that dir; `WASM_FILE` still overrides `/main.wasm`.
  - `ISOLATION_ENV` (JSON): injected as `window.__beatmoEnv` via `page.addInitScript`.
  - New scenarios `stall-300`, `edit-mute`, `edit-vol`.
- Create: `scripts/audio_lane_ab.mjs`. It runs the bench for each arm, interleaved, N rounds, records load per run, and writes `bench-results/audio_lane_ab_<date>.md` + `.json`.
- Modify: `Makefile`: target `bench-lane-ab`.

**Interfaces:**
- Consumes: `markEdit`, `stopPlaybackProbe(..., { editMarks })` from Task 1.
- Produces:
  - `ISOLATION_SCENARIOS` names `stall-300`, `edit-mute`, `edit-vol`;
  - env keys used by arms: `BEATMO_AUDIO_LANE` (`"0"`/`"1"`), added in Task 9. Before Task 9 the key is simply ignored by Go.
  - Arm names `baseline` (SITE_DIR = `$SCRATCH/baseline_site`), `legacy` (new build, lane off), `lane` (new build, lane on).

- [ ] **Step 1: Bench serving + env injection.** In the bench's server, replace the path logic with:

```js
const SITE_DIR = process.env.SITE_DIR ? path.resolve(process.env.SITE_DIR) : jsDir;
// …inside the handler:
const filePath = file === "/main.wasm" ? WASM_FILE : path.join(SITE_DIR, file.replace(/^\//, ""));
```

  When `SITE_DIR` is set and `WASM_FILE` is not, default `WASM_FILE` to `path.join(SITE_DIR, "main.wasm")` and skip the build. After `browser.newPage(...)`:

```js
const ISO_ENV = process.env.ISOLATION_ENV ? JSON.parse(process.env.ISOLATION_ENV) : null;
if (ISO_ENV) {
  await page.addInitScript((cfg) => { window.__beatmoEnv = Object.fromEntries(Object.entries(cfg).map(([k, v]) => [k, String(v)])); }, ISO_ENV);
}
```

- [ ] **Step 2: New scenarios.** Add to `SCENARIOS`:

```js
  { name: "stall-300", load: { stall: [700, 300] } },
  // Edit storms on row 0 (mute/unmute every 600 ms) and row 1 (volume steps
  // every 400 ms) with a 150 ms/500 ms stall underneath: edits must reach the
  // next not-started note even while the main thread stutters.
  { name: "edit-mute", edits: "mute", load: { stall: [500, 150] } },
  { name: "edit-vol", edits: "vol", load: { stall: [500, 150] } },
```

  In the run loop, after `startPlaybackProbe`, if `sc.edits` is set, run this edit loop concurrently with the wait, and pass `{ editMarks: marks }` to `stopPlaybackProbe`:

```js
const marks = [];
const editLoop = (async () => {
  const end = Date.now() + SECS * 1000 - 300;
  let k = 0;
  while (Date.now() < end) {
    if (sc.edits === "mute") {
      marks.push(await markEdit(page, { label: `mute#${k}`, kind: k % 2 === 0 ? "mute" : "unmute", channel: rows[0] }, "toggleMute(0)"));
      await page.waitForTimeout(600);
    } else {
      const v = [0.9, 0.5, 0.7, 0.3][k % 4];
      // Expected per-note vol = row vol × node vol; startup-demo nodes are
      // vol 1, so the expected note vol is v.
      marks.push(await markEdit(page, { label: `vol#${k}`, kind: "vol", channel: rows[1], value: v }, "setRowVolume(1, arg)", v));
      await page.waitForTimeout(400);
    }
    k++;
  }
})();
await page.waitForTimeout(SECS * 1000);
await editLoop;
```

  After the scenario, restore the row state: `toggleMute(0)` if the mute count was odd, and `setRowVolume(1, 1)`.

- [ ] **Step 3: Per-scenario JSON row gains** `probeEditStaleNotes`, `probeEditStaleMaxMS`, `probeNotesCancelled` (from `probeSummaryFields`), plus `arm` = `process.env.AB_ARM || "default"`. The output path becomes `bench-results/audio_isolation${process.env.AB_ARM ? "_" + process.env.AB_ARM : ""}.json`.

- [ ] **Step 4: Gates.** The `edit-*` scenarios are exempt from hard gates in the bench (they are measurements). Only `notes.dropped === 0` stays hard for them.

- [ ] **Step 5: `scripts/audio_lane_ab.mjs`.**

```js
#!/usr/bin/env node
// Interleaved A/B/C runner for the audio lane (spec §7). Arms run in rotation
// ROUNDS times; each run records load; runs above MAX_LOAD are discarded and
// retried once. Output: bench-results/audio_lane_ab_<date>.{json,md}.
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
const repo = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const ROUNDS = Number(process.env.ROUNDS ?? 3);
const MAX_LOAD = Number(process.env.MAX_LOAD ?? 15);
const SCEN = process.env.SCENARIOS ?? "idle,stall-150,stall-300,tab-synth,pan-storm,edit-mute,edit-vol";
const scratch = process.env.AB_SCRATCH; // dir holding baseline_site/ and lane.wasm
if (!scratch) throw new Error("AB_SCRATCH=<dir with baseline_site/ and lane.wasm> required");
const arms = [
  { name: "baseline", env: {}, extra: { SITE_DIR: path.join(scratch, "baseline_site") } },
  { name: "legacy", env: { BEATMO_AUDIO_LANE: "0" }, extra: { WASM_FILE: path.join(scratch, "lane.wasm") } },
  { name: "lane", env: { BEATMO_AUDIO_LANE: "1" }, extra: { WASM_FILE: path.join(scratch, "lane.wasm") } },
].filter((a) => !process.env.ARMS || process.env.ARMS.split(",").includes(a.name));
const load = () => Number(fs.readFileSync("/proc/loadavg", "utf8").split(" ")[0]);
const results = [];
for (let r = 0; r < ROUNDS; r++) {
  for (const arm of arms) {
    for (let attempt = 0; attempt < 2; attempt++) {
      const l0 = load();
      const res = spawnSync("node", ["src/js/webaudio_audio_isolation_bench.browser.test.js"], {
        cwd: repo, stdio: "inherit",
        env: { ...process.env, ...arm.extra, AB_ARM: arm.name, ISOLATION_SCENARIOS: SCEN, ISOLATION_ENV: JSON.stringify(arm.env), WASM_PREBUILT: "1" },
      });
      const l1 = load();
      const file = path.join(repo, "bench-results", `audio_isolation_${arm.name}.json`);
      const data = fs.existsSync(file) ? JSON.parse(fs.readFileSync(file, "utf8")) : null;
      const ok = data && Math.max(l0, l1) <= MAX_LOAD;
      results.push({ round: r, arm: arm.name, load: [l0, l1], exit: res.status, discarded: !ok, scenarios: data?.scenarios ?? [] });
      if (ok) break;
    }
  }
}
const date = new Date().toISOString().slice(0, 10);
fs.writeFileSync(path.join(repo, "bench-results", `audio_lane_ab_${date}.json`), JSON.stringify(results, null, 2));
// Markdown: per scenario × arm, median (min–max) of the key probe metrics.
const keys = ["probeArrivedLate", "probeLateOver20", "probeLagIdealP99MS", "probeIoiErrP95MS", "probeEditStaleNotes", "probeEditStaleMaxMS", "probeNotesMissed", "probeNotesDropped", "probeMasterClicks", "updateAvgMS", "drawAvgMS"];
const med = (v) => { const s = v.filter(Number.isFinite).sort((a, b) => a - b); return s.length ? s[Math.floor(s.length / 2)] : null; };
let md = `# Audio lane A/B — ${date}\n\nRounds ${ROUNDS}, max load ${MAX_LOAD}. Cells: median (min–max) over kept runs.\n\n`;
for (const sc of SCEN.split(",")) {
  md += `## ${sc}\n\n| metric | ${arms.map((a) => a.name).join(" | ")} |\n|---|${arms.map(() => "---").join("|")}|\n`;
  for (const k of keys) {
    const cells = arms.map((a) => {
      const v = results.filter((x) => x.arm === a.name && !x.discarded).map((x) => x.scenarios.find((s) => s.scenario === sc)?.[k]).filter((x) => x != null);
      return v.length ? `${med(v)} (${Math.min(...v)}–${Math.max(...v)})` : "n/a";
    });
    md += `| ${k} | ${cells.join(" | ")} |\n`;
  }
  md += "\n";
}
fs.writeFileSync(path.join(repo, "bench-results", `audio_lane_ab_${date}.md`), md);
console.log(md);
```

  Makefile:

```make
# Interleaved A/B/C of the audio lane (spec §7): frozen baseline build, new
# build lane-off, new build lane-on. AB_SCRATCH must hold baseline_site/ and
# lane.wasm (build it with: GOOS=js GOARCH=wasm go build -o $AB_SCRATCH/lane.wasm ./cmd).
bench-lane-ab:
	AB_SCRATCH=$(AB_SCRATCH) ROUNDS=$(or $(ROUNDS),3) GO=$(GO) node scripts/audio_lane_ab.mjs
```

- [ ] **Step 6: Record BEFORE.** Build the current tree to `$SCRATCH/lane.wasm`. Then run `ARMS=baseline,legacy ROUNDS=3 AB_SCRATCH=$SCRATCH node scripts/audio_lane_ab.mjs`. Expected: the two arms are statistically indistinguishable, which proves the harness is fair. Save as `bench-results/audio_lane_ab_before_<date>.md` (rename the outputs).

- [ ] **Step 7: Finish.** `node --check` both files. No commit.

---

## Phase 2 — Engine APIs (both platforms)

### Task 3: Clock and predictor primitives

**Files:**
- Modify: `src/go/internal/gamestate/state.go` (add `StepAtAudioTime`, `AnchorKey`)
- Modify: `src/go/core/engine/predictor.go`, `predictor_api.go`, `predictor_logic.go`, `predictor_paths.go`, `predictor_compute.go` (epoch + decisions)
- Test: `src/go/internal/gamestate/state_lane_test.go`, `src/go/core/engine/predictor_lane_test.go`

**Interfaces:**
- Produces:
  - `func (s *State) StepAtAudioTime(t float64, bpm, div int) (int, bool)`: floor step whose ideal audio time is ≤ t; `ok == false` when there is no audio anchor.
  - `func (s *State) AnchorKey() (beatBase, audioStart float64)`.
  - `func (p *Predictor) Epoch() uint64`: bumped by every call that can change an already-computed cell (`SetPaths`, `UpdateNode`, `DeleteNode`, `RebaseAt`, `resetBuffersLocked`). NOT bumped by `Ensure` extending forward.
  - `func (p *Predictor) DecisionAt(row, idx int) (model.NodeDecision, bool)`: the custom `Params.Logic` decision recorded for an audible cell; `ok == false` for cells without custom logic.

- [ ] **Step 1: Failing tests.**

```go
// src/go/internal/gamestate/state_lane_test.go
package gamestate

import ("testing"; "time")

func TestStepAtAudioTimeInvertsAudioTimeOfStep(t *testing.T) {
	var s State
	s.StartClock(time.Now(), 10.0) // audioStart = 10, beatBase = 0
	for idx := 0; idx < 64; idx++ {
		at := s.AudioTimeOfStep(idx, 4, 120)
		got, ok := s.StepAtAudioTime(at+1e-9, 120, 4)
		if !ok || got != idx {
			t.Fatalf("idx %d: StepAtAudioTime(%.6f)=%d ok=%v", idx, at, got, ok)
		}
	}
	var none State
	if _, ok := none.StepAtAudioTime(1, 120, 4); ok {
		t.Fatal("no anchor must report ok=false")
	}
}
```

```go
// src/go/core/engine/predictor_lane_test.go
package engine

import ("testing"; "github.com/ingyamilmolinar/beatmo/core/model")

func TestPredictorEpochBumpsOnChangeNotOnExtend(t *testing.T) {
	p, nodes := newLoopPredictorForTest(t) // helper: 1 row, 4-node loop (see Step 3)
	e0 := p.Epoch()
	p.Ensure(64)
	if p.Epoch() != e0 { t.Fatal("Ensure must not bump the epoch") }
	n := nodes[0]; n.Params.Volume = 0.5
	p.UpdateNode(n.ID, n)
	if p.Epoch() == e0 { t.Fatal("UpdateNode must bump the epoch") }
}

func TestPredictorDecisionAtRecordsCustomLogic(t *testing.T) {
	p, nodes := newLoopPredictorForTest(t)
	n := nodes[1]
	n.Params.Logic = func(c model.NodeContext) model.NodeDecision { return model.NodeDecision{VolumeMul: 0.25, PitchDelta: 2} }
	p.UpdateNode(n.ID, n)
	p.Ensure(16)
	dec, ok := p.DecisionAt(0, 1)
	if !ok || dec.VolumeMul != 0.25 || dec.PitchDelta != 2 { t.Fatalf("DecisionAt(0,1)=%+v ok=%v", dec, ok) }
	if _, ok := p.DecisionAt(0, 0); ok { t.Fatal("cell without custom logic must report ok=false") }
}
```

- [ ] **Step 2: Run them to see them fail.**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestStepAtAudioTime' ./internal/gamestate/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPredictorEpoch|TestPredictorDecisionAt' ./core/engine/
```

Expected: compile errors (undefined methods / helper).

- [ ] **Step 3: Implement.**

`state.go`:

```go
// StepAtAudioTime is the inverse of AudioTimeOfStep on the audio clock: the
// floor subdivision whose ideal start is ≤ t. ok is false without an audio
// anchor (the audio lane needs the audio clock; there is no wall fallback).
func (s *State) StepAtAudioTime(t float64, bpm, div int) (int, bool) {
	if s.audioStart <= 0 || bpm <= 0 {
		return 0, false
	}
	if div <= 0 {
		div = 1
	}
	beats := s.beatBase + (t-s.audioStart)*float64(bpm)/60.0
	return int(math.Floor(beats*float64(div) + 1e-9)), true
}

// AnchorKey exposes the clock anchor for change detection (audio lane
// fingerprints): any re-anchor (play, seek, BPM) changes it.
func (s *State) AnchorKey() (beatBase, audioStart float64) { return s.beatBase, s.audioStart }
```

Predictor:
- In `predictor.go`, add the fields `epoch atomic.Uint64` and `decisions map[[2]int]model.NodeDecision` (init in the constructor).
- Add `func (p *Predictor) Epoch() uint64 { return p.epoch.Load() }`.
- Add `p.epoch.Add(1)` at the top of the locked sections of `SetPaths`, `UpdateNode`, `DeleteNode`, `RebaseAt`, and inside `resetBuffersLocked`. In the same places, clear `decisions`: `p.decisions = map[[2]int]model.NodeDecision{}`.
- In `evalAudible`, for regular nodes with `Params.Logic != nil` that end audible, call `p.decisions[[2]int{row, idx}] = dec`. In every other return path for a regular cell, call `delete(p.decisions, [2]int{row, idx})`.
- `DecisionAt` reads the map under `p.mu.RLock()`.
- In `predictor_lane_test.go` add the helper `newLoopPredictorForTest(t)`. It builds 4 regular nodes on one looping row via `SetPaths([][]model.BeatInfo{{…4 infos…}}, []bool{true}, []int{0}, nodes)`, with `NewPredictor` + `SetPaths` mirroring the existing `predictor_api_test.go` setup (copy its constructor call), and returns `(p, []model.Node)`.

- [ ] **Step 4: Run the tests and the existing packages.**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/gamestate/ ./core/engine/
```

Expected: PASS, including the existing `TestDirtyRebuildEquivalentToFullWalk`.

- [ ] **Step 5: Finish.** gofmt the touched files. No commit.

### Task 4: Go audio API + desktop mixer cancel/stop

**Files:**
- Modify: `src/go/internal/audio/types.go` (`BatchParam.VoiceID uint64`)
- Create: `src/go/internal/audio/lane_api.go` (no build tag: `VoiceRevision` + `bumpVoiceRev`)
- Create: `src/go/internal/audio/lane_desktop.go` (`//go:build !test && !js`)
- Create: `src/go/internal/audio/lane_wasm.go` (`//go:build js && wasm && !test`)
- Create: `src/go/internal/audio/lane_stub.go` (`//go:build test`)
- Modify: `engine_mixer.go` (`voiceState.vid`, `Schedule(..., vid)`, locked `m.pos` read, trigger notify deferred to first render), `engine_stop.go` (`cancelVoices`, `stopLaneAt` event queue, `cancelAllLaneFuture`, `processBlock` applies stop events), `engine_play.go` (`playParamsIdentity` gains `vid`), `batch_desktop.go` (pass `r.VoiceID`)
- Modify: `sample_edit_descriptor.go` (`ApplySampleEditLive`, `SetSampleEdit`, `ClearSampleEdit` call `bumpVoiceRev(instID)`), `sample_pcm_notjs.go` + `sample_pcm_wasm.go` (`RegisterSamplePCM` → `bumpVoiceRev(id)`)
- Test: `src/go/internal/audio/lane_mixer_test.go` (`//go:build !test && !js`, real build), `src/go/internal/audio/voice_revision_test.go` (no tag)

**Interfaces:**
- Produces (package `audio`, all platforms):
  - `func CancelVoices(ids []uint64)`: silently remove lane voices that have not started.
  - `func StopLaneAt(inst string, t float64)`: at audio time `t`, fade out lane voices of `inst` that started before `t`.
  - `func CancelAllLaneFuture()`: remove every lane voice that has not started.
  - `func VoiceRevision(inst string) uint64`: `InstrumentParamsRev(inst) + RecipeDefaultsRev() + extraRev[inst]`.
  - `BatchParam.VoiceID` honored by `PlayBatch` on every platform.
- Consumes: nothing from earlier tasks.

- [ ] **Step 1: Failing tests.**

```go
// src/go/internal/audio/voice_revision_test.go
package audio

import "testing"

func TestVoiceRevisionMovesOnSampleEdit(t *testing.T) {
	r0 := VoiceRevision("kick")
	ApplySampleEditLive("kick", SampleEdit{Gain: 0.5})
	t.Cleanup(func() { ClearSampleEdit("kick") })
	if VoiceRevision("kick") == r0 {
		t.Fatal("a sample edit must change the voice revision")
	}
}
```

```go
// src/go/internal/audio/lane_mixer_test.go
//go:build !test && !js

package audio

import "testing"

// laneTestMixer returns a mixer that is never attached to a device: tests
// drive processBlock directly through renderForTest.
func laneTestMixer() *mixer {
	return &mixer{workBuf: make([]float64, blockSize), voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize), postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize), instSlots: map[string]int{}}
}

func (m *mixer) renderForTest(samples int) []byte {
	p := make([]byte, samples*2)
	_, _ = m.Read(p)
	return p
}

func peakOf(p []byte) int {
	pk := 0
	for i := 0; i+1 < len(p); i += 2 {
		v := int(int16(uint16(p[i]) | uint16(p[i+1])<<8))
		if v < 0 { v = -v }
		if v > pk { pk = v }
	}
	return pk
}

func TestMixerCancelBeforeStartIsSilent(t *testing.T) {
	m := laneTestMixer()
	m.Schedule("lanetest", newTestSineVoice(), 4096, -1, -1, 7)
	m.cancelVoices([]uint64{7})
	if pk := peakOf(m.renderForTest(16384)); pk != 0 {
		t.Fatalf("cancelled voice rendered (peak %d)", pk)
	}
}

func TestMixerCancelAfterStartDoesNothing(t *testing.T) {
	m := laneTestMixer()
	m.Schedule("lanetest", newTestSineVoice(), 0, -1, -1, 9)
	m.renderForTest(1024)
	m.cancelVoices([]uint64{9})
	if pk := peakOf(m.renderForTest(1024)); pk == 0 {
		t.Fatal("cancel must not touch a voice that already started (edit semantics (a))")
	}
}

func TestMixerCancelAllFutureSparesPreviews(t *testing.T) {
	m := laneTestMixer()
	m.Schedule("lanetest", newTestSineVoice(), 2048, -1, -1, 11) // lane
	m.Schedule("lanetest", newTestSineVoice(), 2048, -1, -1, 0)  // preview
	m.cancelAllLaneFuture()
	if pk := peakOf(m.renderForTest(8192)); pk == 0 {
		t.Fatal("the preview (vid 0) must survive CancelAllLaneFuture")
	}
}

func TestMixerStopLaneAtSparesLaterVoices(t *testing.T) {
	m := laneTestMixer()
	m.Schedule("lanetest", newTestSineVoice(), 0, -1, -1, 21)     // sounding before t
	m.Schedule("lanetest", newTestSineVoice(), 8192, -1, -1, 22)  // starts after t
	m.stopLaneAtSample("lanetest", 4096)
	m.renderForTest(4096 + 2048) // past t + fade
	if !m.hasVoiceForTest(22) { t.Fatal("voice starting after t must survive StopLaneAt") }
	if m.hasVoiceForTest(21) { t.Fatal("voice started before t must be faded out and removed") }
}
```

  If `processBlock` dereferences the voice's channel (`vs.ch`) or needs a registered instrument, register `"lanetest"` the way the existing real-build mixer tests do. Read `mixer_signal_test.go` / `engine_mixer_test.go` for their setup helper and reuse it rather than hand-building one.

- [ ] **Step 2: Run them to see them fail.**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestVoiceRevision ./internal/audio/ ; CGO_ENABLED=1 xvfb-run -a ../../.tools/go/bin/go test -run 'TestMixer(Cancel|StopLaneAt)' ./internal/audio/
```

Expected: compile errors.

- [ ] **Step 3: Implement.**

`lane_api.go`:

```go
package audio

import "sync"

// Voice revision: everything that changes the rendered PCM of an
// instrument's next note. The audio lane folds it into each committed note's
// identity so a synth/sampler edit replaces the not-yet-started notes.
var (
	voiceRevMu sync.Mutex
	voiceRevX  = map[string]uint64{}
)

func bumpVoiceRev(inst string) {
	voiceRevMu.Lock()
	voiceRevX[inst]++
	voiceRevMu.Unlock()
}

// VoiceRevision combines the param overlay/binding revision, the recipe
// defaults revision and the sample-edit / PCM revision of inst.
func VoiceRevision(inst string) uint64 {
	voiceRevMu.Lock()
	x := voiceRevX[inst]
	voiceRevMu.Unlock()
	return InstrumentParamsRev(inst)*1_000_003 + RecipeDefaultsRev()*7919 + x
}
```

Mixer (`engine_mixer.go`, `engine_stop.go`):
- `voiceState` gains `vid uint64`. `Schedule(id string, v Voice, delaySamples, row, abs int, vid uint64)` (update every caller: `playParamsIdentity` and the existing tests that call `mix.Schedule`).
- Read `m.pos` under `m.mu`:

```go
	m.mu.Lock()
	start := m.pos + delaySamples
	m.mu.Unlock()
	vs := &voiceState{start: start, id: id, slot: slot, v: v, ch: ch, row: row, abs: abs, vid: vid}
```

- Move the analyzer `NotifyTrigger` + scope/export `PushSamples` block from `Schedule` into `processBlock`, at the first render of the voice: alongside `publishEmitReport`, guarded by a new `vs.notified bool`. A cancelled or early-committed lane note must not light meters ~H early. Keep the pooled-buffer logic unchanged.
- Add to `engine_stop.go`:

```go
// cancelVoices silently removes lane voices that have not produced a sample
// yet. Lock order matches Read: pendingMu → mu.
func (m *mixer) cancelVoices(ids []uint64) {
	if len(ids) == 0 { return }
	set := make(map[uint64]struct{}, len(ids))
	for _, id := range ids { if id != 0 { set[id] = struct{}{} } }
	m.pendingMu.Lock()
	keep := m.pendingAdd[:0]
	for _, vs := range m.pendingAdd {
		if _, ok := set[vs.vid]; ok { continue }
		keep = append(keep, vs)
	}
	m.pendingAdd = keep
	m.mu.Lock()
	for i := 0; i < len(m.voices); i++ {
		vs := m.voices[i]
		if _, ok := set[vs.vid]; ok && vs.start > m.pos {
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices = m.voices[:len(m.voices)-1]
			i--
		}
	}
	m.mu.Unlock()
	m.pendingMu.Unlock()
}

// cancelAllLaneFuture removes every lane voice (vid != 0) not yet started.
func (m *mixer) cancelAllLaneFuture() {
	m.pendingMu.Lock()
	keep := m.pendingAdd[:0]
	for _, vs := range m.pendingAdd { if vs.vid == 0 { keep = append(keep, vs) } }
	m.pendingAdd = keep
	m.mu.Lock()
	for i := 0; i < len(m.voices); i++ {
		if vs := m.voices[i]; vs.vid != 0 && vs.start > m.pos {
			m.voices[i] = m.voices[len(m.voices)-1]
			m.voices = m.voices[:len(m.voices)-1]
			i--
		}
	}
	m.mu.Unlock()
	m.pendingMu.Unlock()
}

type laneStopEvent struct { inst string; atSample int }

// stopLaneAtSample queues a stop applied by processBlock when the block
// containing atSample renders: lane voices of inst that started before it
// get RequestStop (anti-pop fade); voices starting at/after it are spared.
func (m *mixer) stopLaneAtSample(inst string, atSample int) {
	m.mu.Lock()
	m.laneStops = append(m.laneStops, laneStopEvent{inst, atSample})
	m.mu.Unlock()
}
```

  Add the field `laneStops []laneStopEvent` to `mixer`. At the top of `processBlock` (under `m.mu`, which the caller already holds), apply due events:

```go
	if len(m.laneStops) > 0 {
		blockEnd := m.pos + offset + blockLen
		keep := m.laneStops[:0]
		for _, ev := range m.laneStops {
			if ev.atSample >= blockEnd { keep = append(keep, ev); continue }
			for _, vs := range m.voices {
				if vs.vid != 0 && vs.id == ev.inst && vs.start < ev.atSample {
					if ap, ok := vs.v.(*antiPopVoice); ok { ap.RequestStop() }
				}
			}
		}
		m.laneStops = keep
	}
```

  Add `func (m *mixer) hasVoiceForTest(vid uint64) bool`: scan `voices` + `pendingAdd` under the locks. Put it in `lane_mixer_test.go` (test file, same package).

`lane_desktop.go`:

```go
//go:build !test && !js

package audio

func CancelVoices(ids []uint64) { if mix != nil { mix.cancelVoices(ids) } }
func CancelAllLaneFuture()      { if mix != nil { mix.cancelAllLaneFuture() } }

// StopLaneAt converts audio time t (the Now() clock) to a mixer sample
// position through the same delay math playParamsIdentity uses.
func StopLaneAt(inst string, t float64) {
	if mix == nil { return }
	d := t - Now()
	if d < 0 { d = 0 }
	mix.mu.Lock()
	at := mix.pos + int(d*float64(sampleRate))
	mix.mu.Unlock()
	mix.stopLaneAtSample(inst, at)
}
```

`engine_play.go`: `playParamsIdentity(id, vol, pitch, dur, row, abs, vid uint64, when ...float64)`; `PlayParamsAt` passes 0; add `PlayLaneAt(id string, vol, pitch, dur, when float64, row, abs int, vid uint64)` calling `playParamsIdentity(..., vid, when)`. `batch_desktop.go` calls `PlayLaneAt` when `r.VoiceID != 0`, otherwise the existing `PlayParamsAt`.

`lane_stub.go` (`//go:build test`): no-op `CancelVoices`, `StopLaneAt`, `CancelAllLaneFuture`.

`lane_wasm.go` (`//go:build js && wasm && !test`):

```go
package audio

import "syscall/js"

func CancelVoices(ids []uint64) {
	fn := js.Global().Get("cancelLaneVoices")
	if !fn.Truthy() || len(ids) == 0 { return }
	arr := make([]any, len(ids))
	for i, v := range ids { arr[i] = float64(v) } // vids < 2^53 by construction
	fn.Invoke(js.ValueOf(arr))
}
func StopLaneAt(inst string, t float64) {
	if fn := js.Global().Get("stopLaneAt"); fn.Truthy() { fn.Invoke(inst, t) }
}
func CancelAllLaneFuture() {
	if fn := js.Global().Get("cancelAllLaneFuture"); fn.Truthy() { fn.Invoke() }
}
```

`batch_wasm.go`: add `vids []any` to `batchScratch` (`ensureLen` + fill `s.vids[i] = float64(r.VoiceID)`) and pass `jsVids` as the **11th** argument of `playSoundsBatchFlat`. In the object fallback, set `o.Set("vid", float64(r.VoiceID))`.

- [ ] **Step 4: Run the tests.**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ && CGO_ENABLED=1 xvfb-run -a ../../.tools/go/bin/go test -race -run 'TestMixer' ./internal/audio/ && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/audio/
```

Expected: PASS, race-clean, and the WASM build compiles.

- [ ] **Step 5: Finish.** No commit.

### Task 5: audio.js lane registry (vids, tombstones, cancel, stopAt, cancelAll)

**Files:**
- Modify: `src/js/audio.js`
- Create: `src/js/audio_lane_cancel.browser.test.js`
- Modify: `src/go/internal/ui/js_exports_catalogue_drift_test.go` (`cancelLaneVoices`, `stopLaneAt`, `cancelAllLaneFuture`), `src/js/wasm_bridge_smoke.browser.test.js` (existence-only entries)

**Interfaces:**
- Consumes: Task 4's Go side calls `cancelLaneVoices(vidsArray)`, `stopLaneAt(inst, t)`, `cancelAllLaneFuture()`, and passes `vids` as the 11th `playSoundsBatchFlat` argument.
- Produces (window): those three functions plus `laneVoiceStateForTest(vid) → 'pending'|'scheduled'|'started'|'cancelled'|'unknown'`.

- [ ] **Step 1: Failing browser test** `src/js/audio_lane_cancel.browser.test.js`. It uses the real `audio.js` in a minimal page (pattern from `pitchshift_webaudio_fallback.browser.test.js`) plus the playback probe on master:

```js
// audio_lane_cancel.browser.test.js — JS Test Charter §1 (WebAudio behavior):
// lane voices (vid > 0) can be cancelled silently before they start, a
// tombstoned deferred replay never plays, stopLaneAt fades only sources that
// started before t, and cancelAllLaneFuture spares previews (vid 0).
import { chromium } from "playwright";
import http from "http"; import fs from "fs"; import path from "path"; import { fileURLToPath } from "url";
const jsDir = path.dirname(fileURLToPath(import.meta.url));
const HTML = `<!doctype html><meta charset="utf-8"><script type="module">
  import './audio.js'; window.__ready = true;</script>`;
const server = http.createServer((req, res) => {
  if (req.url === "/") { res.writeHead(200, { "Content-Type": "text/html" }); res.end(HTML); return; }
  fs.readFile(path.join(jsDir, req.url.slice(1).split("?")[0]), (e, d) => {
    if (e) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": req.url.endsWith(".js") ? "application/javascript" : "application/octet-stream" }); res.end(d);
  });
});
await new Promise((r) => server.listen(0, r));
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
let fail = 0;
const check = (c, m) => { if (c) console.log("ok   " + m); else { fail++; console.error("FAIL " + m); } };
try {
  const page = await browser.newPage();
  await page.goto(`http://localhost:${server.address().port}/`);
  await page.waitForFunction(() => window.__ready);
  const r = await page.evaluate(async () => {
    document.dispatchEvent(new Event("pointerdown"));
    await window.audioReady;
    const ctx = window.__audioCtx;
    await startAudioProbe({ channels: false });
    const t0 = ctx.currentTime;
    const ev = (vid, when) => playSoundsBatchFlat(["kick"], [1], [0], [1], [when], [true], [0], [vid], [when], [when], [vid]);
    ev(101, t0 + 0.40);          // cancelled well before start → silent
    ev(102, t0 + 0.80);          // kept → heard
    ev(0,   t0 + 0.90);          // preview-like (vid 0) → survives cancelAll
    ev(103, t0 + 1.20);          // removed by cancelAllLaneFuture
    await new Promise((r) => setTimeout(r, 100));
    cancelLaneVoices([101]);
    const s101 = laneVoiceStateForTest(101);
    await new Promise((r) => setTimeout(r, 750));
    cancelAllLaneFuture();
    await new Promise((r) => setTimeout(r, 700));
    const raw = await stopAudioProbe();
    const onsets = (raw.reports.__master__ || []).flatMap((x) => x.onsets.map((o) => o.f / raw.sampleRate - t0));
    return { s101, onsets, clicks: (raw.reports.__master__ || []).reduce((a, x) => a + x.clicks.length, 0) };
  });
  const near = (t) => r.onsets.some((o) => Math.abs(o - t) < 0.02);
  check(r.s101 === "cancelled", `cancelled voice state (${r.s101})`);
  check(!near(0.40), "cancelled voice is silent");
  check(near(0.80), "kept lane voice sounds");
  check(near(0.90), "vid-0 voice survives cancelAllLaneFuture");
  check(!near(1.20), "cancelAllLaneFuture removed the future lane voice");
  check(r.clicks === 0, `no clicks (${r.clicks})`);
} finally { await browser.close(); server.close(); }
if (fail) process.exit(1);
console.log("PASS audio_lane_cancel");
```

  Add a second evaluate block for the tombstone + stopLaneAt cases:
  - Tombstone: force a cold miss with `evictRenderCache?.("snare")` if exported, otherwise use an instrument never warmed. Enqueue vid 201 at t0 + 0.3, cancel it immediately while the render is in flight, and assert no onset near 0.3 after the render lands.
  - stopLaneAt: start vid 301 at t0 + 0.1 (sustained instrument `organ-church`) and vid 302 at t0 + 0.6. Call `stopLaneAt("organ-church", t0 + 0.4)`. Assert an onset near 0.6, and master RMS between 0.45 and 0.55 at most 1 % of the 0.1 note's level.

- [ ] **Step 2: Run it to see it fail.** Run `GO=… node src/js/audio_lane_cancel.browser.test.js`. Expected: FAIL (`laneVoiceStateForTest` / `cancelLaneVoices` not defined).

- [ ] **Step 3: Implement in `audio.js`.**
  - `playSoundsBatchFlat(ids, vols, pitches, durs, whens, hasWhen, rows, abss, ideals, grids, vids)`: `ev.vid = Number.isFinite(vids?.[i]) ? vids[i] : 0`. `sanitizeAudioEvent` carries `vid`, and `carryTiming` copies it.
  - Registry near `trackSource`:

```js
// ───────── Audio lane registry (vid > 0) ─────────
// Lane notes are committed up to the horizon ahead and may be cancelled or
// replaced before they start. vid 0 = preview / UI sound, never touched here.
const LANE_CANCEL_GUARD_SEC = 0.008;
const laneVoices = new Map();      // vid → { src, gain, inst, when }
const laneTombstones = new Set();  // vids cancelled while queued/deferred
let laneGeneration = 0;            // bumped by cancelAllLaneFuture

function laneRegister(vid, src, gain, inst, when) {
  if (!vid) return;
  laneVoices.set(vid, { src, gain, inst, when });
  src.addEventListener('ended', () => laneVoices.delete(vid), { once: true });
}

function laneCancelOne(vid, now) {
  const v = laneVoices.get(vid);
  if (!v) { laneTombstones.add(vid); return false; } // not started yet anywhere: drop on arrival
  if (v.when <= now + LANE_CANCEL_GUARD_SEC) return true; // effectively started: leave it
  try { v.src.stop(0); } catch (_) {}
  try { v.src.disconnect(); v.gain.disconnect(); } catch (_) {}
  laneVoices.delete(vid);
  if (audioProbe && v.pk) audioProbe.noteCancelled(v.pk);
  return false;
}

window.cancelLaneVoices = (vids) => {
  if (!hasCtx() || !vids) return;
  const now = ctx.currentTime;
  for (let i = 0; i < vids.length; i++) laneCancelOne(Number(vids[i]), now);
};

window.stopLaneAt = (inst, t) => {
  if (!hasCtx()) return;
  for (const v of laneVoices.values()) {
    if (v.inst !== inst || !(v.when < t)) continue;
    const g = v.gain.gain;
    const at = Math.max(t, ctx.currentTime);
    try {
      g.cancelScheduledValues(at);
      g.setValueAtTime(CHAIN_SPEC.voiceHeadroom, at);
      g.linearRampToValueAtTime(0, at + 0.02);
      v.src.stop(at + 0.021);
    } catch (_) {}
  }
};

window.cancelAllLaneFuture = () => {
  laneGeneration++;
  if (!hasCtx()) return;
  const now = ctx.currentTime;
  for (const vid of Array.from(laneVoices.keys())) laneCancelOne(vid, now);
  lastScheduledPlayWhen = -1;
};

window.laneVoiceStateForTest = (vid) => {
  if (laneTombstones.has(vid)) return 'cancelled';
  const v = laneVoices.get(vid);
  if (!v) return 'unknown';
  return v.when <= (hasCtx() ? ctx.currentTime : 0) ? 'started' : 'scheduled';
};
```

  - In `enqueueAudioEvents`, stamp lane events with the current generation: `if (ev.vid) ev.laneGen = laneGeneration;`.
  - At the top of `processAudioEvent`:

```js
  if (ev.vid) {
    if (laneTombstones.has(ev.vid)) { laneTombstones.delete(ev.vid); if (audioProbe && ev.pk) audioProbe.noteCancelled(ev.pk); return; }
    if (ev.laneGen !== laneGeneration) { if (audioProbe && ev.pk) audioProbe.noteCancelled(ev.pk); return; }
  }
```

  - `carryTiming` copies `vid` and `laneGen`.
  - In `startRenderVoice` and the raw-sample branch, after `src.start(when)`, call `laneRegister(vid, src, fadeGain, id, when)` with `vid` threaded from the event: add a `vid` param to `startRenderVoice` (after `path`), and store `pk` on the registry entry so the probe hook works.
  - Fix `stopSound`'s fade: replace `fadeGain.gain.setValueAtTime(fadeGain.gain.value, now)` with `setValueAtTime(Math.min(fadeGain.gain.value, CHAIN_SPEC.voiceHeadroom), now)`. A future source's gain must never jump to 1.0 (click).
  - Fix `stopSound`'s tracking race: replace the delayed `set.clear()` with deleting only the sources it stopped, `for (const s of stopped) set.delete(s)`.

- [ ] **Step 4: Run the tests.**

```
GO=… node src/js/audio_lane_cancel.browser.test.js && GO=… node src/js/timing_traces.browser.test.js && node --check src/js/audio.js && cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestJSExportCatalogueDrift ./internal/ui/
```

Expected: all PASS.

- [ ] **Step 5: Finish.** Add the new test file to the default (parallel) browser suite; no config change is needed because of the `*.browser.test.js` glob. No commit.

---

## Phase 3 — The lane core (deterministic Go)

### Task 6: `laneNote` and `laneNoteAt`

**Files:**
- Create: `src/go/internal/ui/audio_lane_note.go`
- Test: `src/go/internal/ui/audio_lane_note_test.go` (`//go:build test`)

**Interfaces:**
- Consumes: `Predictor.AudibleAt/TriggeredAt/DecisionAt` (Task 3), `audio.VoiceRevision` (Task 4), `State.AudioTimeOfStep`, `evalNodeParamsOnly`, `nodeNudgeSecondsAt`, `seqBeatInfoAtRow`.
- Produces:

```go
type laneKind uint8
const (
	laneSound laneKind = iota + 1 // a voice to play
	laneStop                      // a mute node: StopLaneAt(Inst, Time)
)
type laneNote struct {
	Row, Abs   int
	Kind       laneKind
	Inst       string
	Vol, Pitch, Dur float64
	Time       float64 // audio-clock start (grid + nudge + AudioOffsetSec)
	Grid       float64 // grid + nudge, no offset (probe "predicted time")
	Hash       uint64  // identity: any field change ⇒ different Hash
}
// laneCtx is computed once per tick (not per note).
type laneCtx struct {
	snap    *seqPathSnapshot
	bpm, div int
	anySolo bool
	offset  float64
}
func (s *playbackSlate) laneContext() (laneCtx, bool)
func (s *playbackSlate) laneNoteAt(c *laneCtx, row, abs int) (laneNote, bool) // ok=false: nothing at (row,abs)
```

- [ ] **Step 1: Failing test.** It builds a test game with a 2-row circuit and checks `laneNoteAt` against what the legacy sequencer dispatches (`scheduleHook` + `SetPlayFunc` capture) over 64 steps, including a mute node, a probability node, a group rule and a muted row:

```go
//go:build test

package ui

import ("math"; "testing")

func TestLaneNoteAtMatchesLegacyDispatch(t *testing.T) {
	g := newLaneCircuitGame(t) // helper in this file: 2 rows, 8-node loops, node 3 = mute on row 0,
	                            // node 5 probability 0.5 on row 1, a group pitch rule on row 1; plays at 120 BPM div 4
	type got struct{ inst string; vol, pitch float64 }
	legacy := map[[2]int]got{}
	g.scheduleHook = nil
	captureLegacyDispatch(t, g, 64, func(row, abs int, inst string, vol, pitch float64) { legacy[[2]int{row, abs}] = got{inst, vol, pitch} })
	c, ok := g.playbackSlate.laneContext()
	if !ok { t.Fatal("no lane context while playing") }
	for row := 0; row < 2; row++ {
		for abs := 0; abs < 64; abs++ {
			n, ok := g.playbackSlate.laneNoteAt(&c, row, abs)
			want, played := legacy[[2]int{row, abs}]
			if played != (ok && n.Kind == laneSound) {
				t.Fatalf("(%d,%d): legacy played=%v lane sound=%v", row, abs, played, ok && n.Kind == laneSound)
			}
			if played && (n.Inst != want.inst || math.Abs(n.Vol-want.vol) > 1e-12 || math.Abs(n.Pitch-want.pitch) > 1e-12) {
				t.Fatalf("(%d,%d): lane %+v vs legacy %+v", row, abs, n, want)
			}
		}
	}
	// Mute cell → laneStop.
	n, ok := g.playbackSlate.laneNoteAt(&c, 0, 3)
	if !ok || n.Kind != laneStop { t.Fatalf("mute cell: %+v ok=%v", n, ok) }
	// Muting row 1 silences its notes.
	g.drum.Rows[1].Muted = true
	c, _ = g.playbackSlate.laneContext()
	for abs := 0; abs < 64; abs++ {
		if n, ok := g.playbackSlate.laneNoteAt(&c, 1, abs); ok && n.Kind == laneSound { t.Fatalf("muted row produced %+v", n) }
	}
}
```

  Also add `laneTestNow(g *Game) *float64` to this test file: it returns the pointer behind the fake audio clock that `newLaneCircuitGame` installs with `audio.SetNowForTest`. Tasks 8 and 10 advance time through it.

  `captureLegacyDispatch` drives `g.seqScheduleTime()` step by step with the existing test seams (`SetPlayFunc` + `scheduleAbsForTest` from `game_node_logic_test.go`; reuse it instead of re-implementing). `newLaneCircuitGame` builds the circuit with the existing helpers used by `game_node_logic_test.go` and `mute_node_audio_test.go` (`buildTestGame`, `tryAddNode`, `addEdge`, `graph.SetNodeParams`, `updateBeatInfos`, `SetPlayingForTest`, `state.StartClock(time.Now(), 1.0)` with `audio.SetNowForTest`).

- [ ] **Step 2: Run it to see it fail.** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLaneNoteAt ./internal/ui/`. Expected: compile error (undefined `laneNoteAt`).

- [ ] **Step 3: Implement** `audio_lane_note.go`:

```go
package ui

import (
	"hash/fnv"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (s *playbackSlate) laneContext() (laneCtx, bool) {
	g := s.g
	snap := g.seqPathSnapshot()
	if snap == nil || g.drum == nil || g.engine == nil || g.engine.Predictor == nil {
		return laneCtx{}, false
	}
	bpm := g.state.AppliedBPM()
	if bpm <= 0 { bpm = g.bpm }
	div := g.grid.MaxDiv()
	if bpm <= 0 || div <= 0 { return laneCtx{}, false }
	any := false
	for _, r := range g.drum.Rows { if r.Solo { any = true; break } }
	return laneCtx{snap: snap, bpm: bpm, div: div, anySolo: any, offset: RuntimeProf().AudioOffsetSec}, true
}

// laneNoteAt is the PURE note function: what (row, abs) sounds like under
// current state. No counters, no gates, no side effects — the audio lane may
// call it any number of times for the same cell.
func (s *playbackSlate) laneNoteAt(c *laneCtx, row, abs int) (laneNote, bool) {
	g := s.g
	if row < 0 || row >= len(g.drum.Rows) { return laneNote{}, false }
	info := seqBeatInfoAtRow(c.snap, row, abs)
	inst := g.drum.Rows[row].Instrument
	pred := g.engine.Predictor
	grid := g.state.AudioTimeOfStep(abs, c.div, c.bpm) + g.nodeNudgeSecondsAt(row, abs, info)
	n := laneNote{Row: row, Abs: abs, Inst: inst, Grid: grid, Time: grid + c.offset}
	switch info.NodeType {
	case model.NodeTypeMute:
		if !pred.TriggeredAt(row, abs) { return laneNote{}, false }
		n.Kind = laneStop
	case model.NodeTypeRegular:
		if !pred.AudibleAt(row, abs) { return laneNote{}, false }
		r := g.drum.Rows[row]
		if r.Muted || (c.anySolo && !r.Solo) { return laneNote{}, false }
		if !g.drum.IsInstrumentAvailable(inst) && g.playFn == nil && g.scheduleHook == nil { return laneNote{}, false }
		vol, pitch, dur := g.evalNodeParamsOnly(row, abs, info)
		if dec, ok := pred.DecisionAt(row, abs); ok {
			vol, pitch, dur = applyNodeDecision(vol, pitch, dur, dec)
		}
		if vol < 0 { vol = 0 }
		n.Kind, n.Vol, n.Pitch, n.Dur = laneSound, vol, pitch, dur
	default:
		return laneNote{}, false
	}
	n.Hash = laneNoteHash(&n, uint64(info.NodeID), audio.VoiceRevision(inst))
	return n, true
}

func laneNoteHash(n *laneNote, node, voiceRev uint64) uint64 {
	h := fnv.New64a()
	var b [8]byte
	w := func(x uint64) { for i := 0; i < 8; i++ { b[i] = byte(x >> (8 * i)) }; _, _ = h.Write(b[:]) }
	_, _ = h.Write([]byte(n.Inst))
	w(uint64(n.Kind)); w(node); w(voiceRev)
	w(math.Float64bits(n.Vol)); w(math.Float64bits(n.Pitch)); w(math.Float64bits(n.Dur))
	w(math.Float64bits(math.Round(n.Time*1e6) / 1e6)) // µs: float noise must not churn
	return h.Sum64()
}
```

  Add the RuntimeProfile fields (constraint table) now, so `RuntimeProf().AudioOffsetSec` exists. The builders get the browser/desktop values from Global Constraints, with `AudioLaneEnabled: false` on both until Task 12.

- [ ] **Step 4: Run the test.** Same command. Expected: PASS.

- [ ] **Step 5: Finish.** No commit.

### Task 7: `audioLane` reconciler (fake source + fake sink)

**Files:**
- Create: `src/go/internal/ui/audio_lane.go`
- Test: `src/go/internal/ui/audio_lane_test.go` (`//go:build test`, no Game)

**Interfaces:**
- Consumes: `laneNote`, `laneKind` (Task 6).
- Produces:

```go
type audioSink interface {
	Schedule(vid uint64, n laneNote)
	Cancel(vids []uint64)
	StopAt(inst string, t float64)
	CancelAllFuture()
}
// laneSource is everything the lane reads; the Game implements it in Task 8,
// tests implement it with a scripted grid.
type laneSource interface {
	Playing() bool
	Now() float64
	TransportGen() uint64
	GlobalKey() uint64           // inputs shared by all rows (paths, predictor epoch, BPM, anchor, solo, offset)
	Rows() int
	RowKey(row int) uint64       // per-row inputs (instrument, volume, mute, solo, VoiceRevision)
	Steps(from, to float64) (lo, hi int, ok bool) // abs range whose ideal grid time ∈ [from, to]
	NoteAt(row, abs int) (laneNote, bool)
	VoiceRevOnly(row int) bool   // true when ONLY the row's VoiceRevision changed since last tick (rate-limit path)
}
type laneConfig struct {
	Horizon, Guard, LateTol, FullReconcile, VoiceRecommitMin float64
}
type laneStats struct {
	Scheduled, Cancelled, Late, DroppedLate, Reconciles, FullReconciles uint64
}
type audioLane struct { /* see Step 3 */ }
func newAudioLane(src laneSource, sink audioSink, cfg laneConfig) *audioLane
func (l *audioLane) Tick()
func (l *audioLane) Reset()                  // CancelAllFuture + forget everything (stop/pause)
func (l *audioLane) Committed(row, abs int) (laneNote, bool)
func (l *audioLane) Stats() laneStats
```

- [ ] **Step 1: Failing tests** (`audio_lane_test.go`):

```go
//go:build test

package ui

import "testing"

type fakeSink struct {
	sched map[uint64]laneNote
	cancelled, stops []uint64
	stopAts []string
	all int
	now *float64
}
func newFakeSink(now *float64) *fakeSink { return &fakeSink{sched: map[uint64]laneNote{}, now: now} }
func (f *fakeSink) Schedule(vid uint64, n laneNote) { f.sched[vid] = n }
func (f *fakeSink) Cancel(v []uint64) { f.cancelled = append(f.cancelled, v...); for _, id := range v { if n, ok := f.sched[id]; ok && n.Time > *f.now { delete(f.sched, id) } } }
func (f *fakeSink) StopAt(inst string, t float64) { f.stopAts = append(f.stopAts, inst) }
func (f *fakeSink) CancelAllFuture() { f.all++; for id, n := range f.sched { if n.Time > *f.now { delete(f.sched, id) } } }
// heard: what actually plays = scheduled notes whose time has passed.
func (f *fakeSink) heard() map[[2]int]laneNote { out := map[[2]int]laneNote{}; for _, n := range f.sched { if n.Time <= *f.now { out[[2]int{n.Row, n.Abs}] = n } }; return out }

// gridSource: rows × steps every 0.125 s (120 BPM, div 4), every step sounds
// with vol = rowVol[row]; keys derive from the mutable fields.
type gridSource struct {
	now *float64; playing bool; gen uint64
	rowVol []float64; rowMute []bool; voiceRev []uint64; bpmScale float64
	ticksSinceVoice []bool
}
// …implement laneSource on gridSource: Steps maps times via step = floor(t/(0.125*bpmScale));
// NoteAt returns laneSound unless rowMute; Hash covers vol, voiceRev, time.

func newGridLane(t *testing.T, rows int) (*audioLane, *gridSource, *fakeSink, *float64) {
	now := new(float64); *now = 1.0
	src := &gridSource{now: now, playing: true, rowVol: make([]float64, rows), rowMute: make([]bool, rows), voiceRev: make([]uint64, rows), bpmScale: 1}
	for i := range src.rowVol { src.rowVol[i] = 1 }
	sink := newFakeSink(now)
	l := newAudioLane(src, sink, laneConfig{Horizon: 1.0, Guard: 0.008, LateTol: 0.025, FullReconcile: 0.25, VoiceRecommitMin: 0.03})
	return l, src, sink, now
}

func TestLaneCommitsHorizonAhead(t *testing.T) {
	l, _, sink, now := newGridLane(t, 2)
	l.Tick()
	if got := len(sink.sched); got != 2*8 { t.Fatalf("want 16 notes committed for 1 s at 8 steps/s × 2 rows, got %d", got) }
	*now += 0.6 // a 600 ms stall: no ticks
	if len(sink.heard()) == 0 { t.Fatal("notes inside the stall must already be committed") }
}

func TestLaneVolumeEditReachesNextNotStartedNote(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.rowVol[0] = 0.5
	*now += 0.004
	l.Tick()
	for _, n := range sink.sched {
		if n.Time > *now+0.008 && n.Vol != 0.5 { t.Fatalf("not-started note kept old vol: %+v", n) }
	}
}

func TestLaneChangeInsideGuardDoesNotDouble(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	*now = 1.125 - 0.004 // next note (abs 9 at 1.125) is inside the guard
	src.rowVol[0] = 0.5
	l.Tick()
	cnt := 0
	for _, n := range sink.sched { if n.Abs == 9 { cnt++ } }
	if cnt != 1 { t.Fatalf("note inside guard must be neither cancelled nor doubled: %d copies", cnt) }
}

func TestLaneMuteCancelsFutureNotes(t *testing.T) {
	l, src, sink, now := newGridLane(t, 1)
	l.Tick()
	src.rowMute[0] = true
	l.Tick()
	for _, n := range sink.sched { if n.Time > *now+0.008 { t.Fatalf("muted row still has future note %+v", n) } }
}

func TestLaneStopCancelsFuture(t *testing.T) {
	l, _, sink, _ := newGridLane(t, 2)
	l.Tick()
	l.Reset()
	if sink.all != 1 || len(sink.sched) != 0 { t.Fatalf("Reset must CancelAllFuture: all=%d left=%d", sink.all, len(sink.sched)) }
}

func TestLaneLatePolicy(t *testing.T) {
	l, _, sink, now := newGridLane(t, 1)
	l.Tick()
	*now += 1.5 // stall longer than H: notes between 2.0 and 2.5 never committed
	l.Tick()
	st := l.Stats()
	if st.DroppedLate == 0 { t.Fatal("notes due more than LateTol ago must be dropped, not rushed") }
	for _, n := range sink.sched { if n.Time < *now-0.025-1e-9 { t.Fatalf("rushed a stale note %+v", n) } }
}

func TestLaneVoiceRecommitRateLimited(t *testing.T) {
	l, src, _, now := newGridLane(t, 1)
	l.Tick()
	before := l.Stats().Reconciles
	for i := 0; i < 10; i++ { src.voiceRev[0]++; *now += 0.004; l.Tick() } // knob drag: 40 ms, 10 ticks
	if got := l.Stats().Reconciles - before; got > 2 { t.Fatalf("voice-only changes must be rate-limited to ≤ 1 per 30 ms: %d reconciles", got) }
	*now += 0.04; l.Tick() // trailing reconcile after the drag settles
	if l.Stats().Reconciles-before < 1 { t.Fatal("the final voice revision must be reconciled") }
}
```

- [ ] **Step 2: Run them to see them fail.** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLane' ./internal/ui/`. Expected: compile errors.

- [ ] **Step 3: Implement** `audio_lane.go`:

```go
package ui

type laneEntry struct { vid uint64; note laneNote }

type audioLane struct {
	src  laneSource
	sink audioSink
	cfg  laneConfig
	nextVID       uint64
	committed     map[[2]int]laneEntry
	globalKey     uint64
	rowKeys       []uint64
	gen           uint64
	hiTime        float64   // committed through this grid time
	lastFull      float64
	lastVoiceRec  []float64 // per row: last voice-only reconcile time
	pendingVoice  []bool
	stats         laneStats
	cancelScratch []uint64
}

func newAudioLane(src laneSource, sink audioSink, cfg laneConfig) *audioLane {
	return &audioLane{src: src, sink: sink, cfg: cfg, committed: map[[2]int]laneEntry{}}
}

func (l *audioLane) Reset() {
	l.sink.CancelAllFuture()
	l.committed = map[[2]int]laneEntry{}
	l.hiTime = 0
	l.rowKeys = nil
}

func (l *audioLane) Committed(row, abs int) (laneNote, bool) { e, ok := l.committed[[2]int{row, abs}]; return e.note, ok }
func (l *audioLane) Stats() laneStats { return l.stats }

func (l *audioLane) Tick() {
	s := l.src
	if !s.Playing() {
		if len(l.committed) > 0 { l.Reset() }
		return
	}
	if g := s.TransportGen(); g != l.gen { l.Reset(); l.gen = g }
	now := s.Now()
	rows := s.Rows()
	if len(l.rowKeys) != rows {
		l.rowKeys = make([]uint64, rows)
		l.lastVoiceRec = make([]float64, rows)
		l.pendingVoice = make([]bool, rows)
		l.globalKey = ^uint64(0) // force a full reconcile
	}
	full := false
	if gk := s.GlobalKey(); gk != l.globalKey { l.globalKey = gk; full = true }
	if now-l.lastFull >= l.cfg.FullReconcile { full = true } // safety net
	dirty := make([]bool, rows)
	for r := 0; r < rows; r++ {
		k := s.RowKey(r)
		if k != l.rowKeys[r] {
			if s.VoiceRevOnly(r) && now-l.lastVoiceRec[r] < l.cfg.VoiceRecommitMin {
				l.pendingVoice[r] = true // reconcile when the rate limit allows
				continue
			}
			l.rowKeys[r] = k
			dirty[r] = true
			l.lastVoiceRec[r] = now
		} else if l.pendingVoice[r] && now-l.lastVoiceRec[r] >= l.cfg.VoiceRecommitMin {
			dirty[r] = true
			l.lastVoiceRec[r] = now
		}
		if dirty[r] { l.pendingVoice[r] = false }
	}
	guard := now + l.cfg.Guard
	horizon := now + l.cfg.Horizon
	if full {
		l.lastFull = now
		l.stats.FullReconciles++
		for r := range dirty { dirty[r] = true }
	}
	// 1) Reconcile dirty rows over the already-committed window.
	for r := 0; r < rows; r++ {
		if !dirty[r] { continue }
		l.stats.Reconciles++
		l.reconcileRow(r, guard, l.hiTime)
	}
	// Rows that disappeared: cancel their not-started notes.
	for key, e := range l.committed {
		if key[0] >= rows && e.note.Time > guard { l.cancel(key, e) }
	}
	// 2) Extend the window to the horizon (new cells only).
	from := l.hiTime
	if from < now-l.cfg.LateTol { from = now - l.cfg.LateTol }
	lo, hi, ok := s.Steps(from, horizon)
	if ok {
		for r := 0; r < rows; r++ {
			for abs := lo; abs <= hi; abs++ {
				key := [2]int{r, abs}
				if _, done := l.committed[key]; done { continue }
				n, ok := s.NoteAt(r, abs)
				if !ok || n.Grid < from || n.Grid > horizon { continue }
				l.commit(key, n, now, guard)
			}
		}
		l.hiTime = horizon
	}
	l.flushCancels()
	// 3) Forget entries well in the past.
	for key, e := range l.committed { if e.note.Time < now-1.0 { delete(l.committed, key) } }
}

func (l *audioLane) reconcileRow(r int, guard, hiTime float64) {
	lo, hi, ok := l.src.Steps(guard, hiTime)
	if !ok { return }
	for abs := lo; abs <= hi; abs++ {
		key := [2]int{r, abs}
		e, had := l.committed[key]
		if had && e.note.Time <= guard { continue } // started (or inside guard): leave it
		n, want := l.src.NoteAt(r, abs)
		switch {
		case had && want && n.Hash == e.note.Hash:
			// unchanged
		case had && want:
			l.cancel(key, e)
			l.commit(key, n, 0, guard)
		case had && !want:
			l.cancel(key, e)
		case !had && want && n.Time > guard:
			l.commit(key, n, 0, guard)
		}
	}
}

func (l *audioLane) commit(key [2]int, n laneNote, now, guard float64) {
	if n.Kind == laneStop {
		l.sink.StopAt(n.Inst, n.Time)
		l.committed[key] = laneEntry{note: n}
		return
	}
	if n.Time <= guard {
		if now > 0 && n.Time >= now-l.cfg.LateTol {
			n.Time = guard // play now: late, counted
			l.stats.Late++
		} else {
			l.stats.DroppedLate++
			l.committed[key] = laneEntry{note: n} // remember: never rush it later
			return
		}
	}
	l.nextVID++
	l.sink.Schedule(l.nextVID, n)
	l.committed[key] = laneEntry{vid: l.nextVID, note: n}
	l.stats.Scheduled++
}

func (l *audioLane) cancel(key [2]int, e laneEntry) {
	if e.vid != 0 { l.cancelScratch = append(l.cancelScratch, e.vid); l.stats.Cancelled++ }
	delete(l.committed, key)
}

func (l *audioLane) flushCancels() {
	if len(l.cancelScratch) == 0 { return }
	l.sink.Cancel(l.cancelScratch)
	l.cancelScratch = l.cancelScratch[:0]
}
```

  Note on the reconcile/cancel ordering: `reconcileRow` queues cancels and issues schedules immediately, and `flushCancels` sends the queued cancels at the end of the tick. The sink must therefore apply ops in order: the production sink in Task 9 has a FIFO outbox, and the browser's tombstone makes the order irrelevant for the same vid. Replacements always get a NEW vid.

- [ ] **Step 4: Run the tests.** Same command. Expected: PASS.

- [ ] **Step 5: Finish.** No commit.

### Task 8: Game as `laneSource` + key-coverage property test

**Files:**
- Create: `src/go/internal/ui/audio_lane_source.go`
- Test: `src/go/internal/ui/audio_lane_source_test.go` (`//go:build test`)

**Interfaces:**
- Consumes: Task 6 (`laneContext`, `laneNoteAt`), Task 3 (`StepAtAudioTime`, `AnchorKey`, `Predictor.Epoch`), Task 4 (`VoiceRevision`).
- Produces: `type gameLaneSource struct{ s *playbackSlate; ctx laneCtx; ctxOK bool; lastVoice []uint64; lastRowNonVoice []uint64 }`, which implements `laneSource`, plus `func (s *playbackSlate) newGameLaneSource() *gameLaneSource`. Its `refresh()` must be called once per `Tick` before the lane reads it; `Tick` callers do this.

- [ ] **Step 1: Failing property test.** It applies random edits drawn from every mutation class in the inventory:
  - node add/delete/move, edge add/delete, node type;
  - node volume/pitch/logic, group rule;
  - row add/delete, instrument, row volume, mute/solo;
  - BPM, seek, sample edit, synth param.

  After each edit it asserts: **if `GlobalKey()` and every `RowKey(r)` are unchanged, then `laneNoteAt` over the next 2 s is identical to the pre-edit result.** In other words, the fingerprint never misses a change that matters.

```go
//go:build test

package ui

import ("math/rand"; "testing")

func TestLaneKeysCoverEveryEditClass(t *testing.T) {
	g := newLaneCircuitGame(t) // from Task 6
	src := g.playbackSlate.newGameLaneSource()
	rng := rand.New(rand.NewSource(42))
	snapshot := func() (map[[2]int]uint64, uint64, []uint64) {
		src.refresh()
		notes := map[[2]int]uint64{}
		lo, hi, _ := src.Steps(src.Now(), src.Now()+2)
		for r := 0; r < src.Rows(); r++ { for a := lo; a <= hi; a++ { if n, ok := src.NoteAt(r, a); ok { notes[[2]int{r, a}] = n.Hash } } }
		keys := make([]uint64, src.Rows())
		for r := range keys { keys[r] = src.RowKey(r) }
		return notes, src.GlobalKey(), keys
	}
	for i := 0; i < 400; i++ {
		n0, g0, k0 := snapshot()
		applyRandomLaneEdit(t, g, rng) // one random edit from the class table above, via the SAME code paths the UI uses
		n1, g1, k1 := snapshot()
		if g0 == g1 && equalU64(k0, k1) && !equalNoteMaps(n0, n1) {
			t.Fatalf("edit #%d (%s) changed notes but no lane key moved", i, lastLaneEditName)
		}
	}
}

func TestLaneRowDeleteShiftsCommittedNotes(t *testing.T) {
	g := newLaneCircuitGame(t)
	sink := newFakeSink(laneTestNow(g))
	lane := enableAudioLaneForTest(t, g, sink) // Task 9 helper; add a stub here that wires newAudioLane(newGameLaneSource) until Task 9 lands
	lane.Tick()
	g.drum.DeleteRow(0)
	g.updateBeatInfos()
	lane.Tick()
	for vid, n := range sink.sched {
		if n.Row == 0 && n.Inst != g.drum.Rows[0].Instrument && n.Time > *laneTestNow(g)+0.008 {
			t.Fatalf("vid %d: a note of the deleted row survived at the new index: %+v", vid, n)
		}
	}
}
```

  `applyRandomLaneEdit` and `lastLaneEditName` live in the test file. Each case calls the real entry point: `g.tryAddNode`, `g.deleteNodeInternal`, `g.moveNode`, `g.addEdge`, `g.graph.SetNodeParams`, `g.drum.SetInstrument`, `g.drum.Rows[r].Volume = v`, `g.drum.toggleMute`, `g.drum.SetBPM`, `g.Seek`, `audio.ApplySampleEditLive`, `audio.SetInstrumentParam`, `g.drum.AddRow` / `DeleteRow`. Each is followed by the same reconciliation the UI performs (`g.updateBeatInfos()` where the UI does it). Use `t.Cleanup` to clear process-global audio state (`audio.ClearSampleEdit`, `audio.ResetInstrumentParams` or equivalent).

- [ ] **Step 2: Run them to see them fail.** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLaneKeys|TestLaneRowDelete' ./internal/ui/`. Expected: compile errors.

- [ ] **Step 3: Implement** `audio_lane_source.go`:

```go
package ui

import (
	"hash/fnv"
	"math"
	"unsafe"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type gameLaneSource struct {
	s       *playbackSlate
	ctx     laneCtx
	ctxOK   bool
	prevNon []uint64 // per row: key without VoiceRevision
	voiceOnly []bool
}

func (s *playbackSlate) newGameLaneSource() *gameLaneSource { return &gameLaneSource{s: s} }

func (src *gameLaneSource) refresh() { src.ctx, src.ctxOK = src.s.laneContext() }

func (src *gameLaneSource) Playing() bool    { return src.s.g.Playing() && !src.s.g.Paused() && src.ctxOK }
func (src *gameLaneSource) Now() float64     { return audio.Now() }
func (src *gameLaneSource) TransportGen() uint64 { return src.s.g.audioGen.Load() }
func (src *gameLaneSource) Rows() int        { return len(src.s.g.drum.Rows) }

func (src *gameLaneSource) GlobalKey() uint64 {
	g := src.s.g
	h := fnv.New64a()
	w := func(x uint64) { var b [8]byte; for i := range b { b[i] = byte(x >> (8 * i)) }; _, _ = h.Write(b[:]) }
	w(uint64(uintptr(unsafe.Pointer(src.ctx.snap))))
	w(g.engine.Predictor.Epoch())
	w(uint64(src.ctx.bpm)); w(uint64(src.ctx.div))
	bb, as := g.state.AnchorKey()
	w(math.Float64bits(bb)); w(math.Float64bits(as))
	if src.ctx.anySolo { w(1) } else { w(0) }
	w(math.Float64bits(src.ctx.offset))
	w(g.laneGraphVersion.Load()) // bumped by NodeChangedHook and group hook (Task 9)
	return h.Sum64()
}

func (src *gameLaneSource) rowNonVoiceKey(r int) uint64 {
	row := src.s.g.drum.Rows[r]
	h := fnv.New64a()
	_, _ = h.Write([]byte(row.Instrument))
	var b [8]byte
	for i, x := range []uint64{math.Float64bits(row.Volume), boolU(row.Muted), boolU(row.Solo)} {
		_ = i; for k := range b { b[k] = byte(x >> (8 * k)) }; _, _ = h.Write(b[:])
	}
	return h.Sum64()
}

func (src *gameLaneSource) RowKey(r int) uint64 {
	if len(src.prevNon) != src.Rows() { src.prevNon = make([]uint64, src.Rows()); src.voiceOnly = make([]bool, src.Rows()) }
	non := src.rowNonVoiceKey(r)
	src.voiceOnly[r] = non == src.prevNon[r]
	src.prevNon[r] = non
	return non*31 + audio.VoiceRevision(src.s.g.drum.Rows[r].Instrument)
}

func (src *gameLaneSource) VoiceRevOnly(r int) bool { return r < len(src.voiceOnly) && src.voiceOnly[r] }

func (src *gameLaneSource) Steps(from, to float64) (int, int, bool) {
	c := src.ctx
	lo, ok1 := src.s.g.state.StepAtAudioTime(from-c.offset-0.05, c.bpm, c.div) // −50 ms: max groove nudge
	hi, ok2 := src.s.g.state.StepAtAudioTime(to-c.offset+0.05, c.bpm, c.div)
	if !ok1 || !ok2 { return 0, -1, false }
	if lo < 0 { lo = 0 }
	return lo, hi, true
}

func (src *gameLaneSource) NoteAt(row, abs int) (laneNote, bool) {
	if !src.ctxOK { return laneNote{}, false }
	return src.s.laneNoteAt(&src.ctx, row, abs)
}

func boolU(b bool) uint64 { if b { return 1 }; return 0 }
```

  Add `laneGraphVersion atomic.Uint64` to `Game` (`game_struct.go`). Bump it in the NodeChangedHook closure (`game_new.go:~353`) and in `onGroupsChanged` (`game_groups.go:56`). In `Steps` the ±50 ms widening covers groove nudges; `audioLane.Tick` filters with `n.Grid` against the exact window. Confirm the nudge bound by reading `nodeNudgeSecondsAt` (`game_subdivisions.go:171`). If nudges can exceed 50 ms, widen to that bound.

- [ ] **Step 4: Run the tests.** Same command. Expected: PASS. If `TestLaneKeysCoverEveryEditClass` fails, add the missing input to `GlobalKey`/`RowKey`; never special-case the test.

- [ ] **Step 5: Finish.** No commit.

---

## Phase 4 — Integration

### Task 9: Wire the lane into the Game (sink, drives, lane mode, transport, parity)

**Files:**
- Create: `src/go/internal/ui/audio_lane_sink.go` (FIFO outbox + `flushLaneOps()` called from `audioLoop`)
- Create: `src/go/internal/ui/audio_lane_wire.go` (`audioLaneOn()`, `laneTick()`, `enableAudioLaneForTest`)
- Modify: `src/go/internal/ui/game_struct.go` (fields `audioLane *audioLane`, `laneSrc *gameLaneSource`, `laneSink *outboxSink`, `laneWake chan struct{}`, `laneGraphVersion atomic.Uint64`, `laneOn atomic.Bool`)
- Modify: `src/go/internal/ui/game_new.go` (construct; `laneOn = RuntimeProf().AudioLaneEnabled && !runningUnderGoTest()`)
- Modify: `src/go/internal/ui/game_sequencer_schedule.go` (end of locked body: `s.laneTick()`; lane mode: skip dispatch + mute `audio.Stop`)
- Modify: `src/go/internal/ui/game_audio_schedule.go` (split `scheduleSound` into `scheduleSoundUI` + dispatch)
- Modify: `src/go/internal/ui/game_audio_loop.go` (select on `laneWake`; `flushLaneOps()` before `PlayBatch`)
- Modify: `src/go/internal/ui/game_update.go` (Stop/Pause: `g.audioLane.Reset()` when on), `game_media_session_wasm.go`, `game_groups.go`, Game `Close` (Reset)
- Modify: `src/go/internal/ui/runtime_profile_env.go` + `src/go/internal/env/vars.go` (`BEATMO_AUDIO_LANE` Bool, `BEATMO_AUDIO_HORIZON_SEC` Float)
- Test: `src/go/internal/ui/audio_lane_game_test.go` (`//go:build test`)

**Interfaces:**
- Consumes: everything from Tasks 3–8.
- Produces:

```go
func (g *Game) audioLaneOn() bool
func enableAudioLaneForTest(t *testing.T, g *Game, sink audioSink) *audioLane // (test file) installs a lane with sink, laneOn=true, restores on cleanup
type outboxSink struct{ mu sync.Mutex; ops []laneOp } // production audioSink; ops flushed in order by flushLaneOps
```

- [ ] **Step 1: Failing Game-level tests** (`audio_lane_game_test.go`), each driving real `g.Update()` with `audio.SetNowForTest` advancing:
  - `TestLaneStallToleranceGame`: lane on with a fake sink. Advance 0.6 s with **no** `Update`. Every note due in that interval was already in `sink.sched` before the stall.
  - `TestLaneStopCancelsFuture`: press Stop through `g.drum` (the same path `stopPressed` uses); `sink.all == 1`, and nothing is scheduled after now.
  - `TestLaneBPMChangeRecommitsAtNewTimes`: `g.drum.SetBPM(180)` mid-play. After the next tick, every not-started note's `Time` equals `AudioTimeOfStep` under the new anchor + offset ± 1 µs, and no note remains at an old-BPM time.
  - `TestLaneModeSkipsLegacyDispatch`: with the lane on, `playFn` receives nothing from `seqScheduleTime` (all audio comes through the sink), highlights still advance (`nextBeatIdxs` increases), and parity reports no mismatch over 4 s (`assertNoParityMismatches` existing helper).
  - `TestLaneMuteNodeUsesStopAt`: a mute node produces `sink.stopAts` with the row instrument and **no** `audio.Stop` (`audio.SetStopHook` records nothing).
  - `TestLaneVoiceTriggerFiresAtGridTimeNotCommit` (see the trigger-driven UI note in Step 3).
  - `TestLegacyPathUnchangedWhenLaneOff`: with the lane off (default under test), one existing representative assertion (copy `TestSoundPlaysAfterHighlight`'s body) still passes.

- [ ] **Step 2: Run them to see them fail.** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLane|TestLegacyPathUnchanged' ./internal/ui/`. Expected: compile errors / FAIL.

- [ ] **Step 3: Implement.**

`audio_lane_sink.go`:

```go
package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type laneOpKind uint8
const (laneOpSchedule laneOpKind = iota + 1; laneOpCancel; laneOpStopAt; laneOpCancelAll)
type laneOp struct { kind laneOpKind; vid uint64; note laneNote; vids []uint64; inst string; t float64 }

// outboxSink records lane ops in FIFO order; audioLoop applies them via
// flushLaneOps, so the audio APIs run on the audio dispatch goroutine (never
// under seqMu — a cold desktop render must not block Update).
type outboxSink struct{ mu sync.Mutex; ops []laneOp; wake chan struct{} }

func (o *outboxSink) push(op laneOp) {
	o.mu.Lock(); o.ops = append(o.ops, op); o.mu.Unlock()
	select { case o.wake <- struct{}{}: default: }
}
func (o *outboxSink) Schedule(vid uint64, n laneNote) { o.push(laneOp{kind: laneOpSchedule, vid: vid, note: n}) }
func (o *outboxSink) Cancel(v []uint64) { o.push(laneOp{kind: laneOpCancel, vids: append([]uint64(nil), v...)}) }
func (o *outboxSink) StopAt(inst string, t float64) { o.push(laneOp{kind: laneOpStopAt, inst: inst, t: t}) }
func (o *outboxSink) CancelAllFuture() { o.push(laneOp{kind: laneOpCancelAll}) }

func (o *outboxSink) take() []laneOp { o.mu.Lock(); ops := o.ops; o.ops = nil; o.mu.Unlock(); return ops }

// flushLaneOps applies queued ops in order. Consecutive schedules are batched
// into one audio.PlayBatch (one Go→JS crossing on WASM).
func (g *Game) flushLaneOps() {
	if g.laneSink == nil { return }
	ops := g.laneSink.take()
	batch := g.laneBatchScratch[:0]
	flush := func() { if len(batch) > 0 { audio.PlayBatch(batch); batch = batch[:0] } }
	for _, op := range ops {
		switch op.kind {
		case laneOpSchedule:
			n := op.note
			if g.voicePrewarmer != nil { g.voicePrewarmer.warmPitch(n.Inst, n.Pitch) }
			batch = append(batch, audio.BatchParam{ID: n.Inst, Vol: n.Vol, Pitch: n.Pitch, Dur: n.Dur,
				When: n.Time, HasWhen: true, Row: n.Row, Abs: n.Abs, Ideal: n.Time, Grid: n.Grid, VoiceID: op.vid})
			g.openTimingTrace(n.Row, n.Abs, n.Time, g.audioGen.Load(), g.parityGen.Load())
		case laneOpCancel:
			flush(); audio.CancelVoices(op.vids)
		case laneOpStopAt:
			flush(); audio.StopLaneAt(op.inst, op.t)
		case laneOpCancelAll:
			flush(); audio.CancelAllLaneFuture()
		}
	}
	flush()
	g.laneBatchScratch = batch
	g.drainEmissionReports()
}
```

  Field: `laneBatchScratch []audio.BatchParam` on `Game`.

`audio_lane_wire.go`:

```go
package ui

func (g *Game) audioLaneOn() bool { return g.audioLane != nil && g.laneOn.Load() }

// laneTick runs inside seqScheduleTime's seqMu section (every drive point).
func (s *playbackSlate) laneTick() {
	g := s.g
	if !g.audioLaneOn() { return }
	g.laneSrc.refresh()
	g.audioLane.Tick()
}
```

  Construct in `game_new.go` after the predictor:

```go
	g.laneWake = make(chan struct{}, 1)
	g.laneSink = &outboxSink{wake: g.laneWake}
	g.laneSrc = g.playbackSlate.newGameLaneSource()
	p := RuntimeProf()
	g.audioLane = newAudioLane(g.laneSrc, g.laneSink, laneConfig{Horizon: p.AudioHorizonSec, Guard: p.CancelGuardSec,
		LateTol: p.LateToleranceSec, FullReconcile: p.LaneFullReconcileSec, VoiceRecommitMin: p.LaneVoiceRecommitMinSec})
	g.laneOn.Store(p.AudioLaneEnabled && !runningUnderGoTest())
```

  Also add `if env.AudioLane.IsSet() { p.AudioLaneEnabled = env.AudioLane.Get() }` and the same for `AudioHorizonSec` in `applyEnvProfileOverride`, and declare in `env/vars.go`:

```go
	AudioLane = Bool("BEATMO_AUDIO_LANE", false, ScopeDebug, "Override RuntimeProfile.AudioLaneEnabled (A/B the audio lane vs the legacy grid-time dispatch)."+notSetSentinel)
	AudioHorizonSec = Float("BEATMO_AUDIO_HORIZON_SEC", 0, ScopeDebug, "Override RuntimeProfile.AudioHorizonSec."+notSetSentinel)
```

`game_audio_loop.go`: replace the blocking receive with

```go
		var first soundReq
		select {
		case <-g.audioQuit:
			return
		case <-g.laneWake:
			g.flushLaneOps()
			continue
		case req, ok := <-g.audioCh:
			if !ok { return }
			first = req
		}
```

  Also call `g.flushLaneOps()` right before `audio.PlayBatch(batch)` (keeping lane order relative to UI sounds).

`game_sequencer_schedule.go`:
- After the per-row loop (still inside the locked function), call `s.laneTick()`.
- In the mute branch, wrap `audio.Stop(inst)` with `if !g.audioLaneOn() { … }`.
- In the regular branch, replace the `g.scheduleSound(row, idx, info, inst, vol, pitch, dur, beatIdealAudio, true)` call with:

```go
						if g.audioLaneOn() {
							// Audio was committed ahead by the lane; the UI lane only records it.
							if n, ok := g.audioLane.Committed(row, idx); ok && n.Kind == laneSound {
								g.markSeqAudioEnqueued(row, idx)
								g.recordParityAudio(row, idx, n.Time, n.Inst, n.Vol, n.Pitch, n.Dur, g.audioGen.Load(), g.parityGen.Load())
							}
							g.scheduleSoundUI(row, idx, info, inst, pitch, dur, beatIdealAudio+RuntimeProf().AudioOffsetSec)
						} else {
							g.scheduleSound(row, idx, info, inst, vol, pitch, dur, beatIdealAudio, true)
						}
```

`game_audio_schedule.go`: extract the highlight tail of `scheduleSound` (from `if sec := g.expectedHighlightSeconds(...)` to the end) into

```go
func (s *playbackSlate) scheduleSoundUI(row, idx int, info model.BeatInfo, inst string, pitch, dur, when float64) {
	g := s.g
	if sec := g.expectedHighlightSeconds(inst, pitch, dur); sec > 0 {
		frames := int64(sec * ebitenTPS)
		if frames < 1 { frames = 1 }
		g.highlightVisual(row, idx, info, frames)
		g.setNodeHighlightUntil(info.NodeID, when, when+sec)
		g.nodeAnimSet(info.NodeID, 1)
	}
}
```

  `scheduleSound` calls it with its `when` (same behaviour as before), plus a `Game` forwarder.

Trigger-driven UI: `RecordVoiceTrigger` (synth glow, sampler playhead) must fire at the note's TIME, not at commit, which is up to H early.
- In `audio.PlayBatch` (`batch_wasm.go`) and `playParamsIdentity` (`engine_play.go`), skip `RecordVoiceTrigger(id)` when `VoiceID != 0` / `vid != 0`.
- In lane mode, `scheduleSoundUI` (UI lane, grid time) calls `audio.RecordVoiceTrigger(inst)` when the committed note is a `laneSound`.

Test: `TestLaneVoiceTriggerFiresAtGridTimeNotCommit`. Commit at t; assert `audio.SinceLastTrigger(inst)` does not reset until the UI lane reaches the beat. Read `RecordVoiceTrigger`'s file for the exact accessor name (`SinceLastTrigger` or equivalent) before writing it.

Transport:
- Next to each `g.audioGen.Add(1)` (Pause `game_update.go:860`, Stop `:936`, media session `game_media_session_wasm.go:33`, groups `game_groups.go:67`), add `if g.audioLaneOn() { g.audioLane.Reset() }`. Groups: replace that call with `g.laneGraphVersion.Add(1)` (a reconcile is enough; the global Reset would cut).
- In `Game.Close`, call `g.audioLane.Reset(); g.flushLaneOps()`.

Test helper (in `audio_lane_game_test.go`):

```go
func enableAudioLaneForTest(t *testing.T, g *Game, sink audioSink) *audioLane {
	t.Helper()
	p := RuntimeProf()
	g.audioLane = newAudioLane(g.laneSrc, sink, laneConfig{Horizon: p.AudioHorizonSec, Guard: p.CancelGuardSec, LateTol: p.LateToleranceSec, FullReconcile: p.LaneFullReconcileSec, VoiceRecommitMin: p.LaneVoiceRecommitMinSec})
	g.laneOn.Store(true)
	t.Cleanup(func() { g.laneOn.Store(false) })
	return g.audioLane
}
```

  Replace the Task 8 stub of this helper with this version.

- [ ] **Step 4: Run the tests.**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestLane|TestLegacyPathUnchanged' ./internal/ui/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 ./internal/ui/ 2>&1 | tail -30
```

Expected: the new tests PASS, and the full ui suite has no failures that were not present before this task. Compare against a pre-task run of the same command saved to a file first; prove causation for any delta.

- [ ] **Step 5: Finish.** Run `GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./cmd && CGO_ENABLED=1 ../../.tools/go/bin/go build ./cmd`. No commit.

### Task 10: Oracle equivalence (lane heard == legacy dispatched, under random edits)

**Files:**
- Test: `src/go/internal/ui/audio_lane_oracle_test.go` (`//go:build test`)

**Interfaces:**
- Consumes: `enableAudioLaneForTest`, `fakeSink` (Task 7), `newLaneCircuitGame` (Task 6), `applyRandomLaneEdit` (Task 8).

- [ ] **Step 1: Write the test.** Two identical games, A (legacy, `SetPlayFunc` capture) and B (lane, fake sink), get the same circuit and the same seeded random edit script at the same audio times. Advance both through 20 s in 4 ms steps with `g.Update()`. Assert:
  - the multiset of `(row, abs, inst, vol, pitch)` that **A dispatched** equals what **B's sink heard** (scheduled and not cancelled before start);
  - excluding notes whose `Time` falls inside `[editTime, editTime + AudioLookaheadSec]` after each edit, which is exactly where legacy plays stale notes and the lane does not. Count those separately and assert the lane's count of stale notes is 0.

```go
func TestLaneHeardEqualsLegacyDispatched(t *testing.T) {
	for seed := int64(1); seed <= 5; seed++ {
		a, b := newLaneCircuitGame(t), newLaneCircuitGame(t)
		sink := newFakeSink(laneTestNow(b))
		enableAudioLaneForTest(t, b, sink)
		legacy := captureLegacyHeard(t, a) // SetPlayFunc capture
		script := newLaneEditScript(seed, 20.0)  // [(time, editFn)] shared by both games
		runLaneScript(t, a, script); runLaneScript(t, b, script)
		diff := diffHeard(legacy(), sink.heard(), script.Windows(RuntimeProf().AudioLookaheadSec))
		if len(diff) > 0 { t.Fatalf("seed %d: %d mismatches, first %v", seed, len(diff), diff[0]) }
	}
}
```

  Implement `captureLegacyHeard`, `newLaneEditScript`, `runLaneScript`, `diffHeard` and `laneTestNow` in the test file (each 10–25 lines). `runLaneScript` sets the fake audio clock with `audio.SetNowForTest` and calls `g.Update()`, applying script edits when due.

- [ ] **Step 2: Run it.** Run `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestLaneHeardEqualsLegacyDispatched -v ./internal/ui/`. Expected: PASS. A failure is a real divergence between `laneNoteAt` and the stateful UI walk: fix `laneNoteAt` (or the predictor), never the oracle.

- [ ] **Step 3: Finish.** No commit.

### Task 11: Browser end-to-end (lane on) + bridge catalogue

**Files:**
- Create: `src/js/audio_lane_e2e.browser.test.js`
- Modify: `src/js/wasm_bridge_smoke.browser.test.js`, `src/go/internal/ui/js_exports_catalogue_drift_test.go` (if Task 5 didn't already add the entries)

**Interfaces:**
- Consumes: `startPlaybackWhenReady`, `startPlaybackProbe`, `stopPlaybackProbe`, `markEdit` (Tasks 1–2); env `BEATMO_AUDIO_LANE=1` via `window.__beatmoEnv` (Task 9).

- [ ] **Step 1: Write the test.** It loads the real app (main.wasm) with `__beatmoEnv = { BEATMO_AUDIO_LANE: "1" }` and plays the startup demo at 120 BPM with the probe. Phases:
  1. **`stall-300`** (300 ms busy-loop every 700 ms) for 8 s. Assert `arrivedLate === 0`, `late.over20ms === 0`, `notes.missed === 0`, `notes.dropped === 0`, `quality.masterClicks === 0`.
  2. **`edit-storm`** for 8 s: `toggleMute(0)` every 600 ms, `setRowVolume(1, v)` every 400 ms, and `setInstrumentParam(rows[2], <first visible knob from recipeForInstrument>, alternating values)` every 50 ms. Assert `edits.staleNotes === 0` for mute and vol, and `notes.missed === 0` excluding muted intervals.
  3. **Stop mid-horizon:** `stopPlay()`, then 1.2 s of probe. Assert no onset later than 30 ms after the stop mark on any channel.

  Every assertion is a hard gate (the lane's contract), except under `AUDIO_PROBE_STRICT=0`, which is not offered: these are the acceptance criteria.

- [ ] **Step 2: Run it.** Run `cmp -s $SCRATCH/lane.wasm src/js/main.wasm || cp $SCRATCH/lane.wasm src/js/main.wasm; GO=… WASM_PREBUILT=1 node src/js/audio_lane_e2e.browser.test.js` (check load < 15 first). Expected: PASS.

- [ ] **Step 3: Run the dependent browser suites on the lane build.**

```
for t in xplat_parity xplat_audio_compare timing_traces wasm_bridge_smoke webaudio_timing_stress webaudio_perf_e2e audio_lane_cancel; do WASM_PREBUILT=1 GO=… node src/js/$t.browser.test.js > /tmp/claude-1000/…/lane_$t.log 2>&1; echo "$t $?"; done
```

Expected: all exit 0. xplat parity runs with the lane on (default in Task 12) and off (env).

- [ ] **Step 4: Finish.** No commit.

---

## Phase 5 — Measure AFTER, tune, turn on

### Task 12: A/B/C measurement, tuning, defaults, docs

**Files:**
- Modify: `src/go/internal/ui/runtime_profile.go` (flip `AudioLaneEnabled: true` on both builders, with a bench citation per field per the RuntimeProfile rule)
- Modify: `src/go/internal/ui/runtime_profile_test.go` or the equivalent pinning test (update the expected defaults)
- Create: `bench-results/audio_lane_ab_<date>.md` (generated) and `bench-results/audio_lane_findings_<date>.md` (hand-written summary)
- Modify: `CLAUDE.md` (§ Browser audio isolation: the lane replaces the "Still coupled" paragraph; Key Code Paths gains the lane files; Quick Start gains `make bench-lane-ab`)
- Modify: memory `project_browser_audio_isolation_probe.md` + a new `project_audio_lane.md` and its MEMORY.md index line

- [ ] **Step 1: AFTER run.** Build `$SCRATCH/lane.wasm` from the final tree. With load < 15, run `ROUNDS=3 AB_SCRATCH=$SCRATCH node scripts/audio_lane_ab.mjs` (all three arms, all scenarios). Expected (spec §1 table):

  | Arm | Expectation |
  |---|---|
  | lane | `probeArrivedLate` 0 and `probeLateOver20` 0 in idle / stall-150 / stall-300 / tab-synth / pan-storm; `probeIoiErrP95MS` ≤ 5; `probeEditStaleNotes` 0; Update/Draw within +5 % of legacy |
  | legacy | ≈ baseline on every metric (the legacy path is unchanged) |

- [ ] **Step 2: Tune if a target misses.** Vary `BEATMO_AUDIO_HORIZON_SEC` (0.5 / 1.0 / 1.5) through `ISOLATION_ENV` and rerun only the failing scenario across the arms. Record the chosen value with its numbers in the `runtime_profile.go` citation.

- [ ] **Step 3: Flip the defaults.** Set `AudioLaneEnabled: true` (browser and desktop). Then run:

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... 2>&1 | tail -40
cd src/go && CGO_ENABLED=1 xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/... ./internal/ui/ -run 'Lane|Mixer|ExportImport|Parity' 2>&1 | tail -40
```

  Expected: no new failures vs the pre-plan baseline. Every delta is investigated and causation proven.

- [ ] **Step 4: Desktop sanity.** Run `make run`, play the startup demo, toggle mute and drag a synth knob (manual check), and record observations in the findings doc. Then `PARITY_TIMING=log make run-debug` for 60 s: no timing-gate warnings attributable to the lane.

- [ ] **Step 5: Docs + memory.** Update CLAUDE.md and memory as listed. The findings doc links the AB markdown and states every target's result. A missed target is stated plainly, never rounded.

- [ ] **Step 6: Finish.** No commit; report to the user with the before/after table.
