/**
 * Generator audible regression test (WASM — primary platform), post
 * native-synth-deprecation model.
 *
 * The gen_type "Generator" selector was REMOVED by the native-deprecation
 * migration (MigrateGenType in Go; defensive `delete params.gen_type` in
 * audio.js). Waveform choice now lives in two places:
 *   - bespoke drums: the per-family `<family>_wave` knob (Sine/Saw/Square/
 *     Triangle), e.g. `snare_wave`, flowing through the family param block in
 *     synth_param_abi.gen.js into render_snare_p;
 *   - FM + Noise generators: the unified `modular` voice's `osc_type`
 *     (0=Sine … 4=FM 5=Noise White 6=Noise Pink) via render_modular_p.
 *
 * REGRESSION GUARD: user reported that switching the oscillator off the
 * native waveform in WASM produced SILENCE. This drives the real
 * synth→Go→JS→C path (setInstrumentParam → platformInstrumentParamsChanged →
 * updateInstrumentParams → renderToCache → render_X_p) and asserts every
 * generator produces an audible buffer, that the per-stage enable flags land
 * on the right C fields (the exact heap region — osc_enabled..drive_enabled,
 * modular indices 27-31 — whose stale-ABI misalignment caused the silence),
 * and that the native render is bit-stable across a knob round-trip.
 *
 * COVERED-BY-GO (C render correctness — NOT re-tested here):
 *   internal/audio/native_wave_knob_test.go (wave knob mutates per family),
 *   internal/audio/modular_render_test.go + modular_stage_io_test.go
 *   (osc bypass = documented silence, env bypass = audible no-ADSR render).
 * This test owns ONLY the WASM↔JS↔C bridge boundary for those params.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => { try { console.log("[PAGE]", msg.type(), msg.text()); } catch (_) {} });

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

const headDiff = (a, b) => {
  const n = Math.min(a.head.length, b.head.length);
  let d = 0;
  for (let i = 0; i < n; i++) d += Math.abs(a.head[i] - b.head[i]);
  return d;
};

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof setInstrumentParam === "function");
  await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function");
  await page.waitForFunction(() => window.audioReady !== undefined);
  await page.evaluate(async () => { await window.audioReady; });

  // Early ABI sanity: a stale synth_param_abi.gen.js is the root cause of the
  // "silent generator" bug — surface it before the audio assertions.
  const abi = await page.evaluate(() => window.__synthABI || null);
  console.log("[TEST] __synthABI =", JSON.stringify(abi));
  if (abi && abi.stale) fail("synth_param_abi.gen.js is STALE — modular-voiced instruments will be silent (run `make gen-synth-abi`).");
  if (abi && !abi.hasModularFields) fail("ABI is missing the modular enable-flag fields — stage toggles cannot reach C.");

  // EVERY legacy family migrated to the modular engine — render_snare_p /
  // render_hihat_p / render_fm_bass_p etc. and their family ABI blocks are ALL
  // deleted (FM in Phase-7, the LAST). This per-recipe wave-knob check still uses
  // fm-bass, but it is now a MIGRATED instrument: its RENDER_INFO is
  // paramBlock:'modular', so setInstrumentParam("fm-bass","fm_wave",v) flows
  // through the platform-push seam (modularPushParamsForInstrument →
  // fmRecipeToModular), which translates the legacy fm_wave knob onto the modular
  // block's gen1_wave column, rendered by render_modular_p (source==10 FM voice).
  // The migrated drum wave knobs are covered end-to-end by the xplat audio-compare
  // suite (hihat-wave-sine / fm-bass-wave-saw / snare-wave-triangle paramCases) and
  // the Go native wave knob test.
  const INST = "fm-bass"; // a MIGRATED FM instrument (render_modular_p, source==10)
  const MOD = "modular"; // the unified modular voice (render_modular_p)

  // ── Baseline: the migrated fm-bass must be audible. ──
  await page.evaluate((id) => resetInstrumentParams(id), INST);
  const native = await page.evaluate((id) => window.__testCaptureSynthRender(id), INST);
  if (!native) throw new Error("native render snapshot null");
  console.log(`[TEST] Native peak=${native.peak} rms=${native.rms} tailRMS=${native.tailRMS}`);
  if (!(native.peak > 0)) fail(`Native ${INST} is silent (peak=${native.peak})`);

  // ── Per-recipe wave knob: every non-default waveform must be audible AND
  //    actually change the render (renders are peak-normalized to 0.8, so the
  //    head SHAPE — not the peak — is the discriminator). ──
  // fm_wave enum: 0=Sine (fm-bass default) 1=Saw 2=Square 3=Triangle. Sweep the
  // three NON-default waveforms for fm-bass (avoid 0 = the native default).
  for (const [wv, label] of [[1, "Saw"], [2, "Square"], [3, "Triangle"]]) {
    await page.evaluate(([id, v]) => setInstrumentParam(id, "fm_wave", v), [INST, wv]);
    const snap = await page.evaluate((id) => window.__testCaptureSynthRender(id), INST);
    if (!snap) { fail(`fm_wave=${wv} (${label}): render snapshot null`); continue; }
    console.log(`[TEST] fm_wave=${wv} (${label}) peak=${snap.peak} rms=${snap.rms}`);
    if (!(snap.peak > 0)) {
      fail(`fm_wave=${wv} (${label}) produced SILENCE (peak=${snap.peak}). ` +
        `Switching the waveform off the native default must still render audio.`);
    }
    for (const x of snap.head) {
      if (!Number.isFinite(x)) { fail(`${label}: non-finite sample ${x} in head`); break; }
    }
    if (headDiff(snap, native) < 1e-6) {
      fail(`fm_wave=${wv} (${label}) rendered identically to Native — the wave knob ` +
        `did not reach the source==10 FM voice via render_modular_p ` +
        `(stale modular ABI block, or the push seam dropped the fm_wave→gen1_wave translation?).`);
    }
  }

  // ── Modular generator sweep: every osc_type must produce an audible buffer
  //    (this is the post-deprecation home of the FM + Noise generators). ──
  // osc_type enum: 0=Sine 1=Saw 2=Square 3=Triangle 4=FM 5=Noise White 6=Noise Pink.
  await page.evaluate((id) => resetInstrumentParams(id), MOD);
  const OSCS = [
    [0, "Sine"], [1, "Saw"], [2, "Square"], [3, "Triangle"],
    [4, "FM"], [5, "Noise White"], [6, "Noise Pink"],
  ];
  for (const [ot, label] of OSCS) {
    await page.evaluate(([id, v]) => setInstrumentParam(id, "osc_type", v), [MOD, ot]);
    const snap = await page.evaluate((id) => window.__testCaptureSynthRender(id), MOD);
    if (!snap) { fail(`osc_type=${ot} (${label}): render snapshot null`); continue; }
    console.log(`[TEST] osc_type=${ot} (${label}) peak=${snap.peak} rms=${snap.rms}`);
    if (!(snap.peak > 0)) {
      fail(`osc_type=${ot} (${label}) produced SILENCE (peak=${snap.peak}).`);
    }
    for (const x of snap.head) {
      if (!Number.isFinite(x)) { fail(`${label}: non-finite sample ${x} in head`); break; }
    }
  }

  // ── LIVE PLAYBACK: capture the REAL master output (post channel chain +
  //    limiter) for a triggered hit. This is the path the user actually hears —
  //    render being audible doesn't prove the graph output is. ──
  const captureHit = async (id) => {
    await page.evaluate((i) => { window.startOutputCapture(); window.playSound(i, 1.0); }, id);
    await page.waitForTimeout(700);
    const buf = await page.evaluate(() => { window.stopOutputCapture(); return Array.from(window.getOutputCapture()); });
    let peak = 0, rms = 0;
    for (const x of buf) { const a = Math.abs(x); if (a > peak) peak = a; rms += x * x; }
    rms = Math.sqrt(rms / Math.max(1, buf.length));
    return { peak, rms, n: buf.length };
  };

  await page.evaluate((id) => resetInstrumentParams(id), INST);
  const liveNative = await captureHit(INST);
  console.log(`[TEST] LIVE Native master peak=${liveNative.peak.toFixed(5)} rms=${liveNative.rms.toFixed(5)} n=${liveNative.n}`);
  if (!(liveNative.peak > 0)) fail(`LIVE Native ${INST}: master output is SILENT (capture peak=${liveNative.peak})`);

  await page.evaluate((id) => setInstrumentParam(id, "snare_wave", 1), INST); // Saw
  const liveSaw = await captureHit(INST);
  console.log(`[TEST] LIVE snare_wave=1 (Saw) master peak=${liveSaw.peak.toFixed(5)} rms=${liveSaw.rms.toFixed(5)} n=${liveSaw.n}`);
  if (!(liveSaw.peak > 0)) {
    fail(`LIVE re-waved (Saw) master output is SILENT (peak=${liveSaw.peak}). ` +
      `This reproduces "audio is gone after switching the waveform".`);
  }

  await page.evaluate((id) => { resetInstrumentParams(id); setInstrumentParam(id, "osc_type", 5); }, MOD);
  const liveNoise = await captureHit(MOD);
  console.log(`[TEST] LIVE modular osc_type=5 (Noise White) master peak=${liveNoise.peak.toFixed(5)} rms=${liveNoise.rms.toFixed(5)} n=${liveNoise.n}`);
  if (!(liveNoise.peak > 0)) fail(`LIVE modular noise master output is SILENT (peak=${liveNoise.peak})`);

  // ── Per-stage enable flags must reach the C voice through the modular ABI.
  //    This is the exact heap region (osc_enabled..drive_enabled, indices 27-31)
  //    whose stale-ABI misalignment caused the silence — exercise it directly. ──
  await page.evaluate((id) => { resetInstrumentParams(id); setInstrumentParam(id, "osc_type", 1); }, MOD); // Saw (deterministic)
  const sawAll = await page.evaluate((id) => window.__testCaptureSynthRender(id), MOD);
  // Envelope bypass → constant level (no ADSR): different render, still audible.
  await page.evaluate((id) => setInstrumentParam(id, "env_enabled", 0), MOD);
  const envOff = await page.evaluate((id) => window.__testCaptureSynthRender(id), MOD);
  console.log(`[TEST] env_enabled=0 peak=${envOff.peak.toFixed(5)} rms=${envOff.rms.toFixed(5)}`);
  if (!(envOff.peak > 0)) fail("env_enabled=0 silenced the voice — bypass must pass audio (enable flag not reaching C?)");
  if (headDiff(sawAll, envOff) < 1e-6) fail("env_enabled=0 produced an identical render — the enable flag did not reach the C voice");
  // Oscillator bypass → silence by design (the generator is the source). This
  // confirms the osc_enabled index lands on the right C field (a stale ABI
  // would silence the voice regardless of this flag).
  await page.evaluate((id) => { setInstrumentParam(id, "env_enabled", 1); setInstrumentParam(id, "osc_enabled", 0); }, MOD);
  const oscOff = await page.evaluate((id) => window.__testCaptureSynthRender(id), MOD);
  console.log(`[TEST] osc_enabled=0 peak=${oscOff.peak.toFixed(5)}`);
  if (oscOff.peak > 0) fail(`osc_enabled=0 should silence the generator (it is the source); got peak=${oscOff.peak}`);
  await page.evaluate((id) => resetInstrumentParams(id), MOD);

  // ── Back to defaults restores an audible bespoke voice, bit-identical to the
  //    original Native render (selector default is bit-stable). ──
  await page.evaluate((id) => resetInstrumentParams(id), INST);
  const restored = await page.evaluate((id) => window.__testCaptureSynthRender(id), INST);
  if (!restored || !(restored.peak > 0)) fail(`Native not restored after wave-knob round-trip (peak=${restored && restored.peak})`);
  if (restored && native && restored.length === native.length) {
    const diff = headDiff(native, restored);
    if (diff > 1e-6) fail(`Native render drifted after wave-knob round-trip (head diff=${diff})`);
  }

  if (isCoverageEnabled()) { await flushCoverage(page, "synth_generator_audible"); }
} finally {
  await browser.close();
  server.close();
}

if (failed) {
  console.error("[TEST] synth generator audible: FAILURES (see above)");
  process.exit(1);
}
console.log("[TEST] synth generator audible: all generators audible, enable flags reach C, Native bit-stable. PASS");
