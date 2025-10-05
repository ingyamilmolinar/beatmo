# Tunkul Beat Sequencer

Tunkul is a grid based sequencer written in Go using Ebiten. Nodes placed on the grid
form a graph that drives the drum machine in the bottom pane. The project can run
as a desktop app or compile to WebAssembly.

## Environment setup
Install system packages and Node dependencies needed for the real Ebiten
library and Chromium tests:

```sh
sudo make dependencies
```

The script installs X11, ALSA and OpenGL libraries as well as runs `npm ci` and
`npx playwright install --with-deps chromium` so browser tests can run.

## Testing
Unit tests can run in two modes. For the fast, stubbed Ebiten path use the
alternate module file:

```sh
cd src/go && go test -tags test -modfile=go.test.mod -timeout 20s ./...
```

Or run the full Makefile suite (C lib + WASM + Go + browser harnesses):

```sh
make test
```

If you have a working X11 setup (or run under `xvfb-run`) you can test against
the real Ebiten library:

```sh
make test-real
```

For convenience, there is also a headless wrapper that runs the stubbed suite
under `xvfb-run`:

```sh
make test-xvfb
```

## Debugging
The UI and game layers now emit verbose logs describing user interactions and
internal state changes. Run the game from the repository root and check the
console output for messages prefixed with `[game]` and `[drumview]`.

### Headless browser tests
To experiment with UI automation you can attempt to run the WASM build inside a
headless browser. This requires a Chromium or Firefox binary. In this container
the packages depend on `snapd` which is not available, so headless tests cannot
run by default.

## Performance instrumentation and tests
- Enable periodic UI perf logs by setting `PERF_LOG=1` when running Tunkul. It prints every ~2 seconds:
  fps, Update/Draw average/max (ms), audio queue latency and Go→JS audio call timings.
- Engine ticker jitter logs to stderr as: `PERF engine ticker: avg=.. max=.. count=..`.
- Browser audio bridge emits debug events; inspect via `window.getAudioDebug()` in devtools.

### Quick perf checks
- Desktop (stubbed Ebiten): `cd src/go && go test -tags test ./internal/ui -run PerfCounters`
- Browser (Update-only harness): `node src/js/perf.browser.test.js`
- Browser (full render using main.wasm): `node src/js/perf_e2e.browser.test.js`

## Git hooks
Enable the optional pre-commit hook so every commit formats the code, runs the tests with the stubbed Ebiten module and builds the wasm binary:

```sh
git config core.hooksPath .githooks
```
