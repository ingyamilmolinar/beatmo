/**
 * Audio-tab source of truth: browser vs the native golden (WASM)
 *
 * The programmatic replacement for eyeballing a screenshot of the audio
 * panel. Three checks, ordered so the first failure already names the layer
 * that broke:
 *
 *  1. CROSS-PLATFORM, BYTE FOR BYTE. `audioTabTapDigest()` runs the SAME Go
 *     derivations (internal/ui/audio_tab_tap.go) over the SAME canonical
 *     fixture that produced testdata/audio_tab_tap_golden.json natively.
 *     Identical code + identical input must give identical bytes, so every
 *     section digest must match exactly. This is what makes the comparison
 *     apples-to-apples: it does NOT compare a live browser analyser against
 *     a live native one, which would be comparing a Blackman-windowed FFT
 *     against a Hann-windowed one and calling the difference a bug.
 *
 *  2. DETERMINISM IN THE BROWSER. Called twice, same bytes. Catches state
 *     leaking across calls under wasm specifically.
 *
 *  3. LIVE SHAPE. `audioTabTapLiveShape(id)` measures what the real
 *     AnalyserNode bridge actually delivers for a real channel, and asserts
 *     every display band contains at least one FFT bin. THIS is the check
 *     that would have caught the 2026-09-19 blank Spectrum: the bridge
 *     delivered 64 bins spanning 0..Nyquist (375 Hz apart at 48 kHz) while
 *     the narrowest band is 24 Hz wide, so the four lowest bands — a kick's
 *     entire spectrum — held nothing and reported the reference floor. The
 *     audio was perfect. Desktop was perfect. Only the shape was wrong, and
 *     nothing compared shapes.
 *
 * Charter fit (CLAUDE.md § JS Test Charter): cross-platform parity (§5) plus
 * the WASM<->WebAudio analyser boundary (§1/§4) — neither reachable from Go.
 *
 * Regenerating: if a change to the derivations is intended, run
 *   AUDIO_TAP_GOLDEN_UPDATE=1 go test -tags test -run TestAudioTabTapMatchesGolden ./internal/ui/
 * and re-run this test. Both halves read the same file.
 */

import { readFileSync } from "node:fs";
import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const GOLDEN = new URL("../go/internal/ui/testdata/audio_tab_tap_golden.json", import.meta.url).pathname;

let cleanup;
let page;

/** Names the layer a section belongs to, so failures read top-down. */
const layerOf = (name) => (name.includes(".") ? name.slice(0, name.indexOf(".")) : "other");

try {
  const golden = JSON.parse(readFileSync(GOLDEN, "utf8"));
  if (!Array.isArray(golden.sections) || golden.sections.length === 0) {
    throw new Error(`golden at ${GOLDEN} has no sections`);
  }
  console.log(`xplat_audio_tab_tap: golden has ${golden.sections.length} sections @ ${golden.sample_rate} Hz`);

  ({ page, cleanup } = await setupFullWasm());

  // --- 1 + 2: the shared derivation, in the browser -----------------------
  const [got, again] = await page.evaluate(() => [
    window.audioTabTapDigest?.(),
    window.audioTabTapDigest?.(),
  ]);
  if (!got) throw new Error("audioTabTapDigest() returned nothing — the export is missing");

  if (JSON.stringify(got) !== JSON.stringify(again)) {
    throw new Error("audioTabTapDigest() is not deterministic in the browser: two back-to-back " +
      "calls over the same fixture produced different digests");
  }

  if (got.sample_rate !== golden.sample_rate) {
    throw new Error(`sample_rate: browser ${got.sample_rate}, golden ${golden.sample_rate}`);
  }

  const gotByName = new Map(got.sections.map((s) => [s.name, s]));
  const failures = [];
  for (const want of golden.sections) {
    const have = gotByName.get(want.name);
    if (!have) {
      failures.push(`${want.name}: MISSING in the browser (golden has ${want.count} elements)`);
      continue;
    }
    if (have.count !== want.count) {
      // Called out separately: a length change means the data's SHAPE moved,
      // which silently redraws everything downstream of it.
      failures.push(`${want.name}: length ${have.count}, want ${want.count} — the data's SHAPE differs between platforms`);
      continue;
    }
    // digest_q, NOT digest: raw IEEE-754 bits are not reproducible between
    // amd64 and wasm downstream of a transcendental. Measured on this very
    // fixture, math.Exp(-1000/90) is 0.034329453106454272 natively and
    // 0.03432945310645428 under GOOS=js — one ULP, because amd64 uses FMA
    // and wasm does not, reaching ~8e-15 dB. digest_q is the same byte-for-
    // byte comparison over values snapped to a 1e-6 grid: nine orders above
    // that noise, four orders below anything a pixel could show. Sections
    // made only of integers or of +-*/ carry an IDENTICAL digest and
    // digest_q, so for those this is still an exact-bits comparison.
    if (have.digest_q !== want.digest_q) {
      failures.push(`${want.name}: ${have.count} elements, values differ (${have.digest_q.slice(0, 12)} != ${want.digest_q.slice(0, 12)})`);
    }
  }
  for (const have of got.sections) {
    if (!golden.sections.some((w) => w.name === have.name)) {
      failures.push(`${have.name}: UNEXPECTED section in the browser (${have.count} elements)`);
    }
  }

  if (failures.length > 0) {
    const layers = [...new Set(failures.map((f) => layerOf(f)))];
    throw new Error(
      `the browser does not reproduce the native golden — ${failures.length} section(s) differ ` +
      `in layer(s): ${layers.join(", ")}\n` +
      (layers.includes("shape")
        ? "  ** a SHAPE section differs: the two platforms are handing the renderer " +
          "differently-sized data. Everything derived from it follows. Fix that first.\n"
        : "") +
      failures.map((f) => `  - ${f}`).join("\n"),
    );
  }
  console.log(`  OK  cross-platform: all ${golden.sections.length} sections byte-identical to the native golden`);

  // --- 3: the LIVE bridge's shape -----------------------------------------
  const rowIDs = await page.evaluate(async () => {
    let ids = [], stable = 0;
    for (let i = 0; i < 80 && stable < 4; i++) {
      await new Promise((r) => setTimeout(r, 250));
      const n = window.drumRowCount?.() || 0;
      const now = [];
      for (let j = 0; j < n; j++) { const id = window.rowInstrument?.(j); if (id) now.push(id); }
      stable = now.length > 0 && now.join(",") === ids.join(",") ? stable + 1 : 0;
      ids = now;
    }
    return ids;
  });
  if (rowIDs.length === 0) throw new Error("no drum rows after waiting for the circuit");

  await page.evaluate(() => { setActiveEQTab?.("spectrum"); startPlay?.(); });
  await new Promise((r) => setTimeout(r, 1500));

  // Master plus one instrument channel. The defect emptied BOTH platforms'
  // bass bands but only ever showed on an instrument channel, because
  // master is broadband and its mid/high bands kept the plot looking alive.
  const probes = ["main", rowIDs[0]];
  const shapeFailures = [];
  for (const id of probes) {
    await page.evaluate((c) => setEQChannel?.(c), id);
    await new Promise((r) => setTimeout(r, 600));
    const sh = await page.evaluate((c) => window.audioTabTapLiveShape?.(c), id);
    if (!sh) { shapeFailures.push(`${id}: audioTabTapLiveShape returned nothing`); continue; }

    const binHz = sh.binHzMilli / 1000;
    console.log(`  ${id}: ${sh.spectrumBins} bins @ ${binHz.toFixed(1)} Hz, wave=${sh.waveformLen}, ` +
      `band bins=[${sh.bandBinCounts.join(",")}]`);

    if (sh.spectrumBins !== sh.declaredBins) {
      shapeFailures.push(`${id}: the bridge delivered ${sh.spectrumBins} spectrum bins but Go declares ` +
        `${sh.declaredBins} (audio.BridgeSpectrumBins) — a JS default is overriding the Go contract`);
    }
    if (sh.waveformLen !== sh.declaredWave) {
      shapeFailures.push(`${id}: the bridge delivered ${sh.waveformLen} waveform samples but Go declares ` +
        `${sh.declaredWave} (audio.BridgeWaveformSamples)`);
    }
    sh.bandMeasured.forEach((ok, b) => {
      if (!ok) {
        shapeFailures.push(`${id}: display band ${b} contains NO FFT bin (${binHz.toFixed(1)} Hz per bin) — ` +
          `it reports the reference floor whatever the signal does, so anything living there renders blank`);
      }
    });
  }
  if (shapeFailures.length > 0) {
    throw new Error("the LIVE analyser bridge is delivering the wrong shape:\n" +
      shapeFailures.map((f) => `  - ${f}`).join("\n"));
  }
  console.log("  OK  live bridge: declared shape honoured and every display band has bins");

  console.log("xplat_audio_tab_tap: PASS");
} catch (error) {
  console.error("xplat_audio_tab_tap: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) {
    await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "xplat_audio_tab_tap");
  }
  if (cleanup) await cleanup();
}
