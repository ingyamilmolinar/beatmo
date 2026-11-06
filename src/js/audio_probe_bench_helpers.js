// audio_probe_bench_helpers.js — shared glue so every browser bench can report
// what the listener actually HEARD (audio.js playback probe +
// audio_probe_worklet.js + audio_probe_analysis.js), not only what the
// scheduler asked for.
//
// Usage in a bench (page = Playwright page on index.html / play_ui.html):
//
//   await startPlaybackProbe(page);            // after the AudioContext runs
//   ... play ...
//   const { report } = await stopPlaybackProbe(page, "BPM=200");
//   const flat = probeSummaryFields(report);   // merge into the bench JSON
//   failures.push(...probeGateFailures(report, PROBE_LIMITS_DEFAULT, "BPM=200"));
//
// Env:
//   AUDIO_PROBE=0            disable (benches then skip the probe entirely)
//   AUDIO_PROBE_STRICT=1     probe gates fail the bench (default: report only)
//   AUDIO_PROBE_DUMP=<dir>   write each raw log as JSON for offline analysis

import fs from "fs";
import path from "path";
import { analyzeAudioProbe, formatAudioProbeReport } from "./audio_probe_analysis.js";

export const PROBE_ENABLED = process.env.AUDIO_PROBE !== "0";
export const PROBE_STRICT = process.env.AUDIO_PROBE_STRICT === "1";

// Hard contract for request → BufferSource.start on render-cache hits: the
// Go batch is started synchronously, so it must never wait for a UI frame.
// Gated on p90 ≤ one 60 Hz frame. Not ~0 and not p99: this is wall-clock time
// inside a synchronous loop, so a 32-event catch-up batch on a loaded CPU (and
// the OS descheduling the browser on an oversubscribed host) stretches the
// tail — p99 read 9-45 ms at load 12-19 while p90 stayed low. The defect it
// guards shifts the WHOLE distribution: the legacy microtask flush measured
// p99 115.7 ms under a 150 ms stall, vs 0.5 ms synchronous.
export const PROBE_JSQUEUE_MAX_MS = 16;

// systemLoad is the 1-minute load average (0 where /proc/loadavg does not
// exist). Timing benches are meaningless on an oversubscribed machine (CLAUDE.md
// § Browser audio isolation, gotcha 2): skip or discard runs above a limit.
export function systemLoad() {
  try {
    return Number(fs.readFileSync("/proc/loadavg", "utf8").split(" ")[0]);
  } catch (_) {
    return 0;
  }
}

// Limits for "the listener hears the circuit as predicted". Timing limits are
// on UNPLANNED lateness (onset − scheduler request) and rhythm evenness, not
// on absolute latency: the lookahead cushion is deliberate.
export const PROBE_LIMITS_DEFAULT = {
  missedFrac: 0.02,       // ≤2% of judged notes silent in the output
  dropped: 0,             // requested but never started
  lateOver20ms: 0,        // a note >20 ms late is an audible flam/stumble
  lagVsIdealP99Ms: 10,
  ioiErrP95Ms: 5,         // rhythm evenness
  slips: 0,               // audio-thread underruns (audible dropouts)
  masterClicks: 0,
};

export async function startPlaybackProbe(page, opts = {}) {
  if (!PROBE_ENABLED) return { ok: false, disabled: true };
  const res = await page.evaluate(async (o) => {
    if (typeof window.startAudioProbe !== "function") return { ok: false, error: "startAudioProbe missing" };
    return window.startAudioProbe(o);
  }, opts);
  if (!res?.ok) console.warn(`[probe] not started: ${res?.error ?? "unknown"}`);
  return res;
}

export async function stopPlaybackProbe(page, label = "", analysisOpts = {}) {
  if (!PROBE_ENABLED) return { raw: null, report: null };
  const raw = await page.evaluate(async () => (typeof window.stopAudioProbe === "function" ? window.stopAudioProbe() : null));
  if (!raw) return { raw: null, report: null };
  const dumpDir = process.env.AUDIO_PROBE_DUMP;
  if (dumpDir) {
    fs.mkdirSync(dumpDir, { recursive: true });
    const safe = (label || "probe").replace(/[^a-z0-9_.-]+/gi, "_");
    // Marks ride along so a dump can be re-analysed offline (analysis fixes
    // never require re-running a browser).
    fs.writeFileSync(path.join(dumpDir, `${safe}.json`), JSON.stringify({ ...raw, editMarks: analysisOpts.editMarks || [] }));
  }
  const report = analyzeAudioProbe(raw, analysisOpts);
  console.log(`  [probe${label ? " " + label : ""}] ${formatAudioProbeReport(report)}`);
  return { raw, report };
}

// Flat fields for bench-results JSON / bench-compare tables.
export function probeSummaryFields(r) {
  if (!r) return {};
  return {
    probeNotesJudged: r.notes.judged,
    probeNotesHeard: r.notes.heard,
    probeNotesHeardLate: r.notes.heardLate,
    probeFxDelayedNotes: r.fxDelayed ? r.fxDelayed.notes : 0,
    probeFxDelayedLagP99MS: r.fxDelayed ? r.fxDelayed.lagMs.p99 : null,
    probeNotesMissed: r.notes.missed,
    probeNotesDropped: r.notes.dropped,
    probeNotesSpurious: r.notes.spurious,
    probeLagGridP50MS: r.lagVsGridMs.p50,
    probeLagGridP99MS: r.lagVsGridMs.p99,
    probeLagGridStdMS: r.lagVsGridMs.std,
    probeLagIdealP50MS: r.lagVsIdealMs.p50,
    probeLagIdealP99MS: r.lagVsIdealMs.p99,
    probeLagIdealMaxMS: r.lagVsIdealMs.max,
    probeLateOver10: r.late.over10ms,
    probeLateOver20: r.late.over20ms,
    probeLateOver50: r.late.over50ms,
    probeStartLateP99MS: r.startLateMs.p99,
    probeArrivedLate: r.arrivedLate,
    probeJsQueueP90MS: r.jsQueueMs.p90,
    probeJsQueueP99MS: r.jsQueueMs.p99,
    probeRenderWaitP99MS: r.renderWaitMs.p99,
    probeDetectP50MS: r.detectMs.p50,
    probeIoiErrP95MS: r.ioiErrMs.p95,
    probeIoiErrMaxMS: r.ioiErrMs.max,
    probeLookaheadShifts: r.lookahead.shifts,
    probeLookaheadMaxShiftMS: r.lookahead.maxShiftMs,
    probeSlips: r.audioThread.slips,
    probeSlipMS: r.audioThread.slipMs,
    probeMasterClicks: r.quality.masterClicks,
    probeMasterCuts: r.quality.masterCuts,
    probeMasterClipped: r.quality.masterClipped,
    probeChannelClicks: r.quality.channelClicks,
    probeLevelOutliers: r.quality.levelOutliers,
    probeLevelJudged: r.quality.levelJudged,
    probeMainStallsOver50: r.mainThread.over50ms,
    probeMainStallMaxMS: r.mainThread.maxMs,
    probeOutputLatencyMS: r.outputLatencyMs,
    probeNotesCancelled: r.notes.cancelled,
    probeEditStaleNotes: r.edits.staleNotes,
    probeEditStaleMaxMS: r.edits.staleMs.max,
  };
}

// Returns failure strings (empty = pass). Callers decide whether they are
// fatal (PROBE_STRICT) or informational.
export function probeGateFailures(r, limits = PROBE_LIMITS_DEFAULT, label = "") {
  const out = [];
  if (!r) return out;
  const L = { ...PROBE_LIMITS_DEFAULT, ...limits };
  const tag = label ? `[${label}] ` : "";
  const chk = (cond, msg) => { if (!cond) out.push(tag + msg); };
  chk(r.notes.judged > 0, "probe judged no notes (nothing reached the output window)");
  if (r.notes.judged > 0) {
    chk(r.notes.missed / r.notes.judged <= L.missedFrac, `missed ${r.notes.missed}/${r.notes.judged} notes > ${(L.missedFrac * 100).toFixed(1)}%`);
  }
  chk(r.notes.dropped <= L.dropped, `dropped ${r.notes.dropped} notes > ${L.dropped}`);
  chk(r.late.over20ms <= L.lateOver20ms, `${r.late.over20ms} notes >20ms late > ${L.lateOver20ms}`);
  if (r.lagVsIdealMs.p99 != null) chk(r.lagVsIdealMs.p99 <= L.lagVsIdealP99Ms, `lag vs request p99 ${r.lagVsIdealMs.p99}ms > ${L.lagVsIdealP99Ms}ms`);
  if (r.ioiErrMs.p95 != null) chk(r.ioiErrMs.p95 <= L.ioiErrP95Ms, `IOI error p95 ${r.ioiErrMs.p95}ms > ${L.ioiErrP95Ms}ms`);
  chk(r.audioThread.slips <= L.slips, `${r.audioThread.slips} audio-thread slips (${r.audioThread.slipMs}ms) > ${L.slips}`);
  if (r.quality.masterClicks != null) chk(r.quality.masterClicks <= L.masterClicks, `${r.quality.masterClicks} clicks on master > ${L.masterClicks}`);
  return out;
}

// startPlaybackWhenReady presses Play only once the playback-readiness gate
// (warm-up of every row's voices) has opened, and confirms the transport is
// actually running. A bench that calls startPlay() straight after loading or
// building a circuit is silently ignored by the gate ("[warmup] play ignored
// while warming") — webaudio_three_stage_latency measured nothing but `null`
// for that reason.
export async function startPlaybackWhenReady(page, { bpm, timeoutMs = 30000 } = {}) {
  await page.waitForFunction(
    () => typeof window.warmupProgress !== "function" || window.warmupProgress().ready === true,
    null, { timeout: timeoutMs },
  );
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    const playing = await page.evaluate((b) => {
      if (b) setBPM(b);
      if (typeof isPlaying === "function" && isPlaying()) return true;
      startPlay();
      return typeof isPlaying !== "function" || isPlaying();
    }, bpm ?? 0);
    if (playing) return;
    if (Date.now() > deadline) throw new Error("startPlaybackWhenReady: transport never started");
    await page.waitForTimeout(200);
  }
}

// markEdit performs an edit in the page and returns its mark, stamped with the
// AudioContext time read in the SAME evaluate — so marks work against any
// build, including the frozen pre-lane baseline that has no mark API.
// editFnSource is the body of a function taking `arg`, e.g. "toggleMute(0)".
export async function markEdit(page, mark, editFnSource, arg) {
  const t = await page.evaluate(({ src, arg }) => {
    const t = window.__audioCtx ? window.__audioCtx.currentTime : 0;
    // eslint-disable-next-line no-new-func
    new Function("arg", src)(arg);
    return t;
  }, { src: editFnSource, arg });
  return { ...mark, t };
}
