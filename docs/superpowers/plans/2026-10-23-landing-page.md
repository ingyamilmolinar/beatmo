# Landing page — implementation plan

Status: tasks 1–7 implemented 2026-10-23, task 8 on 2026-10-24 (uncommitted); results in the
spec's "Implementation results" and "Revision 3 results" sections. Not done: an actual deploy
(needs the GCP credentials and your go-ahead).

Spec: `docs/superpowers/specs/2026-10-23-landing-page-design.md` (revision 2).
Principle: smallest thing that works, every step measured with the cold-load harness
(`src/js/landing_load.browser.test.js`) against the baseline recorded in the spec.

## Task 1 — Measurement harness (done first)

`src/js/landing_load.browser.test.js`: cold context per run, CDP network + CPU throttling
(`LANDING_PROFILE=4g|slow4g|desktop`), production-like serving (gzip or brotli wasm, hashed name
stamped into `index.html`, Range for video), arms `baseline` (a saved pre-change `index.html`),
`landing`, `skip`, interleaved. Reports fetch start / download end / first frame / ready / bytes
before ready / video bytes before ready. Gate: landing/baseline median TTI ≤ 1.10.

Evidence: baseline numbers in the spec.

## Task 2 — Boot path in `index.html` (game first)

- First `<head>` script: `const WASM = "main.wasm"` (deploy stamps the hashed name), start
  `fetch(WASM)` → `window.__beatmoWasm`, decide landing vs skip, set `<html data-landing>`,
  inject `landing.js` only on the landing path, register the capture-phase key guard.
- Boot module: `instantiateStreaming(window.__beatmoWasm, …)`; on ready store
  `localStorage["beatmo.booted"] = WASM` (replaced in task 8 by the explicit opt-out).
- Evidence: baseline arm vs new page with `?landing=0` — wasm fetch start drops from ~415 ms.

## Task 3 — Crystal (`landing_crystal.js`)

Port the mockup's growth + camera + drawing into one module that runs in a worker (OffscreenCanvas)
or on the main thread. Fix the O(n²) `free()` scan with an occupancy lookup; adjacency lists for
beat signals. Messages in: `init{canvas,w,h,dpr,seed}`, `resize`, `hole{rect}`, `zoom`, `speed`,
`rule`, `theme{bg,cols}`, `beat{t,down}`, `plant{x,y}`, `pointer{x,y}`, `pause`.
Pure Node test: determinism, divergence, invariants, bounded count.
Evidence: frame time in the worker on the 4g profile (CPU 4×) while the game boots.

## Task 4 — Landing overlay (`landing.js` + inline shell)

Shell markup + CSS inline in `index.html` (hidden unless `data-landing`). Module: readiness poll,
phases on the ring, video deferred until `wasmInstantiated`, VTT metadata cues → captions / chapter
chips / theme / beat clock, controls (R, −/+, T, zoom, plant, ←/→, Space), "Tap for sound", Play
handover (`startPlay()`, terminate worker, remove layer, focus canvas).
Evidence: harness `landing` arm ratio ≤ 1.10 on 4g; screenshots desktop + phone.

## Task 5 — Tour + recording (Go, desktop only)

- `internal/ui/tour.go` (`!js`): step table per cut, scripted input through the `input.go` function
  variables, hooked at the top of `Game.Update`; capture hook at the end of `Game.Draw` that pipes raw
  RGBA to `ffmpeg` at a constant 30 fps of `audio.RecordingElapsed()`; writes the VTT.
- `cmd/beatmo`: `-tour desktop|phone`, `-record <dir>` (js stub).
- `scripts/showcase_video.sh` + `make showcase-video`: build, run both cuts under xvfb with a scratch
  `XDG_CONFIG_HOME`, final encode (H.264/AAC MP4 to a byte budget, WebP poster), copy into
  `src/js/landing/`.
- Test (`-tags test`): each step reaches its state; no drawn string contains "tetris".
Evidence: repeated-frame count per recording; A/V sync spot check (first kick onset vs frame);
encoded sizes; key frames inspected.

## Task 6 — Deploy + guards

Deploy: hashed + brotli `main.wasm`, stage landing files, raw (uncompressed) video/poster upload,
new Content-Types. Update `manifest_drift_test.go` (variable wasm ref + landing refs),
`asset_size_test.go` budgets, `verify_deploy_headers.sh` types. CLAUDE.md rows.
Evidence: `DRY_RUN=1` deploy listing; Go deploy tests green.

## Task 7 — Final measurement + visual verification

Harness all arms × {4g, desktop} (+ brotli); screenshots of the landing (desktop, phone, ready
state, a caption, after a theme cue) opened and inspected.

## Task 8 — Opt-out, local server, tests everywhere (2026-10-24)

- "Do not show tutorial demo again" (`localStorage["beatmo.landing.optOut"]`); the landing shows on
  every visit until it is ticked; the build-based skip is removed.
- `make serve-landing` → `src/js/landing_server.js` (shared with the browser tests): the deployed
  site locally.
- Tour: a `reached` check on every step + invariants after every step (`tour_checks.go`); tests for
  the reporting itself, frame pacing, the WAV onset reader, the cue file; committed-media test.
- Browser: `landing_behavior.browser.test.js` (both layouts, opt-out, every visit, keys, chapters,
  captions, fallbacks); crystal view protocol tests.
- Re-record both cuts with the fixes the checks found (spec "Revision 3 results").

