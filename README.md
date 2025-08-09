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
Unit tests can run in two modes. Using the stubbed Ebiten API requires the
alternate module file:

```sh
make test-mock
```

If you have a working X11 setup (or run under `xvfb-run`) you can instead test
against the real Ebiten library:

```sh
make test-real
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

## Git hooks
Enable the optional pre-commit hook so every commit formats the code, runs the tests with the stubbed Ebiten module and builds the wasm binary:

```sh
git config core.hooksPath .githooks
```
