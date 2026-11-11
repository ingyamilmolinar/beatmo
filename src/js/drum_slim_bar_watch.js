// drum_slim_bar_watch.js — runtime watcher for the intermittent, GPU-only
// "slim bars" artifact in the drum-row cell grid (a few cells render ~1px wide
// while neighbours are full width, persisting while the timeline is free).
//
// The artifact does not reproduce in software renderers, so this reads the REAL
// canvas (where the GPU draws) each frame, measures per-row cell-width runs,
// and when it catches a slim bar flanked by full-width cells it logs the exact
// internal state (via the slimBarDiag() WASM export — which compositing path
// drew the frame, offset, gen) and snapshots the canvas.
//
// Usage on a local serve-lan build (no rebuild of index.html needed):
//   open the browser console and run:
//     import('./drum_slim_bar_watch.js').then(m => m.startSlimBarWatch())
//   then reproduce (play, toggle track/free, let it run). On a hit it prints
//   "[slim-bar] ANOMALY ..." and stores the PNG on window.__slimBarCapture
//   (a click-to-download link is also logged).

const SLIM_MAX = 2; // logical px: at/below = a slim bar
const WIDE_MIN = 5; // logical px: at/above = a full-width cell

// Mirror of Go detectSlimBars (drum_render_diag.go): first slim run flanked on
// both sides by full-width runs, else -1.
function detectSlimBars(widths) {
  for (let i = 1; i < widths.length - 1; i++) {
    if (widths[i] <= SLIM_MAX && widths[i - 1] >= WIDE_MIN && widths[i + 1] >= WIDE_MIN) {
      return i;
    }
  }
  return -1;
}

function saturated(r, g, b, a) {
  if (a === 0) return false;
  const mx = Math.max(r, g, b);
  const mn = Math.min(r, g, b);
  return mx > 90 && mx - mn > 50;
}

// Measure saturated-colour run widths along one scanline of `img` (ImageData),
// then convert physical px -> logical px by dividing by `scale` so the
// SLIM_MAX/WIDE_MIN thresholds match the Go detector's logical-pixel units.
function runWidths(img, y, x0, x1, scale) {
  const W = img.width;
  const data = img.data;
  const runs = [];
  let c = 0;
  for (let x = x0; x < x1; x++) {
    const i = (W * y + x) * 4;
    if (saturated(data[i], data[i + 1], data[i + 2], data[i + 3])) {
      c++;
    } else if (c > 0) {
      runs.push(Math.round(c / scale));
      c = 0;
    }
  }
  if (c > 0) runs.push(Math.round(c / scale));
  return runs.filter((w) => w > 0);
}

export function startSlimBarWatch(opts = {}) {
  const intervalMs = opts.intervalMs || 120;
  if (typeof window.slimBarDiag !== "function") {
    console.error("[slim-bar] slimBarDiag() export not found — is this a current WASM build?");
    return () => {};
  }
  const canvas = document.querySelector("canvas");
  if (!canvas) {
    console.error("[slim-bar] no <canvas> found");
    return () => {};
  }
  const scratch = document.createElement("canvas");
  const sctx = scratch.getContext("2d", { willReadFrequently: true });
  let stopped = false;
  let frames = 0;

  function tick() {
    if (stopped) return;
    frames++;
    try {
      const d = window.slimBarDiag();
      if (d && Array.isArray(d.rows) && d.rows.length) {
        // Physical-pixel scale: canvas backing store vs CSS layout size.
        const rect = canvas.getBoundingClientRect();
        const scale = rect.width > 0 ? canvas.width / rect.width : 1;
        if (scratch.width !== canvas.width || scratch.height !== canvas.height) {
          scratch.width = canvas.width;
          scratch.height = canvas.height;
        }
        sctx.drawImage(canvas, 0, 0);
        const x0 = Math.max(0, Math.round(d.cellX0 * scale) + 1);
        const x1 = Math.min(canvas.width, Math.round(d.cellX1 * scale) - 1);
        const h = Math.max(1, Math.round((d.rowHeight || 12) * scale));
        for (const r of d.rows) {
          const y = Math.round(r.yMid * scale);
          if (y < 0 || y >= canvas.height || x1 <= x0) continue;
          const img = sctx.getImageData(x0, y, x1 - x0, 1);
          const widths = runWidths(img, 0, 0, x1 - x0, scale);
          const at = detectSlimBars(widths);
          if (at >= 0) {
            const png = scratch.toDataURL("image/png");
            window.__slimBarCapture = { state: d, row: r, widths, slimIndex: at, png, frames };
            console.error(
              "[slim-bar] ANOMALY row=%d color=%s slimIndex=%d renderPath=%s legacyKind=%s offset=%d rowsLayerOffset=%d gen=%d widths=%o",
              r.row, r.color, at, d.renderPath, d.legacyKind, d.offset, d.rowsLayerOffset, d.rowsLayerGen, widths,
            );
            console.log("[slim-bar] full state:", d);
            const a = document.createElement("a");
            a.href = png;
            a.download = "slim_bar_capture.png";
            a.textContent = "⬇ download slim_bar_capture.png";
            a.style.cssText = "position:fixed;top:4px;left:4px;z-index:99999;background:#000;color:#0ff;padding:4px;font:12px monospace";
            document.body.appendChild(a);
            stopped = true; // capture once; reload to re-arm
            return;
          }
        }
      }
    } catch (e) {
      console.warn("[slim-bar] watcher error", e);
    }
    setTimeout(tick, intervalMs);
  }
  console.log("[slim-bar] watcher armed — reproduce now (play, toggle track/free). First anomaly will be captured.");
  setTimeout(tick, intervalMs);
  return () => {
    stopped = true;
    console.log("[slim-bar] watcher stopped after", frames, "frames");
  };
}

if (typeof window !== "undefined") {
  window.startSlimBarWatch = startSlimBarWatch;
}
