// serve_lan.js
//
// Builds the WASM and serves the test bundle on 0.0.0.0 so devices on the same
// network (your phone over WiFi) can reach it. Prints a LAN URL plus an
// ASCII-QR code if `qrencode` is installed, otherwise just the URL with a hint.
//
// Invoked by `make serve-lan` (which builds wasm first via dependency).
// Direct usage:
//   GO=$(pwd)/.tools/go/bin/go BEATMO_LAN_PORT=8080 node src/js/serve_lan.js
//
// The server routes "/" to index.html and "/?probe=audio" to the standalone
// in-page audio probe (see sanity_audio_probe_page.html). Hit either from
// your phone.

import os from "os";
import { spawnSync } from "child_process";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

function lanIPv4Addresses() {
  const ifaces = os.networkInterfaces();
  const out = [];
  for (const [name, addrs] of Object.entries(ifaces || {})) {
    if (!addrs) continue;
    for (const a of addrs) {
      if (a.family !== "IPv4" && a.family !== 4) continue;
      if (a.internal) continue;
      out.push({ name, address: a.address });
    }
  }
  return out;
}

function tryQR(url) {
  const r = spawnSync("qrencode", ["-t", "ANSIUTF8", url], { encoding: "utf8" });
  if (r.status === 0 && r.stdout) return r.stdout;
  return null;
}

function header(text) {
  const bar = "=".repeat(text.length + 4);
  return `\n${bar}\n  ${text}\n${bar}\n`;
}

if (process.env.BEATMO_LAN_SERVE !== "1") {
  process.env.BEATMO_LAN_SERVE = "1";
}

console.log("[serve-lan] Building WASM...");
if (!buildMainWasm({ logLevel: process.env.LOG_LEVEL || "INFO" })) {
  console.error("[serve-lan] WASM build failed");
  process.exit(1);
}

const server = await createServer();
const port = server.address().port;

const ifaces = lanIPv4Addresses();
const primary = ifaces[0];
const lanURL = primary ? `http://${primary.address}:${port}/` : `http://<your-lan-ip>:${port}/`;
const runnerURL = primary ? `http://${primary.address}:${port}/?run=tests` : `http://<your-lan-ip>:${port}/?run=tests`;

console.log(header("Beatmo LAN serve mode"));
console.log(`  Localhost  : http://localhost:${port}/`);
if (ifaces.length === 0) {
  console.log(`  LAN        : (no non-internal IPv4 address detected — connect to a network)`);
} else {
  console.log(`  LAN        : ${lanURL}`);
  if (ifaces.length > 1) {
    console.log(`  Other      :`);
    for (let i = 1; i < ifaces.length; i++) {
      console.log(`               http://${ifaces[i].address}:${port}/  (${ifaces[i].name})`);
    }
  }
}
console.log(`  Test runner: ${runnerURL}`);
console.log(`               (runs every scenario module registered in src/js/scenarios/)`);
console.log("");

if (primary) {
  const qr = tryQR(runnerURL);
  if (qr) {
    console.log("  Scan to open the test runner on your phone:");
    console.log(qr);
  } else {
    console.log("  Hint: install `qrencode` to render a scannable QR here:");
    console.log("        Debian/Ubuntu: sudo apt install qrencode");
    console.log("        macOS:         brew install qrencode");
    console.log("");
  }
}

console.log("[serve-lan] Press Ctrl+C to stop.\n");

function shutdown(signal) {
  console.log(`\n[serve-lan] ${signal} received, shutting down.`);
  try { server.close(); } catch (_) {}
  process.exit(0);
}
process.on("SIGINT", () => shutdown("SIGINT"));
process.on("SIGTERM", () => shutdown("SIGTERM"));
