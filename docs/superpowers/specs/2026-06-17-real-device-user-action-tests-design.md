# Real-Device User-Action Bridge Tests (real OS APIs)

**Date:** 2026-06-17
**Branch:** add-core-node-types
**Status:** design approved

## Goal

Add real-device (BrowserStack) test coverage for the **user-action** WASM/JS
bridges — the ones with browser/OS-specific behavior headless can't fully
exercise — using **real OS APIs at maximum fidelity** (real file `<input>` +
OS file dialog, real blob downloads, real native keyboard, real audio-unlock
gesture). No bridge-injection shortcuts: exports are used only to *assert*
results, never to *perform* the action. Start with WAV import + JSON
import/export (Phase 1), then the remaining user-action bridges (Phase 2).

## Non-negotiable fidelity principle

- **Actions** = real touch (CDP) + real OS APIs. File selection goes through the
  real, transparent `<input type=file>` overlay the app mounts over the
  Import/Upload buttons (`index.html` `_fpRegisterRect`), driven by Playwright's
  real file-chooser API (`page.on('filechooser', fc => fc.setFiles(...))` /
  `locator.setInputFiles(...)`). This exercises the entire real path: real DOM
  input → OS file dialog hook → real `change` event → `_fpConsumePending` →
  `openJSONFile`/`openWAVFile` → `importJSON`. The only thing not real is a human
  finger choosing the file — the maximum automation allows.
- **Assertions** = state-read exports (`rowInstrument`, `totalRows`, `getBPM`,
  `instrumentsList`, `exportJSON`, …) + real download capture where the platform
  exposes it.
- Anything automation genuinely cannot observe on a device (e.g. the iOS
  "save to Files" dialog) runs the real action and asserts what's observable,
  and **logs the unverifiable part** — never a silent gap, never a fake pass.

## Harness

One consolidated suite `src/js/device_user_actions.browser.test.js`:
- Uses the `platform/launch.js` abstraction so the SAME test runs on
  `chromium-local` / `webkit-local` (fast iteration, no BrowserStack minutes) and
  on `browserstack:<slug>` real devices (via `TEST_PLATFORM` or per-device loop).
  Develop + validate locally first, then confirm on real hardware.
- Reuses `buildMainWasm` + the static server + `waitForWasmReady` (the
  ≤20s-window polling that survives BrowserStack's 30s CDP relay cap) + per-device
  timeout/skip + `markSessionStatus`, mirroring `visual_device_parity`.
- iOS webkit + Android chromium from the device matrix; one session per device
  (amortizes the slow paid connect). New make target `test-device-actions`.
- **Probe first:** a setup step confirms `setInputFiles`/`filechooser` actually
  works against the real `<input type=file>` overlay on the connected device. If
  a device genuinely cannot, the file-import scenarios are skipped with the
  reason (logged), not faked.

## Phase 1 — WAV import + JSON import/export

Fixtures: reuse an existing small JSON project fixture + a small valid WAV
(generated or from testdata). Scenarios (real OS path):
1. **JSON import** — mobile: open overflow menu (real touch) → the real
   `<input accept=.json>` overlay → `setFiles(fixture.json)` → assert
   rows/instruments/bpm match the fixture. Desktop: real Import tap → filechooser
   → setFiles.
2. **WAV import (Upload)** — real `<input accept=.wav>` overlay → `setFiles(fixture.wav)`
   → assert a sampler instrument was created (`instrumentsList` grows / new id) and
   it renders audio.
3. **JSON export** — real Export tap → real `downloadJSON` blob+anchor. Capture via
   Playwright's `page.waitForEvent('download')` where the platform exposes it;
   always assert `exportJSON()` content correctness; **round-trip**: feed the
   exported bytes back through the real import path and assert equivalence.

## Phase 2 — remaining user-action bridges

recording start/stop/**saveRecording** (real zip download), mobile **native
keyboard** for BPM/search/rename (real `<input z-index:10000>` → real typing →
commit), **audio unlock** on iOS (real gesture → AudioContext suspended→running →
audible), **media session** play/pause. Same fidelity rules.

## Out of scope

- Pure state-read / geometry / diagnostic exports (no OS behavior).
- Pure-Go-logic mutations already covered by Go tests.
- Committing/staging (working tree only).

## Verification

- Local: `TEST_PLATFORM=webkit-local GO=$(pwd)/.tools/go/bin/go node
  src/js/device_user_actions.browser.test.js` (and chromium-local) → green.
- Real devices: `make test-device-actions` (BrowserStack) → green / explicit
  skips with reasons.
- New export(s), if any, registered in the bridge-smoke catalogue (drift guard).
