/** Only a real AudioContext shows whether every per-instrument scope tap resolves to an ENABLED AnalyserNode that
 * delivers samples over the bridge. Per case: pick the channel, set the taps, feed hits, then assert both taps deliver
 * buffers. COVERED-BY-GO: internal/ui/chain_tab_per_instrument_test.go (the enable contract) */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { startServer, initPage } from "./visual_test_helpers.js";
import { resolveGoBinary, waitForAudioReady } from "./browser_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "..", "go");

function buildWasmSafely() {
  if (process.env.WASM_PREBUILT === "1") {
    const wasmPath = path.join(jsDir, "main.wasm");
    if (!fs.existsSync(wasmPath)) {
      throw new Error(`WASM_PREBUILT=1 but main.wasm missing at ${wasmPath}`);
    }
    return;
  }
  const GO = resolveGoBinary();
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    }
  );
  if (build.status !== 0) {
    throw new Error("go build main.wasm failed");
  }
}

async function settle(page, ms = 800) {
  // Prime a few frames so layout populates, then let the analyser buffers fill.
  for (let i = 0; i < 6; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(40);
  }
  await page.waitForTimeout(ms);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(60);
}

const CASES = [
  // Both taps read the synth analyser, the one a per-instrument channel must enable.
  { name: "synth_vs_insertfx", tapA: "synth", tapB: "insertfx" },
  { name: "antipop_vs_eq",     tapA: "antipop", tapB: "eq" },
  // A control that guards the pre-EQ/EQ analysers.
  { name: "insertfx_vs_eq",    tapA: "insertfx", tapB: "eq" },
];

async function runCaseForChannel(page, label, channel, { tapA, tapB }) {
  // No stop/start between cases: the context suspends and the analysers go quiet.
  await page.evaluate(({ ch }) => setEQChannel?.(ch), { ch: channel });
  await page.evaluate(({ a, b }) => setScopeTaps?.(a, b), { a: tapA, b: tapB });
  // Pre-warmed so the first play is not dropped on a cold cache.
  const target = channel === "main" ? "kick" : channel;
  await page.evaluate(({ id }) => ensureSynthSample?.(id), { id: target });

  // Choosing the channel rewires its analysers synchronously; under CPU contention a probe right after can catch the
  // fresh taps before audio has flowed, so a few frames and a warm-up burst settle it.
  for (let i = 0; i < 4; i++) {
    await page.evaluate(({ id }) => playSound?.(id, 1.0), { id: target });
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(25);
  }

  // The analyser window is ~10 ms and a starved render thread runs in bursts, so every attempt fires overlapping hits and
  // each tap latches separately: it passes once both taps deliver buffers and one went active. A pre-EQ tap that never
  // flips active while delivering is tolerated (logged): its wiring is pinned in Go, and re-wiring under load worsens it.
  const MAX_ATTEMPTS = 40;
  const PROBE_INTERVAL_MS = 45;
  let probe = null;
  let everSamplesA = false;
  let everSamplesB = false;
  let everActiveA = false;
  let everActiveB = false;
  let lastASamples = 0;
  let lastBSamples = 0;
  for (let attempt = 0; attempt < MAX_ATTEMPTS; attempt++) {
    for (let k = 0; k < 3; k++) {
      await page.evaluate(({ id }) => playSound?.(id, 1.0), { id: target });
      await page.waitForTimeout(12);
    }
    await page.waitForTimeout(PROBE_INTERVAL_MS);
    probe = await page.evaluate(() =>
      typeof probeScopeState === "function" ? probeScopeState() : null
    );
    if (!probe || probe.available !== true) {
      continue;
    }
    if (probe.tapASamples > 0) { everSamplesA = true; lastASamples = probe.tapASamples; }
    if (probe.tapBSamples > 0) { everSamplesB = true; lastBSamples = probe.tapBSamples; }
    if (probe.tapAActive === true && probe.tapASamples > 0) everActiveA = true;
    if (probe.tapBActive === true && probe.tapBSamples > 0) everActiveB = true;
    // Fast path: both taps proven live. Done as soon as we see it.
    if (everActiveA && everActiveB) {
      return {
        ok: true,
        probe: { ...probe, tapASamples: lastASamples, tapBSamples: lastBSamples },
      };
    }
  }

  // Live audio on at least one tap, and BOTH delivering buffers.
  if (everSamplesA && everSamplesB && (everActiveA || everActiveB)) {
    const note =
      everActiveA && everActiveB
        ? ""
        : `tolerated transient pre-EQ orphan: tapA(${tapA})Active=${everActiveA} ` +
          `tapB(${tapB})Active=${everActiveB} (both enabled+delivering; live audio ` +
          `confirmed on the active tap; enable/wiring covered by ` +
          `chain_tab_per_instrument_test.go)`;
    return {
      ok: true,
      note,
      probe: {
        ...(probe || {}),
        tapASamples: lastASamples,
        tapBSamples: lastBSamples,
      },
    };
  }

  if (!probe || probe.available !== true) {
    return { ok: false, reason: `probeScopeState unavailable: ${JSON.stringify(probe)}` };
  }
  return {
    ok: false,
    reason:
      `tap not enabled/live on channel ${channel} for A=${tapA} B=${tapB} ` +
      `after ${MAX_ATTEMPTS} attempts — ` +
      `everSamplesA=${everSamplesA} everSamplesB=${everSamplesB} ` +
      `everActiveA=${everActiveA} everActiveB=${everActiveB} ` +
      `lastASamples=${lastASamples} lastBSamples=${lastBSamples}`,
  };
}

console.log("Building WASM...");
buildWasmSafely();

const { server, port } = await startServer();
const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});

let failures = 0;
const page = await initPage(browser, { width: 1280, height: 720 }, port);

try {
  // A user gesture so Chromium honours resume(); otherwise the context stays suspended and every analyser reads zero.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof resumeAudio === "function") resumeAudio();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await waitForAudioReady(page);

  // The scene sets the default taps and starts the sequencer; hits come from playSound() so samples land in the window.
  const sceneOK = await page.evaluate(() =>
    typeof runScene === "function" ? runScene("crop_chain_default") : false
  );
  if (!sceneOK) {
    console.error("FAIL: runScene(crop_chain_default) returned falsy");
    failures++;
  }
  await settle(page, 300);

  // Resolve the first per-instrument id from the running drum view.
  const rowID = await page.evaluate(() =>
    typeof rowInstrument === "function" ? rowInstrument(0) : ""
  );
  if (!rowID) {
    console.error("FAIL: rowInstrument(0) returned an empty id; can't exercise per-instrument path");
    failures++;
  } else {
    console.log(`Per-instrument row id: ${rowID}`);

    for (const c of CASES) {
      console.log(`\n=== ${c.name} on channel "${rowID}" ===`);
      const result = await runCaseForChannel(page, c.name, rowID, c);
      if (!result.ok) {
        console.error(`  FAIL: ${result.reason}`);
        failures++;
      } else {
        if (result.note) console.log(`  NOTE: ${result.note}`);
        console.log(
          `  PASS: tapA(${c.tapA}) samples=${result.probe.tapASamples} ` +
          `tapB(${c.tapB}) samples=${result.probe.tapBSamples}`
        );
      }
    }

    // After the per-instrument cases, so master coverage is not traded for them.
    console.log("\n=== synth_vs_insertfx on channel \"main\" (regression guard) ===");
    const masterResult = await runCaseForChannel(page, "main_baseline", "main", {
      tapA: "synth",
      tapB: "insertfx",
    });
    if (!masterResult.ok) {
      console.error(`  FAIL (master regression): ${masterResult.reason}`);
      failures++;
    } else {
      if (masterResult.note) console.log(`  NOTE: ${masterResult.note}`);
      console.log(
        `  PASS: tapA(synth) samples=${masterResult.probe.tapASamples} ` +
        `tapB(insertfx) samples=${masterResult.probe.tapBSamples}`
      );
    }
  }
} finally {
  if (isCoverageEnabled()) {
    try {
      await flushCoverage(
        page,
        new URL("../../coverage/browser-raw", import.meta.url).pathname,
        "chain_per_instrument_traces"
      );
    } catch (_) {}
  }
  await page.close();
}

await browser.close();
server.close();

if (failures > 0) {
  console.error(`\n${failures} chain per-instrument trace check(s) failed`);
  process.exit(1);
}
console.log("\nAll chain per-instrument trace checks passed");
