// platform/devices.js
//
// BrowserStack device registry. Maps short slugs (used in
// TEST_PLATFORM=browserstack:<slug>) to the capabilities BrowserStack expects.
//
// Slug naming: <device-family><model-or-os>-<browser>.
// Add new entries here; keep capabilities minimal — global fields (username,
// accessKey, local, localIdentifier, name, project, build) are injected by
// browserstack.js so credentials never live in this file.
//
// Browser kind drives whether we connect via Playwright chromium or webkit:
//   - "chromium" -> chromium.connect()   (Android Chrome)
//   - "webkit"   -> webkit.connect()     (iOS Safari, macOS Safari)

export const BROWSERSTACK_DEVICES = Object.freeze({
  // iOS Safari — real iPhones. WebKit on iOS shares engine + audio quirks
  // with Mobile Safari, including the AudioWorklet behavior that the local
  // webkit-local platform can only approximate.
  "iphone15-safari": {
    kind: "webkit",
    caps: {
      browser: "playwright-webkit",
      os: "ios",
      os_version: "17",
      device: "iPhone 15",
      real_mobile: true,
    },
  },
  "iphone14-safari": {
    kind: "webkit",
    caps: {
      browser: "playwright-webkit",
      os: "ios",
      os_version: "16",
      device: "iPhone 14",
      real_mobile: true,
    },
  },

  // Android Chrome — real Pixel devices.
  "pixel8-chrome": {
    kind: "chromium",
    caps: {
      browser: "playwright-chromium",
      os: "android",
      os_version: "14.0",
      device: "Google Pixel 8",
      real_mobile: true,
    },
  },
  "pixel7-chrome": {
    kind: "chromium",
    caps: {
      browser: "playwright-chromium",
      os: "android",
      os_version: "13.0",
      device: "Google Pixel 7",
      real_mobile: true,
    },
  },

  // macOS Safari — useful for cross-checking WebKit behavior between iOS and
  // desktop Safari without an actual phone.
  "mac-safari": {
    kind: "webkit",
    caps: {
      browser: "playwright-webkit",
      os: "osx",
      os_version: "Sonoma",
      browser_version: "latest",
    },
  },
});

export function resolveDevice(slug) {
  const entry = BROWSERSTACK_DEVICES[slug];
  if (!entry) {
    const known = Object.keys(BROWSERSTACK_DEVICES).join(", ");
    throw new Error(
      `BrowserStack: unknown device slug "${slug}". Known: ${known}. ` +
      `Add a new entry to src/js/platform/devices.js to support more devices.`
    );
  }
  return entry;
}
