# WASM Load-Time & Asset-Size Profiling + Compression

**Date:** 2026-06-17
**Status:** Approved (brainstorming)
**Branch:** add-core-node-types

## Problem

The browser build ships **`main.wasm` at 30.3 MB uncompressed**, and the GCS
static deploy (`scripts/deploy-wasm-gcs.sh`) serves every asset **uncompressed**
— no `Content-Encoding`, no gzip/brotli anywhere. Users on beatmo.io download
the full 30 MB before the game can instantiate. We have no test that measures
asset size, download time, or time-to-interactivity, so regressions are
invisible and there is no baseline to optimize against.

Measured baseline (gzip −9, this machine):

| Asset | Raw | gzip | Ratio |
|---|---|---|---|
| **main.wasm** | **30.3 MB** | **6.88 MB** | **4.39×** |
| drums.single.js | 431 KB | 167 KB | 2.63× |
| audio.js | 168 KB | 47 KB | 3.62× |
| synth_param_abi.gen.js | 44 KB | 8 KB | 5.36× |

**Gzip alone cuts the wasm download from 30 MB to 6.9 MB.** That is the single
biggest available win and it is the focus of this pass.

## Goals

1. **Measure** asset sizes (raw + gzip), the boot waterfall, and
   time-to-first-interactivity (TTI), with **hard, tight budget gates** that
   fail when we exceed them — so the numbers drive immediate optimization.
2. **Ship compression**: serve assets gzip-compressed from the GCS deploy, and
   strip debug info from the wasm build.

TTI is defined as **"first frame + input wired"**: WASM instantiated, `go.run()`
executed, Go-registered JS exports callable, loading indicator hidden,
`setSimpleDraw(false)` applied, and the first full-UI frame painted.

## Non-goals (YAGNI)

- `wasm-opt` / binaryen pass (explicitly dropped this pass).
- Brotli (GCS static can't content-negotiate; no `brotli` CLI on this host).
- Code-splitting / lazy WASM loading.
- TinyGo (Ebiten is incompatible).

## Design

### A. Boot instrumentation — `src/js/index.html`

Add a lightweight performance waterfall (no behavior change, ~15 lines, safe in
production and reusable for future RUM). A `window.__beatmoBoot` object records
`performance.now()` at each milestone:

| Key | When |
|---|---|
| `scriptStart` | top of the module `<script>` that loads wasm |
| `wasmFetchStart` | immediately before `fetch("main.wasm")` |
| `wasmInstantiated` | `instantiateStreaming` promise resolves |
| `goRun` | immediately after `go.run(instance)` |
| `exportsReady` | when `setSimpleDraw` first becomes a function (exports registered) |
| `firstFrame` | a `requestAnimationFrame` fired right after `setSimpleDraw(false)` — **this is TTI** |

Each milestone also emits a matching `performance.mark()`. The existing
`disableSimpleDraw()` poll loop is the natural place to detect `exportsReady`
and schedule the `firstFrame` rAF.

### B. Measurement harness

#### B1. Go asset-size budget test — `src/go/internal/deploy/asset_size_test.go`

`TestWasmAssetSizeBudget`:

- Derives the asset list from the **same manifest the drift test parses**
  (`ENTRY_FILES` / `ASSET_FILES` / `WASM_FILES` in `deploy-wasm-gcs.sh`) so the
  list can never drift from what actually ships.
- For each shipped file: compute raw size and in-process `gzip.BestCompression`
  size; always log a readable size/ratio table.
- **Hard budgets** (tight; env-overridable for CI headroom via
  `BEATMO_WASM_RAW_MAX_BYTES`, `BEATMO_WASM_GZIP_MAX_BYTES`,
  `BEATMO_ASSETS_GZIP_MAX_BYTES`):
  - `main.wasm` raw ≤ **31 MB** (current 30.3 MB; leaves ~2% headroom).
  - `main.wasm` gzip ≤ **7.0 MB** (current 6.88 MB).
  - total gzipped transfer of all shipped assets ≤ **7.3 MB**.
- **Skips cleanly** (`t.Skip`) when `main.wasm` is absent — it is a build
  artifact, not committed (per CLAUDE.md). Runs in CI after `make wasm`.

Initial budgets are set just above the measured *post-strip* numbers so the gate
is tight. Each budget carries a one-line comment citing its measured basis.

#### B2. Browser load/TTI test — `src/js/wasm_load_startup.browser.test.js`

Playwright suite, co-located with the other browser perf tests:

- Its local HTTP test server serves `.wasm`/`.js` with
  **`Content-Encoding: gzip`** (pre-gzipping the body), mirroring
  production-after-the-fix. This doubles as a guard that
  `WebAssembly.instantiateStreaming` works through a gzip-encoded response.
- After load, read `window.__beatmoBoot` and compute deltas for the full
  waterfall; always print it.
- Capture compressed transfer size via
  `performance.getEntriesByName(...main.wasm).encodedBodySize`.
- **Hard budget gates** (tight; env-overridable, matching the existing `PERF_*`
  convention):
  - `WASM_LOAD_TTI_MAX_MS` — TTI (`firstFrame − scriptStart`). Default set from
    the measured value on this harness with a small tolerance.
  - `WASM_LOAD_INSTANTIATE_MAX_MS` — `wasmInstantiated − wasmFetchStart`.
  - `WASM_LOAD_TRANSFER_MAX_BYTES` — compressed wasm transfer ≤ ~7.0 MB.
- Added to the CLAUDE.md browser-test catalogue.

### C. Shipped optimizations

#### C1. Strip flags — `Makefile`

Add `-s -w` to the `wasm` and `wasm-debug` ldflags (keep
`-X main.defaultLog=...`). Strips DWARF + symbol table. Measure and record the
raw/gzip delta; the C1 result sets the final B1 budgets.

#### C2. Deploy gzip — `scripts/deploy-wasm-gcs.sh`

- After staging into `BUILD_DIR`, gzip `main.wasm` and the large JS assets
  in place (same filenames).
- Set `Content-Encoding: gzip` on those objects, and append **`no-transform`**
  to their `Cache-Control` so GCS does **not** decompressively transcode —
  `instantiateStreaming` then receives the gzip stream with a real
  `Content-Length` (faster, cacheable).
- Keep `Content-Type: application/wasm` on `main.wasm` (mandatory for streaming
  compile; `Content-Encoding` is orthogonal).
- Extend `scripts/verify_deploy_headers.sh` + `internal/deploy/headers_verify_test.go`
  to assert `Content-Encoding: gzip` (and `no-transform`) on `main.wasm`.
- The manifest drift test is unaffected (filenames unchanged).

## Data flow

```
make wasm (-s -w)            →  main.wasm (stripped)
   └─ TestWasmAssetSizeBudget  (raw + gzip budgets, hard gate)

index.html boot waterfall    →  window.__beatmoBoot
   └─ wasm_load_startup test   (gzip-served, TTI + transfer budgets, hard gate)

deploy-wasm-gcs.sh           →  gzip bytes + Content-Encoding: gzip, no-transform
   └─ verify_deploy_headers    (asserts the encoding header live)
```

## Testing approach (TDD)

1. Write B1 + B2 first against the **uncompressed/unstripped baseline** — they
   capture current numbers and B2 proves gzip-streaming instantiation works.
2. Apply C1 (strip) → re-measure → set tight B1 budgets.
3. Apply C2 (deploy gzip) + header verification.
4. Lock the measured post-compression numbers in as the hard budgets. They are
   intentionally tight so the immediate follow-up optimization work has a gate
   to push against.

## Risks

- **GCS decompressive transcoding** would defeat gzip + break streaming length.
  Mitigated by `Cache-Control: no-transform` + the live header probe.
- **CI timing variance** (software-GL) could flake a tight TTI gate. Mitigated
  by env-overridable thresholds (tight default, CI can widen) and by gating
  deterministic sizes hard while keeping the timing override-able.
- **`instantiateStreaming` + gzip** edge cases — de-risked by B2 serving gzip
  locally before C2 touches production.
