// landing_crystal.browser.test.js — pure Node (no browser): the landing
// crystal's growth rules (src/js/landing_crystal.js). Determinism per seed,
// divergence across seeds, the graph invariants every rule set must keep, and
// a bounded node count over a long run; then the VIEW the worker runs, driven
// through its message protocol (the same messages landing.js posts) on a fake
// canvas and a hand-cranked animation clock. Spec:
// docs/superpowers/specs/2026-10-23-landing-page-design.md ("The crystal").
import assert from "node:assert/strict";
import { Crystal, RULES, MAX_NODES, seedFromString, createCrystalView } from "./landing_crystal.js";

const snapshot = (c) => JSON.stringify([c.nodes.map((n) => [n.i, n.j, n.col, n.lv]), c.edges]);
const run = (seed, rule, ticks) => { const c = new Crystal(seed, rule); for (let k = 0; k < ticks; k++) c.grow(); return c; };

// Same seed + rules → byte-identical graph; different seeds diverge.
const seedA = seedFromString("2026-10-23 21:41"), seedB = seedFromString("2026-10-23 21:42");
for (let r = 0; r < RULES.length; r++) {
  assert.equal(snapshot(run(seedA, r, 800)), snapshot(run(seedA, r, 800)), `${RULES[r].name}: not deterministic`);
  assert.notEqual(snapshot(run(seedA, r, 800)), snapshot(run(seedB, r, 800)), `${RULES[r].name}: seeds do not diverge`);
}

// Invariants after a long run, for every rule set (and through prunes).
for (let r = 0; r < RULES.length; r++) {
  const c = new Crystal(seedA, r);
  let maxNodes = 0;
  for (let k = 0; k < 20000; k++) {
    c.grow();
    maxNodes = Math.max(maxNodes, c.nodes.length);
    if (k % 2500 === 0 || k === 19999) {
      const name = RULES[r].name;
      const seen = new Set();
      for (const n of c.nodes) {
        const key = n.i + "," + n.j;
        assert.ok(!seen.has(key), `${name}: two nodes on one cell ${key}`);
        seen.add(key);
      }
      for (const [a, b] of c.edges) {
        assert.ok(c.nodes[a] && c.nodes[b], `${name}: edge to a missing node`);
        assert.notEqual(a, b, `${name}: self edge`);
      }
      // Degrees and adjacency agree with the edge list.
      const deg = new Array(c.nodes.length).fill(0);
      for (const [a, b] of c.edges) { deg[a]++; deg[b]++; }
      c.nodes.forEach((n, i) => assert.equal(n.deg, deg[i], `${name}: degree drift at node ${i}`));
    }
  }
  assert.ok(maxNodes <= MAX_NODES + 4, `${RULES[r].name}: node count ${maxNodes} unbounded`);
  assert.ok(c.level > 0, `${RULES[r].name}: never levelled up (level ${c.level}, ${c.nodes.length} nodes)`);
}

// Births never land in the hole as it was when they were born.
{
  const c = new Crystal(seedA, 0);
  for (let k = 0; k < 3000; k++) {
    const before = c.nodes.length;
    c.grow();
    for (let id = before; id < c.nodes.length; id++) assert.ok(!c.inHole(c.nodes[id].i, c.nodes[id].j), "birth inside the hole");
  }
}

// Growth speed: a tick must stay far below one frame on a slow phone (this
// machine × ~4). Measured, reported, and gated loosely.
{
  const c = new Crystal(seedA, 0);
  const t0 = performance.now();
  for (let k = 0; k < 20000; k++) c.grow();
  const per = (performance.now() - t0) / 20000;
  console.log(`grow(): ${(per * 1000).toFixed(1)} µs/tick over 20k ticks, ${c.nodes.length} nodes, level ${c.level}`);
  assert.ok(per < 0.5, `grow() ${per.toFixed(3)} ms/tick`);
}
// ---------------------------------------------------------------------------
// The view: messages in, frames out.
// ---------------------------------------------------------------------------
const rafQueue = [];
globalThis.requestAnimationFrame = (cb) => rafQueue.push(cb);
let clockMs = performance.now();
function frames(n, stepMs = 16) {
  for (let k = 0; k < n; k++) {
    clockMs += stepMs;
    for (const cb of rafQueue.splice(0)) cb(clockMs);
  }
}
function fakeCanvas() {
  const log = { fillStyles: [], calls: 0 };
  const props = {};
  const ctx = new Proxy(props, {
    get(o, k) { if (k in o) return o[k]; return () => { log.calls++; }; },
    set(o, k, v) { if (k === "fillStyle") log.fillStyles.push(v); o[k] = v; return true; },
  });
  return { canvas: { width: 0, height: 0, getContext: () => ctx }, log };
}
const THEME = { bg: "#120A1C", acc: "#FFB30A", cols: ["#EE00DD", "#00C8E0", "#3FD67A", "#FFB30A", "#C24FFF", "#FF5C2A"] };
function view(seed = seedA, extra = {}) {
  const { canvas, log } = fakeCanvas();
  rafQueue.length = 0;
  const v = createCrystalView(canvas, { w: 800, h: 450, dpr: 2, seed, rule: 0, theme: THEME, ...extra });
  v.handle({ type: "hole", cx: 400, cy: 211, hw: 232, hh: 130 });
  return { v, canvas, log, nodes: () => v.handle({ type: "stats" }).nodes };
}

// Size: the backing store follows the window × the device pixel ratio.
{
  const { v, canvas } = view();
  assert.equal(canvas.width, 1600);
  assert.equal(canvas.height, 900);
  v.handle({ type: "resize", w: 390, h: 844, dpr: 1 });
  assert.equal(canvas.width, 390);
  assert.equal(canvas.height, 844);
}

// It grows on its own; "pause" freezes the growth, un-pausing resumes it.
{
  const { v, nodes } = view();
  frames(60);
  const a = nodes();
  assert.ok(a > 1, `the crystal did not grow in 60 frames (${a} nodes)`);
  v.handle({ type: "pause", paused: true });
  frames(120);
  assert.equal(nodes(), a, "the crystal grew while paused");
  v.handle({ type: "pause", paused: false });
  frames(60);
  assert.ok(nodes() > a, "the crystal did not resume after un-pausing");
}

// Two views on one hand-cranked clock, each with its own animation queue.
function ownQueue(x) { return { ...x, q: rafQueue.splice(0) }; }
function crankBoth(views, n, each) {
  for (let k = 0; k < n; k++) {
    clockMs += 16;
    if (each) each(k);
    for (const x of views) { for (const cb of x.q.splice(0)) cb(clockMs); x.q.push(...rafQueue.splice(0)); }
  }
}

// Speed: the fastest setting grows more than the slowest over the same time.
{
  const a = ownQueue(view()), b = ownQueue(view());
  a.v.handle({ type: "speed", idx: 0 });
  b.v.handle({ type: "speed", idx: 5 });
  crankBoth([a, b], 90);
  assert.ok(b.nodes() > a.nodes() * 2, `speed 8x grew ${b.nodes()} nodes, 0.25x ${a.nodes()}`);
}

// Planting: a click outside the recording adds a node there (once — the cell
// is then taken); a click on the recording adds nothing.
{
  const { v, nodes } = view();
  v.handle({ type: "pause", paused: true });
  frames(5); // the camera needs a frame to know its scale
  const n0 = nodes();
  v.handle({ type: "plant", x: 30, y: 30 });
  assert.equal(nodes(), n0 + 1, "a plant outside the recording added no node");
  v.handle({ type: "plant", x: 30, y: 30 });
  assert.equal(nodes(), n0 + 1, "planting on a taken cell added a node");
  v.handle({ type: "plant", x: 400, y: 211 });
  assert.equal(nodes(), n0 + 1, "a plant on the recording added a node");
  assert.equal(v.handle({ type: "stats" }).plants, 3, "stats.plants does not count every plant request");
}

// Rules: switching restarts the growth from the seed under the new rules.
{
  const { v, nodes } = view();
  frames(200);
  const grown = nodes();
  v.handle({ type: "rule", idx: 2 });
  assert.ok(nodes() < grown, `switching rules kept ${nodes()} of ${grown} nodes`);
  assert.equal(v.handle({ type: "stats" }).level, 0);
}

// Theme: every frame starts by painting the theme's background.
{
  const { v, log } = view();
  v.handle({ type: "theme", theme: { ...THEME, bg: "#010203" } });
  log.fillStyles.length = 0;
  frames(1);
  assert.equal(log.fillStyles[0], "#010203", `the frame began with ${log.fillStyles[0]}`);
}

// Zoom, pointer, clock and zoomReset are accepted and keep it drawing.
{
  const { v, log } = view();
  for (const m of [{ type: "zoom", factor: 3 }, { type: "zoom", factor: 100 }, { type: "pointer", x: 100, y: 100 }, { type: "clock", playing: true, t: 1, bpm: 140, beat0: 0.5 }, { type: "zoomReset" }]) {
    assert.equal(v.handle(m), null, `${m.type} returned a value`);
  }
  const before = log.calls;
  frames(30);
  assert.ok(log.calls > before, "the view stopped drawing after camera / clock messages");
}

// Stats: one sample per drawn frame (capped at the last 600), ordered quantiles.
{
  const { v } = view();
  frames(40);
  const st = v.handle({ type: "stats" });
  assert.equal(st.frames, 40);
  assert.ok(st.p95 >= st.p50 && st.p50 >= 0, JSON.stringify(st));
  frames(700);
  assert.equal(v.handle({ type: "stats" }).frames, 600);
}

// Same seed + same messages at the same times → the same crystal.
{
  const x = ownQueue(view(seedB)), y = ownQueue(view(seedB));
  crankBoth([x, y], 120, (k) => { if (k === 30) for (const z of [x, y]) z.v.handle({ type: "plant", x: 20, y: 400 }); });
  assert.equal(x.nodes(), y.nodes(), "the same seed and inputs grew different crystals");
}
console.log("landing_crystal: OK");
