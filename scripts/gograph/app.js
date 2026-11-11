(function () {
  const G = JSON.parse(document.getElementById("model").textContent);
  // Normalize omitempty-dropped slices so drill code can assume arrays.
  G.edges = G.edges || [];
  G.packages = G.packages || [];
  G.packages.forEach(p => {
    p.types = p.types || [];
    p.funcs = p.funcs || [];
    p.imports = p.imports || [];
    p.types.forEach(t => { t.methods = t.methods || []; });
    p.kind = "pkg";
  });

  const svg = document.getElementById("graph");
  const panel = document.getElementById("panel");
  const filter = document.getElementById("filter");
  const crumb = document.getElementById("crumb");
  const note = document.getElementById("note");
  document.getElementById("meta").textContent =
    G.packages.length + " pkgs · " + G.edges.length + " call edges · " +
    G.config.goos + (G.config.tags ? " tags=" + G.config.tags : "");

  // Legend: node size ∝ LOC; color = complexity level; dashed outline = a type.
  (function buildLegend() {
    const header = document.querySelector("header");
    if (!header) return;
    const chip = (color, label, dash) =>
      '<span style="display:inline-block;width:10px;height:10px;background:' + color +
      ';border:' + (dash ? "1px dashed #c9d1d9" : "0") +
      ';border-radius:2px;margin:0 4px 0 10px;vertical-align:middle"></span>' + label;
    const legend = document.createElement("span");
    legend.style.marginLeft = "auto";
    legend.style.fontSize = "11px";
    legend.style.opacity = "0.9";
    legend.innerHTML = "size ∝ LOC · " + chip("#3fb950", "simple") + chip("#d4a72c", "moderate") +
      chip("#f0883e", "complex") + chip("#f85149", "very complex") + chip("#484f58", "type (dashed)", true);
    header.appendChild(legend);
  })();

  const NS = "http://www.w3.org/2000/svg";
  function el(name, attrs) {
    const e = document.createElementNS(NS, name);
    for (const k in attrs) e.setAttribute(k, attrs[k]);
    return e;
  }

  const W = svg.clientWidth || 900, H = svg.clientHeight || 600;
  let view = { x: 0, y: 0, k: 1 };

  // ---- Composable size + complexity (identical treatment for pkg / type / func) ----
  const MIN_R = 9, MAX_R = 58;
  function locOf(it) { return it.loc || 1; }
  function complexityColor(cc) {
    if (cc <= 4) return "#3fb950";   // simple
    if (cc <= 9) return "#d4a72c";   // moderate
    if (cc <= 15) return "#f0883e";  // complex
    return "#f85149";                // very complex
  }
  // Complexity composes over the hierarchy: a function's is its own cyclomatic
  // complexity; a type's is the max over its methods; a package's is the max
  // over its functions and its types' methods.
  function complexityOf(it) {
    if (it.kind === "func") return it.cyclo || 0;
    if (it.kind === "type") {
      let m = 0;
      (it.methods || []).forEach(x => { if ((x.cyclo || 0) > m) m = x.cyclo || 0; });
      return m;
    }
    let m = 0;
    it.funcs.forEach(f => { if ((f.cyclo || 0) > m) m = f.cyclo || 0; });
    it.types.forEach(t => t.methods.forEach(x => { if ((x.cyclo || 0) > m) m = x.cyclo || 0; }));
    return m;
  }
  // Radius is proportional-by-AREA to LOC, scaled to the biggest node in the
  // CURRENT scene so every level fills the canvas well (radius ∝ sqrt(LOC)).
  function radiusFor(items) {
    const mx = Math.max(1, ...items.map(locOf));
    return it => MIN_R + (MAX_R - MIN_R) * Math.sqrt(locOf(it) / mx);
  }
  function shortName(path) { const s = path.split("/"); return s.slice(-2).join("/"); }
  function fitLabel(s, r) { const max = Math.max(3, Math.floor(r / 3.0)); return s.length > max ? s.slice(0, max - 1) + "…" : s; }

  // ---- deterministic RNG + shared collision/repulsion pass ----
  let seed = 1234567;
  function rnd() { seed = (seed * 1103515245 + 12345) & 0x7fffffff; return seed / 0x7fffffff; }
  function separateNodes(nodes, collideOnly, margin, repK) {
    const N = nodes.length;
    for (let i = 0; i < N; i++) {
      for (let j = i + 1; j < N; j++) {
        const a = nodes[i], b = nodes[j];
        let dx = b.x - a.x, dy = b.y - a.y, d2 = dx * dx + dy * dy;
        if (d2 < 1e-6) { dx = rnd() - 0.5; dy = rnd() - 0.5; d2 = dx * dx + dy * dy + 1e-6; }
        const dist = Math.sqrt(d2), ux = dx / dist, uy = dy / dist;
        if (!collideOnly) { const rep = repK / d2; a.x -= ux * rep; a.y -= uy * rep; b.x += ux * rep; b.y += uy * rep; }
        const md = a.r + b.r + margin;
        if (dist < md) { const pu = (md - dist) / 2; a.x -= ux * pu; a.y -= uy * pu; b.x += ux * pu; b.y += uy * pu; }
      }
    }
  }

  // ---- package layout (force-directed; call edges matter). Computed once. ----
  const pkgRadius = radiusFor(G.packages);
  const pkgPos = (function layoutPackages() {
    const nodes = G.packages.map(p => ({ path: p.path, r: pkgRadius(p), x: 0, y: 0 }));
    const idx = {}; nodes.forEach((n, i) => { idx[n.path] = i; });
    nodes.forEach((nd, i) => {
      const a = i * 2.399963229, rad = 40 + 34 * Math.sqrt(i);
      nd.x = W / 2 + Math.cos(a) * rad + (rnd() - 0.5) * 6;
      nd.y = H / 2 + Math.sin(a) * rad + (rnd() - 0.5) * 6;
    });
    for (let it = 0; it < 300; it++) {
      separateNodes(nodes, false, 16, 2600);
      G.edges.forEach(e => {
        const a = nodes[idx[e.from]], b = nodes[idx[e.to]];
        if (!a || !b) return;
        const w = 0.0012 * Math.min(e.calls, 20), dx = b.x - a.x, dy = b.y - a.y;
        a.x += dx * w; a.y += dy * w; b.x -= dx * w; b.y -= dy * w;
      });
      for (const nd of nodes) { nd.x += (W / 2 - nd.x) * 0.002; nd.y += (H / 2 - nd.y) * 0.002; }
    }
    for (let it = 0; it < 60; it++) separateNodes(nodes, true, 16, 0);
    const pos = {}; nodes.forEach(nd => { pos[nd.path] = { x: nd.x, y: nd.y }; });
    return pos;
  })();

  // ---- member layout (packing; members have no edges among them) ----
  function layoutMembers(items, rf) {
    const nodes = items.map(it => ({ it, r: rf(it), x: 0, y: 0 }));
    const N = nodes.length;
    if (!N) return nodes;
    const meanR = nodes.reduce((s, n) => s + n.r, 0) / N;
    const c = (2 * meanR + 12) * 0.62;
    // seed on a size-sorted golden-angle spiral: biggest near the center
    const order = nodes.map((_, i) => i).sort((a, b) => nodes[b].r - nodes[a].r);
    order.forEach((ni, rank) => {
      const nd = nodes[ni], a = rank * 2.399963229, rad = c * Math.sqrt(rank);
      nd.x = W / 2 + Math.cos(a) * rad + (rnd() - 0.5) * 4;
      nd.y = H / 2 + Math.sin(a) * rad + (rnd() - 0.5) * 4;
    });
    const iters = N > 250 ? 60 : 130;
    for (let it = 0; it < iters; it++) {
      separateNodes(nodes, false, 12, 1400);
      for (const nd of nodes) { nd.x += (W / 2 - nd.x) * 0.003; nd.y += (H / 2 - nd.y) * 0.003; }
    }
    for (let it = 0; it < 40; it++) separateNodes(nodes, true, 12, 0);
    return nodes;
  }

  // ---- focus state ----
  // focus: null (packages) | {kind:'pkg', p} | {kind:'type', p, t}
  let focus = null;
  let selected = null;     // selected leaf ref, for node highlight
  let selectedName = null; // its member name, for edge highlight
  const MAX_NODES = 160;
  const MAX_EDGES = 1600;  // cap drawn intra-package edges to keep it readable

  // Directed-arrow marker defs (re-added each render since the svg is cleared).
  function defsArrow() {
    const defs = el("defs", {});
    [["arrow", "#8b949e"], ["arrowhi", "#58a6ff"]].forEach(([id, col]) => {
      const m = el("marker", { id: id, viewBox: "0 0 10 10", refX: "9", refY: "5", markerWidth: "6", markerHeight: "6", orient: "auto-start-reverse" });
      m.appendChild(el("path", { d: "M0,0 L10,5 L0,10 z", fill: col }));
      defs.appendChild(m);
    });
    svg.appendChild(defs);
  }
  // Directed edge caller→callee, trimmed to node radii so the arrow sits on the
  // target's boundary. Width grows with the call count.
  function drawMemberEdge(a, b, calls, hi) {
    let dx = b.x - a.x, dy = b.y - a.y;
    const d = Math.hypot(dx, dy) || 1, ux = dx / d, uy = dy / d;
    const x1 = a.x + ux * (a.r + 1), y1 = a.y + uy * (a.r + 1);
    const x2 = b.x - ux * (b.r + 5), y2 = b.y - uy * (b.r + 5);
    svg.appendChild(el("path", {
      class: "medge" + (hi ? " medge-hi" : ""),
      d: `M${x1},${y1} L${x2},${y2}`,
      "stroke-width": Math.max(0.6, Math.min(4, Math.log2(calls + 1))),
      "marker-end": hi ? "url(#arrowhi)" : "url(#arrow)",
    }));
  }

  function memberItems() {
    if (focus === null) return null;
    if (focus.kind === "pkg") {
      const its = [];
      focus.p.funcs.forEach(f => its.push({ kind: "func", name: f.name, loc: f.loc, cyclo: f.cyclo, ref: f }));
      focus.p.types.forEach(t => its.push({ kind: "type", name: t.name, loc: t.loc, methods: t.methods, ref: t }));
      return its;
    }
    return focus.t.methods.map(m => ({ kind: "func", name: m.name, loc: m.loc, cyclo: m.cyclo, ref: m }));
  }

  function applyView() {
    svg.setAttribute("viewBox", `${-view.x / view.k} ${-view.y / view.k} ${W / view.k} ${H / view.k}`);
  }
  function fitTo(draw) {
    if (!draw.length) return;
    let a = Infinity, b = Infinity, c = -Infinity, d = -Infinity;
    draw.forEach(n => { a = Math.min(a, n.x - n.r); b = Math.min(b, n.y - n.r); c = Math.max(c, n.x + n.r); d = Math.max(d, n.y + n.r); });
    const pad = 40, bw = c - a, bh = d - b;
    const k = Math.max(0.2, Math.min(4, Math.min(W / (bw + pad * 2), H / (bh + pad * 2))));
    view.k = k; view.x = W / 2 - (a + bw / 2) * k; view.y = H / 2 - (b + bh / 2) * k;
  }

  function drawNode(x, y, r, col, label, tip, dim, onClick, isType, sel) {
    const g = el("g", { class: "node" + (dim ? " dim" : "") });
    const circ = el("circle", { cx: x, cy: y, r: r, fill: col, "fill-opacity": 0.42, stroke: sel ? "#ffffff" : col, "stroke-width": sel ? 2.5 : 1.5 });
    if (isType) circ.setAttribute("stroke-dasharray", "5 3"); // types visually distinct
    g.appendChild(circ);
    const title = el("title", {}); title.textContent = tip; g.appendChild(title);
    if (r >= 11) {
      const t = el("text", { x: x, y: y + 4, "text-anchor": "middle", "font-size": Math.max(8, Math.min(12, r * 0.45)) });
      t.textContent = fitLabel(label, r);
      g.appendChild(t);
    }
    g.addEventListener("click", ev => { ev.stopPropagation(); onClick(); });
    svg.appendChild(g);
  }

  function render(refit) {
    while (svg.firstChild) svg.removeChild(svg.firstChild);
    const q = filter.value.trim().toLowerCase();
    note.textContent = "";

    if (focus === null) {
      // Package overview: all packages shown, call edges drawn, non-matches dimmed.
      G.edges.forEach(e => {
        const a = pkgPos[e.from], b = pkgPos[e.to];
        if (!a || !b) return;
        svg.appendChild(el("path", { class: "edge call", d: `M${a.x},${a.y} L${b.x},${b.y}`, "stroke-width": Math.max(0.5, Math.min(6, Math.log2(e.calls + 1))) }));
      });
      const draw = [];
      G.packages.forEach(p => {
        const c = pkgPos[p.path], r = pkgRadius(p), col = complexityColor(complexityOf(p));
        const dim = q && !p.path.toLowerCase().includes(q);
        const tip = p.path + " — LOC " + p.loc + ", " + p.types.length + " types, " + p.funcs.length + " funcs, max cc " + complexityOf(p);
        drawNode(c.x, c.y, r, col, shortName(p.path), tip, dim, () => drillPkg(p), false, false);
        draw.push({ x: c.x, y: c.y, r: r });
      });
      if (refit) fitTo(draw);
    } else {
      // Member view: types & functions (or a type's methods) as nodes.
      let items = memberItems();
      const totalAll = items.length;
      if (q) items = items.filter(it => it.name.toLowerCase().includes(q));
      const matchCount = items.length;
      let capped = false;
      if (matchCount > MAX_NODES) { items = items.slice().sort((a, b) => locOf(b) - locOf(a)).slice(0, MAX_NODES); capped = true; }
      const rf = radiusFor(items.length ? items : [{ loc: 1 }]);
      const laid = layoutMembers(items, rf);

      // Intra-package caller→callee edges between visible member nodes (types &
      // functions). Only in the package view — a type's methods view has none.
      if (focus.kind === "pkg" && focus.p.memberEdges && focus.p.memberEdges.length) {
        const posByName = {};
        laid.forEach(n => { posByName[n.it.name] = n; });
        defsArrow();
        let drawn = 0;
        for (const me of focus.p.memberEdges) {
          if (drawn >= MAX_EDGES) break;
          const a = posByName[me.from], b = posByName[me.to];
          if (!a || !b) continue;
          const hi = selectedName && (me.from === selectedName || me.to === selectedName);
          drawMemberEdge(a, b, me.calls, hi);
          drawn++;
        }
      }

      laid.forEach(n => {
        const it = n.it;
        const col = complexityColor(complexityOf(it));
        const sel = selected === it.ref;
        const tip = it.kind === "type"
          ? it.name + " (type) — LOC " + it.loc + ", " + (it.methods || []).length + " methods, max cc " + complexityOf(it)
          : it.name + "() — LOC " + it.loc + ", cc " + (it.cyclo || 0) + ", call-depth " + (it.ref.callDepth || 0);
        drawNode(n.x, n.y, n.r, col, it.name, tip, false,
          () => (it.kind === "type" ? drillType(it.ref) : selectFunc(it)), it.kind === "type", sel);
      });
      if (!totalAll) note.textContent = "No exported members in this " + (focus.kind === "type" ? "type" : "package") + ".";
      else if (capped) note.textContent = "Showing the largest " + MAX_NODES + " of " + matchCount + " members — type in the filter to narrow.";
      else if (q) note.textContent = matchCount + " of " + totalAll + " members match.";
      if (refit) fitTo(laid);
    }
    renderCrumb();
    applyView();
  }

  // ---- navigation ----
  function drillPkg(p) { focus = { kind: "pkg", p: p }; selected = selectedName = null; filter.value = ""; showPackage(p); render(true); }
  function drillType(t) { focus = { kind: "type", p: focus.p, t: t }; selected = selectedName = null; filter.value = ""; showType(t); render(true); }
  function selectFunc(it) { selected = it.ref; selectedName = it.name; showFunc(it.ref); render(false); }
  function goPackages() { focus = null; selected = selectedName = null; filter.value = ""; defaultPanel(); render(true); }
  function goPkgLevel() { const p = focus.p; focus = { kind: "pkg", p: p }; selected = selectedName = null; filter.value = ""; showPackage(p); render(true); }

  function renderCrumb() {
    crumb.innerHTML = "";
    const seg = (text, cls, fn) => { const s = document.createElement("span"); s.className = cls; s.textContent = text; if (fn) s.onclick = fn; return s; };
    const sep = () => { const s = document.createElement("span"); s.className = "sep"; s.textContent = "/"; return s; };
    if (focus === null) { crumb.appendChild(seg("Packages", "cur")); return; }
    crumb.appendChild(seg("Packages", "seg", goPackages));
    crumb.appendChild(sep());
    if (focus.kind === "pkg") { crumb.appendChild(seg(shortName(focus.p.path), "cur")); }
    else {
      crumb.appendChild(seg(shortName(focus.p.path), "seg", goPkgLevel));
      crumb.appendChild(sep());
      crumb.appendChild(seg(focus.t.name, "cur"));
    }
  }

  // ---- side panel ----
  function defaultPanel() {
    panel.innerHTML = "<h2>Packages</h2><div>Click a package to open its types &amp; functions as nodes. " +
      "Click a type to open its methods. Scroll to zoom (at the cursor), drag to pan, filter to find any member.</div>";
  }
  function showPackage(p) {
    panel.innerHTML = "<h2>" + p.path + "</h2>" +
      row("LOC", p.loc) + row("Files", p.files) +
      row("Types", p.types.length) + row("Funcs", p.funcs.length) +
      row("Imports", p.imports.length) + rowHot("Max complexity", complexityOf(p), 10);
    const callers = G.edges.filter(e => e.to === p.path).map(e => e.from);
    const callees = G.edges.filter(e => e.from === p.path).map(e => e.to);
    panel.innerHTML += "<h2>Calls out</h2>" + (callees.map(shortName).join(", ") || "—");
    panel.innerHTML += "<h2>Called by</h2>" + (callers.map(shortName).join(", ") || "—");
    panel.innerHTML += "<div style='margin-top:8px;color:#8b949e'>Each type &amp; function is a node; arrows point caller→callee (a method call counts toward its type). Click a type to open its methods, a function for its metrics.</div>";
  }
  function showType(t) {
    panel.innerHTML = "<h2>" + t.name + " · " + t.kind + "</h2>" +
      row("LOC", t.loc) + row("Methods", (t.methods || []).length) + rowHot("Max complexity", complexityOf({ kind: "type", methods: t.methods }), 10) +
      "<div style='margin-top:8px;color:#8b949e'>Each method is a node — click one for its metrics.</div>";
  }
  function showFunc(f) {
    panel.innerHTML = "<h2>" + f.name + "()</h2>" +
      row("LOC", f.loc) + rowHot("Nesting depth", f.nesting, 4) +
      rowHot("Cyclomatic", f.cyclo, 10) + row("Call-graph depth", f.callDepth);
  }
  function row(k, v) { return "<div class='metric'><span>" + k + "</span><span>" + v + "</span></div>"; }
  function rowHot(k, v, thresh) {
    const cls = v >= thresh ? "hot" : "cool";
    return "<div class='metric'><span>" + k + "</span><span class='" + cls + "'>" + v + "</span></div>";
  }

  // ---- pan / cursor-anchored zoom ----
  let drag = null;
  svg.addEventListener("mousedown", e => drag = { x: e.clientX, y: e.clientY, vx: view.x, vy: view.y });
  window.addEventListener("mouseup", () => drag = null);
  window.addEventListener("mousemove", e => {
    if (!drag) return;
    view.x = drag.vx + (e.clientX - drag.x);
    view.y = drag.vy + (e.clientY - drag.y);
    applyView();
  });
  svg.addEventListener("wheel", e => {
    e.preventDefault();
    const kOld = view.k;
    const kNew = Math.max(0.2, Math.min(4, kOld * (e.deltaY < 0 ? 1.1 : 0.9)));
    if (kNew === kOld) return;
    // Cursor position in the viewBox's W/H coordinate space (getBoundingClientRect
    // gives the on-screen element box; scale into W/H units so this stays correct
    // even if the rendered size differs from W/H).
    const rect = svg.getBoundingClientRect();
    const sx = (e.clientX - rect.left) * (W / rect.width);
    const sy = (e.clientY - rect.top) * (H / rect.height);
    // world = (s - view)/k ; keep the world point under the cursor fixed:
    //   view_new = s - (kNew/kOld) * (s - view_old)
    view.x = sx - (kNew / kOld) * (sx - view.x);
    view.y = sy - (kNew / kOld) * (sy - view.y);
    view.k = kNew;
    applyView();
  }, { passive: false });

  // Filter re-lays-out member views (item set changes); at package level it only
  // dims, so keep the user's current zoom there (no refit).
  filter.addEventListener("input", () => render(focus !== null));

  defaultPanel();
  render(true);
})();
