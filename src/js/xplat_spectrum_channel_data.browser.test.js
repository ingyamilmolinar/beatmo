/** A per-instrument channel must render Spectrum BARS in the browser: a display band holding no FFT bin reads as
 * silence, so the bridge's bin width must stay under the narrowest band (24 Hz). Desktop's own FFT hides any gap there,
 * so only browser pixels catch it. COVERED-BY-GO: nothing (desktop's path is a different implementation). */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { captureScreenshot, extractRegion } from "./visual_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

// A live channel reads ~0.1 on a hit and a dead branch ~1e-36, so this cannot pass by accident; -80 dBFS is the display
// floor, where no bar draws.
const MIN_RMS = 1e-3;

let cleanup;
let page;

try {
  console.log("xplat_spectrum_channel_data: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => {
    // Otherwise nothing draws the plot and the pixel check means nothing.
    setActiveEQTab?.("spectrum");
  });

  // After the demo replaces the boot circuit: a row set read earlier names instruments about to vanish, whose channels then
  // read denormal noise forever.
  const rowIDs = await page.evaluate(async () => {
    let ids = [];
    let stable = 0;
    for (let i = 0; i < 80 && stable < 4; i++) {
      await new Promise((r) => setTimeout(r, 250));
      const n = window.drumRowCount?.() || 0;
      const now = [];
      for (let j = 0; j < n; j++) {
        const id = window.rowInstrument?.(j);
        if (id) now.push(id);
      }
      stable = now.length > 0 && now.join(",") === ids.join(",") ? stable + 1 : 0;
      ids = now;
    }
    return ids;
  });
  if (rowIDs.length === 0) {
    throw new Error("no drum rows after waiting for the circuit — drumRowCount()/rowInstrument(i) returned nothing");
  }

  await page.evaluate(() => startPlay?.());
  await new Promise((r) => setTimeout(r, 2500)); // let the pattern run

  // The app's own channel-selection chokepoint, as the dropdown does, not enableChannelAnalyzer (that tests the fixture).
  // Row ids come from real exports: a guarded call to a Go test helper once probed an instrument not in the circuit.
  const result = await page.evaluate(async (ids) => {
    const out = { ids, channels: {} };
    for (const id of ids) {
      setEQChannel?.(id);
      // Silent between hits, so the max across a couple of bars: the claim is data WHEN IT SOUNDS.
      let rms = 0;
      let peak = 0;
      for (let i = 0; i < 24; i++) {
        await new Promise((r) => setTimeout(r, 120));
        const s = channelAnalyzerSnapshot?.(id);
        if (!s) continue;
        rms = Math.max(rms, Number(s.rms) || 0);
        peak = Math.max(peak, Number(s.peak) || 0);
      }
      out.channels[id] = { rms, peak };
    }
    return out;
  }, rowIDs);

  console.log("  rows:", JSON.stringify(result.ids));

  // Not an assertion: a sparse row can go a whole window without firing and look unwired. What is assertable is that a
  // channel demonstrably carrying audio renders its bands, so the loudest is probed.
  const QUIET = [];
  const LIVE = [];
  for (const [id, m] of Object.entries(result.channels)) {
    const ok = m.rms >= MIN_RMS;
    console.log(`  ${ok ? "LIVE " : "quiet"} ${id}: rms=${m.rms.toExponential(2)} peak=${m.peak.toExponential(2)}`);
    (ok ? LIVE : QUIET).push({ id, rms: m.rms });
  }
  if (LIVE.length === 0) {
    throw new Error("no row channel carried any signal — playback never started, or every channel is unwired");
  }
  LIVE.sort((a, b) => b.rms - a.rms);

  // A live data path is necessary, not sufficient: the defect class is RESOLUTION, which only pixels catch. WebGL keeps no
  // drawing buffer, so the canvas is read through Playwright's screenshot. The control takes the full width (a bass-only
  // control would misfire under the defect); the subject takes the bass third.
  const measurePlot = async (widthFrac = 1) => {
    const rect = await page.evaluate(() => window.subjectRectJS?.("eq_tab_spectrum"));
    if (!rect || !rect.visible) return null;
    const buf = await captureScreenshot(page);
    // The BASS end specifically: the defect empties only the low bands, the left of a log axis, and a lower-half crop would
    // pass once 500 Hz lit up.
    const region = extractRegion(buf, {
      x: rect.x,
      y: rect.y + rect.h * 0.5,
      w: rect.w * widthFrac,
      h: rect.h * 0.45,
    });
    const { data } = region;
    const total = region.width * region.height;
    if (total === 0) return null;
    let bar = 0;
    for (let i = 0; i < data.length; i += 4) {
      // Bar fill is a warm gold; the plot ground is near-black purple.
      if (data[i] > 90 && data[i] > data[i + 2] + 40) bar++;
    }
    return { total, bar, ratio: bar / total, rect };
  };

  // One wall-clock screenshot can land between hits (peak-hold ticks, no fill), a correct reading of a quiet moment; the
  // best frame across a couple of bars is the claim.
  const measurePlotPeak = async (widthFrac = 1, samples = 10, gapMs = 180) => {
    let best = null;
    for (let i = 0; i < samples; i++) {
      const m = await measurePlot(widthFrac);
      if (m && (!best || m.ratio > best.ratio)) best = m;
      if (i < samples - 1) await new Promise((r) => setTimeout(r, gapMs));
    }
    return best;
  };

  // If master reads empty too, the probe is reading the wrong pixels.
  await page.evaluate(() => setEQChannel?.("main"));
  await new Promise((r) => setTimeout(r, 1500));
  const master = await measurePlotPeak(1);
  console.log(`  control (master, full width): ${master ? (master.ratio * 100).toFixed(2) : "n/a"}% bar pixels rect=${master ? JSON.stringify(master.rect) : "-"}`);
  // Measured over the same bass third: master's mid and high bands draw even under the defect.
  if (!master || master.ratio < 0.01) {
    throw new Error("the pixel probe reads the master plot as empty; it is measuring the wrong region, not detecting the bug");
  }

  // Any channel carrying audio decides the claim; pinning row 0 lets a quiet row masquerade as the defect.
  const probeID = LIVE[0].id;
  await page.evaluate((id) => setEQChannel?.(id), probeID);
  await new Promise((r) => setTimeout(r, 2000));
  const plot = await measurePlotPeak(1 / 3);

  if (!plot) {
    throw new Error("could not read the spectrum subject rect / canvas");
  }
  console.log(`  plot (${probeID}): ${(plot.ratio * 100).toFixed(2)}% bar-coloured pixels in the bass third (best of 10 frames)`);
  // Empty reads ~0 %; a kick stays well above 1 % even mid-decay.
  if (plot.ratio < 0.01) {
    throw new Error(
      `the Spectrum plot is empty at the BASS end for a selected instrument ` +
      `channel (${(plot.ratio * 100).toFixed(2)}% bar pixels) — the low display ` +
      `bands are holding no FFT bins (see analyzer.SpectrumFFTSize / ` +
      `audio.BridgeSpectrumBins)`,
    );
  }

  if (QUIET.length > 0) {
    console.log(
      `  NOTE: no hit sampled for ${QUIET.map((d) => `${d.id} (rms=${d.rms.toExponential(2)})`).join(", ")}` +
      ` — sparse rows in this circuit; informational only, see above.`,
    );
  }
  console.log("xplat_spectrum_channel_data: PASS - a live instrument channel renders its BASS bands");
} catch (error) {
  console.error("xplat_spectrum_channel_data: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) {
    await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "xplat_spectrum_channel_data");
  }
  if (cleanup) await cleanup();
}
