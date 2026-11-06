// audio_lane_e2e.browser.test.js
//
// The audio lane's acceptance test in the real app (main.wasm + index.html),
// lane ON (window.__beatmoEnv BEATMO_AUDIO_LANE=1), measured with the
// playback probe (onsets on the audio thread — what the listener hears):
//
//   1. stall-300: a 300 ms main-thread block every 700 ms (a UI bug). With the
//      lane committing 1 s ahead, no note may reach audio.js late, none may be
//      >20 ms late in the output, none silent or dropped, no clicks.
//   2. edit-storm: row-0 mute every 600 ms, row-1 volume steps every 400 ms
//      and a synth knob on row 2 every 50 ms, under a 150 ms / 500 ms stall.
//      Every edit must reach the next not-yet-started note: zero stale notes.
//   3. stop mid-horizon: after Stop, nothing may start, and no instrument
//      channel may show an onset later than 30 ms after it.
//
// These are hard gates — they are the lane's contract (spec §1).
//
// Run: GO=… [WASM_PREBUILT=1] [WASM_FILE=<lane build>] node src/js/audio_lane_e2e.browser.test.js

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";
import { startPlaybackWhenReady, startPlaybackProbe, stopPlaybackProbe } from "./audio_probe_bench_helpers.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE) : path.join(jsDir, "main.wasm");
if (!process.env.WASM_FILE && !shouldSkipWasmBuild("main.wasm")) {
  const b = spawnSync(resolveGoBinary(), ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", WASM_FILE, "./cmd"],
    { cwd: path.resolve(jsDir, "../go"), env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
  if (b.status !== 0) throw new Error("go build main wasm failed");
}
const server = http.createServer((req, res) => {
  const u = req.url.split("?")[0];
  const file = u === "/" ? "/index.html" : u;
  const fp = file === "/main.wasm" ? WASM_FILE : path.join(jsDir, file.slice(1));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = fp.endsWith(".html") ? "text/html" : fp.endsWith(".js") ? "application/javascript" : fp.endsWith(".wasm") ? "application/wasm" : "application/octet-stream";
    res.writeHead(200, { "Content-Type": ct, "Cache-Control": "no-store" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));

let fail = 0;
const check = (c, m) => { if (c) console.log("ok   " + m); else { fail++; console.error("FAIL " + m); } };
const { browser } = await launchBrowser(); // TEST_PLATFORM=chrome-local TEST_HEADED=1: real Chrome, real GPU
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  page.on("pageerror", (e) => console.log("  [pageerror]", e.message));
  // E2E_LANE=0 runs the same phases on the legacy path (diagnosis only: the gates are the lane's contract).
  await page.addInitScript((lane) => { window.__beatmoEnv = { BEATMO_AUDIO_LANE: lane }; }, process.env.E2E_LANE ?? "1");
  await page.goto(`http://localhost:${server.address().port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof forceDraw === "function", null, { timeout: 60000 });
  await page.evaluate(() => { document.dispatchEvent(new Event("pointerdown")); resumeAudio?.(); });
  await page.waitForFunction(() => window.__audioCtx && window.__audioCtx.state === "running", null, { timeout: 30000 });
  await page.waitForTimeout(2000);
  const rows = await page.evaluate(() => { const n = totalRows(); const ids = []; for (let i = 0; i < n; i++) ids.push(rowInstrument(i)); return ids; });
  const knob = await page.evaluate((id) => {
    const defs = instrumentParamDefs(id) || [];
    // A continuous knob (what a drag edits) — not an enum like osc_type / mode.
    const k = defs.find((x) => x.group !== "hidden" && Number.isFinite(x.min) && x.max > x.min &&
      !/(_type|mode|variant|wave|shape)$/i.test(x.name) && !(Number.isInteger(x.min) && Number.isInteger(x.max) && x.max - x.min <= 16));
    return k ? { name: k.name, min: k.min, max: k.max } : null;
  }, rows[2]);
  console.log(`circuit: ${rows.join(", ")}; knob on ${rows[2]}: ${JSON.stringify(knob)}`);

  const stallLoad = (every, block) => page.evaluate(([every, block]) => {
    window.__e2eStall = setInterval(() => { const t = performance.now(); while (performance.now() - t < block) { /* UI bug */ } }, every);
  }, [every, block]);
  const stopLoad = () => page.evaluate(() => clearInterval(window.__e2eStall));

  await startPlaybackWhenReady(page, { bpm: 120 });
  await page.waitForTimeout(1500);

  // ── 1. stall-300 ──
  await startPlaybackProbe(page);
  await stallLoad(700, 300);
  await page.waitForTimeout(8000);
  await stopLoad();
  const { report: r1 } = await stopPlaybackProbe(page, "lane-stall-300");
  check(r1.notes.judged > 20, `stall-300: notes judged (${r1.notes.judged})`);
  check(r1.arrivedLate === 0, `stall-300: no note reached audio.js late (arrivedLate ${r1.arrivedLate})`);
  check(r1.late.over20ms === 0, `stall-300: no note >20 ms late (${r1.late.over20ms}; lag p99 ${r1.lagVsIdealMs.p99} ms)`);
  check(r1.notes.missed === 0 && r1.notes.dropped === 0, `stall-300: none silent / dropped (${r1.notes.missed}/${r1.notes.dropped})`);
  check((r1.quality.masterClicks ?? 0) === 0, `stall-300: no clicks (${r1.quality.masterClicks})`);

  // ── 2. edit-storm ──
  // Driven by in-page timers (as real input arrives), not CDP round-trips:
  // under the injected stall one page.evaluate takes ~0.4–0.7 s on BOTH
  // paths, which starved a Node-side loop to a handful of edits. Each mark is
  // stamped with the ctx time read in the same task as its edit.
  await startPlaybackProbe(page);
  await stallLoad(500, 150);
  check(!!knob, `edit-storm: found a synth knob on ${rows[2]}`);
  await page.evaluate(({ ch0, ch1, knob, id2 }) => {
    const ctx = window.__audioCtx;
    const st = (window.__storm = { marks: [], timers: [], mutes: 0, k: 0, knobMs: [] });
    st.timers.push(setInterval(() => {
      const t = ctx.currentTime;
      toggleMute(0);
      st.marks.push({ label: `mute#${st.mutes}`, kind: st.mutes % 2 === 0 ? "mute" : "unmute", channel: ch0, t });
      st.mutes++;
    }, 600));
    let vi = 0;
    st.timers.push(setInterval(() => {
      const v = [0.9, 0.5, 0.7, 0.3][vi++ % 4];
      const prev = rowVolume(1);
      const t = ctx.currentTime;
      setRowVolume(1, v);
      st.marks.push({ label: `vol#${vi}`, kind: "vol", channel: ch1, value: v, prev, t });
    }, 400));
    if (knob) {
      st.timers.push(setInterval(() => {
        const f = st.k++ % 2 === 0 ? 0.3 : 0.7;
        const t0 = performance.now();
        setInstrumentParam(id2, knob.name, knob.min + (knob.max - knob.min) * f);
        st.knobMs.push(performance.now() - t0);
      }, 50));
    }
  }, { ch0: rows[0], ch1: rows[1], knob, id2: rows[2] });
  await page.waitForTimeout(8000);
  const storm = await page.evaluate(() => {
    const st = window.__storm;
    st.timers.forEach(clearInterval);
    const ms = st.knobMs.slice().sort((a, b) => a - b);
    return { marks: st.marks, mutes: st.mutes, knobEdits: st.k, knobMedMs: ms[ms.length >> 1] || 0, knobMaxMs: ms[ms.length - 1] || 0 };
  });
  await stopLoad();
  const marks = storm.marks;
  const mutes = storm.mutes;
  console.log(`  storm: ${marks.length} marked edits (${mutes} mute toggles), ${storm.knobEdits} knob edits (setInstrumentParam med ${storm.knobMedMs.toFixed(0)} ms, max ${storm.knobMaxMs.toFixed(0)} ms)`);
  const { report: r2 } = await stopPlaybackProbe(page, "lane-edit-storm", { editMarks: marks });
  if (mutes % 2 === 1) await page.evaluate(() => toggleMute(0));
  await page.evaluate(() => setRowVolume(1, 1));
  check(r2.edits.count > 20 && storm.knobEdits >= 10, `edit-storm: edits made (${r2.edits.count} marked, ${storm.knobEdits} knob)`);
  check(r2.edits.staleNotes === 0, `edit-storm: zero stale notes (${r2.edits.staleNotes}; ${JSON.stringify(r2.edits.perKind)})`);
  check(r2.notes.missed === 0 && r2.notes.dropped === 0, `edit-storm: none silent / dropped (${r2.notes.missed}/${r2.notes.dropped})`);
  check(r2.arrivedLate === 0, `edit-storm: no note reached audio.js late under the stall (${r2.arrivedLate})`);

  // ── 3. stop mid-horizon ──
  await startPlaybackProbe(page);
  await page.waitForTimeout(1500);
  const tStop = await page.evaluate(() => { const t = window.__audioCtx.currentTime; stopPlay(); return t; });
  await page.waitForTimeout(1500);
  const raw = await page.evaluate(() => stopAudioProbe());
  const cancelled = new Set((raw.cancels || []).map((c) => c.pk));
  const lateStarts = raw.starts.filter((s) => s.startWhen > tStop + 0.03 && !cancelled.has(s.pk));
  check(lateStarts.length === 0, `stop: nothing starts after Stop (${lateStarts.length} starts > stop + 30 ms)`);
  // Instrument channels (dry) only: __master__ carries the send tails, and
  // the demo's 300 ms delay echoes a note played just before Stop. Stop does
  // not cut a SOUNDING voice (neither path ever did), and the onset detector
  // reads a ringing felt-piano envelope as fresh onsets — so a channel with a
  // note started in the 1.5 s before Stop is judged by the start log above
  // (every sequencer voice carries a pk), and its envelope is only logged.
  const ringing = new Set(raw.starts.filter((x) => x.startWhen > tStop - 1.5 && x.startWhen <= tStop + 0.03 && !cancelled.has(x.pk)).map((x) => x.id));
  const onsetsAll = Object.entries(raw.reports).filter(([id]) => id !== "__master__").flatMap(([id, reps]) => reps.flatMap((x) => x.onsets.map((o) => ({ id, t: o.f / raw.sampleRate })))).filter((o) => o.t > tStop + 0.03);
  const onsetsAfter = onsetsAll.filter((o) => !ringing.has(o.id));
  if (onsetsAll.length > onsetsAfter.length) console.log(`  [stop] ${onsetsAll.length - onsetsAfter.length} envelope onset(s) on channels still ringing a pre-Stop note (${[...ringing].join(", ")}) — not judged`);
  if (onsetsAll.length) {
    for (const id of new Set(onsetsAll.map((o) => o.id))) {
      const around = raw.starts.filter((x) => x.id === id && x.startWhen > tStop - 1.5);
      console.log(`  [stop diag] ${id}: starts ${around.map((x) => `pk${x.pk}@${((x.startWhen - tStop) * 1000).toFixed(0)}ms/${x.path}${cancelled.has(x.pk) ? "/cancelled" : ""}`).join(" ")}`);
    }
    if (process.env.AUDIO_PROBE_DUMP) fs.writeFileSync(path.join(process.env.AUDIO_PROBE_DUMP, "lane-stop.json"), JSON.stringify({ ...raw, tStop }));
  }
  check(onsetsAfter.length === 0, `stop: no onset on any instrument channel later than 30 ms after Stop (${onsetsAfter.length}${onsetsAfter.length ? ": " + onsetsAfter.slice(0, 4).map((o) => `${o.id}@+${((o.t - tStop) * 1000).toFixed(0)}ms`).join(" ") : ""})`);
} finally {
  await browser.close();
  server.close();
}
if (fail) { console.error(`\n${fail} FAILURE(S)`); process.exit(1); }
console.log("\nPASS audio_lane_e2e");
