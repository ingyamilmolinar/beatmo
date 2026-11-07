// A Beatmo-shaped lattice graph grown by local rules from a seed (the visitor's local date and time). In a module Worker
// on an OffscreenCanvas so the game's boot cannot stutter it, else on the main thread; the Crystal class is pure.

// pick: which nodes may sprout; out: outward bias; straight: keep heading; L: step lengths; loop: close on collision;
// maxDeg; retire: failures before "crowded"; mutate: a new colour; branch: non-tip sprouting under tips-only rules.
export const RULES = [
  { name: "Crystal", pick: "any", out: 0.45, straight: 0.2, L: [1, 3], loop: 0.4, maxDeg: 3, retire: 6, mutate: 0.08 },
  { name: "Coral", pick: "tips", out: 0.35, straight: 0.35, L: [1, 2], loop: 0.03, maxDeg: 3, retire: 5, mutate: 0.04, branch: 0.2 },
  { name: "Circuit", pick: "any", out: 0.2, straight: 0.7, L: [2, 5], loop: 0.3, maxDeg: 3, retire: 10, mutate: 0.12 },
  { name: "Weave", pick: "any", out: 0.35, straight: 0.3, L: [1, 2], loop: 0.9, maxDeg: 4, retire: 8, mutate: 0.1 },
];
export const SPEEDS = [0.25, 0.5, 1, 2, 4, 8];
export const TICK_MS = 27.5; // one growth tick at 1×
export const MAX_NODES = 3500; // prune above this
const R0 = 7; // initial hole half-height, lattice units
const COLS = 6; // colour slots (indices into the theme's cols)
const DIRS = [[1, 0], [-1, 0], [0, 1], [0, -1]];

// FNV-1a over a string → uint32 seed.
export function seedFromString(s) {
  let h = 2166136261;
  for (const c of s) { h ^= c.charCodeAt(0); h = Math.imul(h, 16777619) >>> 0; }
  return h >>> 0;
}

const key = (i, j) => i + "," + j;

export class Crystal {
  constructor(seed, ruleIdx = 0, aspect = 16 / 9) {
    this.seed = seed >>> 0;
    this.aspect = aspect; // hole w/h, for the initial ring of seeds
    this.screenAspect = aspect; // viewport w/h, for the outward bias + level
    this.rule = ((ruleIdx % RULES.length) + RULES.length) % RULES.length;
    this.reset();
  }

  rnd() { // mulberry32
    let t = (this.st = (this.st + 0x6d2b79f5) | 0);
    t = Math.imul(t ^ (t >>> 15), 1 | t);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  }

  reset() {
    this.st = this.seed | 0;
    this.nodes = []; // {i,j,col,lv,dx,dy,born,flash,deg,fail}
    this.adj = []; // node id → neighbour ids
    this.edges = []; // [a,b]
    this.occ = new Map(); // "i,j" → id
    this.alive = [];
    this.level = 0;
    this.step = 1;
    this.tick = 0;
    this.hole = [R0 * this.aspect, R0];
    this.bucket = 1;
    this.buckets = new Map();
    for (let k = 0; k < 6; k++) {
      const side = k % 4, a = this.rnd() * 2 - 1;
      let i, j;
      if (side === 0) { i = Math.round(-this.hole[0] - 1); j = Math.round(a * this.hole[1]); }
      if (side === 1) { i = Math.round(this.hole[0] + 1); j = Math.round(a * this.hole[1]); }
      if (side === 2) { i = Math.round(a * this.hole[0]); j = Math.round(-this.hole[1] - 1); }
      if (side === 3) { i = Math.round(a * this.hole[0]); j = Math.round(this.hole[1] + 1); }
      if (!this.occ.has(key(i, j))) this.alive.push(this.add(i, j, (this.rnd() * COLS) | 0, 0, 0, 0));
    }
  }

  setRule(idx) { this.rule = ((idx % RULES.length) + RULES.length) % RULES.length; this.reset(); }

  bkey(i, j) { return Math.floor(i / this.bucket) + "," + Math.floor(j / this.bucket); }
  addToBucket(id) {
    const n = this.nodes[id], k = this.bkey(n.i, n.j);
    let b = this.buckets.get(k);
    if (!b) this.buckets.set(k, (b = []));
    b.push(id);
  }
  rebucket() {
    this.bucket = Math.max(1, this.step);
    this.buckets = new Map();
    for (let id = 0; id < this.nodes.length; id++) this.addToBucket(id);
  }

  add(i, j, col, lv, dx, dy) {
    const id = this.nodes.length;
    this.nodes.push({ i, j, col, lv, dx, dy, born: this.tick, flash: -1e9, deg: 0, fail: 0 });
    this.adj.push([]);
    this.occ.set(key(i, j), id);
    this.addToBucket(id);
    return id;
  }
  link(a, b) {
    this.edges.push([a, b]);
    this.adj[a].push(b); this.adj[b].push(a);
    this.nodes[a].deg++; this.nodes[b].deg++;
  }
  // r < bucket size, so the 3×3 neighbouring buckets cover it.
  free(i, j, r) {
    const bi = Math.floor(i / this.bucket), bj = Math.floor(j / this.bucket);
    for (let x = bi - 1; x <= bi + 1; x++) {
      for (let y = bj - 1; y <= bj + 1; y++) {
        const b = this.buckets.get(x + "," + y);
        if (!b) continue;
        for (const id of b) {
          const n = this.nodes[id];
          if (n.i - i <= r && i - n.i <= r && n.j - j <= r && j - n.j <= r) return false;
        }
      }
    }
    return true;
  }
  inHole(i, j) { return Math.abs(i) < this.hole[0] && Math.abs(j) < this.hole[1]; }

  reach() {
    const r = [1, 1, 1, 1];
    for (const n of this.nodes) {
      if (n.i > r[0]) r[0] = n.i;
      if (-n.i > r[1]) r[1] = -n.i;
      if (n.j > r[2]) r[2] = n.j;
      if (-n.j > r[3]) r[3] = -n.j;
    }
    return r;
  }

  grow() {
    const R = RULES[this.rule];
    this.tick++;
    for (let t = 0; t < 4; t++) {
      if (!this.alive.length) break;
      const k = (this.rnd() * this.alive.length) | 0, aid = this.alive[k], a = this.nodes[aid];
      const retire = () => { this.alive[k] = this.alive[this.alive.length - 1]; this.alive.pop(); };
      const fail = () => { if (++a.fail > R.retire) retire(); };
      if (a.deg >= R.maxDeg || this.inHole(a.i, a.j) || (R.pick === "tips" && a.deg > 1 && this.rnd() > (R.branch || 0))) { retire(); continue; }
      const outH = Math.abs(a.i) / this.screenAspect > Math.abs(a.j) * (this.rnd() * 0.8 + 0.6);
      const r = this.rnd();
      let dx, dy;
      if (r < R.straight && (a.dx || a.dy)) { dx = a.dx; dy = a.dy; }
      else if (r < R.straight + R.out) { if (outH) { dx = Math.sign(a.i) || 1; dy = 0; } else { dx = 0; dy = Math.sign(a.j) || 1; } }
      else { [dx, dy] = DIRS[(this.rnd() * 4) | 0]; }
      const L = this.step * (R.L[0] + ((this.rnd() * (R.L[1] - R.L[0] + 1)) | 0));
      const ni = Math.round((a.i + dx * L) / this.step) * this.step;
      const nj = Math.round((a.j + dy * L) / this.step) * this.step;
      if ((ni === a.i && nj === a.j) || this.inHole(ni, nj)) { fail(); continue; }
      const hit = this.occ.get(key(ni, nj));
      if (hit !== undefined) {
        if (this.rnd() < R.loop && hit !== aid && this.nodes[hit].deg < R.maxDeg + 1 && !this.adj[aid].includes(hit)) this.link(aid, hit);
        fail();
        continue;
      }
      if (!this.free(ni, nj, Math.max(1, Math.floor(this.step * 0.9)))) { fail(); continue; }
      const col = this.rnd() < R.mutate ? (this.rnd() * COLS) | 0 : a.col;
      const b = this.add(ni, nj, col, this.level, dx, dy);
      this.link(aid, b);
      this.alive.push(b);
    }
    // Coarsens as the WEAKEST side widens, so density stays constant through the infinite zoom.
    const rr = this.reach();
    const weak = Math.min(rr[0] / this.screenAspect, rr[1] / this.screenAspect, rr[2], rr[3]);
    const want = Math.max(0, Math.floor(Math.log2(weak / (R0 + 4))));
    if (want > this.level) { this.level = want; this.step = 1 << want; this.rebucket(); }
    if (this.nodes.length > MAX_NODES) this.prune();
  }

  // Detail 4+ levels down (sub-pixel) and nodes under the recording go first, then the oldest: bounded on an endless run.
  prune() {
    const keep = this.nodes.map((n) => n.lv >= this.level - 3 && !this.inHole(n.i, n.j));
    let kept = keep.reduce((a, k) => a + (k ? 1 : 0), 0);
    if (kept > MAX_NODES * 0.85) {
      const order = this.nodes.map((n, i) => i).filter((i) => keep[i]).sort((a, b) => this.nodes[a].born - this.nodes[b].born || a - b);
      for (const i of order) { if (kept <= MAX_NODES * 0.85) break; keep[i] = false; kept--; }
    }
    const remap = new Map(), nn = [];
    this.nodes.forEach((n, i) => { if (keep[i]) { remap.set(i, nn.length); nn.push(n); } });
    this.edges = this.edges.filter(([a, b]) => keep[a] && keep[b]).map(([a, b]) => [remap.get(a), remap.get(b)]);
    this.alive = this.alive.filter((i) => keep[i]).map((i) => remap.get(i));
    this.nodes = nn;
    this.adj = nn.map(() => []);
    for (const n of nn) n.deg = 0;
    for (const [a, b] of this.edges) { this.adj[a].push(b); this.adj[b].push(a); nn[a].deg++; nn[b].deg++; }
    this.occ = new Map();
    nn.forEach((n, i) => this.occ.set(key(n.i, n.j), i));
    this.rebucket();
    this.prunes = (this.prunes || 0) + 1;
  }

  // A visitor tap plants a seed node on the current lattice.
  plant(fi, fj) {
    const i = Math.round(fi / this.step) * this.step, j = Math.round(fj / this.step) * this.step;
    if (this.occ.has(key(i, j)) || this.inHole(i, j)) return -1;
    const id = this.add(i, j, (this.rnd() * COLS) | 0, this.level, 0, 0);
    this.alive.push(id);
    return id;
  }

  // More on a downbeat. Returns [[from,to],…] signals.
  beat(down) {
    const pool = this.alive.length > 8 ? this.alive : null;
    const count = pool ? pool.length : this.nodes.length;
    const out = [];
    if (!count) return out;
    for (let k = 0; k < (down ? 8 : 4); k++) {
      const id = pool ? pool[(this.rnd() * count) | 0] : (this.rnd() * count) | 0;
      const n = this.nodes[id];
      if (!n) continue;
      n.flash = this.tick;
      const nb = this.adj[id];
      if (nb.length) out.push([id, nb[(this.rnd() * nb.length) | 0]]);
    }
    return out;
  }
}

// Auto-fit by the weakest side × the visitor's zoom, no panning; the canvas is an OffscreenCanvas or an element, the
// messages the same.
export function createCrystalView(canvas, opts) {
  const cx = canvas.getContext("2d", { alpha: false });
  const now = () => performance.now();
  const raf = typeof requestAnimationFrame === "function" ? requestAnimationFrame : (cb) => setTimeout(() => cb(now()), 16);
  let W = 1, H = 1, dpr = 1, cssW = 1, cssH = 1;
  let theme = opts.theme || { bg: "#120A1C", acc: "#FFB30A", cols: ["#EE00DD", "#00C8E0", "#3FD67A", "#FFB30A", "#C24FFF", "#FF5C2A"] };
  // The recording's on-screen rect, CSS px: centre + half extents.
  let hole = { cx: 0.5, cy: 0.47, hw: 0.3, hh: 0.17, set: false };
  let autoScale = 0, userZoom = 1, userZoomTarget = 1, speedIdx = 2, paused = false;
  let growAcc = 0, last = now(), mx = -1, my = -1;
  let clock = { playing: false, t: 0, at: now(), bpm: 140, beat0: 0 };
  let lastBeat = -1, signals = [], kickAt = -1e9;
  const crystal = new Crystal(opts.seed, opts.rule || 0, 16 / 9);
  const frameMs = []; // drawing cost samples, for the bench
  let plants = 0; // plant requests received (a visitor tap reached the crystal)
  const flashMs = new Map(); // node id → wall time of its last flash
  const bornMs = new Map(); // node id → wall time of birth (fade-in)
  let prunesSeen = 0;

  function resize(w, h, d) {
    cssW = w; cssH = h; dpr = d;
    W = canvas.width = Math.max(1, Math.round(w * d));
    H = canvas.height = Math.max(1, Math.round(h * d));
    crystal.screenAspect = w / h;
  }
  resize(opts.w, opts.h, opts.dpr || 1);

  function beatNow(t) {
    const ms = 60000 / clock.bpm;
    if (clock.playing) {
      const vt = clock.t + (t - clock.at) / 1000 - clock.beat0;
      return Math.floor((vt * clock.bpm) / 60);
    }
    return Math.floor(t / ms);
  }

  function frame(t) {
    const f0 = now();
    const dt = Math.min(100, t - last);
    last = t;
    if (!paused) {
      growAcc += dt * SPEEDS[speedIdx];
      let n = 0;
      while (growAcc > TICK_MS && n++ < 40) {
        const before = crystal.nodes.length;
        crystal.grow();
        for (let id = before; id < crystal.nodes.length; id++) bornMs.set(id, t);
        growAcc -= TICK_MS;
      }
      if (n >= 40) growAcc = 0;
    }
    const r = crystal.reach();
    const ox = hole.cx * W, oy = hole.cy * H;
    const hw = (W - ox) * 0.92, hwl = ox * 0.92, hb = (H - oy) * 0.92, ht = oy * 0.92;
    const ts = Math.max(hw / r[0], hwl / r[1], hb / r[2], ht / r[3]);
    autoScale = autoScale ? autoScale + (ts - autoScale) * Math.min(1, dt / 1200) : ts;
    userZoom += (userZoomTarget - userZoom) * Math.min(1, dt / 120);
    const scale = autoScale * userZoom;
    // Growth sees the AUTO camera only: the visitor's zoom never changes the shape.
    if (hole.set) crystal.hole = [(hole.hw * W) / autoScale, (hole.hh * H) / autoScale];
    if (crystal.prunes !== prunesSeen) { prunesSeen = crystal.prunes; flashMs.clear(); bornMs.clear(); signals = []; } // ids were remapped

    const b = beatNow(t);
    if (b !== lastBeat && !paused) {
      lastBeat = b;
      const down = ((b % 4) + 4) % 4 === 0;
      kickAt = t;
      for (const [a, z] of crystal.beat(down)) {
        flashMs.set(a, t);
        signals.push({ a, b: z, t0: t });
      }
    }

    const T = theme;
    cx.globalAlpha = 1;
    cx.fillStyle = T.bg;
    cx.fillRect(0, 0, W, H);
    const px = (i) => ox + i * scale, py = (j) => oy + j * scale;
    const level = crystal.level;

    // Lattice dots at the levels that are a comfortable size on screen.
    for (let lv = Math.max(0, level - 3); lv <= level + 2; lv++) {
      const g = (1 << lv) * scale;
      if (g < 12 * dpr || g > 200 * dpr) continue;
      const a = Math.min(1, (g / dpr - 12) / 14);
      cx.fillStyle = `rgba(255,255,255,${0.07 * a})`;
      cx.beginPath();
      const x0 = ((ox % g) + g) % g, y0 = ((oy % g) + g) % g;
      for (let x = x0; x < W; x += g) for (let y = y0; y < H; y += g) cx.rect(x - dpr, y - dpr, 2 * dpr, 2 * dpr);
      cx.fill();
    }

    const lodA = (lv) => {
      const v = ((1 << lv) * scale / dpr - 2) / 6;
      return lv >= level - 1 ? Math.max(0.55, Math.min(1, v)) : Math.max(0, Math.min(1, v));
    };
    const pad = 20 * dpr;
    const vis = (x, y) => x > -pad && y > -pad && x < W + pad && y < H + pad;

    // Edges, batched per (colour, level): one stroke per batch.
    const nodes = crystal.nodes;
    const batches = new Map();
    for (const [a, z] of crystal.edges) {
      const na = nodes[a], nb = nodes[z];
      const lv = Math.min(na.lv, nb.lv);
      if (lodA(lv) <= 0) continue;
      const x1 = px(na.i), y1 = py(na.j), x2 = px(nb.i), y2 = py(nb.j);
      if (!vis(x1, y1) && !vis(x2, y2)) continue;
      const k = nb.col * 64 + lv;
      let p = batches.get(k);
      if (!p) batches.set(k, (p = []));
      p.push(x1, y1, x2, y2);
    }
    cx.lineWidth = 1.3 * dpr;
    for (const [k, p] of batches) {
      cx.globalAlpha = 0.4 * lodA(k % 64);
      cx.strokeStyle = T.cols[(k / 64) | 0];
      cx.beginPath();
      for (let q = 0; q < p.length; q += 4) { cx.moveTo(p[q], p[q + 1]); cx.lineTo(p[q + 2], p[q + 1]); cx.lineTo(p[q + 2], p[q + 3]); }
      cx.stroke();
    }

    // Plain nodes batched per (colour, level); lit ones drawn individually with a glow.
    const nb2 = new Map(), lit = [];
    for (let id = 0; id < nodes.length; id++) {
      const n = nodes[id];
      const A = lodA(n.lv);
      if (A <= 0) continue;
      const x = px(n.i), y = py(n.j);
      if (!vis(x, y)) continue;
      const near = mx >= 0 ? Math.max(0, 1 - Math.hypot(x - mx, y - my) / (180 * dpr)) : 0;
      const fl = flashMs.get(id);
      const f = fl === undefined ? 0 : Math.max(0, 1 - (t - fl) / 350);
      const bo = bornMs.get(id);
      const g = bo === undefined ? 1 : Math.min(1, (t - bo) / 300);
      const s = Math.min(10 * dpr, Math.max(1.5 * dpr, (1 << n.lv) * scale * 0.35)) * (1 + 0.6 * f + 0.4 * near) * g;
      if (f > 0 || near > 0.05) { lit.push([x, y, s, A, f, near, n.col]); continue; }
      const k = n.col * 64 + n.lv;
      let p = nb2.get(k);
      if (!p) nb2.set(k, (p = []));
      p.push(x - s / 2, y - s / 2, s);
    }
    for (const [k, p] of nb2) {
      cx.globalAlpha = 0.75 * lodA(k % 64);
      cx.fillStyle = T.cols[(k / 64) | 0];
      cx.beginPath();
      for (let q = 0; q < p.length; q += 3) cx.rect(p[q], p[q + 1], p[q + 2], p[q + 2]);
      cx.fill();
    }
    for (const [x, y, s, A, f, near, col] of lit) {
      cx.globalAlpha = A * (0.75 + 0.25 * Math.max(f, near));
      cx.fillStyle = f > 0 ? "#FDF2FF" : T.cols[col];
      cx.shadowColor = T.cols[col];
      cx.shadowBlur = (14 * f + 10 * near) * dpr;
      cx.fillRect(x - s / 2, y - s / 2, s, s);
    }
    cx.shadowBlur = 0;

    // Signals travel one beat along the L-shaped edge.
    const beatMs = 60000 / clock.bpm;
    signals = signals.filter((s) => t - s.t0 < beatMs && nodes[s.a] && nodes[s.b]);
    for (const s of signals) {
      const u = (t - s.t0) / beatMs, a = nodes[s.a], z = nodes[s.b];
      const seg1 = Math.abs(z.i - a.i), seg2 = Math.abs(z.j - a.j), tot = seg1 + seg2 || 1;
      let x, y;
      if (u * tot < seg1) { x = a.i + (z.i - a.i) * ((u * tot) / (seg1 || 1)); y = a.j; }
      else { x = z.i; y = a.j + (z.j - a.j) * ((u * tot - seg1) / (seg2 || 1)); }
      cx.globalAlpha = lodA(Math.min(a.lv, z.lv));
      cx.fillStyle = "#FDF2FF";
      cx.shadowColor = T.cols[z.col];
      cx.shadowBlur = 10 * dpr;
      cx.beginPath();
      cx.arc(px(x), py(y), 2.6 * dpr, 0, 7);
      cx.fill();
    }
    cx.shadowBlur = 0;

    // Drawn here, behind the video, so a busy main thread cannot freeze it.
    if (hole.set) {
      const kick = Math.exp(-((t - kickAt) / beatMs) * 5);
      cx.globalAlpha = 1;
      cx.shadowColor = T.cols[0];
      cx.shadowBlur = (20 + 30 * kick) * dpr;
      cx.fillStyle = T.bg;
      cx.fillRect(ox - hole.hw * W, oy - hole.hh * H, 2 * hole.hw * W, 2 * hole.hh * H);
      cx.shadowBlur = 0;
    }
    cx.globalAlpha = 1;
    frameMs.push(now() - f0);
    if (frameMs.length > 600) frameMs.shift();
    raf(frame);
  }

  function handle(m) {
    switch (m.type) {
      case "resize": resize(m.w, m.h, m.dpr); break;
      case "hole": hole = { cx: m.cx / cssW, cy: m.cy / cssH, hw: m.hw / cssW, hh: m.hh / cssH, set: true }; break;
      case "zoom": userZoomTarget = Math.max(0.35, Math.min(6, userZoomTarget * m.factor)); break;
      case "zoomReset": userZoomTarget = 1; break;
      case "speed": speedIdx = Math.max(0, Math.min(SPEEDS.length - 1, m.idx)); break;
      case "rule": crystal.setRule(m.idx); autoScale = 0; flashMs.clear(); bornMs.clear(); signals = []; break;
      case "theme": theme = m.theme; break;
      case "clock": clock = { playing: m.playing, t: m.t, at: now(), bpm: m.bpm || clock.bpm, beat0: m.beat0 || 0 }; break;
      case "pause": paused = m.paused; break;
      case "pointer": mx = m.x * dpr; my = m.y * dpr; break;
      case "plant": {
        plants++;
        const s = autoScale * userZoom;
        const id = crystal.plant((m.x * dpr - hole.cx * W) / s, (m.y * dpr - hole.cy * H) / s);
        if (id >= 0) bornMs.set(id, now());
        break;
      }
      case "stats": {
        const xs = frameMs.slice().sort((a, b) => a - b);
        return { frames: xs.length, p50: xs[Math.floor(xs.length * 0.5)] || 0, p95: xs[Math.floor(xs.length * 0.95)] || 0, nodes: crystal.nodes.length, level: crystal.level, plants };
      }
    }
    return null;
  }
  raf(frame);
  return { handle };
}

// Worker glue: the page transfers an OffscreenCanvas in the first message.
if (typeof WorkerGlobalScope !== "undefined" && self instanceof WorkerGlobalScope) {
  let view = null;
  self.onmessage = (e) => {
    const m = e.data;
    if (m.type === "init") { view = createCrystalView(m.canvas, m); return; }
    if (!view) return;
    const r = view.handle(m);
    if (r) self.postMessage({ type: "stats", stats: r });
  };
}
