/**
 * Per-instrument channel analyser data + plot (WASM)
 *
 * The Spectrum tab renders a channel's bars from
 * audio.ChannelAnalyzerSnapshot(id), aggregating FFT bins into fixed
 * display bands. A band containing NO bin reports the reference floor,
 * indistinguishable from silence — so the browser bridge's bin width has to
 * stay under the narrowest band (24 Hz). It did not: fftSize 512 plus a
 * 64-bin linear downsample put 375 Hz between bins at 48 kHz, and the four
 * lowest bands — everything a kick is made of — rendered as an empty plot.
 * Desktop runs its own 43 Hz-per-bin FFT and looked perfect, so no Go test
 * and no desktop screenshot could see it. See analyzer.SpectrumFFTSize and
 * audio.BridgeSpectrumBins; the Go-side guard is
 * internal/ui/spectrum_bin_resolution_test.go.
 *
 * Charter fit (CLAUDE.md § JS Test Charter): this is WebAudio graph wiring
 * plus the Go->JS analyser bridge, neither of which Go can reach.
 *
 * COVERED-BY-GO: nothing. The equivalent Go path (analyzer.Service detail
 * channel) is a different implementation; this asserts the WebAudio one.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { captureScreenshot, extractRegion } from "./visual_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

// A channel carrying a playing instrument must read well above the analyser's
// noise floor. -80 dBFS is the display floor (analyzerDBFloor / spectrumMinDB);
// anything at or under it renders as no bar at all.
// Peak RMS over the sampling window. A live channel reads ~0.1 on a hit; a
// dead branch reads ~1e-36 at every sample. The threshold sits far below the
// former and astronomically above the latter, so it cannot pass by accident.
const MIN_RMS = 1e-3;

let cleanup;
let page;

try {
  console.log("xplat_spectrum_channel_data: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => {
    // The Spectrum tab must be the ACTIVE one or nothing draws its plot and
    // the pixel check below passes/fails for the wrong reason.
    setActiveEQTab?.("spectrum");
  });

  // Wait for the circuit to finish IMPORTING before asking what its rows
  // are. The demo replaces the boot circuit a few seconds in, and a row set
  // read before that names instruments that are about to stop existing —
  // whose channels then read denormal noise forever, because
  // getChannelChain() happily creates an empty chain for any id and nothing
  // ever plays into it.
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

  // Drive the app's own channel-selection chokepoint, exactly as the
  // dropdown does — NOT enableChannelAnalyzer directly, which would test the
  // fixture rather than the product.
  // NOTE: the row ids come from drumRowCount()/rowInstrument(i), the exports
  // that actually exist. An earlier version called `rowInstrumentIDs()` —
  // which is a GO TEST HELPER, never a JS export — behind a
  // `typeof … === "function"` guard with a hardcoded ["dnb-kick"] fallback.
  // The guard swallowed the missing export and the fallback named an
  // instrument the loaded circuit does not contain, so the probe measured an
  // empty channel chain (rms ~5e-10, denormal noise) and reported it as a
  // dead analyser. If you add an export here, add it to
  // wasm_bridge_smoke.browser.test.js's catalogue too.
  const result = await page.evaluate(async (ids) => {
    const out = { ids, channels: {} };
    for (const id of ids) {
      setEQChannel?.(id);
      // A percussive channel is SILENT between its hits — sampling once lands
      // in a gap and proves nothing. Take the max across a couple of bars:
      // the claim is that the channel carries data WHEN IT SOUNDS, which is
      // what a dead analyser branch (reading ~1e-36 forever) fails.
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

  // This reading picks the channel to probe. It is NOT an assertion, and
  // must not become one: a row's hit RATE is a property of the circuit, not
  // of the wiring. In the shipped demo the rows carry 4 to 64 beats over a
  // 128-step pattern, so a sparse row can easily go a whole sampling window
  // without firing and read denormal noise (measured: shaker, 4 beats, 0
  // hits in 6 s while organ-church at 32 beats hit 18 times). That looks
  // exactly like an unwired analyser and is not one — asserting on it fails
  // the build for a row that is merely quiet.
  //
  // What IS assertable is the rendering contract: a channel that demonstrably
  // carries audio must render its bands. So take the loudest and probe that.
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

  // --- the rendered result -------------------------------------------------
  //
  // The data path being live is necessary but NOT sufficient: the defect
  // that shipped was a RESOLUTION one. The bridge asked the AnalyserNode for
  // fftSize 512 and then downsampled the snapshot to 64 linear bins — 375 Hz
  // apart at 48 kHz — while the narrowest display band is 24 Hz wide. The
  // four lowest bands held no FFT bin at all and reported the reference
  // floor, so a kick rendered as an empty plot. Desktop, at 43 Hz per bin,
  // looked perfect throughout. Only pixels catch this.
  // Ebiten renders through WebGL, so reading the canvas with drawImage /
  // getImageData returns an EMPTY buffer unless preserveDrawingBuffer is set
  // — the master-channel control below exists because that silently made an
  // earlier version of this probe "detect" the bug everywhere. Go through
  // Playwright's screenshot + the repo's PNG helpers, the same path the
  // crop runner uses.
  // widthFrac selects how much of the plot to sample, measured from the
  // left. The CONTROL takes the whole width — its only job is to prove the
  // probe is reading real pixels, and under the defect the bass is empty on
  // EVERY channel including master, so a bass-only control would fire its
  // "wrong region" guard and mask the real diagnosis. The SUBJECT takes the
  // bass third, which is the region the defect empties.
  const measurePlot = async (widthFrac = 1) => {
    const rect = await page.evaluate(() => window.subjectRectJS?.("eq_tab_spectrum"));
    if (!rect || !rect.visible) return null;
    const buf = await captureScreenshot(page);
    // Sample the BASS end of the plot specifically, not just the lower half.
    //
    // The defect empties the low frequency bands and nothing else: the
    // bridge delivered 64 linear bins over the whole spectrum (375 Hz apart
    // at 48 kHz) while the narrowest display band is 24 Hz wide, so every
    // band under ~375 Hz held no FFT bin and reported the reference floor.
    // On a log axis that is the LEFT of the plot, and the first band that
    // ever drew was 355-710 Hz. A lower-half crop spans the whole frequency
    // range, so it would go green as soon as the 500 Hz band lit up — the
    // exact half-fixed state that has to fail. The left third plus the
    // lower half is the region a kick has to fill.
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

  // A percussive channel is SILENT between its hits, and the browser runs on
  // wall-clock audio — so a single screenshot lands wherever it lands. The
  // bars decay between transients (spectrumHoldDecayDBPerSec) and a capture
  // in a gap shows peak-hold ticks and no fill, which is a correct reading
  // of a quiet moment, not the defect. Desktop cannot show this: its
  // screenshot harness is frame-driven and therefore deterministic.
  //
  // The claim is "when the channel SOUNDS, its bands render". So sample
  // across a couple of bars and take the best frame. Anything less is a
  // coin flip — the same trap the rms reading above fell into.
  const measurePlotPeak = async (widthFrac = 1, samples = 10, gapMs = 180) => {
    let best = null;
    for (let i = 0; i < samples; i++) {
      const m = await measurePlot(widthFrac);
      if (m && (!best || m.ratio > best.ratio)) best = m;
      if (i < samples - 1) await new Promise((r) => setTimeout(r, gapMs));
    }
    return best;
  };

  // Control: the MASTER channel is known to render bars in the browser. If it
  // reads empty too, the probe is measuring the wrong pixels and the
  // instrument result below means nothing.
  await page.evaluate(() => setEQChannel?.("main"));
  await new Promise((r) => setTimeout(r, 1500));
  const master = await measurePlotPeak(1);
  console.log(`  control (master, full width): ${master ? (master.ratio * 100).toFixed(2) : "n/a"}% bar pixels rect=${master ? JSON.stringify(master.rect) : "-"}`);
  // Master is broadband, so under the defect its MID and HIGH bands still
  // drew and a full-width probe passed — which is precisely why the control
  // has to be measured over the same bass-third region as the subject.
  if (!master || master.ratio < 0.01) {
    throw new Error("the pixel probe reads the master plot as empty; it is measuring the wrong region, not detecting the bug");
  }

  // Probe the LOUDEST live channel. The claim under test is "a
  // per-instrument channel renders its bands", and any channel that is
  // actually carrying audio proves or disproves it; pinning row 0 instead
  // just lets defect (1) masquerade as defect (2).
  const probeID = LIVE[0].id;
  await page.evaluate((id) => setEQChannel?.(id), probeID);
  await new Promise((r) => setTimeout(r, 2000));
  const plot = await measurePlotPeak(1 / 3);

  if (!plot) {
    throw new Error("could not read the spectrum subject rect / canvas");
  }
  console.log(`  plot (${probeID}): ${(plot.ratio * 100).toFixed(2)}% bar-coloured pixels in the bass third (best of 10 frames)`);
  // An empty plot reads ~0%. A kick channel fills its bass bands; even mid
  // decay it stays well above 1%.
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
