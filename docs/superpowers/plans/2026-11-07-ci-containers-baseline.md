# CI baseline (before containers)

Run 42407821616, commit 7ace23058, 2026-11-07, `ubuntu-latest` (now Ubuntu 26.04).

| Job | Time | Result | First real error |
|---|---|---|---|
| Build | 2m19s | pass | — |
| Test Go | 1m11s | fail | `make refs-check` exit 127: the Makefile hard-codes `../../.tools/go/bin/go` (36 recipes), absent in CI |
| Test Go (race) | 7m24s | fail | `cmd/refs` `TestRunAdopt_*`: `exec: "ffmpeg": executable file not found` |
| Lint | 1m31s | fail | 133 golangci-lint findings (unused 50, staticcheck 49, gocritic 18, errcheck 8, ineffassign 3, gofmt 3) with the unpinned `latest` linter |
| Browser tests (34 batches) | ~1m each | fail | `Playwright does not support chromium on ubuntu26.04-x64` |

The browser and Test Go failures are environment drift (a runner OS the pinned
tools do not support; a tool path only the dev machine has). Every job after
Build stopped at its first error, so later errors are unknown until these are
fixed.

## After (run 42420625651, commit 981e5e8f7)

All jobs in the pinned Debian 12 image, in parallel; push to verdict **11m43s**.

| Job | Time | Result |
|---|---|---|
| CI images (tags already published) | 8s | pass |
| Build (incl. check-ma-js) | 1m38s | pass |
| Lint (golangci + lint-unused + eslint + cppcheck) | 4m19s | pass |
| Test Go `-tags test`, 4 shards | 3–5 min each | pass |
| Go coverage merge | 42s | pass |
| Test audio (real DSP) | 3m38s | pass |
| Test Go real Ebiten + xvfb, 4 shards | 4–7.5 min each | 2 fail (cello, pending decision) |
| Test Go race | 9m31s | pass |
| C coverage | 4m22s | pass (99.04%) |
| Reference corpus | 1m36s | pass (refs-check skipped: 6 local-only refs) |
| Browser, 50 jobs | ~1–4 min each | 46 pass; Firefox "Go program has already exited", rimshot capture, one soprano pitch reading |

## Green (run 42535932639, commit 395097214, 2026-11-08)

Every job passes, no performance test needed a retry; push to verdict **12m43s**
(both images rebuilt in the run before; cached here). The last red items and
their causes:

| Item | Cause | Fix |
|---|---|---|
| cello (2 Go tests) | the physical-model test expected D3 for a D2 note; the tuned cello reads 3× in YIN with its fundamental present | expected note from `InstrumentPitchInfo`; measured template exemption |
| rimshot capture | the test aligned on 3 coarse lags; the true lobe (+0.946) sat 5 samples off | raw max-\|corr\| alignment, 0.9 for every voice |
| soprano 87 Hz, sampler PCM silence | output capture fell back to the main-thread ScriptProcessor whenever its worklet module had not loaded yet (always under load), which reorders blocks; sampler tests timed their window from context creation | capture waits for the worklet; windows open at the first sound |
| Firefox "Go program has already exited" | no WebGL2 in the container (blocklisted, then no GL device) → Ebiten exits | browser targets on Xvfb, Firefox headful there (`firefoxLaunchOptions`) |
| three-stage latency count=0 | the parity timing gate's 10 s rolling reset emptied the measurement | a measurement restarts the gate's window |
| landing crystal plant | fixed 150 ms wait for the worker | condition wait |
