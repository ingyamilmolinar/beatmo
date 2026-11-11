# Landing page — a living crystal around a real recording of the game

Status: DESIGN, revision 2 (2026-10-23). Revision 2 replaces the delivery plan of revision 1
(Go `landing.wasm`, `version.json` + Cache Storage, a 15 MB video loaded alongside the game, in-game
frame retiming) with a leaner one measured against a throttled cold-load baseline; see
"Revision 2: what changed and why". Implementation plan:
`docs/superpowers/plans/2026-10-23-landing-page.md`. Mockup (throwaway JS, open in a browser):
`docs/superpowers/specs/assets/landing/mockup-v8.html`.

## Intent

beatmo.io currently opens on the 6.6 MB (gzip) game WASM and a loading screen. A first-time
visitor — often arriving from a shared link — sees nothing that explains what Beatmo is until the game
finishes loading.

The landing page replaces that wait with the pitch itself:

- **in the centre**, a real recording of the game playing a techno arrangement of *Korobeiniki*, showing off the main
  features quickly but legibly;
- **around it**, a living Beatmo graph that grows like a crystal from a seed derived from the
  visitor's local date and time, so every visit grows a different one;
- **on top**, a few words explaining the game and captions that explain each feature as the
  recording reaches it;
- **in the top-right corner**, a **Play** button that lights up the moment the real game, loading
  underneath, is ready.

It is not a static site. The page has no scrollbar, fills the window, and reacts to mouse, touch and
keyboard input.

The *why*: seeing the game work, hearing it, and watching its signature idea (signals walking a grid
graph) come alive is a far better first five seconds than a grey loading screen. And the crystal
makes the wait itself feel like part of the product.

## Goals

1. First paint is effectively instant: plain HTML/CSS with no framework, and the poster frame of the
   recording shows before any WASM arrives.
2. **The landing never delays the game.** Nothing the landing loads is fetched ahead of `main.wasm`;
   the game's download starts from the first bytes of `index.html`. Gate: the cold time-to-ready
   under a throttled phone profile stays within +10% of the page without the landing.
3. The crystal animates smoothly *while the game loads*, unaffected by the game's compile and startup
   on the main thread.
4. The recording is produced by the game itself, from a scripted tour, and can be regenerated with
   one command (`make showcase-video`), run by hand when someone decides the video needs updating.
5. Captions, and the crystal's beat flashes, are in sync with the recording by construction rather
   than hand-timed.
6. Play is enabled exactly when the game can accept input and make sound. Pressing it hands over to
   the live game.
7. Returning visitors whose cached game WASM matches the deployed one go straight into the game.
8. Desktop and phone (portrait) layouts, each with its own recording.
9. The structure is loose enough that the landing can grow other versions or directions later.

## Non-goals (v1)

- A marketing site: no scrolling sections, pricing, blog or sign-up.
- Performance tuning beyond the architecture choices below. The mockup is visibly laggy; measure and
  optimise after v1 lands (decided).
- Panning the crystal. Zoom only (decided).
- Localising captions beyond what the game's i18n already provides (the builder may reuse it).

## Anti-goals (would make this a failure even if it "works")

- The landing delays the game: it competes for bandwidth or main-thread time enough to push the
  game's time-to-interactive out noticeably.
- The recording is faked — a hand-animated mock, edited screen capture, or a cursor and text overlay
  baked in. It must be the real game, driven through the real input path, with no visible pointer or
  text.
- Captions drift from the video, or describe something the video is not showing.
- The crystal is a canned animation that looks the same every visit.
- A visitor who comes back with an up-to-date cached game has to sit through the landing.

---

## Decisions already made (with the user)

### Page layout and behaviour

- **Full viewport, no scroll** (`overflow:hidden`, `touch-action:none`).
- **Centre: the recording at a fixed on-screen size.** It is a `<video>` with `muted` + `playsinline`
  + `loop` and a poster frame. The world zoom never scales the recording; only the crystal behind it
  zooms.
- **Top-left:** the BEATMO wordmark (the accent colour on "MO") and a one-sentence pitch.
- **Top-right: Play.** While the game loads it shows a progress ring and "Loading game…". When the
  game is ready it becomes a glowing accent-colour **Play**. Below it sits a "Tap for sound" chip: the
  video starts muted (browser autoplay policy), and the first tap or key press anywhere unmutes it —
  from the tap's `click` / the `keydown`, never `pointerdown` (see revision 4). Where autoplay is
  refused (iOS Low Power Mode, autoplay blocked) the chip reads "Tap to play" and that tap plays it.
- **Caption card** beside the recording, on the side of the screen region the current step refers
  to. It has a title line and a subtitle, sits on a translucent blurred backdrop with an accent rail,
  and fades between steps. The text is driven by the recording's WebVTT cue file.
- **Chapter chips** along the bottom, one per tour chapter (Circuit, Nodes, Chords, Effects, Sound,
  Themes). The active chip fills with progress, and clicking one seeks the video to that chapter.
- **Crystal ID** bottom-left: `crystal #<hex> · grown from <local YYYY-MM-DD HH:MM>`.
- **Live controls** bottom-right. Each is both a keyboard shortcut and a tappable button, and
  changing one shows a brief message naming the new value:
  - `R` — cycle the rule set; the crystal regrows from the *same seed* under the new rules.
  - `−` / `+` — growth speed steps: ¼×, ½×, 1×, 2×, 4×, 8×.
  - `T` — cycle the theme.
  - Scroll wheel or pinch — zoom the crystal (no panning), clamped to about 0.35×–6×; `0` resets.
  - Click or tap on empty space — plant a seed node into the crystal.
  - `←` / `→` — previous / next chapter; Space — pause growth.
- **Theme follows the recording.** When the recording switches theme (the Themes chapter), the page
  and crystal recolour to the same preset. The palette data comes from the game's own presets
  (`theme_presets.go`). The user loved this.
- **Phone (portrait):** the portrait recording sits a little above centre, the pitch line and caption
  card collapse (the caption moves under the video), chapter chips wrap, and the controls become a
  centred row.

### The crystal

- It is a grid graph shaped like the game's circuit: nodes on integer lattice coordinates and
  orthogonal (L-shaped) edges. It is plain JS (revision 2): the game's `core/model.Graph` buys nothing
  for a lattice map plus an edge list, and the Go build cost 780 KB against ~6 KB of JS.
- It grows by local, cellular-automaton-style rules. **One rule machinery is shared by everyone**;
  only the seed differs, so each visit gets a different initial state and different emergent shape.
- **The seed is a hash of the visitor's local date and time** (to the minute in the mockup: FNV-1a →
  a small PRNG). The builder decides the granularity; see the builder decisions below.
- **The growth rule (as prototyped):**
  - Each tick, pick random *live* nodes.
  - Each picked node tries an orthogonal step of `L ∈ [Lmin, Lmax]` lattice steps, snapped to the
    current lattice. The direction is chosen by `straight` (keep the parent's heading), `out` (head
    outward from the recording), or uniformly at random.
  - A birth needs an empty neighbourhood one lattice step wide (a "lonely cell").
  - Landing exactly on an existing node closes a loop with probability `loop`.
  - A birth inherits its parent's instrument colour, and mutates to a new one with probability
    `mutate`.
  - A node with `maxDeg` edges, or one that has failed `retire` times, or one swallowed by the
    recording's footprint, retires from the live set.
- **The infinite zoom:**
  - The lattice step is `2^level`, where `level = floor(log2(weakest_reach / base))`. As the crystal
    widens, the grid it grows on coarsens, so on-screen density stays roughly constant.
  - The camera zooms out only as fast as the **weakest of the four sides** grows. The others may
    overflow off-screen, which keeps the recording always surrounded.
  - Detail whose lattice step falls below about 3 px fades out. The two newest levels never vanish
    entirely.
  - Nodes four or more levels below the current lattice are pruned, so memory stays bounded on an
    endless run.
  - The recording's footprint is a "hole": older fine detail ends up behind the video, which is part
    of the effect.
- **Growth tempo:** 4× the mockup's first version, i.e. a growth tick every ~27.5 ms at 1×, a few
  attempts per tick (mockup v8). The user asked for faster, so treat it as a starting point to tune.
- **Rule sets in v1** (from the mockup's table; parameters are a starting point):

  | Name | Character | pick | out | straight | L | loop | maxDeg | retire | mutate |
  |---|---|---|---|---|---|---|---|---|---|
  | Crystal | dense, branching, round | any live | .45 | .2 | 1–3 | .4 | 3 | 6 | .08 |
  | Coral | sparse tendrils | tips (branch .2) | .35 | .35 | 1–2 | .03 | 3 | 5 | .04 |
  | Circuit | long straight PCB-like runs | any live | .2 | .7 | 2–5 | .3 | 3 | 10 | .12 |
  | Weave | tight, loop-heavy mesh | any live | .35 | .3 | 1–2 | .9 | 4 | 8 | .10 |

  The rule sets live in **one table**, so adding a rule set is one row.
- **Rhythm link:**
  - On each beat of the recording, derived from the song's BPM and the recording's first-beat offset
    (both in the cue file) against `video.currentTime`, nodes flash and signals travel along edges.
    The song is a fixed 140 BPM loop, so no per-trigger file is needed.
  - Downbeats fire more nodes than other beats.
  - The recording's border glows on the kick.
- **The visitor's controls only change how the crystal looks:** zoom, speed and rules never touch the
  recording or the game.
- **Interaction feedback:** nodes near the pointer brighten.

### Tech stack (revision 2)

- **No WASM in the landing, no framework, no build step.** Three small static files:
  - the landing **shell** (markup + CSS) inline in `index.html`, so it paints with the first HTML
    bytes and adds no request;
  - `landing.js` — an ES module for the overlay: captions, chapter chips, controls, readiness, Play
    handover, the "Do not show tutorial demo again" opt-out;
  - `landing_crystal.js` — the crystal: rule table, seeded PRNG, growth, level-up, pruning and its
    Canvas2D drawing. It runs in a **module Web Worker that owns an `OffscreenCanvas`**, so the game's
    compile, Go startup and warm-up on the main thread cannot stutter it. Where `OffscreenCanvas` is
    missing (Safari < 16.4) the same module runs on the main thread.
- **Why not Go:** measured 2026-10-23 (stripped, gzip -9): Go `landing.wasm` with `core/model` =
  780 KB, the mockup's whole crystal script = 6 KB. A second Go runtime would also compete with the
  game's compile for the phone's cores. The game's JS-test charter ("logic lives in Go") covers the
  game, not this standalone page; the crystal's pure logic is tested in pure Node.
- **Why not WebGL2 instancing:** a few thousand rects and L-shaped lines per frame is cheap in
  Canvas2D once the mockup's O(n²) neighbourhood scan is replaced by an occupancy lookup (that scan
  is why the mockup lagged). Revisit only if a measured frame budget says so.
- **Same `index.html`.** The landing is a layer over the game's canvas in the existing entry page.

### The recording (revision 2)

- **Song:** the built-in Korobeiniki techno template (file stem `tetris-techno`,
  `internal/assets/templates/tetris-techno.json`, 140 BPM). The tune is public domain, recognisable
  to nearly everyone, and fits the theme.
- **No "Tetris" reference anywhere in the landing (decided by the user).** That covers the page
  text, captions, chapter names, file names, the cue files, and anything visible in the video. The tour
  loads the template programmatically, never through the template menu. A guard test fails if any text
  drawn during the tour, or any landing asset, contains "tetris" (case-insensitive). The file stem is
  only how the code finds the template. *Out of scope:* the template's in-game title is not renamed.
- **The video is media we produce, so it is committed and shipped.** The audio-file guards match
  audio extensions only (`.mp4` passes); the soundtrack lives inside the video container and is never
  shipped as a separate audio file.
- **Two cuts, each with its own layout and its own tour (decided):** a 16:9 desktop recording and a
  portrait phone recording of the mobile layout. Both tours may perform the same user actions for
  now; each resolves its tap targets against its own layout.
- **Short:** about 25–35 s (roughly 16–20 bars at 140 BPM). Size scales linearly with length.
- **Small:** H.264 + AAC in MP4, 30 fps, sized to a byte budget (target ≤ 2 MB desktop, ≤ 1.5 MB
  phone; flat UI compresses well). AV1 is only added if it measurably wins enough to justify a second
  file per cut.
- **A CLI flag runs the scripted tour in the real game:** `-tour desktop|phone` plays it live;
  `-tour <cut> -record <dir>` also captures it:
  - the master bus is recorded through the game's existing desktop recording path
    (`audio.StartRecording` / `StopRecording`, the Rec button's path);
  - each drawn frame is read back and piped as raw RGBA to an `ffmpeg` child process **at a constant
    30 fps against the recording clock**: a frame slot that passes without a new draw repeats the
    previous frame, so the video and the audio share one timeline by construction and nothing is
    retimed afterwards. Repeated slots are counted and reported, so the person running it can
    re-run if the machine stalled;
  - at the end it writes the cue file (below) with the times it actually measured.
- **`make showcase-video`** (one shell script): build, run both tours under `xvfb-run` with an isolated
  `XDG_CONFIG_HOME`, encode the final MP4s + a WebP poster per cut, and copy them and the cue files
  into `src/js/landing/`. Re-recording is manual (decided): no UI change triggers it and CI never
  checks that the video is current.
- **The tour is a small Go table of steps.** Each step is an action plus a dwell in beats, and may
  carry a caption (title, subtitle), a chapter id and the screen side it refers to. Actions go
  through the real input path (the `input.go` function variables, read by the normal `Game.Update`):
  no direct state mutation, no visible cursor, no text overlay. Coverage, in roughly this order:
  circuit playing → pan/zoom → select a node and roll its pitch → chord → insert effect → audio tabs
  and a synth dial → theme switch → pull back. The builder may reorder or trim for pacing.
- **One cue file per cut, WebVTT** (`landing/desktop.vtt`, `landing/phone.vtt`), loaded as a
  `<track kind="metadata">` on the video. The browser parses it and fires `cuechange`; each cue's
  text is a small JSON object:
  - `{"caption": {title, sub, side}, "chapter": "<id>"}` — one per captioned step;
  - `{"theme": {id, bg, acc, cols:[…]}}` — when the recording switches theme, with the resolved
    palette read from the live game, so the page recolours with the exact colours on screen;
  - one whole-length cue `{"song": {bpm, beat0}, "palettes": […all presets…]}` so the crystal's beat
    clock and its `T` theme cycle need no other file.

### Game readiness and Play (revision 2)

- **No new Go export.** The landing polls (4 Hz) the two signals that already exist:
  `window.__beatmoBoot.firstFrame` (set by `index.html`) and `warmupProgress().ready`
  (`js_exports_playback_perf.go`, true when `playbackReady()` is). Ready = both.
- **The ring shows honest phases:** "Downloading game…" until `wasmInstantiated`, "Starting…" until
  `firstFrame`, then "Warming up sounds" with the ring at `done/total` from `warmupProgress()`.
- **Pressing Play:** the press is a document-level gesture, which `audio.js`'s existing unlock
  listener (capture phase) turns into an `AudioContext` resume; the landing then mutes the video, fades
  out, terminates the crystal worker, releases its canvas and video, and calls the existing
  `startPlay()` export so the startup demo is playing when the game appears.
- **While the landing is up, its keys never reach the game:** a capture-phase `keydown` listener on
  `window` (registered before Ebiten's) consumes the landing's shortcuts.

### Every visit, until the visitor opts out (revision 3, 2026-10-24)

Revision 2 skipped the landing for a browser that had already booted the same build. The user changed
that: **the landing (the tutorial demo) shows on every visit until the visitor explicitly ticks "Do
not show tutorial demo again".**

- Deploy still gives `main.wasm` a **content-hashed name** (`main.<sha16>.wasm`, cached `immutable`
  for a year) stamped into `index.html`; the stamped name is how the head script knows it is the
  deployed site (dev, `serve-lan` and browser tests stay on the plain game unless `?landing=1`).
- The opt-out is a checkbox-style button (`#lc-optout`, `role="checkbox"`) under Play and Sound on
  desktop, in the bottom stack (chapters · opt-out · tips) on phones. Ticking stores
  `localStorage["beatmo.landing.optOut"] = "1"` and confirms with a toast; it does not end this visit
  (Play does). Unticking removes the key. If storage throws, the box stays unticked and the toast says
  the browser cannot remember the choice.
- The head script reads the same key: opted out → no landing, none of its files fetched. `?landing=1`
  shows it anyway (with the box ticked, so it can be unticked); `?landing=0` never shows it.
- `make serve-landing` (`src/js/landing_server.js`) serves `src/js` the way the deploy does (stamped
  hashed wasm, gzip/br, video with Range), so the demo behaves locally exactly as on beatmo.io.

---

## Boot order (revision 2)

1. The first `<head>` script starts `fetch(main.<hash>.wasm)` and exposes the promise; the game's boot
   module instantiates from it. The game's download no longer waits for `audio.js` to load and run
   (baseline: the fetch started 415 ms after navigation on the 4G profile).
2. The same script decides landing vs skip and marks `<html data-landing>`, which reveals the inline
   shell (CSS) — no flash on the skip path.
3. Landing path: `landing.js` (module, async) starts the crystal worker and wires the overlay. The
   video element has `preload="none"` and shows its poster (a small WebP).
4. **The video loads only after the game's download has finished** (`__beatmoBoot.wasmInstantiated`):
   from there to ready the game is CPU-bound (compile, Go start, demo import, voice warm-up — measured
   ~3 s on the phone profile, ~2 s on desktop), so the video uses bandwidth nobody else needs. Until
   then the crystal provides the motion. With `navigator.connection.saveData` the video loads only
   when the visitor taps for it.
5. Ready → Play lights up. Play hands over.

## Measured baseline (2026-10-23, before any landing change)

Harness: `src/js/landing_load.browser.test.js` — cold context per run, CDP network + CPU
throttling, gzip-served like production, medians, ms from navigation.

| Profile | wasm fetch start | wasm downloaded | first frame | ready |
|---|---|---|---|---|
| 4g (9 Mbps, 85 ms, CPU 4×, 390×844) | 415 | 6,704 | 9,875 | 10,013 |
| desktop (30 Mbps, 40 ms, CPU 1×, 1280×720) | 180 | 2,316 | 4,456 | 4,558 |

Run-to-run spread ≤ 2%. Serving `main.wasm` brotli instead of gzip (6.57 MB → 4.60 MB at quality
11; the harness used quality 9) moved 4g ready from 10.0 s to 8.9 s — the largest single lever on cold
TTI found, independent of the landing; adopted in deploy (revision 2).

## Repo integration (revision 2)

- **No new Make target for builds.** `make wasm` is unchanged; the landing has no build step.
  `make showcase-video` runs `scripts/showcase_video.sh`.
- **Deploy (`scripts/deploy-wasm-gcs.sh`):**
  - stage `landing.js`, `landing_crystal.js` and `landing/*` (videos, posters, cue files);
  - rename `main.wasm` to `main.<sha16>.wasm` and stamp `index.html`; hashed wasm is
    `immutable, max-age=31536000`;
  - compress `main.wasm` with brotli (`Content-Encoding: br`); text stays gzip; **videos and posters
    are uploaded uncompressed** (gzip would break HTTP Range requests on video);
  - Content-Types for `.mp4`, `.vtt`, `.webp`;
  - `manifest_drift_test.go` sees every new runtime reference; `verify_deploy_headers.sh` knows the
    new types.
- **Size budgets:** the landing's files go into `internal/deploy/asset_size_test.go` (the same
  ratchet style as `main.wasm`).
- **CLAUDE.md:** a Quick Start row for `make showcase-video` and a short architecture subsection.

## Tests (revision 2)

- **Crystal (pure Node, `landing_crystal.browser.test.js` naming for the runner):** same seed → same
  graph after N ticks; different seeds diverge; invariants (no node in the hole, no two nodes within
  one lattice step, every edge orthogonal or L-shaped between existing nodes); node count bounded over
  a long run.
- **Crystal view (same file):** the worker's message protocol on a fake canvas with a hand-cranked
  clock — resize, pause/resume, speed, plant (once per cell, never on the recording), rules reset,
  theme background, stats, determinism under identical inputs.
- **The tour (Go, `-tags test`):** runs each cut's script through real `Game.Update` frames. EVERY
  step declares the state it exists to show (`tour_checks.go`) and the runner checks invariants after
  every step (the circuit never changes; the song keeps playing); the cue file the recorder writes is
  parsed and checked against the script; failures are proven to be reported (missed state, stray tap,
  non-converging repeat, failing action); frame-slot pacing and the WAV onset reader have unit tests;
  the no-"tetris" guard over every string drawn and every landing asset.
- **Committed media (Go):** the VTT matches the current script; the MP4 is faststart, H.264 + AAC,
  25 fps, as long as its cues; the poster is the video's size; both have the aspect the CSS reserves.
- **Browser, behaviour (`landing_behavior.browser.test.js`, real Chrome):** phases → Play, video only
  after the wasm download, cues → chips and captions, chapter seeking, keys never reach the game while
  the landing is up and do after Play, the opt-out (incl. Space/Enter on the focused box), every
  visit, the phone layout at three sizes, and the fallbacks (no H.264, Save-Data, no OffscreenCanvas,
  throwing storage, `?landing=0`, unstamped dev page).
- **Browser, playback on phones (`landing_playback.browser.test.js`, real Chrome):** an Android
  phone, an iPhone screen with autoplay refused (Low Power Mode) and desktop; state is read ONLY from
  console messages the page logs (Playwright's evaluate is a user gesture and waives autoplay rules)
  and the only input is real touch. Autoplay with no input (the clock advances, a chip fills, a
  caption shows, the crystal grows); the first touch keeps it playing and turns the sound on; "Tap
  for sound" first; autoplay refused → a tap on Tap-to-play / a chapter chip / the recording plays
  it; Save-Data → the tap loads and plays it; a touch plants a node; Play → the game plays with a
  running AudioContext. `LANDING_RECORD=<dir>` records the whole loop per platform.
- **Browser, cold load (`landing_load.browser.test.js`):** the bench + gate (landing vs baseline TTI
  ratio ≤ 1.10 on the 4g profile), the crystal draws, Play hands over, the opted-out skip.
- **Visual verification (CLAUDE.md mandate):** screenshots of the landing on desktop and phone,
  opened and inspected; key frames of each recording inspected.

## Revision 2: what changed and why

| Revision 1 | Revision 2 | Evidence / reason |
|---|---|---|
| Go `landing.wasm` (780 KB) in a worker, fetched **before** `main.wasm` | plain JS crystal (~6 KB) in a worker with OffscreenCanvas | 0.78 MB ahead of the game ≈ +0.7 s (+7%) on 4G before a second runtime starts |
| WebGL2 instancing + packed buffers | Canvas2D in the worker | the mockup's lag was an O(n²) scan, not the drawing |
| `version.json` + Cache Storage | hashed wasm name + a localStorage flag | removes a blocking round trip and a 30 MB cache copy |
| one 45–60 s video at ≤2.5 / ≤1.5 Mbps (15 / 9 MB) streaming during the download | 25–35 s, ≤2 / ≤1.5 MB, loaded after the download | the phone cut alone was larger than `main.wasm`; video buffers ahead at link speed |
| `captions.json` + `beats.json` + theme events | one WebVTT metadata track per cut + BPM | native cue events; the song is a fixed-BPM loop |
| frames as PNG stamped with `audio.Now()`, retimed by ffmpeg | raw frames piped to ffmpeg at a constant 30 fps against the recording clock | no retiming step, no per-frame PNG encode |
| new `beatmo:ready` export | poll `firstFrame` + `warmupProgress().ready` | zero Go changes |
| wasm fetch starts after `audio.js` loads | fetch starts in the first `<head>` script | baseline fetch start 415 ms (4g) |
| gzip `main.wasm` | brotli `main.wasm` | 6.57 → 4.60 MB; ready −11% on 4g |

## Builder decisions (left open on purpose)

- Seed granularity (minute vs second) and a `?seed=<hex>` reproduce link.
- Exact growth tempo and rule parameters (start from the table; tune by eye).
- Tour pacing, the exact step list and caption wording, within the coverage list.
- The handover animation details.

## Known seams in the code (starting points)

_From a code survey on 2026-10-23; verify each before relying on it. Paths are relative to
`src/go/`._

### Readiness

- `warmupController.Ready()` (`internal/ui/warmup_controller.go:216`) and
  `Game.playbackReady()` (`game_transport_helpers.go:15`) are the truth.
- `window.warmupProgress()` (`js_exports_playback_perf.go:26`) already returns
  `{done, total, ready}`.
- **Gotcha:** `hooks.EventWarmupComplete` does NOT fire on the fully-cached fast path. Do not key
  readiness on that event; key it on `playbackReady()`.
- `index.html:892-924` sets `__beatmoBoot.exportsReady` / `firstFrame`, but neither knows about
  warm-up.
- The new `beatmo:ready` signal should fire the first frame where `firstFrame` has been marked **and**
  `playbackReady()` is true.

### Templates

- `assets.Templates()` (`internal/assets/templates.go:117`); match `Genre == "tetris-techno"`.
- Scene helper `loadTemplateScene` (`internal/ui/scene_catalog.go:3660`) shows the import pattern
  (`templateLoadedStem` + `g.Import`).
- The template is **140 BPM**. The mockup's 132 was a placeholder; the page reads real beats from
  the cue file.

### Themes

- Setters: `setUserThemePreset(id)` (persists; `theme_state.go:54`) and `setCircuitThemePreset(id)`
  (undoable document edit; :62).
- Preset ids: `vice-city`, `ocean`, `midnight`, `ember`, `neon-mint`, `orchid`.
- The tour should drive theme changes through the real Theme editor UI (palette button → preset row),
  not the setters.
- Run the capture with `XDG_CONFIG_HOME=<scratch>` so prefs are not touched (CLAUDE.md rule).

### Chords

- `setNodeType(id, NodeTypeChord)` / `setNodeChord` (`node_type_cycle.go:54,78`).
- The pickers open through the node sidebar: `openNoteMatrix` (`game_node_sidebar_note_matrix.go:51`)
  and `openChordMatrix` (`game_node_sidebar_chord_matrix.go:52`).
- Scene helpers `noteMatrixSetupFor` / `chordMatrixSetup` (`scene_catalog.go:3914-3959`) show the
  sequence.
- The tour must reach these through taps, not by calling them.

### Input injection

These live in `internal/ui/input.go` and are usable outside tests (no build tag):

- the package function variables `cursorPosition`, `isMouseButtonPressed`, `wheel`, `touchIDs`,
  `touchPosition`, …;
- exported `SetInputForTest`;
- the touch override setters.

`injectTouchTap` (`touch.go:520`, a two-frame press then release) is unexported. There is **no
scripted-input driver yet**: the tour runner is new code that feeds these variables frame by frame.

### Screenshot / scene harness

- `cmd/beatmo.go` flags at lines 49-59; boot order at 261-298 (`Import` → `uistate` → `RunScene` →
  `SetScreenshot`); `ebiten.SetTPS(60)` at :322.
- `maybeCaptureScreenshot` (`game_draw.go:51`) and `captureScreen` (`game_screenshot.go:46`, ReadPixels
  → PNG, with subject crop) are the frame-grab primitives to reuse for per-frame capture.

### Clocks — the main technical risk, confirmed

- `audio.Now()` is the transport clock. On desktop it is overridable through the exported
  `audio.SetNowForTest` (`internal/audio/engine_state.go:56`).
- `gamestate.DtSince` prefers audio time, so faking `audio.Now()` mostly drives the transport.
- **But** `time.Now()` is read directly in:
  - the sequencer: `game_sequencer_schedule.go:75,101`;
  - `game_update.go` (several);
  - `game_audio_schedule.go:31,69`;
  - `game_sync_ui.go`;
  - `game_beat_display.go:113`;
  - `touch.go`;
  - `trace_sprite.go:75`.
- Several package clocks are already injectable: `nowNS`, `samplerNowNanos`, `logsNowFn`,
  `parityRecordNow`, `eq_panel_zone.nowFn`.

### Offline audio (not needed for v1; context only)

- `audio.RenderMasterMixOffline(notes, bpm, sampleRate, totalSamples)`
  (`internal/audio/offline_master_render.go:27`, `!test && !js`) renders a note list through the LIVE
  engine state, deterministically.
- The test-only `liveArrangement` (`export_import_master_audio_harness_test.go:71`) turns the
  predictor into that note list.
- **Limitation:** it renders the whole note list under ONE engine state. The tour changes pitch,
  chords, effects and synth dials mid-song, so a single offline pass would not hear those edits.

### Recording strategy — decided (revision 2)

Real-time capture of the live tour: master bus through the desktop Rec path, frames piped raw to
ffmpeg at a constant 30 fps against the recording clock (a missed slot repeats the last frame and is
counted). The `time.Now()` reads and the single-state limit of `RenderMasterMixOffline` above
therefore do not need to be addressed; the deterministic route stays possible if a CI freshness check
is ever wanted.

---

## Implementation results (2026-10-23)

Measured with `src/js/landing_load.browser.test.js` in real Chrome (Playwright's bundled Chromium has
no H.264), cold contexts, arms interleaved, medians.

| Profile | Arm | wasm fetch start | first frame | ready | bytes before ready |
|---|---|---|---|---|---|
| 4g (3 runs) | baseline (pre-change page) | 398 ms | 9,821 | 9,980 | 6.77 MB |
| | landing (crystal + 0.8 MB phone video) | 124 | 9,912 | 10,060 | 7.59 MB |
| | skip (returning visitor) | 125 | 9,697 | 9,845 | 6.77 MB |
| desktop (6 runs, 2 sets) | landing / baseline first frame | — | 1.010 and 1.079 | | |

- TTI ratio landing/baseline: **1.009** on 4g; on desktop 1.010–1.079 across two sets, dominated
  by machine-load noise (the no-landing skip arm moved by the same amounts). Crystal off
  (`?crystal=0`) and video after ready (`?video=late`) made no measurable difference.
- Crystal draw cost in its worker: p50 0.2–0.3 ms, p95 0.6–0.8 ms per frame at 4× CPU throttle.
  `grow()`: 19.6 µs per tick (Node).
- Brotli `main.wasm` (deploy): 6.57 MB → 4.60 MB, ready −11% on 4g (adopted).

Decisions taken while building:

- **25 fps, not 30.** The game draws ~28 fps under xvfb's software GL; 30 fps repeated a slot in
  six. **Record on a real display** when there is one: 1.2–2.6% repeated frames (GPU) vs 24–52%
  under xvfb on the Synth tab and theme switches. `make showcase-video` picks the display
  automatically.
- **H.264 only, CRF 23.** Desktop 32 s: CRF 23 = 0.99 MB (SSIM 0.997); AV1 at equal quality saves
  ~35%, but the video loads after the game's download, so a second codec buys no TTI. Final files:
  desktop 1.34 MB, phone 0.80 MB (+ 34 / 18 KB posters). Engines without H.264 keep the poster.
- **A/V sync by construction:** frames are stamped with the recording clock; the audio offset
  (mixer render-ahead, 0.11–0.18 s) is measured from the WAV's first onset vs the transport start.
  Final MP4s: first audible onset within 1 ms of `beat0`.
- **The landing shows only on deployed pages** (stamped hashed wasm name), so dev, `serve-lan` and
  the existing real-input browser tests are unaffected; `?landing=1` forces it.
- **Theme palettes come from the resolver** (`resolveThemeTokens`), six vivid slots per preset
  (focus-ring, viz-wave-a, viz-bass, primary, viz-treble, primary-bright) — three of the first
  pick resolved near the background.
- **A product bug the tour found:** on a phone, confirming a chord by re-tapping the picked cell
  closed the picker on the press and the held finger then fired the Audible selector beneath,
  turning the chord back into a note. Fixed in `NodeSidebar.HandleInput` (a press that began on the
  open picker owns its gesture to the release); guard
  `TestChordConfirmHeldPressDoesNotFireTheControlBeneath` (fails without the fix).

## Revision 3 results (2026-10-24): opt-out, local server, tests

- **Behaviour change:** every visit until "Do not show tutorial demo again" (above). The build-based
  skip (`beatmo.booted`) is gone.
- **Found by the new per-step tour checks and fixed:**
  - the phone tour's "tap outside" fell back to a point inside the splitter's grab strip; the
    outside-tap handler passes such presses through, and the release closed the sidebar AND created a
    node — visible in the first committed phone video (a new node with its coordinate badge at the
    grid's bottom-left). The tour now picks a point clear of the strip, the menus, the corner buttons
    and every node. Separately (not changed here): a plain tap on the splitter strip with no menu
    open creates a node, on desktop and phone.
  - the recorder put a late frame into the first missed slot, so after a stall the picture ran ahead
    of the sound by the stall's length; a missed slot now holds the previous frame.
  - `IsEQChannelOpen` looked up a portal id that does not exist (always false).
- **Found by the browser test and fixed:** after Play, keys did not reach the game (focus stayed on a
  removed landing button) — the handover now focuses the game canvas; on phones the opt-out overlapped
  the video — it moved to the bottom stack, the video is 56vh (was 60vh), the toast sits mid-screen.
- **Recording:** GNOME/XWayland throttled the (hidden) recording window to ~1 frame/s: 81% and 94%
  repeated frames over 134 s / 315 s phone runs. Recording now disables vsync: phone 0.1%, desktop
  2.9% repeated, longest freeze 1 frame. `showcase_video.sh` publishes only a clean take (no step
  errors, ≤ 5% repeats, no freeze > 12 frames). New media: desktop 1,323,063 B (32.1 s), phone
  782,718 B (30.8 s); first sound within 1 ms of `beat0` on both (`silencedetect` on the MP4).
- **Cold load after the change** (4g, 3 runs, load average ~6): wasm fetch start 425 → 139 ms;
  ready baseline 11,165 ms, landing 11,404 ms, opted-out 10,970 ms; **TTI ratio 1.014** (gate 1.10).

## Revision 4 (2026-10-24): the recording froze on phones

- **Report:** on mobile the recording stayed on one frame, the chapter chips and captions did not
  move, and no sound could be heard.
- **Root cause (reproduced in emulation):** landing.js unmuted the video on `pointerdown`. On a touch
  screen `pointerdown` fires before the tap counts as a user gesture (activation lands on `pointerup`
  / `touchend`), and unmuting an autoplaying muted video without a gesture makes the browser pause it.
  The landing then remembered it had unmuted, so no later tap — "Tap for sound" included — resumed
  it. Separately: where autoplay is refused outright (iOS Low Power Mode, blocked autoplay) nothing
  ever retried `play()`; with Save-Data "Tap for sound" did nothing; and index.html's document-level
  canvas `touchstart` `preventDefault` also hit the crystal canvas, cancelling the tap's click (no
  plant, no sound from a tap on the background).
- **Why no test saw it:** Playwright's `page.evaluate` / `waitForFunction` run with a user gesture, so
  the first state read gave the page sticky activation and the browser waived every autoplay rule.
- **Fix:** `wantSound()` runs from the tap's `click` (capture phase on the landing) and from keys:
  it loads the recording if needed (Save-Data included — the visitor asked), unmutes it and plays it
  whenever it is paused. A refused autoplay labels the chip "Tap to play". The canvas
  `preventDefault` skips `#lc-crystal` (the landing has `touch-action: none` and handles its own
  touchmove).
- **Evidence:** `landing_playback.browser.test.js` — 25 of 84 checks fail on the pre-fix page
  (Android: the first touch pauses the recording at its current frame; every platform: a refused
  autoplay or Save-Data never plays), 0 after; `landing_behavior` still 236 checks OK; recordings of
  the whole loop on the three platforms inspected frame by frame (all six chapters, the wrap, sound
  on, Play, the game's beat counter advancing).
- **Not verified on a physical phone:** BrowserStack Automate time had run out; Playwright's Linux
  WebKit could not stand in for iOS video (it refuses muted autoplay of a video with sound, crashed
  2 of 4 bare runs, and stalled gesture-started playback at 0.11 s), so iOS is emulated on Chrome's
  engine with Low Power Mode's policy.
