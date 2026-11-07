// Serves src/js/ the way the deploy does, so the landing behaves as in production: a content-hashed wasm stamped into
// index.html (which is what makes the landing show), compression, raw video with Range. Run: node src/js/landing_server.js
// (BEATMO_LANDING_PORT, BEATMO_LANDING_HOST=0.0.0.0 for a phone, WASM_FILE, LANDING_ENCODING).

import http from "http";
import zlib from "zlib";
import fs from "fs";
import os from "os";
import path from "path";
import crypto from "crypto";
import { fileURLToPath } from "url";

const jsDir = path.dirname(fileURLToPath(import.meta.url));

export function contentType(f) {
  if (f.endsWith(".html")) return "text/html; charset=utf-8";
  if (f.endsWith(".js") || f.endsWith(".mjs")) return "application/javascript; charset=utf-8";
  if (f.endsWith(".wasm")) return "application/wasm";
  if (f.endsWith(".mp4")) return "video/mp4";
  if (f.endsWith(".vtt")) return "text/vtt; charset=utf-8";
  if (f.endsWith(".webp")) return "image/webp";
  if (f.endsWith(".png")) return "image/png";
  if (f.endsWith(".json")) return "application/json";
  if (f.endsWith(".css")) return "text/css; charset=utf-8";
  return "application/octet-stream";
}

// Same substitution as scripts/deploy-wasm-gcs.sh (stamp_index).
export function stampIndex(html, wasmName) {
  return html.replace(/(["'])main\.wasm\1/g, `$1${wasmName}$1`);
}

/** Re-hashes the wasm when its file changes, so a `make wasm` is picked up on the next load. opts: wasmFile, encoding,
 * level (default 6; the deploy uses 9), stamp (false = unstamped dev page), baselineIndex (?arm=baseline), root. */
export function createLandingServer(opts = {}) {
  const root = opts.root || jsDir;
  const wasmFile = opts.wasmFile || path.join(root, "main.wasm");
  const encoding = opts.encoding || "gzip";
  const stamp = opts.stamp !== false;
  const level = opts.level || 6;
  let wasm = { mtime: -1, size: -1, name: "main.wasm", body: null };
  function currentWasm() {
    const st = fs.statSync(wasmFile);
    if (st.mtimeMs === wasm.mtime && st.size === wasm.size) return wasm;
    const raw = fs.readFileSync(wasmFile);
    const hash = crypto.createHash("sha256").update(raw).digest("hex").slice(0, 16);
    let body = raw;
    if (encoding === "br") {
      body = zlib.brotliCompressSync(raw, { params: { [zlib.constants.BROTLI_PARAM_QUALITY]: 9, [zlib.constants.BROTLI_PARAM_SIZE_HINT]: raw.length } });
    } else if (encoding === "gzip") {
      body = zlib.gzipSync(raw, { level });
    }
    wasm = { mtime: st.mtimeMs, size: st.size, name: stamp ? `main.${hash}.wasm` : "main.wasm", body };
    return wasm;
  }
  const gzCache = new Map();
  const server = http.createServer((req, res) => {
    const url = new URL(req.url, "http://x");
    const file = url.pathname === "/" ? "/index.html" : decodeURIComponent(url.pathname);
    if (/^\/main(\.[0-9a-f]{16})?\.wasm$/.test(file)) {
      let w;
      try { w = currentWasm(); } catch (e) { res.writeHead(404); res.end(String(e)); return; }
      const h = { "Content-Type": "application/wasm", "Content-Length": w.body.length, "Cache-Control": stamp ? "public, max-age=31536000, immutable" : "no-cache" };
      if (encoding !== "identity") h["Content-Encoding"] = encoding;
      res.writeHead(200, h);
      res.end(w.body);
      return;
    }
    let filePath = path.join(root, file.replace(/^\/+/, ""));
    if (!filePath.startsWith(root)) { res.writeHead(403); res.end(); return; }
    if (file === "/index.html" && url.searchParams.get("arm") === "baseline" && opts.baselineIndex) filePath = opts.baselineIndex;
    fs.readFile(filePath, (err, data) => {
      if (err) { res.writeHead(404); res.end(); return; }
      const ct = contentType(filePath);
      if (filePath.endsWith(".mp4")) {
        // Uploaded uncompressed so Range works (as in production).
        const range = req.headers.range && /bytes=(\d*)-(\d*)/.exec(req.headers.range);
        if (range) {
          const start = range[1] ? Number(range[1]) : 0;
          const end = range[2] ? Math.min(Number(range[2]), data.length - 1) : data.length - 1;
          res.writeHead(206, { "Content-Type": ct, "Content-Range": `bytes ${start}-${end}/${data.length}`, "Accept-Ranges": "bytes", "Content-Length": end - start + 1 });
          res.end(data.subarray(start, end + 1));
        } else {
          res.writeHead(200, { "Content-Type": ct, "Accept-Ranges": "bytes", "Content-Length": data.length });
          res.end(data);
        }
        return;
      }
      if (/\.(html|js|mjs|vtt|json|css)$/.test(filePath)) {
        if (filePath.endsWith(".html") && stamp) {
          let name = "main.wasm";
          try { name = currentWasm().name; } catch (_) {}
          data = Buffer.from(stampIndex(data.toString("utf8"), name));
        }
        const key = filePath + ":" + crypto.createHash("sha1").update(data).digest("hex");
        let body = gzCache.get(key);
        if (!body) { body = zlib.gzipSync(data, { level }); gzCache.set(key, body); }
        res.writeHead(200, { "Content-Type": ct, "Content-Encoding": "gzip", "Content-Length": body.length, "Cache-Control": "no-cache" });
        res.end(body);
        return;
      }
      res.writeHead(200, { "Content-Type": ct, "Content-Length": data.length, "Cache-Control": "no-cache" });
      res.end(data);
    });
  });
  // Now, not on first request: otherwise a cold first page waits seconds and every benchmark's first run measures the
  // server.
  try { currentWasm(); } catch (_) {}
  server.wasmName = () => currentWasm().name;
  server.wasmBytes = () => currentWasm().body.length;
  return server;
}

// `make serve-landing`
if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.BEATMO_LANDING_PORT || 8090);
  const host = process.env.BEATMO_LANDING_HOST || "127.0.0.1";
  const server = createLandingServer({ wasmFile: process.env.WASM_FILE, encoding: process.env.LANDING_ENCODING || "gzip" });
  try {
    server.wasmName();
  } catch (_) {
    console.error(`no main.wasm at ${process.env.WASM_FILE || path.join(jsDir, "main.wasm")} — run make wasm`);
    process.exit(1);
  }
  server.listen(port, host, () => {
    const hosts = host === "0.0.0.0"
      ? ["localhost", ...Object.values(os.networkInterfaces()).flat().filter((i) => i && i.family === "IPv4" && !i.internal).map((i) => i.address)]
      : [host === "127.0.0.1" ? "localhost" : host];
    console.log(`Beatmo, served like beatmo.io (wasm ${server.wasmName()}):`);
    for (const h of hosts) {
      console.log(`  http://${h}:${port}/            the tutorial demo shows on every visit until "Do not show tutorial demo again" is ticked`);
      console.log(`  http://${h}:${port}/?landing=1  always shows it (even after opting out; untick it there to opt back in)`);
      console.log(`  http://${h}:${port}/?landing=0  never shows it`);
    }
  });
}
