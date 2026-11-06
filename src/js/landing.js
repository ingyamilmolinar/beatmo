// landing.js — the landing page overlay (spec
// docs/superpowers/specs/2026-10-23-landing-page-design.md). Loaded by the
// first <head> script only when the landing is shown; the shell (markup + CSS)
// is inline in index.html so it paints before this file arrives.
//
// Owns: the crystal worker, the recording (deferred until the game's download
// has finished), cue-driven captions / chapter chips / theme / beat clock, the
// visitor's controls, readiness, and the Play handover to the game beneath.

const $ = (id) => document.getElementById(id);
const root = document.documentElement;
const landing = $("landing");
const cut = root.getAttribute("data-cut") || "desktop";
const video = $("lc-video");
const frameEl = $("lc-frame");
const boot = () => window.__beatmoBoot || {};

// The recording, its poster and its cue file — one set per layout.
const MEDIA = {
  desktop: { video: new URL("./landing/desktop.mp4", import.meta.url), cues: new URL("./landing/desktop.vtt", import.meta.url) },
  phone: { video: new URL("./landing/phone.mp4", import.meta.url), cues: new URL("./landing/phone.vtt", import.meta.url) },
}[cut];

// Fallback palette until the cue file (which carries the game's own resolved
// presets) has loaded.
let palettes = [{ id: "vice-city", name: "Vice City", bg: "#120A1C", acc: "#FFB30A", cols: ["#EE00DD", "#00C8E0", "#3FD67A", "#FFB30A", "#C24FFF", "#FF5C2A"] }];
let paletteIdx = 0;
let song = { bpm: 140, beat0: 0 };
const RULE_NAMES = ["Crystal", "Coral", "Circuit", "Weave"];
const SPEEDS = [0.25, 0.5, 1, 2, 4, 8];
let ruleIdx = 0, speedIdx = 2, paused = false;
const cap = $("lc-cap");
const chipsEl = $("lc-chapters");
let chapters = []; // {id,label,start,end} from the cue file
let chapterIdx = 0;
let capSide = "left";

// ---------------------------------------------------------------------------
// Crystal: a worker owning an OffscreenCanvas, or the same module on the main
// thread where OffscreenCanvas is missing.
// ---------------------------------------------------------------------------
const canvas = $("lc-crystal");
const pad2 = (n) => String(n).padStart(2, "0");
const D = new Date();
const stamp = `${D.getFullYear()}-${pad2(D.getMonth() + 1)}-${pad2(D.getDate())} ${pad2(D.getHours())}:${pad2(D.getMinutes())}`;
const seedParam = /[?&]seed=([0-9a-f]{1,8})/i.exec(location.search);
let seed = 2166136261;
for (const c of stamp) { seed ^= c.charCodeAt(0); seed = Math.imul(seed, 16777619) >>> 0; }
if (seedParam) seed = parseInt(seedParam[1], 16) >>> 0;
$("lc-seed").textContent = `crystal #${seed.toString(16).padStart(8, "0")} · grown from ${seedParam ? "a shared seed" : stamp}`;

let worker = null, view = null, statsWaiter = null;
function post(m, transfer) {
  if (worker) worker.postMessage(m, transfer || []);
  else if (view) return view.handle(m);
  else pending.push(m);
  return null;
}
const pending = [];
const initMsg = { type: "init", w: innerWidth, h: innerHeight, dpr: Math.min(2, devicePixelRatio || 1), seed, rule: 0, theme: palettes[0] };
// ?crystal=0 skips the crystal (A/B knob for the load benchmark).
const crystalOff = /[?&]crystal=0/.test(location.search);
if (crystalOff) {
  // nothing: the canvas stays the background colour
} else if (canvas.transferControlToOffscreen && typeof Worker === "function") {
  try {
    worker = new Worker(new URL("./landing_crystal.js", import.meta.url), { type: "module" });
    const off = canvas.transferControlToOffscreen();
    worker.postMessage({ ...initMsg, canvas: off }, [off]);
    worker.onmessage = (e) => { if (e.data.type === "stats" && statsWaiter) { statsWaiter(e.data.stats); statsWaiter = null; } };
  } catch (_) { worker = null; }
}
if (!worker && !crystalOff) {
  import("./landing_crystal.js").then((m) => {
    view = m.createCrystalView(canvas, initMsg);
    for (const p of pending.splice(0)) view.handle(p);
  });
}

function sendHole() {
  const r = frameEl.getBoundingClientRect();
  post({ type: "hole", cx: r.left + r.width / 2, cy: r.top + r.height / 2, hw: r.width / 2, hh: r.height / 2 });
  placeCaption();
}
addEventListener("resize", () => { post({ type: "resize", w: innerWidth, h: innerHeight, dpr: Math.min(2, devicePixelRatio || 1) }); sendHole(); });
sendHole();

// ---------------------------------------------------------------------------
// Controls (they only change how the crystal looks).
// ---------------------------------------------------------------------------
function toast(msg, ms) {
  const t = $("lc-toast");
  t.textContent = msg;
  t.style.opacity = 1;
  clearTimeout(toast.h);
  toast.h = setTimeout(() => (t.style.opacity = 0), ms || 1100);
}
function setPalette(i, quiet) {
  paletteIdx = ((i % palettes.length) + palettes.length) % palettes.length;
  const p = palettes[paletteIdx];
  landing.style.setProperty("--bg", p.bg);
  landing.style.setProperty("--acc", p.acc);
  $("lc-theme").textContent = p.name;
  post({ type: "theme", theme: p });
  if (!quiet) toast("Theme: " + p.name);
}
const actions = {
  r: () => { ruleIdx = (ruleIdx + 1) % RULE_NAMES.length; post({ type: "rule", idx: ruleIdx }); $("lc-rule").textContent = RULE_NAMES[ruleIdx]; toast(`Rules: ${RULE_NAMES[ruleIdx]} — same seed, new growth`); },
  "-": () => { speedIdx = Math.max(0, speedIdx - 1); post({ type: "speed", idx: speedIdx }); toast(`Growth ${SPEEDS[speedIdx]}×`); },
  "+": () => { speedIdx = Math.min(SPEEDS.length - 1, speedIdx + 1); post({ type: "speed", idx: speedIdx }); toast(`Growth ${SPEEDS[speedIdx]}×`); },
  t: () => setPalette(paletteIdx + 1),
  0: () => { post({ type: "zoomReset" }); toast("Zoom reset"); },
  " ": () => { paused = !paused; post({ type: "pause", paused }); toast(paused ? "Paused" : "Growing"); },
  arrowright: () => goChapter(chapterIdx + 1),
  arrowleft: () => goChapter(chapterIdx - 1),
};
actions["="] = actions["+"];
actions._ = actions["-"];
for (const b of landing.querySelectorAll(".lc-tips button")) b.addEventListener("click", (e) => { e.stopPropagation(); actions[b.dataset.k](); });
// Keys arrive through the capture guard in index.html's head script, which
// keeps them from the game while the landing is up.
// Every key is the landing's while it is up: preventDefault also keeps Enter /
// Space from clicking whichever landing button the last click left focused
// (Enter on a focused opt-out box would otherwise untick it while playing).
window.__beatmoLandingKey = (e) => {
  wantSound();
  if (e.key === "Enter") { e.preventDefault(); handOver(); return; }
  const a = actions[e.key.toLowerCase()];
  if (a) { e.preventDefault(); a(); }
};
landing.addEventListener("wheel", (e) => { e.preventDefault(); post({ type: "zoom", factor: Math.exp(-e.deltaY * 0.0015) }); }, { passive: false });
const touches = new Map();
let pinch0 = 0;
landing.addEventListener("touchstart", (e) => {
  for (const t of e.changedTouches) touches.set(t.identifier, t);
  if (touches.size === 2) { const [a, b] = [...touches.values()]; pinch0 = Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY); }
}, { passive: true });
landing.addEventListener("touchmove", (e) => {
  e.preventDefault();
  for (const t of e.changedTouches) touches.set(t.identifier, t);
  if (touches.size === 2 && pinch0) {
    const [a, b] = [...touches.values()];
    const d = Math.hypot(a.clientX - b.clientX, a.clientY - b.clientY);
    post({ type: "zoom", factor: d / pinch0 });
    pinch0 = d;
  }
}, { passive: false });
landing.addEventListener("touchend", (e) => { for (const t of e.changedTouches) touches.delete(t.identifier); if (touches.size < 2) pinch0 = 0; });
landing.addEventListener("pointermove", (e) => { if (e.pointerType === "mouse") post({ type: "pointer", x: e.clientX, y: e.clientY }); });
landing.addEventListener("click", (e) => { if (e.target === landing || e.target === canvas) post({ type: "plant", x: e.clientX, y: e.clientY }); });

// ---------------------------------------------------------------------------
// "Do not show tutorial demo again": the landing shows on every visit until
// the visitor ticks this (the head script in index.html reads the same key).
// Ticking only stores the choice; this visit carries on until Play.
// ---------------------------------------------------------------------------
const OPT_OUT_KEY = "beatmo.landing.optOut";
const optBtn = $("lc-optout");
function optedOut() {
  try { return localStorage.getItem(OPT_OUT_KEY) === "1"; } catch (_) { return false; }
}
function syncOptOut() { optBtn.setAttribute("aria-checked", String(optedOut())); }
optBtn.addEventListener("click", (e) => {
  e.stopPropagation();
  const want = !optedOut();
  try {
    if (want) localStorage.setItem(OPT_OUT_KEY, "1");
    else localStorage.removeItem(OPT_OUT_KEY);
  } catch (_) {}
  syncOptOut();
  if (optedOut() !== want) toast("This browser cannot remember that choice", 2200);
  else toast(want ? "The tutorial demo will not show again" : "The tutorial demo will show on every visit", 2200);
});
syncOptOut();

// ---------------------------------------------------------------------------
// The recording: loads only after the game's download has finished (from there
// the game is CPU-bound, so the video uses bandwidth nobody else needs).
// ---------------------------------------------------------------------------
const saveData = !!(navigator.connection && navigator.connection.saveData);
// ?video=late starts the recording only once the game is ready (A/B knob for
// the load benchmark: it isolates the video's cost during the game's boot).
const videoAfterReady = /[?&]video=late/.test(location.search);
// H.264 + AAC plays in Chrome, Safari, Firefox and Edge; an engine without it
// (e.g. an open-source Chromium build) keeps the poster and downloads nothing.
const canPlay = video.canPlayType('video/mp4; codecs="avc1.640028, mp4a.40.2"') !== "";
let videoRequested = false;
function loadVideo(asked) {
  // Save-Data: no download until the visitor asks for the recording (a tap).
  if (videoRequested || !canPlay || (saveData && !asked)) return;
  videoRequested = true;
  video.src = MEDIA.video.href;
  video.preload = "auto";
  play();
}
// play() outside a user gesture may be refused (iOS Low Power Mode, a browser
// set to block autoplay, a video with sound under a strict policy): the poster
// stays and the next tap or key (wantSound) plays it.
function play() {
  const p = video.play();
  if (p && p.catch) p.catch(() => { if (video.paused) soundBtn.textContent = "Tap to play"; });
}
// The visitor's tap or key: the recording plays, with sound. Must run inside
// the gesture (click / keydown), never on pointerdown — on a touch screen
// pointerdown comes BEFORE the tap counts as a user gesture, and unmuting an
// autoplaying video there makes the browser PAUSE it (that froze the
// recording on every phone's first touch). Idempotent; also resumes a
// recording the browser paused.
const soundBtn = $("lc-sound");
function wantSound() {
  if (!landing.isConnected || window.__beatmoLanding.state !== "shown") return;
  loadVideo(true);
  if (!videoRequested) return;
  video.muted = false;
  if (video.paused) play();
  soundBtn.textContent = "Sound on";
}
landing.addEventListener("click", wantSound, true);
soundBtn.addEventListener("click", (e) => e.stopPropagation());
function syncClock() { post({ type: "clock", playing: !video.paused && video.readyState >= 2, t: video.currentTime, bpm: song.bpm, beat0: song.beat0 }); }
for (const ev of ["playing", "pause", "seeked", "ratechange"]) video.addEventListener(ev, syncClock);
video.addEventListener("timeupdate", () => { if (++syncClock.n % 8 === 0) syncClock(); });
syncClock.n = 0;

// ---------------------------------------------------------------------------
// Cues (WebVTT metadata, JSON payloads written by the game's tour recorder).
// ---------------------------------------------------------------------------
const trackEl = document.createElement("track");
trackEl.kind = "metadata";
trackEl.src = MEDIA.cues.href;
trackEl.default = true;
video.appendChild(trackEl);
trackEl.track.mode = "hidden";
const parse = (c) => { try { return JSON.parse(c.text); } catch (_) { return {}; } };
trackEl.addEventListener("load", () => {
  const cues = [...trackEl.track.cues];
  for (const c of cues) {
    const d = parse(c);
    if (d.song) song = d.song;
    if (d.palettes && d.palettes.length) palettes = d.palettes;
    if (d.chapter) {
      const last = chapters[chapters.length - 1];
      if (last && last.id === d.chapter) last.end = c.endTime;
      else chapters.push({ id: d.chapter, label: d.label || d.chapter, start: c.startTime, end: c.endTime });
    }
  }
  chipsEl.textContent = "";
  chapters.forEach((ch, i) => {
    const b = document.createElement("button");
    b.textContent = ch.label;
    b.appendChild(document.createElement("i"));
    b.addEventListener("click", (e) => { e.stopPropagation(); goChapter(i); });
    chipsEl.appendChild(b);
  });
  setPalette(0, true);
  syncClock();
});
trackEl.track.addEventListener("cuechange", () => {
  let caption = null;
  for (const c of trackEl.track.activeCues || []) {
    const d = parse(c);
    if (d.title) caption = d;
    if (d.theme) {
      const i = palettes.findIndex((p) => p.id === d.theme);
      if (i >= 0 && i !== paletteIdx) setPalette(i, true);
    }
  }
  showCaption(caption);
});
function placeCaption() {
  const r = frameEl.getBoundingClientRect();
  if (cut === "phone") {
    // Under the recording, above the bottom stack (chapters at 84px + 30).
    cap.style.left = "12px"; cap.style.right = "12px";
    cap.style.top = Math.min(innerHeight - 180, r.bottom + 8) + "px";
    return;
  }
  const room = (capSide === "left" ? r.left : innerWidth - r.right) - 36;
  const w = Math.max(140, Math.min(260, room));
  cap.style.width = w + "px";
  cap.style.top = r.top + r.height * 0.55 + "px";
  cap.style.left = (capSide === "left" ? r.left - w - 20 : r.right + 16) + "px";
}
function showCaption(d) {
  const key = d ? d.title + "|" + d.sub : "";
  if (key === showCaption.key) return;
  showCaption.key = key;
  cap.classList.remove("on");
  clearTimeout(showCaption.h);
  if (!d) return;
  showCaption.h = setTimeout(() => {
    cap.querySelector(".t").textContent = d.title;
    cap.querySelector(".s").textContent = d.sub || "";
    capSide = d.side === "right" ? "right" : "left";
    placeCaption();
    cap.classList.add("on");
  }, 250);
}
function goChapter(i) {
  if (!chapters.length) return;
  chapterIdx = ((i % chapters.length) + chapters.length) % chapters.length;
  video.currentTime = chapters[chapterIdx].start + 0.01;
}

// ---------------------------------------------------------------------------
// Readiness (no new Go export): the first full frame has drawn and the voice
// warm-up gate is open. The ring shows honest phases.
// ---------------------------------------------------------------------------
const playBtn = $("lc-play"), playLbl = $("lc-play-label"), ring = $("lc-ring");
let playReady = false;
function poll() {
  const b = boot();
  let w = null;
  try { w = typeof window.warmupProgress === "function" ? window.warmupProgress() : null; } catch (_) {}
  if (b.wasmInstantiated != null && !videoAfterReady) loadVideo();
  if (b.firstFrame != null && w && w.ready) {
    loadVideo();
    playReady = true;
    playBtn.disabled = false;
    playBtn.classList.add("ready");
    playLbl.textContent = "Play";
    return;
  }
  if (b.wasmInstantiated == null) { playLbl.textContent = "Loading game…"; ring.style.setProperty("--p", "8%"); }
  else if (b.firstFrame == null) { playLbl.textContent = "Starting…"; ring.style.setProperty("--p", "30%"); }
  else {
    const frac = w && w.total ? w.done / w.total : 0;
    playLbl.textContent = "Warming up sounds…";
    ring.style.setProperty("--p", (30 + 70 * frac).toFixed(0) + "%");
  }
  setTimeout(poll, 250);
}
poll();
// Chapter chips show progress through the active chapter.
setInterval(() => {
  if (!chapters.length || video.paused) return;
  const t = video.currentTime;
  const i = chapters.findIndex((c) => t >= c.start && t < c.end);
  if (i >= 0) chapterIdx = i;
  [...chipsEl.children].forEach((b, k) => {
    const c = chapters[k];
    b.classList.toggle("on", k === i);
    b.firstElementChild.style.width = k === i ? Math.min(100, ((t - c.start) / (c.end - c.start)) * 100) + "%" : "0";
  });
}, 100);

// ---------------------------------------------------------------------------
// Play: the press is a document-level gesture, so audio.js's capture-phase
// unlock listener resumes the AudioContext; then hand the screen to the game.
// ---------------------------------------------------------------------------
function handOver() {
  if (!playReady) return;
  playReady = false;
  video.muted = true;
  video.pause();
  root.removeAttribute("data-landing");
  landing.classList.add("lc-out");
  try { if (typeof window.startPlay === "function") window.startPlay(); } catch (_) {}
  // Keys go to the game from here: hand it the focus (a landing button had it,
  // and that button is about to be removed).
  const game = window.__beatmoGameCanvas();
  if (game) {
    if (!game.hasAttribute("tabindex")) game.tabIndex = -1;
    game.focus({ preventScroll: true });
  }
  setTimeout(() => {
    if (worker) worker.terminate();
    video.removeAttribute("src");
    video.load();
    landing.remove();
  }, 500);
  window.__beatmoLanding.state = "handedOver";
}
playBtn.addEventListener("click", (e) => { e.stopPropagation(); handOver(); });

// Test / bench seam: state + the crystal's own frame-cost stats.
window.__beatmoLanding = {
  state: "shown",
  stats: () => new Promise((res) => {
    if (worker) { statsWaiter = res; worker.postMessage({ type: "stats" }); }
    else res(view ? view.handle({ type: "stats" }) : null);
  }),
  videoRequested: () => videoRequested,
  chapters: () => chapters.map((c) => c.label),
  chapterSpans: () => chapters.map((c) => ({ ...c })),
  optedOut,
};
