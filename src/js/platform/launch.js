// The one launcher for browser tests: opts.platform, else TEST_PLATFORM, else "chromium-local". Also "chrome-local"
// (installed Chrome: real codecs and, with TEST_HEADED=1, a real GPU; headless rasterizes WebGL in software),
// "webkit-local", "firefox-local", "browserstack:<slug>". launchBrowser() → {browser, cleanup, kind}.

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

// A covered or off-workspace headed window is throttled by Chrome unless told not to, and a bench would measure the
// throttle.
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
  // WebKit ignores Chromium flags; autoplay needs a real synthesized gesture.
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

async function launchFirefoxLocal(opts) {
  const browser = await firefox.launch({
    headless: resolveHeadless(opts),
    firefoxUserPrefs: { "media.autoplay.default": 0, "media.autoplay.block-webaudio": false },
  });
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
