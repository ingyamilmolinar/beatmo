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
//   "chrome-local"              -> the installed Google Chrome (channel
//                                  "chrome"): real codecs and, with
//                                  TEST_HEADED=1, a real GPU-composited window —
//                                  what a user runs. Headless Chromium rasterizes
//                                  WebGL in software (400-600 ms frames), which
//                                  hides or invents main-thread costs.
//   "webkit-local"              -> playwright webkit.launch()
//   "firefox-local"             -> playwright firefox.launch() (autoplay
//                                  allowed, like the Chromium flag)
//   "browserstack:<device-slug>" -> see ./browserstack.js (real iPhone/Pixel)

import { chromium, firefox, webkit } from "playwright";

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

// TEST_HEADED=1 shows the window. A headed window that is covered or on
// another workspace is throttled by Chrome (rAF and timers) unless told not
// to — a bench would then measure the throttle.
function headedFlags(headless) {
  return headless ? [] : [
    "--disable-renderer-backgrounding",
    "--disable-background-timer-throttling",
    "--disable-backgrounding-occluded-windows",
  ];
}

function resolveHeadless(opts) {
  if (opts.headless != null) return opts.headless;
  return process.env.TEST_HEADED !== "1";
}

async function launchChromiumLocal(opts, channel) {
  const headless = resolveHeadless(opts);
  const browser = await chromium.launch({
    ...(channel ? { channel } : {}),
    headless,
    args: chromiumLaunchArgs([...headedFlags(headless), ...(opts.extraArgs || [])]),
  });
  return {
    browser,
    kind: "chromium",
    platform: channel ? `${channel}-local` : "chromium-local",
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

// Firefox blocklists WebGL2 when its GPU probe fails (CI containers, no GPU
// session), and Ebiten then exits the Go program; Mesa's software GL is fine.
export const FIREFOX_WEBGL_PREFS = { "webgl.force-enabled": true };

// Headless Firefox needs a GL device a container lacks ("Exhausted GL driver
// options"); under xvfb-run (the Makefile's browser targets set
// BROWSER_TEST_XVFB=1) it runs headful on Xvfb's Mesa GL instead.
export function firefoxLaunchOptions(extra = {}) {
  const onXvfb = process.env.BROWSER_TEST_XVFB === "1" && !!process.env.DISPLAY;
  return {
    ...extra,
    ...(onXvfb ? { headless: false } : {}),
    firefoxUserPrefs: { ...FIREFOX_WEBGL_PREFS, ...(extra.firefoxUserPrefs || {}) },
  };
}

async function launchFirefoxLocal(opts) {
  const browser = await firefox.launch(firefoxLaunchOptions({
    headless: resolveHeadless(opts),
    firefoxUserPrefs: { "media.autoplay.default": 0, "media.autoplay.block-webaudio": false },
  }));
  return {
    browser,
    kind: "firefox",
    platform: "firefox-local",
    cleanup: async () => { try { await browser.close(); } catch (_) {} },
  };
}

export async function launchBrowser(opts = {}) {
  const platform = resolvePlatform(opts);

  if (platform === "chromium-local") return launchChromiumLocal(opts);
  if (platform === "chrome-local") return launchChromiumLocal(opts, "chrome");
  if (platform === "webkit-local") return launchWebkitLocal(opts);
  if (platform === "firefox-local") return launchFirefoxLocal(opts);

  if (platform.startsWith("browserstack:")) {
    const { launchBrowserStack } = await import("./browserstack.js");
    const deviceSlug = platform.slice("browserstack:".length);
    return launchBrowserStack(deviceSlug, opts);
  }

  throw new Error(
    `launchBrowser: unknown platform "${platform}". ` +
    `Expected "chromium-local", "chrome-local", "webkit-local", "firefox-local", or "browserstack:<device>".`
  );
}
