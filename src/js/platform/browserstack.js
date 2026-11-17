// platform/browserstack.js
//
// Connects a Playwright session to a real BrowserStack device and returns
// the same { browser, cleanup } contract as the local launchers in launch.js.
//
// Prereqs:
//   BROWSERSTACK_USERNAME    (env)
//   BROWSERSTACK_ACCESS_KEY  (env)
//   `browserstack-local` (dev dep; already in src/js/package.json)
//
// Wire-up:
//   1. Spawn a browserstack-local tunnel so the device can reach our test
//      server on localhost:<port>.
//   2. Build BrowserStack capabilities from the device registry + a build/name
//      tag derived from the running test file.
//   3. Connect via chromium.connect() or webkit.connect() — BrowserStack
//      multiplexes both kinds through their cdp.browserstack.com endpoint.
//   4. Cleanup tears the session down AND kills the tunnel; the tunnel keeps
//      the BrowserStack session alive until stopped, which would otherwise
//      bill until the session-timeout kicks in.

import path from "path";
import { chromium, webkit } from "playwright";

import { resolveDevice } from "./devices.js";

const BSTACK_WS_BASE = "wss://cdp.browserstack.com/playwright";

function requireEnv(name) {
  const v = process.env[name];
  if (!v) throw new Error(`BrowserStack: ${name} is required (set in env or .env).`);
  return v;
}

function deriveBuildName() {
  // Test filename without extension, e.g. "sanity_audio_device".
  const argv1 = process.argv[1] || "beatmo";
  return path.basename(argv1, ".js");
}

async function startTunnel(accessKey) {
  // Lazy-import so tests that never reach this path don't pay the cost.
  const mod = await import("browserstack-local");
  const Local = mod.Local || (mod.default && mod.default.Local) || mod.default;
  if (!Local) throw new Error("BrowserStack: failed to load browserstack-local module");

  const bs = new Local();
  const localIdentifier = `beatmo-${process.pid}-${Date.now()}`;
  await new Promise((resolve, reject) => {
    bs.start(
      {
        key: accessKey,
        localIdentifier,
        forceLocal: true,
      },
      (err) => (err ? reject(err) : resolve())
    );
  });
  return {
    localIdentifier,
    stop: () =>
      new Promise((resolve) => {
        try { bs.stop(() => resolve()); } catch (_) { resolve(); }
      }),
  };
}

export async function launchBrowserStack(deviceSlug, opts = {}) {
  const username = requireEnv("BROWSERSTACK_USERNAME");
  const accessKey = requireEnv("BROWSERSTACK_ACCESS_KEY");
  const { kind, caps } = resolveDevice(deviceSlug);

  const tunnel = await startTunnel(accessKey);

  const buildName = process.env.BROWSERSTACK_BUILD || deriveBuildName();
  const projectName = process.env.BROWSERSTACK_PROJECT || "Beatmo";
  const sessionName = process.env.BROWSERSTACK_SESSION_NAME || `sanity-${deviceSlug}`;

  const merged = {
    ...caps,
    "browserstack.username": username,
    "browserstack.accessKey": accessKey,
    "browserstack.local": true,
    "browserstack.localIdentifier": tunnel.localIdentifier,
    project: projectName,
    build: buildName,
    name: sessionName,
  };
  const wsEndpoint = `${BSTACK_WS_BASE}?caps=${encodeURIComponent(JSON.stringify(merged))}`;

  let browser;
  try {
    if (kind === "webkit") {
      browser = await webkit.connect({ wsEndpoint, timeout: 120_000 });
    } else if (kind === "chromium") {
      browser = await chromium.connect({ wsEndpoint, timeout: 120_000 });
    } else {
      throw new Error(`BrowserStack: unknown browser kind "${kind}" for device "${deviceSlug}"`);
    }
  } catch (err) {
    try { await tunnel.stop(); } catch (_) {}
    throw err;
  }

  const cleanup = async () => {
    // Always tear down both, even if one throws — orphan tunnels keep the
    // BrowserStack session alive and accrue minutes.
    let firstErr = null;
    try { await browser.close(); } catch (e) { firstErr = e; }
    try { await tunnel.stop(); } catch (e) { firstErr = firstErr || e; }
    if (firstErr) {
      console.warn(`[browserstack] cleanup warning: ${firstErr.message || firstErr}`);
    }
  };

  return {
    browser,
    kind,
    platform: `browserstack:${deviceSlug}`,
    cleanup,
  };
}
