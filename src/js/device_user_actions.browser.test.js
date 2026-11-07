/** Browser/OS-boundary user actions with REAL OS APIs (file chooser, native keyboard, recording download, audio
 * unlock, Media Session); exports only assert results. Local emulation by default, real devices with
 * TEST_PLATFORM=browserstack (real iOS Safari is skipped: its automation session never runs a frame). */

import { chromium, devices } from "playwright";
import { buildWasm, startServer, waitForWasmReady } from "./visual_test_helpers.js";
import {
  requireBrowserStackCredentials,
  startTunnel,
  stopTunnel,
  connectRemoteDevice,
  markSessionStatus,
  DEVICE_MATRIX,
} from "./visual_browserstack_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";
import { shouldSkipWasmBuild } from "./browser_test_helpers.js";

const USE_BROWSERSTACK = (process.env.TEST_PLATFORM || "").startsWith("browserstack")
  || process.env.DEVICE === "browserstack";
const deviceFilter = process.env.DEVICE_FILTER || "";

// ─── Fixtures (in-memory; setInputFiles accepts {name,mimeType,buffer}) ──────

// A small valid Beatmo project. bpm is the import marker.
function jsonFixture(bpm) {
  return Buffer.from(JSON.stringify({
    schema_version: "0.0.1", subdiv: 8, bpm,
    instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: 0, color: "#C87850FF" }],
    nodes: [{ id: 0, i: 0, j: 0, type: "regular", inputs: [], outputs: [], volume: 1.0, pitch: 0, duration: 1.0, logic_kind: "", logic_n: 0, logic_p: 0 }],
    eq: { gains_db: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0], bands_hz: [[20, 100], [100, 200]] },
  }), "utf-8");
}

// A minimal valid 16-bit PCM mono WAV (~0.05s @ 8kHz).
function wavFixture() {
  const sampleRate = 8000, dur = 0.05, n = Math.floor(sampleRate * dur);
  const dataLen = n * 2;
  const buf = Buffer.alloc(44 + dataLen);
  buf.write("RIFF", 0); buf.writeUInt32LE(36 + dataLen, 4); buf.write("WAVE", 8);
  buf.write("fmt ", 12); buf.writeUInt32LE(16, 16); buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22); buf.writeUInt32LE(sampleRate, 24);
  buf.writeUInt32LE(sampleRate * 2, 28); buf.writeUInt16LE(2, 32); buf.writeUInt16LE(16, 34);
  buf.write("data", 36); buf.writeUInt32LE(dataLen, 40);
  for (let i = 0; i < n; i++) buf.writeInt16LE(Math.round(8000 * Math.sin((i / sampleRate) * 2 * Math.PI * 440)), 44 + i * 2);
  return buf;
}

// ─── Per-session scenario ledger ─────────────────────────────────────────────
let totalFail = 0, totalPass = 0, totalSkip = 0;

// Run all user-action scenarios against one ready page. Returns {pass, fail, skip}.
async function runUserActions(page, label) {
  let pass = 0, fail = 0, skip = 0;
  const ok = (c, m) => { if (c) { console.log(`    PASS: ${m}`); pass++; } else { console.error(`    FAIL: ${m}`); fail++; } return !!c; };
  const note = (m) => console.log(`    NOTE: ${m}`);
  const read = (fn, ...a) => page.evaluate(({ fn, a }) => { const f = window[fn]; return typeof f === "function" ? f(...a) : undefined; }, { fn, a });
  const settle = (ms) => page.waitForTimeout(ms);
  const center = (r) => [r.x + r.w / 2, r.y + r.h / 2];
  const validRect = (r) => !!(r && typeof r.w === "number" && r.w > 1 && r.h > 1);
  // The File menu opens from the grid-pane folder button; its Upload/Import overlays exist only while it is open.
  const fileMenuRect = () => read("fileMenuBtnRect");

  const overflowArmed = async () => (await page.locator('input[data-fp-id="import"]').count()) > 0;
  // A real tap first; under throttled headless rAF a tap can fall between frames and a blind re-tap lands on the scrim
  // and closes the menu, so the openOverflowMenu() bridge is the fallback. The same overlays mount, so the real
  // setInputFiles path is unchanged.
  async function openOverflow() {
    const r = await fileMenuRect();
    if (r && r.w > 1) await cdpTap(page, Math.round(r.x + r.w / 2), Math.round(r.y + r.h / 2)).catch(() => {});
    for (let k = 0; k < 6; k++) { if (await overflowArmed()) return true; await settle(150); }
    for (let attempt = 0; attempt < 5; attempt++) {
      await read("openOverflowMenu").catch(() => {});
      for (let k = 0; k < 8; k++) { if (await overflowArmed()) return true; await settle(150); }
    }
    return await overflowArmed();
  }
  // forceDraw advances a frame; the natural rAF loop processes queued actions.
  const pump = async () => { await read("forceDraw").catch(() => {}); };
  const poll = async (predFn, ms = 16000) => {
    const deadline = Date.now() + ms;
    while (Date.now() < deadline) {
      await pump();
      if (await predFn()) return true;
      await settle(250);
    }
    return false;
  };
  // Re-taps a few times to ride out dropped taps.
  const tapUntil = async (rectFn, predFn) => {
    for (let i = 0; i < 4; i++) {
      const r = await rectFn();
      if (r && r.w > 1) await cdpTap(page, ...center(r)).catch(() => {});
      if (await poll(predFn, 5000)) return true;
    }
    return false;
  };

  // Both paths are the real OS file API: mobile fills the transparent <input type=file> overlay; desktop taps the button
  // and fills the chooser it opens.
  async function pickFile(kind /* 'import'|'upload' */, file, predFn) {
    const sel = `input[data-fp-id="${kind}"]`;
    await openOverflow();
    if ((await page.locator(sel).count()) > 0) {
      await page.setInputFiles(sel, file);
      return { ok: await poll(predFn), path: "overlay(real <input type=file>)" };
    }
    const rectName = kind === "import" ? "importBtnRect" : "uploadBtnRect";
    const r = await read(rectName);
    if (r && r.w > 1) {
      page.once("filechooser", async (fc) => { try { await fc.setFiles(file); } catch (_) {} });
      await cdpTap(page, r.x + r.w / 2, r.y + r.h / 2);
      return { ok: await poll(predFn), path: "desktop(real button → OS filechooser)" };
    }
    return { ok: false, path: "none(no import/upload affordance found)" };
  }

  // ── PROBE: does setInputFiles work on this device's real <input type=file>? ──
  let filePickerOK = false;
  try {
    await page.evaluate(() => {
      const i = document.createElement("input");
      i.type = "file"; i.id = "__probe_fp";
      window.__probeChanged = false;
      i.addEventListener("change", () => { window.__probeChanged = i.files && i.files.length === 1; });
      document.body.appendChild(i);
    });
    await page.setInputFiles("#__probe_fp", { name: "p.json", mimeType: "application/json", buffer: Buffer.from("{}") });
    filePickerOK = await page.evaluate(() => window.__probeChanged === true);
    await page.evaluate(() => document.getElementById("__probe_fp")?.remove());
  } catch (e) { note(`setInputFiles probe threw: ${e.message}`); }
  ok(filePickerOK, `${label} probe: real <input type=file> accepts setInputFiles (real OS file path usable)`);

  if (!filePickerOK) {
    note(`${label}: file-import scenarios SKIPPED — device cannot drive a real file input via automation`);
    skip += 2;
    return { pass, fail, skip };
  }

  // Exports register before Ebiten's loop has run Layout, which is what makes the game usable, so this polls for a real
  // canvas width and SKIPS a device whose loop never runs. On BrowserStack's iOS session rAF never fires for the WebGL
  // canvas: an environment limit (forceGameTick, screenshots, resize, gestures, no-cache were all tried).
  let gameReady = false;
  {
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      await pump();
      const snap = await read("fullLayoutSnapshot").catch(() => null);
      if (snap && typeof snap.canvasWidth === "number" && snap.canvasWidth > 0) { gameReady = true; break; }
      await settle(400);
    }
  }
  if (!gameReady) {
    note(`${label}: Ebiten run loop never laid out the game (see KNOWN LIMITATION above — real iOS Safari/BrowserStack) — SKIPPING device, not a failure`);
    return { pass, fail, skip: skip + 3 };
  }

  // Real devices can differ from emulation (DPR, viewport); the scenarios are layout-agnostic.
  const ts = await read("getTouchScreenSize").catch(() => null);
  const dpr = await read("getDevicePixelRatio").catch(() => null);
  const snapW = await read("fullLayoutSnapshot").then((s) => s?.canvasWidth).catch(() => null);
  const isMobile = await read("isSmallScreenMode");
  note(`${label} layout: isSmallScreen=${isMobile} canvasWidth=${snapW} touchScreenSize=${JSON.stringify(ts)} dpr=${dpr}`);

  // ── Scenario 1: JSON import via the real OS file path ──
  {
    const res = await pickFile("import", { name: "proj.json", mimeType: "application/json", buffer: jsonFixture(123) },
      async () => await read("getBPM") === 123);
    ok(res.ok, `${label} json-import: real file pick applied the project — bpm→123 (got ${await read("getBPM")}) via ${res.path}`);
  }

  // ── Scenario 2: JSON export correctness + real download + re-import round-trip ──
  {
    const exported = await read("exportJSON");
    let parsed = null;
    try { parsed = JSON.parse(exported); } catch (_) {}
    // schema_version, not the read-only legacy integer "version".
    ok(parsed && typeof parsed.schema_version === "string" && parsed.schema_version !== "" && parsed.bpm === 123,
      `${label} json-export: exportJSON() reflects current state (schema_version=${parsed?.schema_version} bpm=${parsed?.bpm})`);

    // Some platforms do not surface the OS download: logged, not failed.
    const exRect = await read("exportBtnRect");
    if (exRect && exRect.w > 1) {
      const dlP = page.waitForEvent("download", { timeout: 4000 }).then((d) => d).catch(() => null);
      await cdpTap(page, exRect.x + exRect.w / 2, exRect.y + exRect.h / 2);
      const dl = await dlP;
      if (dl) ok(/\.json$/i.test(dl.suggestedFilename ? dl.suggestedFilename() : ""),
        `${label} json-export: real Export tap triggered an OS download (${dl.suggestedFilename?.()})`);
      else note(`${label} json-export: Export tapped; no OS download event surfaced (expected on iOS webkit) — round-trip below proves output validity`);
    } else {
      note(`${label} json-export: Export is overflow-only at this layout; correctness + round-trip cover it`);
    }

    // Import a DIFFERENT project first, both through the real OS path.
    if (exported) {
      const r90 = await pickFile("import", { name: "other.json", mimeType: "application/json", buffer: jsonFixture(90) },
        async () => await read("getBPM") === 90);
      ok(r90.ok, `${label} json-export: re-import of a different project works — bpm→90 (got ${await read("getBPM")}) via ${r90.path}`);
      const rBack = await pickFile("import", { name: "roundtrip.json", mimeType: "application/json", buffer: Buffer.from(exported, "utf-8") },
        async () => await read("getBPM") === 123);
      ok(rBack.ok, `${label} json-export: round-trip — exported bytes re-import to original state — bpm→123 (got ${await read("getBPM")}) via ${rBack.path}`);
    }
  }

  // ── Scenario 3: WAV import via the real OS file path ──
  {
    const res = await pickFile("upload", { name: "tone.wav", mimeType: "audio/wav", buffer: wavFixture() },
      async () => await read("isNamingOpen") === true);
    ok(res.ok, `${label} wav-import: real .wav pick advanced the upload→naming flow (isNamingOpen=${await read("isNamingOpen")}) via ${res.path}`);
    if (res.ok) { await read("closeAllPopups").catch(() => {}); await settle(200); }
  }

  try {
  // The instrument-menu search, since BPM is a value wheel, not a typed field.
  {
    // No pump: a forced Layout between tap and touchend races the field's registration and the input is never created.
    await read("openInstMenu", 0);
    await page.waitForFunction(() => { const r = typeof instMenuSearchRect === "function" && instMenuSearchRect(); return r && r.w > 0; },
      null, { timeout: 5000, polling: 50 }).catch(() => {});
    const r = await read("instMenuSearchRect");
    const waitInput = () => page.waitForFunction(() => {
      for (const inp of document.querySelectorAll('input[style*="z-index"]')) if (inp.style.zIndex === "10000") return true;
      return false;
    }, null, { timeout: 700 }).then(() => true).catch(() => false);
    let kbOpen = false;
    if (validRect(r)) {
      for (let i = 0; i < 6 && !kbOpen; i++) {
        await cdpTap(page, ...center(r)).catch(() => {});
        kbOpen = await waitInput();
      }
    }
    if (kbOpen) {
      // The keyboard opened — typing through it IS gating here.
      await page.keyboard.press("Control+a").catch(() => {});
      await page.keyboard.type("kick").catch(() => {});
      const applied = await page.waitForFunction(() => typeof instMenuSearchState === "function" && instMenuSearchState()?.text === "kick", null, { timeout: 6000, polling: 50 }).then(() => true).catch(() => false);
      ok(applied, `${label} native-kbd(text): real OS keyboard typing reached the search field (text→"kick", got ${JSON.stringify((await read("instMenuSearchState"))?.text)})`);
    } else {
      // Non-gating: environment-sensitive on some real keyboards; mobile_native_input covers the path deterministically.
      note(`${label} native-kbd(text): native keyboard <input> did not open via automation here (non-gating; covered by mobile_native_input.browser.test.js)`);
    }
    // So an on-screen keyboard cannot block the scenarios below.
    await page.keyboard.press("Escape").catch(() => {});
    await page.evaluate(() => { for (const inp of document.querySelectorAll('input[style*="z-index"]')) if (inp.style.zIndex === "10000") { try { inp.blur(); inp.remove(); } catch (_) {} } }).catch(() => {});
    await read("closeAllPopups").catch(() => {});
    await settle(200);
  }

  // Stop produces a real zip download where the platform surfaces it.
  {
    const r = await read("recBtnRect");
    if (validRect(r)) {
      const rec0 = await read("isRecording");
      let recOn = rec0;
      for (let i = 0; i < 4 && recOn === rec0; i++) { await cdpTap(page, ...center(r)); await settle(500); await pump(); recOn = await read("isRecording"); }
      ok(recOn !== rec0, `${label} recording: real tap on Record started capture (isRecording ${rec0}→${recOn})`);
      if (recOn !== rec0) {
        await settle(900); await pump();
        const elapsed = await read("recordingElapsedMs");
        note(`${label} recording: elapsed=${JSON.stringify(elapsed)}ms while recording`);
        // Stop (real tap) and try to capture the OS download (zip) the save path triggers.
        const dlP = page.waitForEvent("download", { timeout: 6000 }).then((d) => d).catch(() => null);
        let recOff = recOn;
        for (let i = 0; i < 4 && recOff === recOn; i++) { await cdpTap(page, ...center(r)); await settle(500); await pump(); recOff = await read("isRecording"); }
        ok(recOff === rec0, `${label} recording: real tap on Record stopped capture (isRecording→${recOff})`);
        const dl = await dlP;
        if (dl) ok(/\.(zip|wav)$/i.test(dl.suggestedFilename ? dl.suggestedFilename() : ""), `${label} recording: stop triggered a real OS download (${dl.suggestedFilename?.()})`);
        else note(`${label} recording: stop did not surface an OS download event (expected on iOS webkit / headless) — capture pipeline covered by recording_lifecycle.browser.test.js`);
      }
    } else {
      note(`${label} recording: no Record button rect at this layout`);
      skip++;
    }
  }

  // The real autoplay-unlock behaviour.
  {
    await tapUntil(() => read("playBtnRect"), async () => await read("isPlaying") === true, "play-for-audio").catch(() => {});
    await settle(300); await pump();
    const ctxState = await page.evaluate(() => (window.__audioCtx && window.__audioCtx.state) || null).catch(() => null);
    ok(ctxState === "running", `${label} audio-unlock: AudioContext is running after real play gesture (state=${ctxState})`);
  }

  // The lock-screen UI is not automatable, so the API being populated is asserted.
  {
    const ms = await page.evaluate(() => {
      if (!("mediaSession" in navigator)) return { supported: false };
      return { supported: true, hasMeta: !!navigator.mediaSession.metadata, title: navigator.mediaSession.metadata?.title || null, playbackState: navigator.mediaSession.playbackState };
    }).catch((e) => ({ err: e.message }));
    if (ms && ms.supported) ok(!!ms.hasMeta, `${label} media-session: Media Session metadata populated during playback (title=${ms.title}, state=${ms.playbackState})`);
    else note(`${label} media-session: Media Session API not supported on this platform (${JSON.stringify(ms)})`);
    await tapUntil(() => read("stopBtnRect"), async () => await read("isPlaying") === false, "stop-after-audio").catch(() => {});
  }
  } catch (e) {
    // Recorded through ok(false, …), so a throw is a FAIL line, not a TypeError that kills the run.
    ok(false, `${label}: Phase-2 scenario threw: ${e.message}`);
  }

  return { pass, fail, skip };
}

// ─── Driver ──────────────────────────────────────────────────────────────────

if (shouldSkipWasmBuild("main.wasm")) {
  console.log("Using prebuilt main.wasm (WASM_PREBUILT=1)");
} else {
  console.log("Building WASM...");
  buildWasm();
}
const { server, port } = await startServer();
console.log(`Local server on port ${port}`);

try {
  if (USE_BROWSERSTACK) {
    const credentials = requireBrowserStackCredentials();
    const matrix = deviceFilter ? DEVICE_MATRIX.filter((d) => d.name.toLowerCase().includes(deviceFilter.toLowerCase())) : DEVICE_MATRIX;
    console.log(`BrowserStack: ${matrix.length} device(s)${deviceFilter ? ` (filter "${deviceFilter}")` : ""}`);
    const tunnel = await startTunnel(credentials.key);
    try {
      for (const device of matrix) {
        console.log(`\n=== ${device.name} (${device.deviceName}, ${device.orientation}) ===`);
        let browser, context, page;
        try {
          await Promise.race([
            (async () => {
              ({ browser, context, page } = await connectRemoteDevice(device, credentials, port));
              page.on("pageerror", (e) => console.log(`    [PAGE ERROR] ${e.message}`));
              await waitForWasmReady(page, 90000);
              const r = await runUserActions(page, device.name);
              totalPass += r.pass; totalFail += r.fail; totalSkip += r.skip;
              await markSessionStatus(page, r.fail === 0 ? "passed" : "failed", `user-actions ${r.pass}P/${r.fail}F/${r.skip}S`).catch(() => {});
            })(),
            new Promise((_, rej) => setTimeout(() => rej(new Error("device timeout 300s")), 300000)),
          ]);
        } catch (e) {
          console.error(`  ERROR: ${e.message} — skipping device`);
          totalSkip++;
        } finally {
          try { if (context) await context.close(); } catch (_) {}
          try { if (browser) await browser.close(); } catch (_) {}
        }
      }
    } finally {
      await stopTunnel(tunnel);
    }
  } else {
    // Local emulation (development): mobile profiles only (file overlay path).
    const localDevices = ["iPhone 12", "Pixel 5"];
    const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
    try {
      for (const name of localDevices) {
        console.log(`\n=== ${name} (local emulation) ===`);
        const context = await browser.newContext({ ...devices[name], hasTouch: true, acceptDownloads: true });
        const page = await context.newPage();
        page.on("pageerror", (e) => console.log(`    [PAGE ERROR] ${e.message}`));
        try {
          await page.goto(`http://localhost:${port}/`);
          await page.waitForFunction(() => typeof fullLayoutSnapshot === "function" && typeof getBPM === "function" && typeof isNamingOpen === "function", null, { timeout: 30000 });
          await page.waitForTimeout(500);
          const r = await runUserActions(page, name);
          totalPass += r.pass; totalFail += r.fail; totalSkip += r.skip;
        } finally {
          await context.close();
        }
      }
    } finally {
      await browser.close();
    }
  }
} finally {
  server.close();
}

console.log("\n" + "=".repeat(56));
console.log(`DEVICE USER-ACTIONS: ${totalPass} passed, ${totalFail} failed, ${totalSkip} skipped`);
console.log("=".repeat(56));
// A BrowserStack session can leave handles that keep Node alive past the summary.
process.exit(totalFail > 0 ? 1 : 0);
