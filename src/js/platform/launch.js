// platform/launch.js
//
// Single entry point for launching the browser used by browser tests.
// Lets callers target Chromium-local (default, current behavior), WebKit-local
// (closest local proxy for iOS Safari), or a real device through BrowserStack
// without touching the bespoke test runner.
//
// Usage in tests:
//   import { launchBrowser } from "./platform/launch.js";
//   const { browser, cleanup, kind } = await launchBrowser();
//   const page = await browser.newPage();
//   ... run test ...
//   await cleanup();   // closes browser + tears down any tunnel
//
// Platform selection (in order of precedence):
//   1. opts.platform string
//   2. process.env.TEST_PLATFORM
//   3. "chromium-local"
//
// Supported platform strings:
//   "chromium-local"            -> playwright chromium.launch()
//   "webkit-local"              -> playwright webkit.launch()
//   "browserstack:<device-slug>" -> see ./browserstack.js (real iPhone/Pixel)

import { chromium, webkit } from "playwright";

export const DEFAULT_PLATFORM = "chromium-local";

function resolvePlatform(opts = {}) {
  if (opts.platform) return opts.platform;
  if (process.env.TEST_PLATFORM) return process.env.TEST_PLATFORM;
  return DEFAULT_PLATFORM;
}

function chromiumLaunchArgs(extra = []) {
  return [
    "--autoplay-policy=no-user-gesture-required",
    ...extra,
  ];
}

async function launchChromiumLocal(opts) {
  const browser = await chromium.launch({
    headless: opts.headless ?? true,
    args: chromiumLaunchArgs(opts.extraArgs),
  });
  return {
    browser,
    kind: "chromium",
    platform: "chromium-local",
    cleanup: async () => { try { await browser.close(); } catch (_) {} },
  };
}

async function launchWebkitLocal(opts) {
  // WebKit ignores Chromium-specific flags; autoplay still requires a real
  // gesture, which tests must synthesize via clickAt() / page.mouse.click().
  const browser = await webkit.launch({
    headless: opts.headless ?? true,
  });
  return {
    browser,
    kind: "webkit",
    platform: "webkit-local",
    cleanup: async () => { try { await browser.close(); } catch (_) {} },
  };
}

export async function launchBrowser(opts = {}) {
  const platform = resolvePlatform(opts);

  if (platform === "chromium-local") return launchChromiumLocal(opts);
  if (platform === "webkit-local") return launchWebkitLocal(opts);

  if (platform.startsWith("browserstack:")) {
    const { launchBrowserStack } = await import("./browserstack.js");
    const deviceSlug = platform.slice("browserstack:".length);
    return launchBrowserStack(deviceSlug, opts);
  }

  throw new Error(
    `launchBrowser: unknown platform "${platform}". ` +
    `Expected "chromium-local", "webkit-local", or "browserstack:<device>".`
  );
}
