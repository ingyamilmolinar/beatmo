// landing_behavior.browser.test.js
//
// The landing page (the tutorial demo shown before the game; spec
// docs/superpowers/specs/2026-10-23-landing-page-design.md) end to end in a
// real browser, served exactly like beatmo.io (landing_server.js: hashed wasm
// stamped into index.html, compression, Range for the video). Real input only
// (mouse, keyboard, touch); state read from the DOM, the network log and the
// game's own exports.
//
//   1. Desktop first visit: the shell paints at once; the Play button walks
//      the honest phases and enables only when the game is ready; the video
//      is not requested before the game's download is done; cues → chapter
//      chips, captions; keys drive the landing and NEVER reach the game;
//      chapters seek; sound unmutes; the crystal draws and plants on a click.
//   2. "Do not show tutorial demo again": unticked by default; ticking stores
//      it; Space / Enter with the box focused never untick it; Enter plays;
//      the next visit skips the landing and fetches none of it; ?landing=1
//      still shows it (ticked) and unticking brings it back on every visit.
//   3. Every visit: without the opt-out, the landing shows again after a full
//      play (no "you've seen it" skip of any kind).
//   4. Phone cut: portrait layout, controls inside the screen and not
//      overlapping, touch on the opt-out and on Play.
//   5. Fallbacks: ?landing=0, an unstamped dev page, no H.264, Save-Data, no
//      OffscreenCanvas (main-thread crystal), storage that throws.
//
// Run: GO=… node src/js/landing_behavior.browser.test.js
//   WASM_FILE=<main.wasm>  serve this build (default: build src/js/main.wasm
//                          unless WASM_PREBUILT=1)
//   LANDING_CHANNEL=chrome browser channel (default chrome — Playwright's
//                          bundled Chromium has no H.264, so the video checks
//                          are skipped, loudly, without it)
//   LANDING_SHOTS=<dir>    save screenshots of each layout for inspection

import { chromium } from "playwright";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { createLandingServer } from "./landing_server.js";
import { resolveGoBinary } from "./browser_test_helpers.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const goDir = path.resolve(jsDir, "..", "go");
const OPT_OUT_KEY = "beatmo.landing.optOut";
const READY_TIMEOUT = 180000;
const SHOTS = process.env.LANDING_SHOTS || "";

let WASM_FILE = process.env.WASM_FILE || "";
if (!WASM_FILE) {
  WASM_FILE = path.join(jsDir, "main.wasm");
  if (process.env.WASM_PREBUILT !== "1") {
    const b = spawnSync(resolveGoBinary(), ["build", "-ldflags", "-s -w -X main.defaultLog=INFO", "-o", WASM_FILE, "./cmd"], {
      cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit",
    });
    if (b.status !== 0) throw new Error("go build main.wasm failed");
  } else if (!fs.existsSync(WASM_FILE)) {
    throw new Error(`WASM_PREBUILT=1 but ${WASM_FILE} is missing`);
  }
}

// The committed cue files: what the chips and captions must show.
function cueFile(cut) {
  const text = fs.readFileSync(path.join(jsDir, "landing", `${cut}.vtt`), "utf8");
  const cues = [];
  for (const block of text.trim().split(/\n\n+/).slice(1)) {
    const [timing, json] = block.split("\n");
    const [a, b] = timing.split(" --> ").map((s) => { const [h, m, x] = s.split(":"); return Number(h) * 3600 + Number(m) * 60 + Number(x); });
    cues.push({ start: a, end: b, ...JSON.parse(json) });
  }
  const chapters = [];
  for (const c of cues) {
    if (!c.chapter) continue;
    const last = chapters[chapters.length - 1];
    if (last && last.id === c.chapter) last.end = c.end;
    else chapters.push({ id: c.chapter, label: c.label, start: c.start, end: c.end });
  }
  return { cues, chapters, titles: cues.filter((c) => c.title).map((c) => c.title) };
}

const deployed = createLandingServer({ wasmFile: WASM_FILE });
const dev = createLandingServer({ wasmFile: WASM_FILE, stamp: false });
await Promise.all([deployed, dev].map((s) => new Promise((r) => s.listen(0, "127.0.0.1", r))));
const BASE = `http://127.0.0.1:${deployed.address().port}`;
const DEV = `http://127.0.0.1:${dev.address().port}`;

const CHANNEL = process.env.LANDING_CHANNEL ?? "chrome";
let browser, h264;
try {
  browser = await chromium.launch({ channel: CHANNEL || undefined });
} catch (_) {
  browser = await chromium.launch();
}
const failures = [];
let checks = 0;
function check(cond, msg) {
  checks++;
  if (!cond) { failures.push(msg); console.log(`  FAIL ${msg}`); }
}
function section(name) { console.log(`\n== ${name}`); }

const DESKTOP = { viewport: { width: 1280, height: 720 } };
const PHONE = { viewport: { width: 390, height: 844 }, deviceScaleFactor: 2, isMobile: true, hasTouch: true };

// A page that records uncaught errors and every request (with its time).
async function open(ctx, url, { init } = {}) {
  const page = await ctx.newPage();
  page.errors = [];
  page.requests = [];
  page.on("pageerror", (e) => page.errors.push(String(e)));
  page.on("request", (r) => page.requests.push({ url: r.url(), name: new URL(r.url()).pathname.slice(1), at: Date.now() }));
  if (init) await page.addInitScript(init);
  await page.goto(url);
  return page;
}
const landingFiles = (page) => page.requests.filter((r) => /^landing(\.js|_crystal\.js|\/)/.test(r.name)).map((r) => r.name);
const shown = (page) => page.evaluate(() => document.documentElement.hasAttribute("data-landing"));
const optState = (page) => page.evaluate(() => document.getElementById("lc-optout")?.getAttribute("aria-checked"));
const stored = (page) => page.evaluate((k) => { try { return localStorage.getItem(k); } catch (_) { return "throws"; } }, OPT_OUT_KEY);
const toastText = (page) => page.evaluate(() => document.getElementById("lc-toast").textContent);
const isPlaying = (page) => page.evaluate(() => typeof isPlaying === "function" && isPlaying());
const crystal = (page) => page.evaluate(() => window.__beatmoLanding.stats());
async function waitReady(page) {
  await page.waitForFunction(() => document.getElementById("lc-play")?.classList.contains("ready"), null, { timeout: READY_TIMEOUT, polling: 100 });
}
async function waitGameFrame(page) {
  await page.waitForFunction(() => window.__beatmoBoot && window.__beatmoBoot.firstFrame != null, null, { timeout: READY_TIMEOUT, polling: 200 });
}
async function shot(page, name) {
  if (!SHOTS) return;
  fs.mkdirSync(SHOTS, { recursive: true });
  await page.screenshot({ path: path.join(SHOTS, `${name}.png`) });
}
function noErrors(page, where) {
  check(page.errors.length === 0, `${where}: uncaught page errors: ${page.errors.join(" | ")}`);
}

try {
  // ───────────────────────────────────────────────────────────────────────
  section("desktop: first visit");
  {
    const ctx = await browser.newContext(DESKTOP);
    const page = await open(ctx, BASE + "/");
    h264 = await page.evaluate(() => document.createElement("video").canPlayType('video/mp4; codecs="avc1.640028, mp4a.40.2"') !== "");
    if (!h264) console.log(`  (this browser has no H.264 — the video checks are skipped; run with real Chrome: LANDING_CHANNEL=chrome)`);
    check(await shown(page), "the landing is not shown on a first visit to a deployed page");
    check(await page.evaluate(() => document.documentElement.getAttribute("data-cut")) === "desktop", "a 16:9 window did not get the desktop cut");
    const box = await page.locator("#landing").boundingBox();
    check(box && box.width === 1280 && box.height === 720, `the landing does not cover the window: ${JSON.stringify(box)}`);
    check(/desktop\.webp/.test(await page.evaluate(() => getComputedStyle(document.getElementById("lc-frame")).backgroundImage)), "the desktop poster is not the frame's background");
    // Game first: the hashed wasm is requested before anything of the landing.
    await page.waitForFunction(() => window.__beatmoLanding, null, { timeout: 20000 });
    const wasmReq = page.requests.findIndex((r) => /^main\.[0-9a-f]{16}\.wasm$/.test(r.name));
    const firstLanding = page.requests.findIndex((r) => /^landing(\.js|_crystal\.js|\/)/.test(r.name));
    check(wasmReq >= 0 && wasmReq < firstLanding, `the game's wasm (request #${wasmReq}) was not requested before the landing's files (#${firstLanding})`);

    // The Play button walks the phases in order and is disabled until ready.
    const ORDER = ["Loading game…", "Starting…", "Warming up sounds…", "Play"];
    const seen = [];
    let disabledEarly = true, pokedEarly = false;
    const t0 = Date.now();
    while (Date.now() - t0 < READY_TIMEOUT) {
      const s = await page.evaluate(() => ({ label: document.getElementById("lc-play-label").textContent, disabled: document.getElementById("lc-play").disabled, ready: document.getElementById("lc-play").classList.contains("ready") }));
      if (seen[seen.length - 1] !== s.label) seen.push(s.label);
      if (!s.ready && !s.disabled) disabledEarly = false;
      if (s.ready) break;
      if (!pokedEarly && !s.ready) {
        // Play does nothing before ready. A scripted click is no user
        // gesture, so the video's muted start is still checked below (a real
        // key or pointer press unmutes it — by design).
        // One evaluate: landing.js's readiness poll cannot run between the
        // read, the click and the result (three round trips let it enable the
        // button in between under load, and the late read failed a page that
        // was right). If it became ready since `s`, there is nothing to poke.
        const poke = await page.evaluate(() => {
          const b = document.getElementById("lc-play");
          if (b.classList.contains("ready")) return null;
          const disabled = b.disabled;
          b.click();
          return { disabled, state: window.__beatmoLanding.state };
        });
        if (poke) {
          pokedEarly = true;
          check(poke.state === "shown", "Play before ready handed over");
          check(poke.disabled, "the Play button is enabled before ready");
        }
      }
      await page.waitForTimeout(50);
    }
    const idx = seen.map((l) => ORDER.indexOf(l));
    check(idx.every((i) => i >= 0) && idx.every((i, k) => k === 0 || i > idx[k - 1]), `Play phases out of order: ${JSON.stringify(seen)}`);
    check(seen[seen.length - 1] === "Play", `the Play button never became ready: ${JSON.stringify(seen)}`);
    check(disabledEarly, "the Play button was enabled before the game was ready");
    check(await page.evaluate(() => { try { return warmupProgress().ready && window.__beatmoBoot.firstFrame != null; } catch (_) { return false; } }), "Play enabled although the game is not ready");
    await shot(page, "desktop-ready");

    // The video is requested only after the game's download finished.
    const timing = await page.evaluate(() => performance.getEntriesByType("resource").map((e) => ({ name: e.name.split("/").pop(), start: e.startTime, end: e.responseEnd })));
    const wasm = timing.find((e) => /^main\.[0-9a-f]{16}\.wasm$/.test(e.name));
    const mp4 = timing.filter((e) => e.name === "desktop.mp4");
    if (h264) {
      check(mp4.length > 0, "the desktop video was never requested");
      for (const v of mp4) check(v.start >= wasm.end - 1, `the video was requested at ${v.start.toFixed(0)} ms, before the game's download ended at ${wasm.end.toFixed(0)} ms`);
      check(!timing.some((e) => e.name === "phone.mp4"), "the desktop page fetched the phone video");
    }

    // Cues → chapter chips, in the recording's order, with its labels.
    const cues = cueFile("desktop");
    await page.waitForFunction((n) => document.querySelectorAll("#lc-chapters button").length === n, cues.chapters.length, { timeout: 15000 }).catch(() => {});
    const chips = await page.$$eval("#lc-chapters button", (bs) => bs.map((b) => b.textContent));
    check(JSON.stringify(chips) === JSON.stringify(cues.chapters.map((c) => c.label)), `chapter chips ${JSON.stringify(chips)}, the cue file has ${JSON.stringify(cues.chapters.map((c) => c.label))}`);

    if (h264) {
      // It plays, muted, and a caption from the cue file shows.
      await page.waitForFunction(() => { const v = document.getElementById("lc-video"); return v && v.currentTime > 0.5; }, null, { timeout: 15000 }).catch(() => {});
      const v = await page.evaluate(() => { const v = document.getElementById("lc-video"); return { t: v.currentTime, muted: v.muted, paused: v.paused }; });
      check(v.t > 0.5 && !v.paused, `the video is not playing: ${JSON.stringify(v)}`);
      check(v.muted, "the video started with sound before any gesture");
      await page.waitForFunction(() => document.getElementById("lc-cap").classList.contains("on"), null, { timeout: 15000 }).catch(() => {});
      const cap = await page.$eval("#lc-cap .t", (e) => e.textContent);
      check(cues.titles.includes(cap), `the caption shows ${JSON.stringify(cap)}, which is not a caption of the recording`);

      // Chapters: a chip seeks to its chapter and lights up; ← / → step.
      const seekCheck = async (i, how) => {
        await page.waitForFunction((i) => document.querySelectorAll("#lc-chapters button")[i]?.classList.contains("on"), i, { timeout: 4000 }).catch(() => {});
        const s = await page.evaluate(() => ({ t: document.getElementById("lc-video").currentTime, on: [...document.querySelectorAll("#lc-chapters button")].findIndex((b) => b.classList.contains("on")) }));
        const c = cues.chapters[i];
        check(s.t >= c.start - 0.05 && s.t < Math.min(c.end, c.start + 4), `${how}: video at ${s.t.toFixed(2)} s, chapter ${c.label} spans ${c.start}–${c.end}`);
        check(s.on === i, `${how}: chip ${s.on} is lit, want ${i} (${c.label})`);
      };
      await page.click(`#lc-chapters button:nth-child(3)`);
      await seekCheck(2, "chip click");
      await page.keyboard.press("ArrowRight");
      await seekCheck(3, "ArrowRight");
      await page.keyboard.press("ArrowLeft");
      await page.keyboard.press("ArrowLeft");
      await seekCheck(1, "ArrowLeft ×2");

      // Sound: the chip unmutes the video.
      await page.click("#lc-sound");
      check(await page.evaluate(() => !document.getElementById("lc-video").muted), "Tap for sound did not unmute the video");
      check((await page.$eval("#lc-sound", (e) => e.textContent)) === "Sound on", "the sound chip did not say Sound on");
    }

    // Keys drive the landing and never reach the game.
    const rule0 = await page.$eval("#lc-rule", (e) => e.textContent);
    await page.keyboard.press("r");
    check((await page.$eval("#lc-rule", (e) => e.textContent)) !== rule0, "R did not change the crystal's rules");
    const theme0 = await page.$eval("#lc-theme", (e) => e.textContent);
    const bg0 = await page.evaluate(() => document.getElementById("landing").style.getPropertyValue("--bg"));
    await page.keyboard.press("t");
    check((await page.$eval("#lc-theme", (e) => e.textContent)) !== theme0, "T did not change the theme name");
    check((await page.evaluate(() => document.getElementById("landing").style.getPropertyValue("--bg"))) !== bg0, "T did not recolour the landing");
    await page.keyboard.press("+");
    check(/Growth 2×/.test(await toastText(page)), `+ did not speed the growth: ${await toastText(page)}`);
    await page.keyboard.press("Space");
    check(/Paused/.test(await toastText(page)), `Space did not pause the crystal: ${await toastText(page)}`);
    await page.waitForTimeout(400);
    check(!(await isPlaying(page)), "Space reached the game while the landing was up (the transport started)");

    // The crystal draws; a click on it plants a node (paused, so nothing else grows).
    const st0 = await crystal(page);
    check(st0 && st0.frames > 10, `the crystal drew ${st0 && st0.frames} frames`);
    const target = await page.evaluate(() => { const e = document.elementFromPoint(1240, 560); return e && e.id; });
    check(target === "lc-crystal" || target === "landing", `the plant point is covered by #${target}`);
    await page.mouse.click(1240, 560);
    await page.waitForTimeout(150);
    const st1 = await crystal(page);
    check(st1.nodes === st0.nodes + 1, `a click on the crystal planted ${st1.nodes - st0.nodes} nodes`);
    await page.mouse.move(640, 360);
    await page.mouse.wheel(0, -400);
    check(await page.evaluate(() => window.scrollY === 0), "the wheel scrolled the page");

    // ── "Do not show tutorial demo again" ──
    section("desktop: opt out");
    check(await optState(page) === "false", "the opt-out box is ticked on a first visit");
    await page.click("#lc-optout");
    check(await optState(page) === "true", "clicking the opt-out did not tick it");
    check(await stored(page) === "1", "the opt-out was not stored");
    check(/will not show again/.test(await toastText(page)), `no confirmation toast: ${await toastText(page)}`);
    check(await page.evaluate(() => window.__beatmoLanding.state) === "shown", "ticking the opt-out left the landing");
    // The box keeps focus: Space must not untick it, Enter must play — not untick.
    await page.keyboard.press("Space");
    check(await optState(page) === "true" && (await stored(page)) === "1", "Space on the focused opt-out box unticked it");
    await page.keyboard.press("Enter");
    await page.waitForFunction(() => !document.getElementById("landing"), null, { timeout: 5000 }).catch(() => {});
    check(await page.evaluate(() => !document.getElementById("landing") && !document.documentElement.hasAttribute("data-landing")), "Enter did not hand over to the game");
    check(await stored(page) === "1", "Enter on the focused opt-out box unticked it");
    await page.waitForFunction(() => isPlaying(), null, { timeout: 10000 }).catch(() => {});
    check(await isPlaying(page), "after Play the game is not playing");
    check(await page.evaluate(() => window.__beatmoLanding.state) === "handedOver", "the landing's state is not handedOver");
    // Keys reach the game now: Space stops the transport.
    await page.keyboard.press("Space");
    await page.waitForFunction(() => !isPlaying(), null, { timeout: 5000 }).catch(() => {});
    check(!(await isPlaying(page)), "after the handover Space does not reach the game");
    noErrors(page, "desktop first visit");

    // The next visit skips it entirely.
    const again = await open(ctx, BASE + "/");
    await waitGameFrame(again);
    check(!(await shown(again)), "the landing showed after opting out");
    check(landingFiles(again).length === 0, `an opted-out visit fetched landing files: ${landingFiles(again)}`);
    noErrors(again, "opted-out visit");
    await again.close();

    // ?landing=1 shows it anyway, ticked; unticking brings it back for good.
    const forced = await open(ctx, BASE + "/?landing=1");
    check(await shown(forced), "?landing=1 did not show the landing after opting out");
    await forced.waitForFunction(() => document.getElementById("lc-optout").getAttribute("aria-checked") === "true", null, { timeout: 10000 }).catch(() => {});
    check(await optState(forced) === "true", "the opt-out box does not show the stored choice");
    await forced.click("#lc-optout");
    check(await optState(forced) === "false" && (await stored(forced)) === null, "unticking did not clear the stored opt-out");
    check(/every visit/.test(await toastText(forced)), `no toast on unticking: ${await toastText(forced)}`);
    await forced.close();
    const back = await open(ctx, BASE + "/");
    check(await shown(back), "after unticking, the landing did not show on the next visit");
    await back.close();
    await ctx.close();
  }

  // ───────────────────────────────────────────────────────────────────────
  section("desktop: every visit (no opt-out)");
  {
    const ctx = await browser.newContext(DESKTOP);
    const page = await open(ctx, BASE + "/");
    await waitReady(page);
    await page.click("#lc-play");
    await page.waitForFunction(() => !document.getElementById("landing") && isPlaying(), null, { timeout: 15000 }).catch(() => {});
    check(await isPlaying(page), "Play did not start the game");
    // Wait past the game's readiness on this build, then come back.
    await page.waitForTimeout(1500);
    const again = await open(ctx, BASE + "/");
    check(await shown(again), "the landing did not show on a second visit without the opt-out");
    check(await stored(again) === null, "something stored the opt-out without the visitor ticking it");
    noErrors(page, "every-visit play");
    await ctx.close();
  }

  // ───────────────────────────────────────────────────────────────────────
  section("phone cut");
  // Every control inside the screen and none overlapping another — the
  // caption included once the recording shows one — on three phone sizes.
  for (const [w, hgt] of [[390, 844], [375, 667], [360, 780]]) {
    const ctx = await browser.newContext({ ...PHONE, viewport: { width: w, height: hgt } });
    const page = await open(ctx, BASE + "/?landing=1");
    const tag = `phone ${w}x${hgt}`;
    check(await page.evaluate(() => document.documentElement.getAttribute("data-cut")) === "phone", `${tag}: a portrait phone did not get the phone cut`);
    check(/phone\.webp/.test(await page.evaluate(() => getComputedStyle(document.getElementById("lc-frame")).backgroundImage)), `${tag}: the phone poster is not the frame's background`);
    await page.waitForFunction(() => document.querySelectorAll("#lc-chapters button").length > 0, null, { timeout: READY_TIMEOUT }).catch(() => {});
    if (h264) await page.waitForFunction(() => document.getElementById("lc-cap").classList.contains("on"), null, { timeout: READY_TIMEOUT }).catch(() => {});
    const boxes = await page.evaluate(() => {
      const ids = { play: "#lc-play", sound: "#lc-sound", optout: "#lc-optout", brand: ".lc-brand h1", video: "#lc-frame", chapters: "#lc-chapters", tips: ".lc-tips" };
      if (document.getElementById("lc-cap").classList.contains("on")) ids.caption = "#lc-cap";
      const out = {};
      for (const [k, sel] of Object.entries(ids)) { const r = document.querySelector(sel).getBoundingClientRect(); out[k] = { x: r.left, y: r.top, r: r.right, b: r.bottom }; }
      out.chipTops = [...document.querySelectorAll("#lc-chapters button")].map((b) => Math.round(b.getBoundingClientRect().top));
      out.chipRights = [...document.querySelectorAll("#lc-chapters button")].map((b) => b.getBoundingClientRect().right);
      return out;
    });
    if (h264) check(!!boxes.caption, `${tag}: no caption showed`);
    const names = Object.keys(boxes).filter((k) => !k.startsWith("chip"));
    for (const n of names) {
      const b = boxes[n];
      check(b.x >= 0 && b.y >= 0 && b.r <= w && b.b <= hgt, `${tag}: ${n} leaves the screen: ${JSON.stringify(b)}`);
    }
    for (let i = 0; i < names.length; i++) {
      for (let j = i + 1; j < names.length; j++) {
        const a = boxes[names[i]], b = boxes[names[j]];
        const overlap = a.x < b.r && b.x < a.r && a.y < b.b && b.y < a.b;
        check(!overlap, `${tag}: ${names[i]} overlaps ${names[j]}: ${JSON.stringify(a)} ${JSON.stringify(b)}`);
      }
    }
    check(new Set(boxes.chipTops).size === 1, `${tag}: the chapter chips wrap onto several rows: tops ${boxes.chipTops}`);
    check(Math.max(...boxes.chipRights) <= w, `${tag}: a chapter chip leaves the screen`);
    await shot(page, `phone-${w}x${hgt}`);
    await ctx.close();
  }
  {
    // Touch: tick the opt-out, then tap Play (a first visit, not forced).
    const ctx = await browser.newContext(PHONE);
    const page = await open(ctx, BASE + "/");
    check(await shown(page), "phone: the landing is not shown on a first visit");
    await waitReady(page);
    await page.tap("#lc-optout");
    check(await optState(page) === "true" && (await stored(page)) === "1", "phone: tapping the opt-out did not tick it");
    await shot(page, "phone-opted-out");
    await page.tap("#lc-play");
    await page.waitForFunction(() => !document.getElementById("landing") && isPlaying(), null, { timeout: 15000 }).catch(() => {});
    check(await isPlaying(page), "phone: tapping Play did not start the game");
    check(await stored(page) === "1", "phone: the opt-out did not survive Play");
    const again = await open(ctx, BASE + "/");
    await waitGameFrame(again);
    check(!(await shown(again)) && landingFiles(again).length === 0, "phone: the landing came back after opting out");
    noErrors(page, "phone");
    await ctx.close();
  }

  // Desktop layout sanity (same rules as the phone) + a screenshot of the ticked box.
  section("desktop layout");
  {
    const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
    const page = await open(ctx, BASE + "/?landing=1");
    await page.waitForFunction(() => document.querySelectorAll("#lc-chapters button").length > 0, null, { timeout: READY_TIMEOUT }).catch(() => {});
    const boxes = await page.evaluate(() => {
      const ids = { play: "#lc-play", sound: "#lc-sound", optout: "#lc-optout", brand: ".lc-brand", video: "#lc-frame", chapters: "#lc-chapters", tips: ".lc-tips", seed: "#lc-seed" };
      const out = {};
      for (const [k, sel] of Object.entries(ids)) { const r = document.querySelector(sel).getBoundingClientRect(); out[k] = { x: r.left, y: r.top, r: r.right, b: r.bottom }; }
      return out;
    });
    const names = Object.keys(boxes);
    for (let i = 0; i < names.length; i++) {
      for (let j = i + 1; j < names.length; j++) {
        const a = boxes[names[i]], b = boxes[names[j]];
        check(!(a.x < b.r && b.x < a.r && a.y < b.b && b.y < a.b), `desktop: ${names[i]} overlaps ${names[j]}`);
      }
    }
    await page.click("#lc-optout");
    await shot(page, "desktop-opted-out");
    await ctx.close();
  }

  // ───────────────────────────────────────────────────────────────────────
  section("fallbacks");
  {
    // ?landing=0 on the deployed page.
    let ctx = await browser.newContext(DESKTOP);
    let page = await open(ctx, BASE + "/?landing=0");
    await waitGameFrame(page);
    check(!(await shown(page)) && landingFiles(page).length === 0, `?landing=0: landing shown or fetched (${landingFiles(page)})`);
    await ctx.close();

    // An unstamped (dev / browser-test) page: no landing unless asked.
    ctx = await browser.newContext(DESKTOP);
    page = await open(ctx, DEV + "/");
    await waitGameFrame(page);
    check(!(await shown(page)) && landingFiles(page).length === 0, "an unstamped dev page showed the landing");
    page = await open(ctx, DEV + "/?landing=1");
    check(await shown(page), "?landing=1 on a dev page did not show the landing");
    await ctx.close();

    // No H.264 / Save-Data: the poster stays and no video is downloaded.
    for (const [name, init] of [
      ["no H.264", () => { HTMLMediaElement.prototype.canPlayType = () => ""; }],
      ["Save-Data", () => { Object.defineProperty(navigator, "connection", { value: { saveData: true } }); }],
    ]) {
      ctx = await browser.newContext(DESKTOP);
      page = await open(ctx, BASE + "/", { init });
      await waitReady(page);
      await page.waitForTimeout(500);
      check(!page.requests.some((r) => /\.mp4$/.test(r.name)), `${name}: the video was downloaded`);
      check(await page.evaluate(() => !window.__beatmoLanding.videoRequested()), `${name}: the landing asked for the video`);
      check(/desktop\.webp/.test(await page.evaluate(() => getComputedStyle(document.getElementById("lc-frame")).backgroundImage)), `${name}: the poster is gone`);
      await page.click("#lc-play");
      await page.waitForFunction(() => !document.getElementById("landing"), null, { timeout: 5000 }).catch(() => {});
      check(await page.evaluate(() => !document.getElementById("landing")), `${name}: Play did not hand over`);
      noErrors(page, name);
      await ctx.close();
    }

    // No OffscreenCanvas: the crystal runs on the main thread instead.
    ctx = await browser.newContext(DESKTOP);
    page = await open(ctx, BASE + "/", { init: () => { delete HTMLCanvasElement.prototype.transferControlToOffscreen; } });
    await page.waitForTimeout(1500);
    const st = await crystal(page);
    check(st && st.frames > 10, `no OffscreenCanvas: the main-thread crystal drew ${st && st.frames} frames`);
    noErrors(page, "no OffscreenCanvas");
    await ctx.close();

    // Storage that throws (some private modes): the landing still shows, and
    // ticking the box says honestly that it cannot be remembered.
    ctx = await browser.newContext(DESKTOP);
    page = await open(ctx, BASE + "/", { init: () => { Storage.prototype.getItem = Storage.prototype.setItem = Storage.prototype.removeItem = () => { throw new Error("denied"); }; } });
    check(await shown(page), "throwing storage: the landing did not show");
    await page.waitForSelector("#lc-optout");
    await page.waitForFunction(() => window.__beatmoLanding, null, { timeout: 10000 });
    await page.click("#lc-optout");
    check(await optState(page) === "false", "throwing storage: the box shows a choice that was not stored");
    check(/cannot remember/.test(await toastText(page)), `throwing storage: toast ${await toastText(page)}`);
    noErrors(page, "throwing storage");
    await ctx.close();
  }
} finally {
  await browser.close();
  deployed.close();
  dev.close();
}

console.log(`\n${checks} checks${h264 ? "" : " (video checks skipped: no H.264 in this browser)"}`);
if (failures.length) {
  console.error(`landing_behavior: FAILED (${failures.length})`);
  for (const f of failures) console.error("  - " + f);
  process.exit(1);
}
console.log("landing_behavior: OK");
