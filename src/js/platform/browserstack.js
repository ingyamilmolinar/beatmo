// A real BrowserStack device with the local launchers' {browser, cleanup} contract: a browserstack-local tunnel reaches
// our localhost server, capabilities come from the device registry, and cleanup ends the session AND the tunnel (which
// otherwise keeps billing). Needs BROWSERSTACK_USERNAME / BROWSERSTACK_ACCESS_KEY.

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
    // Orphan tunnels keep the session alive and accrue minutes.
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
