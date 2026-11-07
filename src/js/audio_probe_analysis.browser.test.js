// Pure Node (named *.browser.test.js for the runner): synthetic probe logs with known answers, since a probe that
// mis-attributes onsets would make every bench report confident nonsense. Usage: node src/js/audio_probe_analysis.browser.test.js

import { analyzeAudioProbe, clockSlips, stats } from "./audio_probe_analysis.js";

const SR = 48000;
let failures = 0;
function check(cond, msg) {
  if (cond) console.log(`ok   ${msg}`);
  else { failures++; console.error(`FAIL ${msg}`); }
}
const near = (a, b, tol = 0.05) => a != null && Math.abs(a - b) <= tol;

// 100 ms reports over [t0, t1) with the given onsets and an envelope at level inside [on, on+ring).
function busReports({ t0, t1, onsets = [], ring = 0.05, level = 0.5, drift = () => 0, cuts = [], clicks = [] }) {
  const reps = [];
  const envFrames = 64;
  for (let t = t0; t < t1 - 1e-9; t += 0.1) {
    const startFrame = Math.round(t * SR);
    const endFrame = Math.round(Math.min(t + 0.1, t1) * SR);
    const envStart = Math.ceil(startFrame / envFrames) * envFrames;
    const n = Math.floor((endFrame - envStart) / envFrames);
    const env = new Float32Array(Math.max(0, n));
    for (let k = 0; k < env.length; k++) {
      const ts = (envStart + k * envFrames) / SR;
      env[k] = onsets.some((o) => ts >= o && ts < o + ring) ? level : 0;
    }
    const inRep = (x) => x * SR >= startFrame && x * SR < endFrame;
    reps.push({
      type: "report", startFrame, endFrame,
      onsets: onsets.filter(inRep).map((o) => ({ f: Math.round(o * SR), peak: level })),
      clicks: clicks.filter(inRep).map((x) => Math.round(x * SR)),
      cuts: cuts.filter(inRep).map((x) => Math.round(x * SR)),
      clipped: 0, sumSq: 0.01 * (endFrame - startFrame), n: endFrame - startFrame,
      minDriftMs: drift(t), maxDriftMs: drift(t) + 10, maxGapMs: 11, callbacks: 30,
      env, envStart, envFrames,
    });
  }
  return reps;
}

// A 1-channel log: notes every 250 ms from t=1s, grid = ideal - lookahead.
function log({ notes, onsetFor, lookahead = () => 0.08, startLate = () => 0, drop = new Set(), extraOnsets = [], drift, silentRing, fxLatency = () => 0 }) {
  const requests = [], starts = [], onsets = [];
  notes.forEach((t, i) => {
    const pk = i + 1;
    const ideal = t;
    const grid = t - lookahead(i);
    requests.push({ pk, id: "kick", row: 0, abs: i, vol: 1, pitch: 0, when: ideal, ideal, grid, ctx: ideal - 0.05, perf: 1000 * i });
    if (drop.has(i)) return;
    const startWhen = ideal + startLate(i);
    starts.push({ pk, id: "kick", startWhen, ctx: startWhen - 0.01, perf: 1000 * i + 0.5, path: "hit", fxLatency: fxLatency(i) });
    const on = onsetFor(i, startWhen);
    if (on != null) onsets.push(on);
  });
  onsets.push(...extraOnsets);
  const reports = {
    kick: busReports({ t0: 0.5, t1: notes[notes.length - 1] + 1, onsets, ring: silentRing ?? 0.05, drift }),
    __master__: busReports({ t0: 0.5, t1: notes[notes.length - 1] + 1, onsets, drift }),
  };
  return { sampleRate: SR, baseLatency: 0.01, outputLatency: 0.03, requests, starts, reports, loopGaps: [{ ctx: 2, ms: 120 }], loopMaxMs: 120 };
}

const notes = Array.from({ length: 16 }, (_, i) => 1 + i * 0.25);

// 1. Perfect playback: every onset 1 ms after start(when) — the detector bias.
{
  const r = analyzeAudioProbe(log({ notes, onsetFor: (i, s) => s + 0.001 }));
  check(r.notes.judged === 16 && r.notes.heard === 16 && r.notes.missed === 0 && r.notes.masked === 0, `perfect: all 16 heard (${JSON.stringify(r.notes)})`);
  check(near(r.lagVsIdealMs.p50, 1) && near(r.lagVsIdealMs.max, 1), `perfect: lag vs request = 1 ms (got p50 ${r.lagVsIdealMs.p50})`);
  check(near(r.lagVsGridMs.p50, 81), `perfect: lag vs grid = lookahead + 1 = 81 ms (got ${r.lagVsGridMs.p50})`);
  check(r.late.over10ms === 0 && near(r.ioiErrMs.max, 0), "perfect: no late notes, even rhythm");
  check(r.audioThread.slips === 0 && r.lookahead.shifts === 0, "perfect: no slips, no lookahead shifts");
  check(r.mainThread.over100ms === 1, "main-thread stalls are reported");
}

// 2. Two notes 30 ms late: counted late, and the rhythm error shows the stumble.
{
  const late = new Set([5, 9]);
  const r = analyzeAudioProbe(log({ notes, startLate: (i) => (late.has(i) ? 0.03 : 0), onsetFor: (i, s) => s + 0.001 }));
  check(r.late.over20ms === 2 && r.late.over50ms === 0, `late: exactly 2 notes >20ms late (got ${r.late.over20ms})`);
  check(near(r.lagVsIdealMs.max, 31), `late: max lag 31 ms (got ${r.lagVsIdealMs.max})`);
  check(near(r.startLateMs.max, 30), `late: start() lateness 30 ms (got ${r.startLateMs.max})`);
  check(near(r.ioiErrMs.max, 30), `late: IOI error 30 ms (got ${r.ioiErrMs.max})`);
}

// 3. A silent bus → missed; an onset masked by a ringing note → masked, not missed.
{
  const r1 = analyzeAudioProbe(log({ notes, onsetFor: (i, s) => (i === 7 ? null : s + 0.001) }));
  check(r1.notes.missed === 1 && r1.notes.masked === 0, `silent note → missed=1 (got ${JSON.stringify(r1.notes)})`);
  // ring 0.3 s: the previous note is still sounding over note 7's start.
  const r2 = analyzeAudioProbe(log({ notes, silentRing: 0.3, onsetFor: (i, s) => (i === 7 ? null : s + 0.001) }));
  check(r2.notes.missed === 0 && r2.notes.masked === 1, `masked note → masked=1, missed=0 (got ${JSON.stringify(r2.notes)})`);
}

// 4. Requested but never started → dropped; an onset nobody asked for → spurious.
{
  const r = analyzeAudioProbe(log({ notes, drop: new Set([4]), extraOnsets: [2.62], onsetFor: (i, s) => s + 0.001 }));
  check(r.notes.dropped === 1, `dropped note counted (got ${r.notes.dropped})`);
  check(r.notes.spurious === 1, `spurious onset counted (got ${r.notes.spurious})`);
}

// 5. Starts are sample-accurate, so an onset 40 ms later is not that note's.
{
  const r = analyzeAudioProbe(log({ notes, onsetFor: (i, s) => (i === 3 ? s + 0.04 : s + 0.001) }));
  check(r.late.over20ms === 0, `a far onset is not a late match (late>20 = ${r.late.over20ms})`);
  check(r.notes.heard === 15 && r.notes.spurious === 1, `…it's an unmatched note + spurious onset (${JSON.stringify(r.notes)})`);
}

// 5b. Unless the insert chain DECLARED that latency (a pitch shifter): then it is matched, heard late, never silent.
{
  const r = analyzeAudioProbe(log({
    notes,
    fxLatency: (i) => (i >= 8 ? 0.05 : 0),
    silentRing: 0.0,
    onsetFor: (i, s) => (i >= 8 ? s + 0.04 : s + 0.001),
  }));
  check(r.notes.missed === 0 && r.notes.spurious === 0, `declared fx latency: no missed / spurious (${JSON.stringify(r.notes)})`);
  check(r.notes.heard === 16 && r.notes.heardLate === 8, `declared fx latency: 8 notes heard late (${JSON.stringify(r.notes)})`);
  // The effect's delay is reported as such — and is not the scheduler's.
  check(r.fxDelayed.notes === 8 && near(r.fxDelayed.lagMs.p50, 40, 1), `declared fx latency: reported as fxDelayed 8 @ ~40 ms (${JSON.stringify(r.fxDelayed)})`);
  check(r.late.over20ms === 0 && r.lagVsIdealMs.max < 5, `declared fx latency: engine timing untouched (late>20 = ${r.late.over20ms}, lag max ${r.lagVsIdealMs.max})`);
  // Only by the DECLARED latency.
  const r2 = analyzeAudioProbe(log({
    notes,
    fxLatency: (i) => (i === 3 ? 0.01 : 0),
    onsetFor: (i, s) => (i === 3 ? s + 0.06 : s + 0.001),
  }));
  check(r2.notes.heard === 15 && r2.notes.spurious === 1, `an onset past the declared latency is not a match (${JSON.stringify(r2.notes)})`);
}

// 6. Lookahead retarget mid-play: every later note shifts by 20 ms.
{
  const r = analyzeAudioProbe(log({ notes, lookahead: (i) => (i < 8 ? 0.06 : 0.08), onsetFor: (i, s) => s + 0.001 }));
  check(r.lookahead.shifts === 1 && near(r.lookahead.maxShiftMs, 20, 0.2), `one 20 ms lookahead shift (got ${r.lookahead.shifts}/${r.lookahead.maxShiftMs})`);
}

// 7. Minimum drift steps up 25 ms at t = 3 s (an underrun); ±8 ms device jitter must NOT count.
{
  const jitter = (t) => ((Math.round(t * 10) % 3) - 1) * 4;
  const reps = busReports({ t0: 0.5, t1: 6, drift: (t) => jitter(t) + (t >= 3 ? 25 : 0) });
  const s = clockSlips(reps);
  check(s.count === 1 && s.totalMs >= 20 && s.totalMs <= 34, `one ~25 ms slip detected (got ${JSON.stringify(s)})`);
  const quiet = clockSlips(busReports({ t0: 0.5, t1: 6, drift: jitter }));
  check(quiet.count === 0, `burst jitter alone is not a slip (got ${JSON.stringify(quiet)})`);
}

// 8. stats() percentile sanity.
{
  const st = stats([5, 1, 3, 2, 4, NaN, null]);
  check(st.n === 5 && st.min === 1 && st.max === 5 && st.p50 === 3, `stats ignores non-finite, orders (${JSON.stringify(st)})`);
}

// 9. Cancelled notes are not "missed", and edit staleness is measured.
{
  const raw = log({ notes, onsetFor: (i, s) => (i === 6 ? null : s + 0.001) });
  raw.cancels = [{ pk: 7, ctx: 2.0 }]; // note index 6 → pk 7, cancelled before its start
  const r = analyzeAudioProbe(raw);
  check(r.notes.cancelled === 1 && r.notes.missed === 0, `cancelled note excluded from missed (${JSON.stringify(r.notes)})`);
}
{
  // Same pk at the same `when`: one note heard once, not a duplicate.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  const s0 = raw.starts.find((x) => x.pk === 7);
  raw.starts.push({ ...s0, path: "lane-fresh", ctx: s0.ctx + 0.01 });
  const r = analyzeAudioProbe(raw);
  check(r.notes.duplicateStarts === 0 && r.notes.heard === notes.length, `lane-fresh swap is not a duplicate start (dups ${r.notes.duplicateStarts}, ${JSON.stringify(r.notes)})`);
}
{
  // Mute edit at t=2.1: every later note (2.25 … 4.75 = 11 notes) still sounds → stale.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 2.1, label: 'mute row0', kind: 'mute', channel: 'kick' }] });
  check(r.edits.count === 1, 'one edit counted');
  check(r.edits.staleNotes === 11, `stale notes after a mute = every later heard note (got ${r.edits.staleNotes})`);
  check(near(r.edits.perKind.mute.maxStaleMs, (notes[15] - 2.1) * 1000, 1), `max stale ≈ last note − edit (got ${r.edits.perKind.mute.maxStaleMs})`);
}
{
  // Volume edit: notes after the edit carry the new vol except one straggler.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  for (const q of raw.requests) if (q.when > 2.1) q.vol = 0.5;
  raw.requests[5].vol = 1; // note at 2.25 kept the old volume → stale
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 2.1, label: 'vol', kind: 'vol', channel: 'kick', value: 0.5, prev: 1 }] });
  check(r.edits.staleNotes === 1 && near(r.edits.perKind.vol.maxStaleMs, 150, 1), `one stale volume note 150 ms after the edit (got ${JSON.stringify(r.edits.perKind.vol)})`);
}

{
  // Node volumes ≠ 1 (every 4th note 1.5×) must not read as stale.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  raw.requests.forEach((q, i) => {
    const k = i % 4 === 0 ? 1.5 : 1;
    q.vol = (q.when > 2.1 ? 0.5 : 1) * k;
  });
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 2.1, label: 'vol', kind: 'vol', channel: 'kick', value: 0.5, prev: 1 }] });
  check(r.edits.staleNotes === 0, `node volume ≠ 1 is not mistaken for a stale note (got ${JSON.stringify(r.edits.perKind.vol)})`);
}

{
  // Fresh notes must not be judged stale; a real straggler must be.
  const raw = log({ notes, onsetFor: (i, s) => s + 0.001 });
  const ks = [1.2, 1.5, 1, 1.2];
  raw.requests.forEach((q, i) => { q.vol = 0.7 * ks[i % 4]; });
  raw.requests[1].vol = 0.3 * ks[1]; // the note at 1.25 kept the OLD row vol 0.3 → stale
  const r = analyzeAudioProbe(raw, { editMarks: [{ t: 1.0, label: 'vol', kind: 'vol', channel: 'kick', value: 0.7, prev: 0.3 }] });
  check(r.edits.staleNotes === 1, `node vols learned from settled post-edit notes (got ${JSON.stringify(r.edits.perKind.vol)})`);
}

if (failures) {
  console.error(`\n${failures} FAILURE(S)`);
  process.exit(1);
}
console.log("\nPASS audio_probe_analysis");
