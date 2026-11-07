// Slugs for TEST_PLATFORM=browserstack:<slug> (<family><model-or-os>-<browser>). Credentials and global fields are
// injected by browserstack.js, never stored here. kind "chromium" (Android Chrome) or "webkit" (iOS/macOS Safari).

export const BROWSERSTACK_DEVICES = Object.freeze({
  // Real iPhones share Mobile Safari's engine and audio quirks, which webkit-local only approximates.
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

  // Cross-checks WebKit between iOS and desktop without a phone.
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
