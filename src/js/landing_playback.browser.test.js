// landing_playback.browser.test.js
//
// The landing page's recording, captions, chapter chips and crystal really
// PLAY on a phone, the sound really turns on, and Play hands over to a game
// that plays — on an emulated Android phone, an emulated iPhone with autoplay
// refused (Low Power Mode) and desktop, all on real Chrome (see PLATFORMS). Served by landing_server.js exactly like
// beatmo.io (spec docs/superpowers/specs/2026-10-23-landing-page-design.md).
//
// WHY this file exists next to landing_behavior.browser.test.js: Playwright's
// page.evaluate / waitForFunction run with a USER GESTURE (Chromium
// Runtime.callFunctionOn userGesture:true, WebKit emulateUserGesture:true), so
// the first state read gives the page sticky user activation and every
// autoplay rule is waived from then on. A phone visitor has no activation
// until their first tap, and on a touch screen pointerdown comes BEFORE it
// (activation lands on pointerup / touchend). landing.js unmuted the video on
// pointerdown, the browser paused the autoplaying video for unmuting without a
// gesture, and the "already unmuted" flag kept every later tap from resuming
// it: on a real phone the recording froze on the first touch and never made a
// sound (2026-10-24 report), while every evaluate-driven test passed.
//
// So this file NEVER calls page.evaluate / waitForFunction while a landing is
// up: the page reports its own state through console messages (an init
// script), and the only input is real touch / mouse / keyboard.
//
//   1. Autoplay: the recording plays muted with no input at all; its time
//      advances, the chapter chip fills, a caption shows, the crystal grows.
//   2. The first touch anywhere (empty background, the recording) keeps the
//      recording playing and turns its sound on.
//   3. "Tap for sound" as the very first touch: playing, unmuted.
//   4. Autoplay refused (iOS Low Power Mode, a browser set to block autoplay):
//      nothing plays until a tap — then that tap plays it, with sound.
//   5. Save-Data: nothing is downloaded until a tap asks for sound — then it plays.
//   6. A touch on the crystal plants a node.
//   7. Play: the landing goes, the game plays, its AudioContext runs.
//   LANDING_RECORD=<dir>: also record the whole loop per platform (the
//   recording plays through every chapter and wraps, sound on, Play, the game
//   running) to <dir>/<platform>.webm, for a human to watch.
//
// Run: GO=… node src/js/landing_playback.browser.test.js
//   WASM_FILE=<main.wasm>   serve this build (default: build src/js/main.wasm
//                           unless WASM_PREBUILT=1)
//   LANDING_PLATFORMS=android,iphone,desktop   (the default; iphone-webkit is opt-in)
//   LANDING_ROOT=<dir>      serve another copy of src/js (A/B an older landing.js)

import { chromium, webkit, devices } from "playwright";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { createLandingServer } from "./landing_server.js";
import { resolveGoBinary } from "./browser_test_helpers.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const goDir = path.resolve(jsDir, "..", "go");
const BOOT_TIMEOUT = 180000;
const RECORD = process.env.LANDING_RECORD || "";

let WASM_FILE = process.env.WASM_FILE || "";
if (!WASM_FILE) {
  WASM_FILE = path.join(jsDir, "main.wasm");
  if (process.env.WASM_PREBUILT !== "1") {
    const b = spawnSync(resolveGoBinary(), ["build", "-tags", "noscenes", "-ldflags", "-s -w -X main.defaultLog=INFO", "-o", WASM_FILE, "./cmd"], {
      cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit",
    });
    if (b.status !== 0) throw new Error("go build main.wasm failed");
  } else if (!fs.existsSync(WASM_FILE)) {
    throw new Error(`WASM_PREBUILT=1 but ${WASM_FILE} is missing`);
  }
}

// LANDING_ROOT=<dir>: serve another copy of src/js (an A/B against an older landing.js).
const server = createLandingServer({ wasmFile: WASM_FILE, root: process.env.LANDING_ROOT || undefined });
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const BASE = `http://127.0.0.1:${server.address().port}/`;

// Playwright's bundled Chromium has no H.264: the phone/desktop Chrome runs
// need the real Chrome channel (as landing_behavior does).
// autoplays: false = the engine (or the emulation) refuses play() outside a
// user gesture, so nothing plays before the first tap and that tap must play
// it — the iOS Low Power Mode case.
//   iphone: an iPhone 13's screen and touch on Chrome's engine, autoplay
//     refused the way Low Power Mode refuses it.
//   iphone-webkit (opt-in): Playwright's Linux WebKit (GTK/WPE) — measured
//     2026-10-24 to be no stand-in for iOS video: it refuses muted autoplay
//     of any video with an audio track, flips it unmuted, and across repeated
//     runs either crashed the page (2 of 4 bare runs) or left a gesture-
//     started video "playing" with its clock stuck at 0.11 s.
const PLATFORMS = {
  android: { engine: "chrome", device: devices["Pixel 7"], cut: "phone", autoplays: true },
  iphone: { engine: "chrome", device: devices["iPhone 13"], cut: "phone", autoplays: false, init: [refuseAutoplayScript] },
  "iphone-webkit": { engine: "webkit", device: devices["iPhone 13"], cut: "phone", autoplays: false },
  desktop: { engine: "chrome", device: { viewport: { width: 1280, height: 720 } }, cut: "desktop", autoplays: true },
};
const wanted = (process.env.LANDING_PLATFORMS || "android,iphone,desktop").split(",").map((s) => s.trim()).filter(Boolean);

const failures = [];
let checks = 0;
function check(cond, msg) {
  checks++;
  if (!cond) { failures.push(msg); console.log(`  FAIL ${msg}`); }
}

// ---------------------------------------------------------------------------
// The gesture-free state channel: the page logs its own state every 200 ms.
// ---------------------------------------------------------------------------
function probeScript() {
  const rect = (id) => {
    const e = document.getElementById(id);
    if (!e) return null;
    const r = e.getBoundingClientRect();
    return r.width > 0 ? { x: r.left + r.width / 2, y: r.top + r.height / 2 } : null;
  };
  // A point where a touch reaches the crystal (no button, caption or recording on it).
  const emptySpot = () => {
    for (const [fx, fy] of [[0.08, 0.3], [0.92, 0.3], [0.08, 0.6], [0.92, 0.6], [0.5, 0.97], [0.05, 0.9]]) {
      const x = innerWidth * fx, y = innerHeight * fy;
      const el = document.elementFromPoint(x, y);
      if (el && el.id === "lc-crystal") return { x, y };
    }
    return null;
  };
  let busy = false;
  setInterval(async () => {
    if (busy) return;
    busy = true;
    try {
      const v = document.getElementById("lc-video");
      const L = window.__beatmoLanding;
      let crystal = null;
      if (L && document.getElementById("landing")) {
        try { crystal = await Promise.race([L.stats(), new Promise((r) => setTimeout(() => r(null), 150))]); } catch (_) {}
      }
      const on = document.querySelector("#lc-chapters .on");
      const cap = document.getElementById("lc-cap");
      const ctx = window.__audioCtx;
      console.debug("__landingProbe " + JSON.stringify({
        at: performance.now(),
        landing: document.documentElement.hasAttribute("data-landing") && !!document.getElementById("landing"),
        state: L ? L.state : null,
        cut: document.documentElement.getAttribute("data-cut"),
        video: v ? {
          src: v.currentSrc ? v.currentSrc.split("/").pop() : "",
          paused: v.paused, muted: v.muted, volume: v.volume, t: v.currentTime, ready: v.readyState,
          audioBytes: v.webkitAudioDecodedByteCount ?? null,
        } : null,
        videoRequested: L && L.videoRequested ? L.videoRequested() : null,
        chip: on ? { label: on.firstChild && on.firstChild.textContent, fill: parseFloat(on.firstElementChild ? on.firstElementChild.style.width : "0") || 0 } : null,
        caption: cap && cap.classList.contains("on") ? cap.querySelector(".t").textContent : null,
        sound: document.getElementById("lc-sound") ? document.getElementById("lc-sound").textContent : null,
        crystal: crystal ? { nodes: crystal.nodes, frames: crystal.frames, plants: crystal.plants } : null,
        playReady: !!(document.getElementById("lc-play") && document.getElementById("lc-play").classList.contains("ready")),
        gamePlaying: typeof window.isPlaying === "function" ? !!window.isPlaying() : null,
        audioCtx: ctx ? ctx.state : null,
        rects: { sound: rect("lc-sound"), play: rect("lc-play"), frame: rect("lc-frame"), chip: (() => { const b = document.querySelector("#lc-chapters button:nth-child(2)"); if (!b) return null; const r = b.getBoundingClientRect(); return { x: r.left + r.width / 2, y: r.top + r.height / 2 }; })(), empty: emptySpot() },
        activated: navigator.userActivation ? navigator.userActivation.hasBeenActive : null,
      }));
    } catch (e) {
      console.debug("__landingProbe " + JSON.stringify({ error: String(e) }));
    } finally { busy = false; }
  }, 200);
}

// Autoplay refused, the way iOS Low Power Mode (or a browser set to block
// autoplay) refuses it: play() outside a user gesture rejects.
function refuseAutoplayScript() {
  const play = HTMLMediaElement.prototype.play;
  HTMLMediaElement.prototype.play = function () {
    if (!(navigator.userActivation && navigator.userActivation.isActive)) {
      return Promise.reject(new DOMException("autoplay refused (test)", "NotAllowedError"));
    }
    return play.call(this);
  };
}
function saveDataScript() {
  Object.defineProperty(navigator, "connection", { value: { saveData: true }, configurable: true });
}

const browsers = {};
async function browserFor(engine) {
  if (!browsers[engine]) browsers[engine] = engine === "webkit" ? await webkit.launch() : await chromium.launch({ channel: "chrome" });
  return browsers[engine];
}

async function open(platform, { init = [], record } = {}) {
  const p = PLATFORMS[platform];
  const browser = await browserFor(p.engine);
  const ctx = await browser.newContext({ ...p.device, ...(record ? { recordVideo: { dir: record, size: p.device.viewport } } : {}) });
  for (const s of [probeScript, ...(p.init || []), ...init]) await ctx.addInitScript(s);
  const page = await ctx.newPage();
  page.ctx = ctx;
  page.errors = [];
  page.last = null;
  page.on("pageerror", (e) => page.errors.push(String(e)));
  page.on("console", (m) => {
    const t = m.text();
    if (t.startsWith("__landingProbe ")) page.last = JSON.parse(t.slice(15));
  });
  await page.goto(BASE);
  return page;
}
// Wait until the latest probe satisfies pred (never a page.evaluate).
async function until(page, pred, timeout, what) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (page.last && !page.last.error && pred(page.last)) return page.last;
    await page.waitForTimeout(100);
  }
  throw new Error(`timed out after ${timeout} ms waiting for ${what}; last probe ${JSON.stringify(page.last)}`);
}
// Two probes at least ms apart on the PAGE's clock (a busy main thread — the
// game booting under the landing — can hold the reporter back).
async function sample(page, ms) {
  const a = await until(page, () => true, 10000, "a probe");
  const b = await until(page, (s) => s.at >= a.at + ms, ms + 20000, `a probe ${ms} ms after the last`);
  return { a, b };
}
// The recording plays: not paused, and its clock moved during the sample.
function advanced(a, b) {
  return a.video && b.video && !b.video.paused && b.video.t !== a.video.t && (b.video.t - a.video.t > 0.4 || b.video.t < a.video.t);
}
async function tap(page, p, platform) {
  if (PLATFORMS[platform].device.hasTouch) await page.touchscreen.tap(p.x, p.y);
  else await page.mouse.click(p.x, p.y);
}
const playingDesc = (s) => s && s.video ? `paused=${s.video.paused} muted=${s.video.muted} t=${s.video.t.toFixed(2)} src=${s.video.src}` : "no video";

async function close(page) {
  const video = page.video && page.video();
  await page.ctx.close();
  return video ? video.path() : null;
}

// While the recording plays: the chapter chip fills, a caption shows, the
// crystal grows.
async function crystalAndCues(page, tag) {
  const chipped = await until(page, (s) => s.chip && s.chip.fill > 0, 15000, tag("a chapter chip to fill")).catch(() => null);
  check(!!chipped, tag("no chapter chip fills while the recording plays"));
  const captioned = await until(page, (s) => !!s.caption, 20000, tag("a caption")).catch(() => null);
  check(!!captioned, tag("no caption shows while the recording plays"));
  const { a, b } = await sample(page, 1500);
  if (b.crystal && a.crystal) check(b.crystal.nodes !== a.crystal.nodes || b.crystal.frames > a.crystal.frames, tag(`the crystal is not growing (${JSON.stringify(a.crystal)} → ${JSON.stringify(b.crystal)})`));
  else check(false, tag("no crystal stats"));
}

// ---------------------------------------------------------------------------
for (const platform of wanted) {
  const P = PLATFORMS[platform];
  if (!P) throw new Error(`unknown platform ${platform}`);
  const tag = (s) => `${platform}: ${s}`;
  console.log(`\n== ${platform} (${P.engine}${P.device.isMobile ? ", mobile" : ""})`);

  // 1. Autoplay with no input at all, then 2. the first touch on the background.
  {
    const page = await open(platform);
    try {
      const s0 = await until(page, (s) => s.landing && s.video && s.video.src !== "", BOOT_TIMEOUT, tag("the recording's request"));
      check(s0.cut === P.cut, tag(`cut ${s0.cut}, want ${P.cut}`));
      check(s0.activated !== true, tag("the page has user activation before any input (the probe would hide autoplay rules)"));
      if (P.autoplays) {
        await until(page, (s) => s.video && !s.video.paused && s.video.t > 0.3, 30000, tag("the recording to start"));
        const { a, b } = await sample(page, 1500);
        check(advanced(a, b), tag(`with no input the recording does not play (${playingDesc(a)} → ${playingDesc(b)})`));
        check(b.video.muted, tag("the recording autoplays unmuted"));
        await crystalAndCues(page, tag);
      } else {
        await page.waitForTimeout(3000);
        check(page.last.video.paused, tag("this engine was measured refusing autoplay, yet the recording plays"));
        check(page.last.sound === "Tap to play", tag(`autoplay refused, the sound chip says "${page.last.sound}"`));
      }

      // 2. The visitor's first touch: on the empty background.
      const spot = (await until(page, (s) => !!s.rects.empty, 5000, tag("an empty spot"))).rects.empty;
      const plants0 = page.last.crystal ? page.last.crystal.plants : 0;
      await tap(page, spot, platform);
      await page.waitForTimeout(600);
      const t = await sample(page, 1500);
      check(advanced(t.a, t.b), tag(`the first touch stopped the recording (${playingDesc(t.a)} → ${playingDesc(t.b)})`));
      check(t.b.video && !t.b.video.muted && t.b.video.volume > 0, tag(`the first touch did not turn the sound on (${playingDesc(t.b)})`));
      if (t.a.video && t.b.video && t.a.video.audioBytes != null) check(t.b.video.audioBytes > t.a.video.audioBytes, tag("no audio decoded while the sound is on"));
      check(t.b.sound === "Sound on", tag(`sound chip says "${t.b.sound}"`));
      // 6. ...and that touch planted a node in the crystal.
      // (counted by the crystal itself: it also grows on its own)
      check(t.b.crystal && t.b.crystal.plants === plants0 + 1, tag(`a touch on the crystal planted nothing (plant requests ${plants0} → ${t.b.crystal && t.b.crystal.plants})`));

      if (!P.autoplays) await crystalAndCues(page, tag);
      // A second touch, on the recording itself, keeps it going.
      await tap(page, t.b.rects.frame, platform);
      const u = await sample(page, 1500);
      check(advanced(u.a, u.b) && !u.b.video.muted, tag(`a touch on the recording stopped it or muted it (${playingDesc(u.b)})`));

      // 7. Play: the game takes over and plays, with a running AudioContext.
      await until(page, (s) => s.playReady, BOOT_TIMEOUT, tag("Play to enable"));
      await tap(page, page.last.rects.play, platform);
      const g = await until(page, (s) => !s.landing && s.gamePlaying, 20000, tag("the game to play after Play")).catch(() => null);
      check(!!g, tag(`Play did not start the game (${JSON.stringify(page.last && { landing: page.last.landing, playing: page.last.gamePlaying })})`));
      const ac = await until(page, (s) => s.audioCtx === "running", 10000, tag("a running AudioContext")).catch(() => null);
      check(!!ac, tag(`the game's AudioContext is ${page.last && page.last.audioCtx} after Play`));
      check(page.errors.length === 0, tag(`page errors: ${page.errors.join(" | ")}`));
    } catch (e) {
      check(false, tag(e.message));
    }
    await close(page);
  }

  // 3. "Tap for sound" as the very first touch.
  {
    const page = await open(platform);
    try {
      // Autoplaying engines: the recording runs. Refusing ones: the refusal has
      // happened (metadata loaded, still paused).
      await until(page, (s) => s.videoRequested && (P.autoplays ? !s.video.paused && s.video.t > 0.3 : s.video.ready >= 1 && s.video.paused), BOOT_TIMEOUT, tag("the recording to start (or be refused)"));
      await tap(page, page.last.rects.sound, platform);
      await page.waitForTimeout(600);
      const t = await sample(page, 1500);
      check(advanced(t.a, t.b) && !t.b.video.muted, tag(`"Tap for sound" stopped the recording or left it muted (${playingDesc(t.a)} → ${playingDesc(t.b)})`));
    } catch (e) {
      check(false, tag(e.message));
    }
    await close(page);
  }

  // 4. Autoplay refused: the tap that asks for sound plays it.
  for (const [name, target] of [["Tap for sound", "sound"], ["a chapter chip", "chip"], ["the recording", "frame"]]) {
    const page = await open(platform, { init: [refuseAutoplayScript] });
    try {
      await until(page, (s) => s.videoRequested, BOOT_TIMEOUT, tag("the recording's request"));
      await page.waitForTimeout(1500);
      check(page.last.video.paused, tag("autoplay refused, yet the recording plays (the refusal emulation is broken)"));
      const at = (await until(page, (s) => !!s.rects[target], 15000, tag(`${target} on screen`))).rects[target];
      await tap(page, at, platform);
      await page.waitForTimeout(600);
      const t = await sample(page, 1500);
      check(advanced(t.a, t.b), tag(`autoplay refused: a tap on ${name} does not play the recording (${playingDesc(t.b)})`));
      check(t.b.video && !t.b.video.muted, tag(`autoplay refused: after a tap on ${name} the recording is muted`));
    } catch (e) {
      check(false, tag(e.message));
    }
    await close(page);
  }

  // 5. Save-Data: no download until the visitor asks for sound.
  {
    const page = await open(platform, { init: [saveDataScript] });
    try {
      await until(page, (s) => s.playReady, BOOT_TIMEOUT, tag("Play to enable"));
      check(!page.last.videoRequested && page.last.video.src === "", tag("Save-Data: the recording was downloaded without a tap"));
      await tap(page, page.last.rects.sound, platform);
      await page.waitForTimeout(800);
      await until(page, (s) => s.video && s.video.t > 0.2, 15000, tag("Save-Data: the recording to start after the tap")).catch(() => null);
      const t = await sample(page, 1500);
      check(advanced(t.a, t.b) && !t.b.video.muted, tag(`Save-Data: "Tap for sound" does not play the recording (${playingDesc(t.b)})`));
    } catch (e) {
      check(false, tag(e.message));
    }
    await close(page);
  }

  // The whole loop, recorded for a human: the recording plays through every
  // chapter and wraps, sound on, then Play and a few seconds of the game.
  if (RECORD) {
    const dir = path.join(RECORD, platform);
    fs.mkdirSync(dir, { recursive: true });
    const page = await open(platform, { record: dir });
    try {
      await until(page, (s) => s.videoRequested && (!P.autoplays || (!s.video.paused && s.video.t > 0.3)), BOOT_TIMEOUT, tag("the recording to start"));
      await page.waitForTimeout(4000);
      await tap(page, page.last.rects.sound, platform);
      let maxT = 0;
      const end = Date.now() + 60000;
      while (Date.now() < end) {
        await page.waitForTimeout(200);
        const v = page.last.video;
        if (v.t + 1 < maxT) break; // wrapped: the loop is complete
        maxT = Math.max(maxT, v.t);
      }
      check(maxT > 25, tag(`recording: the loop never got past ${maxT.toFixed(1)} s`));
      await page.waitForTimeout(3000);
      await until(page, (s) => s.playReady, BOOT_TIMEOUT, tag("Play to enable"));
      await tap(page, page.last.rects.play, platform);
      await until(page, (s) => !s.landing && s.gamePlaying, 20000, tag("the game to play")).catch(() => {});
      await page.waitForTimeout(5000);
    } catch (e) {
      check(false, tag(e.message));
    }
    const file = await close(page);
    if (file) {
      const out = path.join(RECORD, `${platform}.webm`);
      fs.renameSync(file, out);
      console.log(`  recorded ${out}`);
    }
  }
}

for (const b of Object.values(browsers)) await b.close();
server.close();
console.log(`\nlanding_playback: ${checks} checks, ${failures.length} failed`);
if (failures.length) {
  for (const f of failures) console.log(`  - ${f}`);
  process.exit(1);
}
console.log("landing_playback: OK");
