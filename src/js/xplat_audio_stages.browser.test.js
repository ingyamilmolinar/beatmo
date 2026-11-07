// Every stage between a voice and the speaker computes the same samples on desktop and in Chromium, Firefox and WebKit:
// desktop's offline mixer (cmd/export_audio --stages, rendered fresh) against audio.js's real graph on an
// OfflineAudioContext, tap by tap. The bound is the float-rounding seam of one C compiled two ways, far below audible.

import fs from "fs";
import os from "os";
import path from "path";
import { fileURLToPath } from "url";
import childProcess from "child_process";
import { setupFullWasm } from "./real_input_test_helpers.js";
import { resolveGoBinary, waitForAudioReady } from "./browser_test_helpers.js";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const ENGINES = (process.env.XPLAT_ENGINES || "chromium-local,firefox-local,webkit-local").split(",").filter(Boolean);
const MAX_DIFF = Number(process.env.XPLAT_STAGE_MAX_DIFF || 1e-5);
const QUANTUM_FRACTION = 1e-3;

function desktopStages() {
  const env = { ...process.env, CGO_ENABLED: "1" };
  delete env.GOOS; // export_audio is a native host tool (cgo)
  delete env.GOARCH;
  const r = childProcess.spawnSync(resolveGoBinary(), ["run", "./cmd/export_audio", "--stages"], {
    cwd: path.resolve(repoRoot, "src/go"),
    env,
    encoding: "utf8",
    maxBuffer: 1 << 30,
  });
  if (r.status !== 0) {
    throw new Error(`cmd/export_audio --stages failed (status ${r.status}): ${(r.stderr || String(r.error || "")).slice(-2000)}`);
  }
  return JSON.parse(r.stdout);
}

// Through a native Go test that imports each project into a real Game; it needs a display, so xvfb-run where present.
function desktopCircuits() {
  const out = path.join(fs.mkdtempSync(path.join(os.tmpdir(), "xplat-circuits-")), "circuits.json");
  const env = { ...process.env, CGO_ENABLED: "1", XPLAT_CIRCUIT_OUT: out };
  delete env.GOOS;
  delete env.GOARCH;
  const args = ["test", "./internal/ui/", "-run", "^TestWriteXplatCircuitReference$", "-count=1"];
  const xvfb = childProcess.spawnSync("sh", ["-c", "command -v xvfb-run"], { encoding: "utf8" }).status === 0;
  const r = childProcess.spawnSync(xvfb ? "xvfb-run" : resolveGoBinary(), xvfb ? ["-a", resolveGoBinary(), ...args] : args, {
    cwd: path.resolve(repoRoot, "src/go"), env, encoding: "utf8", maxBuffer: 1 << 28,
  });
  if (r.status !== 0 || !fs.existsSync(out)) {
    throw new Error(`TestWriteXplatCircuitReference failed (status ${r.status}): ${(r.stdout || "").slice(-1500)}${(r.stderr || "").slice(-1500)}`);
  }
  return JSON.parse(fs.readFileSync(out, "utf8"));
}

async function runCircuits(page, circuits, failures, summary) {
  for (const c of circuits.circuits) {
    const res = await page.evaluate(async ({ c, sr, maxDiffBound }) => {
      const err = window.importJSON(c.project);
      if (err) return { error: "import: " + err };
      const bin = atob(c.out);
      const u8 = new Uint8Array(bin.length);
      for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i);
      const want = new Float32Array(u8.buffer);
      let out;
      try {
        out = await window.__offlineRenderForTest({ frames: c.frames, sampleRate: sr, channels: "live", notes: c.notes, stops: [], taps: [] });
      } catch (e) {
        return { error: String((e && e.stack) || e) };
      }
      const got = out.taps.out;
      let maxDiff = 0, at = -1, exact = 0, peak = 0, over = 0;
      for (let i = 0; i < Math.min(want.length, got.length); i++) {
        const d = Math.abs(got[i] - want[i]);
        if (d === 0) exact++;
        if (d > maxDiffBound) over++;
        if (d > maxDiff) { maxDiff = d; at = i; }
        peak = Math.max(peak, Math.abs(want[i]));
      }
      return { started: out.started, n: want.length, gotLen: got.length, maxDiff, at, exact, peak, over, wantAt: want[at], gotAt: got[at] };
    }, { c, sr: circuits.sampleRate, maxDiffBound: MAX_DIFF });
    const name = "circuit/" + c.name;
    if (res.error) { failures.push(`${name}: ${res.error}`); continue; }
    if (res.started !== c.notes.length) failures.push(`${name}: ${res.started} of ${c.notes.length} notes started`);
    if (res.gotLen !== res.n) failures.push(`${name}: ${res.gotLen} frames, desktop ${res.n}`);
    if (!(res.peak > 0)) failures.push(`${name}: desktop rendered silence — the comparison would be vacuous`);
    if (res.maxDiff > MAX_DIFF) {
      failures.push(`${name}: sample ${res.at} = ${res.gotAt.toExponential(4)}, desktop ${res.wantAt.toExponential(4)} (|Δ| ${res.maxDiff.toExponential(2)}; ${res.over} samples over ${MAX_DIFF}, ${res.exact}/${res.n} exact, peak ${res.peak.toFixed(4)})`);
    }
    summary.push({ scene: name, tap: "out", maxDiff: res.maxDiff, exact: res.exact, n: res.n, peak: res.peak });
  }
}

async function runEngine(engine, ref, circuits) {
  process.env.TEST_PLATFORM = engine;
  const { page, cleanup } = await setupFullWasm({ logLevel: "ERROR" });
  const failures = [];
  const summary = [];
  try {
    await page.waitForFunction(
      () => typeof window.__offlineRenderForTest === "function" && typeof window.setInstrumentBlock === "function",
      null,
      { timeout: 60000 }
    );
    await waitForAudioReady(page);
    // Exactly as desktop rendered them: the startup circuit may hold its own versions.
    await page.evaluate((insts) => {
      for (const [id, d] of Object.entries(insts)) {
        window.setInstrumentBlock(id, Float64Array.from(d.block), d.seconds, d.amp, d.melodic, d.gateTail, d.fx);
      }
    }, ref.instruments);

    for (const scene of ref.scenes) {
      const res = await page.evaluate(async ({ scene, sr, maxDiffBound }) => {
        const decode = (b64) => {
          const bin = atob(b64);
          const u8 = new Uint8Array(bin.length);
          for (let i = 0; i < bin.length; i++) u8[i] = bin.charCodeAt(i);
          return new Float32Array(u8.buffer);
        };
        const taps = Object.keys(scene.taps).filter((k) => k !== "out").map((k) => {
          const [channel, stage] = k.split("/");
          return { name: k, channel, stage };
        });
        const channels = {};
        for (const [id, c] of Object.entries(scene.channels || {})) channels[id] = { chain: c.chain || [], eq: c.eq };
        let out;
        const edits = Object.entries(scene.sampleEdits || {});
        try {
          for (const [id, e] of edits) window.updateSampleEdit(id, JSON.stringify(e));
          out = await window.__offlineRenderForTest({
            frames: scene.frames, sampleRate: sr, channels,
            masterVolume: scene.masterVolume, masterEQ: scene.masterEQ,
            notes: scene.notes || [], stops: scene.stops || [], taps,
          });
        } catch (e) {
          return { error: String((e && e.stack) || e) };
        } finally {
          // The identity edit clears the descriptor (desktop: ClearSampleEdit).
          for (const [id] of edits) window.updateSampleEdit(id, JSON.stringify({ StartFrac: 0, EndFrac: 1 }));
        }
        const stats = {};
        for (const [name, b64] of Object.entries(scene.taps)) {
          const want = decode(b64);
          const got = out.taps[name];
          if (!got) { stats[name] = { missing: true }; continue; }
          let maxDiff = 0, at = -1, exact = 0, peak = 0, gotPeak = 0, over = 0;
          const n = Math.min(want.length, got.length);
          for (let i = 0; i < n; i++) {
            const d = Math.abs(got[i] - want[i]);
            if (d === 0) exact++;
            if (d > maxDiffBound) over++;
            if (d > maxDiff) { maxDiff = d; at = i; }
            peak = Math.max(peak, Math.abs(want[i]));
            gotPeak = Math.max(gotPeak, Math.abs(got[i]));
          }
          stats[name] = { n, wantLen: want.length, gotLen: got.length, maxDiff, at, exact, peak, gotPeak, over,
            wantAt: at >= 0 ? want[at] : 0, gotAt: at >= 0 ? got[at] : 0 };
        }
        return { sr: out.sampleRate, started: out.started, stats };
      }, { scene, sr: ref.sampleRate, maxDiffBound: MAX_DIFF });

      if (res.error) { failures.push(`${scene.name}: ${res.error}`); continue; }
      if (res.sr !== ref.sampleRate) failures.push(`${scene.name}: rendered at ${res.sr} Hz, desktop at ${ref.sampleRate}`);
      if (res.started !== scene.notes.length) failures.push(`${scene.name}: ${res.started} of ${scene.notes.length} notes started (a note deferred or was dropped)`);
      for (const [name, s] of Object.entries(res.stats)) {
        if (s.missing) { failures.push(`${scene.name} ${name}: no browser tap`); continue; }
        if (s.wantLen !== s.gotLen) failures.push(`${scene.name} ${name}: ${s.gotLen} frames, desktop ${s.wantLen}`);
        // An upstream quantizer can put an input one float32 ulp onto the neighbouring step: exactly one step, on at most
        // QUANTUM_FRACTION of samples.
        const quantum = (scene.quantum && scene.quantum[name]) || 0;
        const withinQuantum = quantum > 0 && s.maxDiff <= MAX_DIFF + quantum * (1 + 1e-6) && s.over <= QUANTUM_FRACTION * s.n;
        if (s.maxDiff > MAX_DIFF && !withinQuantum) {
          failures.push(`${scene.name} ${name}: sample ${s.at} = ${s.gotAt.toExponential(4)}, desktop ${s.wantAt.toExponential(4)} (|Δ| ${s.maxDiff.toExponential(2)} > ${MAX_DIFF}; ${s.exact}/${s.n} exact, peak ${s.peak.toFixed(4)})`);
        }
        summary.push({ scene: scene.name, tap: name, maxDiff: s.maxDiff, exact: s.exact, n: s.n, peak: s.peak });
      }
      const silent = Object.values(res.stats).every((s) => !(s.peak > 0));
      if (silent && scene.name !== "eq/all-muted") failures.push(`${scene.name}: desktop rendered silence — the comparison would be vacuous`);
    }
    if (circuits) await runCircuits(page, circuits, failures, summary);
  } finally {
    await cleanup();
  }
  const worst = summary.reduce((w, s) => (s.maxDiff > w.maxDiff ? s : w), { maxDiff: 0 });
  const exact = summary.filter((s) => s.exact === s.n).length;
  console.log(`[${engine}] ${summary.length} taps over ${ref.scenes.length} cases${circuits ? ` + ${circuits.circuits.length} circuits` : ""}: ${exact} bit-identical, worst |Δ| ${worst.maxDiff.toExponential(2)} (${worst.scene} ${worst.tap})`);
  if (process.env.XPLAT_STAGE_VERBOSE === "1") {
    for (const s of summary) console.log(`  ${s.scene.padEnd(22)} ${s.tap.padEnd(20)} |Δ| ${s.maxDiff.toExponential(2)}  exact ${s.exact}/${s.n}  peak ${s.peak.toFixed(4)}`);
  }
  return failures.map((f) => `[${engine}] ${f}`);
}

async function main() {
  const ref = desktopStages();
  console.log(`[test] desktop: ${ref.scenes.length} stage cases at ${ref.sampleRate} Hz`);
  const circuits = process.env.XPLAT_CIRCUITS === "0" ? null : desktopCircuits();
  if (circuits) console.log(`[test] desktop: ${circuits.circuits.length} circuits (${circuits.circuits.map((c) => c.name).join(", ")})`);
  const failures = [];
  for (const engine of ENGINES) failures.push(...(await runEngine(engine, ref, circuits)));
  if (failures.length) {
    console.error("FAIL xplat_audio_stages:\n  " + failures.join("\n  "));
    process.exit(1);
  }
  console.log("PASS xplat_audio_stages");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
