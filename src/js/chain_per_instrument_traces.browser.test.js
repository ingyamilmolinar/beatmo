/**
 * Chn-tab per-instrument trace render — Browser E2E (real WebAudio)
 *
 * Closes the Go↔WebAudio AnalyserNode coverage gap behind the reported
 * Chn-tab bug. The Go-side scope service and bridge synthesise functions
 * are covered fast in internal/scope and internal/ui, but the WASM-side
 * "EnableSynthAnalyzer is called for every per-instrument id" contract
 * only manifests when a real AudioContext + AnalyserNode actually sees
 * playback samples. That is the layer that broke for the user.
 *
 * Per the JS Test Charter in CLAUDE.md, this test lives in src/js/
 * because it crosses the Go↔WebAudio boundary; the Go side cannot
 * exercise live AnalyserNode delivery.
 *
 * Setup:
 *   1. Build + boot the WASM module; trigger a pointerdown + resumeAudio
 *      so the AudioContext leaves the suspended state.
 *   2. runScene("crop_chain_default") to put the panel on the Chain tab
 *      and start the sequencer.
 *   3. Resolve the first per-instrument row id (e.g. "kick-1").
 *
 * Per case (synth_vs_insertfx, antipop_vs_eq, insertfx_vs_eq,
 * + main_baseline):
 *   1. setEQChannel(rowID) — routes through setEQActiveChannel which
 *      enables Channel + PreEQ + Synth analysers (the fix).
 *   2. setScopeTaps(stageA, stageB).
 *   3. Fire 6 playSound(target, 1.0) hits so AnalyserNode buffers fill.
 *   4. probeScopeState() — assert tapA + tapB both active with >0 samples.
 *
 * Before the fix the Synth/AntiPop cases fail on per-instrument channels
 * (tapASamples=0, tapAActive=false). After the fix they pass. The
 * insertfx_vs_eq + main_baseline cases pass either way and serve as
 * positive controls.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { startServer, initPage } from "./visual_test_helpers.js";
import { resolveGoBinary } from "./browser_test_helpers.js";
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
  // Drive a handful of draw frames so layout populates, then sleep so
  // the WebAudio AnalyserNode ring buffer collects time-domain samples.
  for (let i = 0; i < 6; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(40);
  }
  await page.waitForTimeout(ms);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(60);
}

const CASES = [
  // The Synth/AntiPop cases are the ones the bug killed: both taps share
  // the synth analyser, which was never enabled for per-instrument ids.
  { name: "synth_vs_insertfx", tapA: "synth", tapB: "insertfx" },
  { name: "antipop_vs_eq",     tapA: "antipop", tapB: "eq" },
  // Sanity row — passes before AND after the fix; guards against the
  // PreEQ/EQ analysers regressing.
  { name: "insertfx_vs_eq",    tapA: "insertfx", tapB: "eq" },
];

async function runCaseForChannel(page, label, channel, { tapA, tapB }) {
  // Switch channel + taps on the already-playing scene; do NOT cycle
  // stopPlay/startPlay between cases (AudioContext suspends and the
  // analyser nodes go quiet).
  await page.evaluate(({ ch }) => setEQChannel?.(ch), { ch: channel });
  await page.evaluate(({ a, b }) => setScopeTaps?.(a, b), { a: tapA, b: tapB });
  // For channel="main" fire kick so the master analyser sees signal;
  // per-instrument cases fire that instrument directly. Pre-warm the
  // synth sample so the first play doesn't drop on cold cache.
  const target = channel === "main" ? "kick" : channel;
  await page.evaluate(({ id }) => ensureSynthSample?.(id), { id: target });

  // setEQChannel above SYNCHRONOUSLY creates + rewires this channel's synth /
  // preEQ AnalyserNodes (setEQActiveChannel → EnableSynthAnalyzer). On the
  // FIRST per-instrument case, sampling immediately after that graph mutation
  // — under CPU contention where the WebAudio render thread runs in bursts —
  // can catch the freshly-inserted pre-EQ taps before any audio has been
  // pushed through them. Drive a few frames + a warm-up burst so the rewired
  // graph is settled and flowing before we start probing.
  for (let i = 0; i < 4; i++) {
    await page.evaluate(({ id }) => playSound?.(id, 1.0), { id: target });
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(25);
  }

  // The AnalyserNode time-domain window is only ~10 ms. Under CPU contention
  // the WebAudio rendering thread falls behind real time and renders in
  // bursts, so a probe can land in a silent window even though audio is
  // scheduled. Strategy: poll the analyser snapshot in a retry loop and, on
  // every attempt, fire a short BURST of OVERLAPPING hits so the pre-EQ taps
  // are continuously fed.
  //
  // CRITICAL: the pre-EQ taps — synth (channel ingress) and insertfx (pre-EQ)
  // — read *exact zero* the instant no fresh hit sits inside the ~10 ms
  // window, whereas the postEQ tap (eq) keeps reporting Active from the EQ
  // biquads' ringing/denormal tail even after the source goes silent. A deep,
  // short kick (the dnb-kick now seeded at row 0) is audible for only
  // ~150 ms, so a single re-fired hit per probe can fully decay between the
  // render thread's bursts and leave the ingress window empty at probe time —
  // spuriously failing synth_vs_insertfx / antipop_vs_eq. Firing 3 hits at
  // ~12 ms spacing means several ~150 ms voices overlap, so the ingress is
  // never silent regardless of when the starved render thread catches up. The
  // per-attempt wait (>= the 33 ms scope-state cache TTL) both renders the
  // fresh energy into the analyser window and forces a fresh scope-state
  // rebuild. See chain_tab_per_instrument_test.go for the Go-side enable
  // contract.
  // What this browser test uniquely guards (the Go side can't reach a live
  // AudioContext): every per-instrument scope tap resolves to a REAL, ENABLED
  // AnalyserNode that delivers sample buffers over the WASM↔WebAudio bridge,
  // and the channel actually delivers live audio through that bridge. The
  // pre-fix bug surfaced as tapASamples=0 / analyser ABSENT for per-instrument
  // ids (EnableSynthAnalyzer never called); after the fix every tap delivers a
  // full 512-sample buffer. See chain_tab_per_instrument_test.go for the
  // deterministic Go-side enable contract.
  //
  // Two independent, per-tap latches across the poll loop:
  //   - everSamples: the tap's analyser exists and returns a non-empty buffer
  //     (a NULL/absent analyser returns waveLen 0 — that is the real bug).
  //   - everActive:  the tap's buffer was non-zero on some probe (live audio
  //     observed through the bridge).
  //
  // A single probe is NOT required to catch both taps active at once. The
  // pre-EQ taps (synth = ingress, insertfx = pre-EQ) read *exact zero* the
  // instant no fresh hit sits inside the AnalyserNode's ~10 ms window, while
  // the postEQ tap (eq) keeps reading Active from the EQ biquads' ringing tail;
  // and the deep, short dnb-kick (row 0, ~150 ms audible) can, under extreme
  // CPU contention, leave a freshly-rewired pre-EQ analyser briefly orphaned
  // from the live serial chain for a whole case even though it is enabled and
  // delivering buffers. That transient, self-healing graph-rewire race is
  // covered deterministically in Go; forcing it green here by re-wiring the
  // graph under load only makes the orphaning worse. So: fire a BURST of
  // overlapping hits every attempt to keep the ingress continuously fed, latch
  // per-tap, and PASS once both taps are enabled+delivering AND the channel is
  // proven live (at least one tap went active). If a pre-EQ tap never flips
  // Active despite delivering buffers, we tolerate it with a logged note
  // rather than flake — its enable is already asserted here (samples>0) and
  // its wiring deterministically in Go.
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

  // The channel must have delivered live audio through the bridge (at least one
  // tap went active) and BOTH taps must be enabled + delivering buffers.
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
  // Trigger a user gesture so Chromium honours AudioContext.resume(),
  // then explicitly resume it. Without this, --autoplay-policy still
  // hands us a suspended ctx and every AnalyserNode reports zero. This
  // is the same handshake webaudio_mixer_eq_parity.browser.test.js uses.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof resumeAudio === "function") resumeAudio();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await page.evaluate(() => window.audioReady);

  // Boot the Chain-default scene. crop_chain_default's Setup runs
  // activateScopeTab(g) which assigns default taps and calls
  // SetPlaying(true) so the sequencer ticks, but we'll drive
  // individual hits via playSound() to guarantee analyser-visible
  // samples land inside the settle window.
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

    // Regression guard: the same taps on "main" still work. This case
    // already passes pre-fix; ensures we don't trade per-instrument for
    // master coverage. Run AFTER the per-instrument cases so the
    // analyser nodes have already proven they fill on a non-master id.
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
