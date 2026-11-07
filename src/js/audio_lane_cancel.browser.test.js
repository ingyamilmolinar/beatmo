// The lane commits notes up to ~1 s ahead and must take them back, verified on the real audio.js with the playback
// probe (so "silent" is silent in the output): a cancelled voice never sounds, a kept one does, previews survive, and a
// stop fades only voices started before it. Usage: node src/js/audio_lane_cancel.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { waitForAudioReady } from "./browser_test_helpers.js";
import { setupFullWasm } from "./real_input_test_helpers.js";

// A page without Go has no instruments, and the lead times assume a QUIET page: the blocks are read once from the
// real app, then installed on a bare audio.js page.
const IDS = ["kick", "cowbell", "tom", "rimshot", "clap", "snare", "hihat", "organ-church"];
let blocks;
{
  const app = await setupFullWasm({ logLevel: "ERROR" });
  try {
    blocks = await app.page.evaluate((ids) => Object.fromEntries(ids.map((id) => {
      const s = window.__instrumentBlockForTest(id);
      return [id, s && { block: Array.from(s.block), seconds: s.seconds, amp: s.amp, melodic: s.melodic, gateTail: s.gateTail, fx: s.fx }];
    })), IDS);
  } finally {
    await app.cleanup();
  }
  const missing = IDS.filter((id) => !blocks[id]);
  if (missing.length) throw new Error(`the app pushed no block for ${missing.join(", ")}`);
}

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const HTML = `<!doctype html><meta charset="utf-8"><script type="module">
  import './audio.js'; window.__ready = true;</script>`;
const server = http.createServer((req, res) => {
  if (req.url === "/") { res.writeHead(200, { "Content-Type": "text/html" }); res.end(HTML); return; }
  fs.readFile(path.join(jsDir, req.url.slice(1).split("?")[0]), (e, d) => {
    if (e) { res.writeHead(404); res.end(); return; }
    const ct = req.url.endsWith(".js") ? "application/javascript" : req.url.endsWith(".wasm") ? "application/wasm" : "application/octet-stream";
    res.writeHead(200, { "Content-Type": ct }); res.end(d);
  });
});
await new Promise((r) => server.listen(0, r));
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
let fail = 0;
const check = (c, m) => { if (c) console.log("ok   " + m); else { fail++; console.error("FAIL " + m); } };
try {
  const page = await browser.newPage();
  page.on("pageerror", (e) => console.log("  [pageerror]", e.message));
  await page.goto(`http://localhost:${server.address().port}/`);
  await page.waitForFunction(() => window.__ready);
  await page.evaluate((bs) => {
    for (const [id, s] of Object.entries(bs)) setInstrumentBlock(id, Float64Array.from(s.block), s.seconds, s.amp, s.melodic, s.gateTail, s.fx);
  }, blocks);

  // Schedule one lane event; read master onsets and the envelope peak over a window (seconds from t0).
  await page.evaluate(() => {
    window.__ev = (id, vid, when, pitch = 0) =>
      playSoundsBatchFlat([id], [1], [pitch], [1], [when], [true], [0], [vid], [when], [when], [vid]);
    window.__master = (raw, t0) => {
      const reps = raw.reports.__master__ || [];
      const onsets = reps.flatMap((x) => x.onsets.map((o) => o.f / raw.sampleRate - t0));
      const clicks = reps.reduce((a, x) => a + x.clicks.length, 0);
      const peakIn = (a, b) => {
        let pk = 0;
        for (const r of reps) {
          if (!r.env) continue;
          for (let k = 0; k < r.env.length; k++) {
            const t = (r.envStart + k * r.envFrames) / raw.sampleRate - t0;
            if (t >= a && t < b && r.env[k] > pk) pk = r.env[k];
          }
        }
        return pk;
      };
      return { onsets, clicks, peakIn };
    };
  });

  // ── A: cancel before start / keep / vid 0 / cancelAll (warm kick) ──
  await page.evaluate(() => document.dispatchEvent(new Event("pointerdown")));
  await waitForAudioReady(page);
  const a = await page.evaluate(async () => {
    await ensureSynthSample("kick");
    const ctx = window.__audioCtx;
    await startAudioProbe({ channels: false });
    const t0 = ctx.currentTime;
    __ev("kick", 101, t0 + 0.40); // cancelled well before start
    __ev("kick", 102, t0 + 0.80); // kept
    __ev("kick", 0, t0 + 1.25);   // preview-like (vid 0), still FUTURE at cancelAll: must survive
    __ev("kick", 103, t0 + 1.60); // future lane voice: removed by cancelAllLaneFuture
    await new Promise((r) => setTimeout(r, 100));
    const refusedFuture = cancelLaneVoices([101]);
    const s101 = laneVoiceStateForTest(101);
    while (ctx.currentTime < t0 + 1.0) await new Promise((r) => setTimeout(r, 20));
    const refusedStarted = cancelLaneVoices([102]); // started at +0.80: refused, reported
    cancelAllLaneFuture();
    while (ctx.currentTime < t0 + 2.0) await new Promise((r) => setTimeout(r, 20));
    const raw = await stopAudioProbe();
    const m = __master(raw, t0);
    return { s101, onsets: m.onsets, clicks: m.clicks, refusedFuture, refusedStarted };
  });
  const nearA = (t) => a.onsets.some((o) => Math.abs(o - t) < 0.02);
  check(a.s101 === "cancelled", `cancelled voice reports state 'cancelled' (${a.s101})`);
  check(Array.isArray(a.refusedFuture) && a.refusedFuture.length === 0, `cancelling a future voice refuses nothing (${JSON.stringify(a.refusedFuture)})`);
  check(JSON.stringify(a.refusedStarted) === "[102]", `cancelling a started voice reports it as refused — Go drops its replacement (${JSON.stringify(a.refusedStarted)})`);
  check(!nearA(0.40), "a lane voice cancelled before start is silent");
  check(nearA(0.80), "a kept lane voice sounds");
  check(nearA(1.25), `a future vid-0 voice survives cancelAllLaneFuture (onsets ${a.onsets.map((x) => x.toFixed(3)).join(",")})`);
  check(!nearA(1.60), "cancelAllLaneFuture removed the future lane voice");
  check(a.clicks === 0, `no clicks (${a.clicks})`);

  // ── B: tombstone — cancel while the render is still in flight ──
  const b = await page.evaluate(async () => {
    const ctx = window.__audioCtx;
    const cand = ["cowbell", "tom", "rimshot", "clap", "snare", "hihat"];
    const id = cand.find((x) => !voiceCachedFor(x, 0)) || null;
    if (!id) return { skipped: true };
    await startAudioProbe({ channels: false });
    const t0 = ctx.currentTime;
    __ev(id, 201, t0 + 0.30); // cold miss → deferred replay after the worker render
    cancelLaneVoices([201]);  // not started anywhere yet → tombstone
    const state = laneVoiceStateForTest(201);
    await ensureSynthSample(id); // the render lands
    while (ctx.currentTime < t0 + 1.0) await new Promise((r) => setTimeout(r, 20));
    const raw = await stopAudioProbe();
    const m = __master(raw, t0);
    return { id, state, onsets: m.onsets };
  });
  if (b.skipped) {
    console.log("skip tombstone case: every candidate instrument was already cached");
  } else {
    check(b.state === "cancelled", `queued cancel is tombstoned (${b.state}, ${b.id})`);
    check(b.onsets.length === 0, `a tombstoned deferred replay never plays (${b.id}: onsets ${b.onsets.map((x) => x.toFixed(3))})`);
  }

  // ── C: stopLaneAt spares the voice that starts after t ──
  const c = await page.evaluate(async () => {
    const ctx = window.__audioCtx;
    const id = "organ-church";
    await ensureSynthSample(id);
    await startAudioProbe({ channels: false });
    const t0 = ctx.currentTime;
    __ev(id, 301, t0 + 0.10);
    __ev(id, 302, t0 + 0.70);
    await new Promise((r) => setTimeout(r, 50));
    stopLaneAt(id, t0 + 0.40);
    while (ctx.currentTime < t0 + 1.1) await new Promise((r) => setTimeout(r, 20));
    const raw = await stopAudioProbe();
    const m = __master(raw, t0);
    return { onsets: m.onsets, before: m.peakIn(0.2, 0.38), gap: m.peakIn(0.47, 0.66), clicks: m.clicks };
  });
  check(c.before > 0.001, `the first voice sounded before t (peak ${c.before.toFixed(4)})`);
  check(c.gap < c.before * 0.05, `stopLaneAt silenced the first voice after t (gap peak ${c.gap.toFixed(4)} vs ${c.before.toFixed(4)})`);
  check(c.onsets.some((o) => Math.abs(o - 0.70) < 0.02), "the voice starting after t still sounds");
  check(c.clicks === 0, `no clicks (${c.clicks})`);

  // A lane voice on a STALE record must not hand its timing to the main thread: it is scheduled on the stale buffer at
  // once and swapped only if the fresh render lands before it starts.
  const d = await page.evaluate(async () => {
    const ctx = window.__audioCtx;
    await ensureSynthSample("kick");
    // Case C's organ rings on; over its tail the onset detector can miss these kicks although they sound.
    await new Promise((r) => setTimeout(r, 3000));
    await startAudioProbe({ channels: false });
    { // generation bump: re-push kick's own block, so the cached render is stale
      const s = __instrumentBlockForTest("kick");
      setInstrumentBlock("kick", s.block, s.seconds, s.amp, s.melodic, s.gateTail, s.fx);
    }
    const t0 = ctx.currentTime;
    __ev("kick", 401, t0 + 0.30);
    const until = performance.now() + 600; // main thread blocked past the start
    while (performance.now() < until) { /* UI stall */ }
    __ev("kick", 402, t0 + 1.10); // no stall: the fresh render lands first
    while (ctx.currentTime < t0 + 1.5) await new Promise((r) => setTimeout(r, 20));
    const raw = await stopAudioProbe();
    const m = __master(raw, t0);
    const startsOf = (pk) => raw.starts.filter((x) => x.pk === pk).map((x) => x.path);
    const pkOf = (when) => (raw.requests.find((r) => Math.abs(r.when - (t0 + when)) < 1e-6) || {}).pk;
    const reps = raw.reports.__master__ || [];
    const cov = reps.length ? [reps[0].startFrame / raw.sampleRate - t0, reps[reps.length - 1].endFrame / raw.sampleRate - t0] : null;
    return { onsets: m.onsets, clicks: m.clicks, s401: startsOf(pkOf(0.30)), s402: startsOf(pkOf(1.10)),
      peaks: [m.peakIn(0.0, 0.25), m.peakIn(0.30, 0.40), m.peakIn(1.10, 1.20)], cov,
      starts: raw.starts.map((x) => [x.pk, +(x.startWhen - t0).toFixed(3), +(x.ctx - t0).toFixed(3), x.path]) };
  });
  console.log(`  [D] peaks pre/0.30/1.10 ${d.peaks.map((x) => x.toFixed(4)).join("/")} coverage ${JSON.stringify(d.cov)} starts ${JSON.stringify(d.starts)}`);
  const nearD = (t) => d.onsets.filter((o) => Math.abs(o - t) < 0.02).length;
  check(nearD(0.30) === 1, `stale record + blocked main thread: the lane voice still starts ON TIME (onsets ${d.onsets.map((x) => x.toFixed(3)).join(",")}; paths ${d.s401})`);
  check(!d.onsets.some((o) => o > 0.33 && o < 1.0), "no late copy of the stalled voice after the main thread returns");
  check(nearD(1.10) === 1 && d.s402[d.s402.length - 1] === "lane-fresh", `fresh render landed first: the voice plays the FRESH buffer once (paths ${d.s402})`);
  check(d.clicks === 0, `no clicks (${d.clicks})`);

  // Scheduling and cancelling many lane voices at other volumes must never cut a voice still ringing.
  const e = await page.evaluate(async () => {
    const ctx = window.__audioCtx;
    const id = "organ-church";
    await ensureSynthSample(id);
    await new Promise((r) => setTimeout(r, 1500));
    await startAudioProbe({ channels: false });
    const t0 = ctx.currentTime;
    const ev = (vid, when, vol) => playSoundsBatchFlat([id], [vol], [0], [1], [when], [true], [0], [vid], [when], [when], [vid]);
    ev(501, t0 + 0.10, 0.83); // a sustained voice on its own volume bus
    while (ctx.currentTime < t0 + 0.40) await new Promise((r) => setTimeout(r, 10));
    const vids = [];
    for (let k = 0; k < 20; k++) { ev(600 + k, t0 + 3.0, (k + 1) / 8); vids.push(600 + k); } // 20 more buses (VOL_Q = 16: distinct keys)
    cancelLaneVoices(vids);
    while (ctx.currentTime < t0 + 1.0) await new Promise((r) => setTimeout(r, 20));
    const raw = await stopAudioProbe();
    const m = __master(raw, t0);
    const reps = raw.reports.__master__ || [];
    const cuts = reps.reduce((a, x) => a + (x.cuts ? x.cuts.length : 0), 0);
    return { before: m.peakIn(0.2, 0.38), after: m.peakIn(0.55, 0.9), clicks: m.clicks, cuts };
  });
  check(e.before > 0.001, `E: the sustained voice sounds (peak ${e.before.toFixed(4)})`);
  check(e.after > e.before * 0.3, `E: new volume buses do not cut a ringing voice (peak after ${e.after.toFixed(4)} vs ${e.before.toFixed(4)})`);
  check(e.clicks === 0 && e.cuts === 0, `E: no click / hard cut (${e.clicks}/${e.cuts})`);
} finally {
  await browser.close();
  server.close();
}
if (fail) { console.error(`\n${fail} FAILURE(S)`); process.exit(1); }
console.log("\nPASS audio_lane_cancel");
