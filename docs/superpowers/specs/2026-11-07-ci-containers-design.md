# CI in pinned containers, cached and parallel — design

Date: 2026-11-07. Status: draft for approval.

## Intent

A tree that builds and passes locally must build and pass in CI, and CI must be
fast. Two kinds of drift broke that:

- **Command drift** — CI ran commands no local target runs (`GOOS=js go build
  ./cmd/...` was red on CI while every local target was green). Fixed on
  2026-11-07: CI's Build job is `make ci-build`, `make test` runs it first,
  guarded by `internal/deploy/ci_local_parity_test.go`.
- **Environment drift** — CI ran on `ubuntu-latest` (moves on its own), Go 1.23
  re-downloading 1.26.5, Node 20 vs local 22, no Emscripten (the browser C engine
  `src/js/drums.single.js` is never rebuilt from source in CI), Chromium only (the
  suite also needs Firefox, WebKit and real Chrome for H.264), and golangci-lint
  installed as `latest`. This design fixes this half.

CI has not passed since 2026-02-28. In the last full run (2026-07-09) every job
failed: Lint, Test Go, race, all 27 browser batches. Getting each job green, with
the root cause of each failure found and fixed, is part of this work, not a
follow-up.

## Decisions made by the user

- **Containers for CI**, built from **minimal images** that hold only what the
  jobs need, on a **stable, secure distro close to the dev machine** (the dev
  machine is Debian 12 "bookworm").
- **Free GitHub-hosted runners first** (the repo is public: unlimited minutes,
  ~20 concurrent jobs). Pay for a drop-in runner (Blacksmith / Namespace) only if
  the free tier misses the target badly.
- **Cache and parallelise as much as possible**, CI-wide and per step.
- **Local default stays native** (fastest). The container is an optional way to
  get an exact CI match locally.
- Listening sessions, showcase video recording (needs a real display), headed
  GPU runs and BrowserStack stay host-only and out of CI.
- **Performance is critical and is tested in CI.** Every bench / stress / budget
  browser test (`webaudio_*_stress`, `*_bench`, `perf_e2e`, `live_edit_budget`,
  `landing_load`, …) runs in CI with its gates hard. A flaky one is handled by a
  **retry**, not by report-only mode or by moving it to the host.

## Goals

0. **Scope of "passes locally":** `make test` + `make test-real` (which includes
   `make test-browser`: all 139 browser tests, on every engine each test asks
   for), plus `make lint` and `make test-race`. CI runs all of it.
1. Every CI job runs inside a pinned image; no `apt-get`, `setup-go` or
   `setup-node` in any job.
2. Each tool version is written down exactly once (a versions file the
   Dockerfiles consume), and local tooling is checked against it.
3. Push-to-verdict wall clock **≤ 15 min** on free runners for a normal push
   (images unchanged), with a recorded baseline and a per-job timing table in the
   PR that lands this. A run that rebuilds an image may take longer. *(My calls —
   the user did not name a number.)*
4. A warm run (nothing relevant changed) is much faster than a cold one: Go build
   + test cache, module cache, npm cache, C objects all restored.
5. Every CI step is a `make` target that also works natively and inside the
   container locally.
6. All CI jobs green.

## Non-goals

- Making the container the local default.
- Self-hosted runners (a public repo's PRs would run arbitrary code on them).
- Changing what the tests test. Sharding, caching and timeouts only — no test is
  skipped, weakened, allowlisted or deleted to make CI green.
- Path filters that skip jobs by changed files *(my call: too easy to skip the one
  job a change breaks; caching gives most of that win safely)*.

## Anti-goals (failure even if CI is green)

- A test that only passes in CI, or only locally.
- A cache that can serve a stale result: a C change that doesn't relink cgo, a
  test result surviving a change to a file it reads.
- A "minimal" image that pulls a tool at run time (network installs in a job).
- Images that silently float (`:latest`, unpinned base, unpinned apt suite).

## Environment

- **Base:** `debian:12-slim` (bookworm), pinned by **digest**, apt from the
  bookworm suite (with security updates applied at image build time). Refreshing
  the digest is a deliberate, reviewed change.
- **Images** (in-repo Dockerfiles, e.g. `ci/images/`), two layers:
  - **build**: C toolchain + cppcheck; Ebiten/cgo dev headers (`libasound2-dev
    libgl1-mesa-dev xorg-dev libxcursor-dev libxrandr-dev libxinerama-dev
    libxi-dev libxxf86vm-dev`); xvfb; Go at `go.mod`'s `toolchain` version;
    Emscripten (emsdk) at the local version (4.0.12); golangci-lint pinned
    (2.10.1 today); python3 (comment census). Used by build, lint, Go test, race,
    C coverage.
  - **browser** = build + Node (local major, 22) + Playwright at the lockfile
    version with Chromium, Firefox and WebKit + Google Chrome stable (pinned
    .deb version) and their runtime libraries.
  The builder may split further if it shrinks pulls without adding complexity.
- **One versions file** (e.g. `ci/versions.env`) is the single source: the
  Dockerfiles take it as build args; `make doctor` compares the host's tools
  (`.tools/go`, `emcc`, `node`, `.tools/golangci-lint`) to it and prints any
  mismatch; the `lint-go` target installs the pinned golangci-lint instead of
  `latest`. Guard tests: the image's Go == `go.mod` toolchain; no workflow names
  a tool version; no Dockerfile hard-codes a version outside the versions file.
- **Registry:** GHCR (free for public packages). The image tag is a hash of the
  Dockerfiles + versions file. A workflow job builds and pushes only when that tag
  is missing; every other job uses `container: ghcr.io/…:<tag>`. Pull requests
  from forks cannot push to GHCR: they must still work (build the image with the
  GitHub Actions layer cache, or use the last published tag when the Dockerfiles
  are unchanged).
- **Local:** `make dev TARGET=<target>` runs any make target in the same image
  (Podman or Docker, whichever is installed; neither is installed today), with the
  repo bind-mounted, the host UID, and named volumes for the Go build cache,
  module cache and npm cache so local container runs stay warm.

## Performance tests and retries

- Performance tests run with the **same gates as a local `make test-browser`**
  (lateness gates keep their report-by-default / `AUDIO_PROBE_STRICT=1` split
  exactly as locally) *(my call: identical to local is the point; turning on
  STRICT in CI is a separate decision once its numbers are known)*.
- A failing performance test is re-run, **up to 2 retries, the performance class
  only** — correctness tests are never retried. A pass after a retry still shows
  in the run summary (test name + attempt count), so a test that needs retries
  regularly is visible, not hidden *(my calls: count and visibility)*.
- The performance tests run in their own batches with nothing else competing for
  the runner's CPU, since sharing a 4-vCPU runner makes timing meaningless.
- A performance test that still fails after its retries, and fails because of
  the runner's hardware rather than the code, is the trigger to move that job to
  a paid faster runner — not to relax its gate.

## Speed

- **No serial gate.** Today every job `needs: [build]`; with caches each job
  builds what it needs, so all jobs start at once *(my call)*.
- **Cancel superseded runs** per branch (`concurrency` + `cancel-in-progress`).
- **Caches** (actions/cache, keyed so `main` seeds what PRs restore): `GOCACHE`
  (per job kind), `GOMODCACHE` (go.sum), npm (lockfile), compiled C objects (C
  source hash). Emscripten's own cache and the Playwright browsers live in the
  image.
- **Shard the long poles.** `internal/ui` (~734 s alone) and the untagged
  real-build suite are split across parallel jobs by test name (deterministic
  partition of `go test -list`); browser tests keep the existing batch matrix with
  a tuned batch size. Shards are make targets (`SHARD=i/N`) so a shard reproduces
  locally. Coverage profiles from shards are merged into the one report CI
  publishes today.
- **Measure first.** Record a per-job and per-package baseline before tuning;
  choose shard counts from measurements, not guesses.

### The C library cache trap (must fix)

`$(C_LIB)` runs `go clean -cache` whenever `libdrums.a` is rebuilt, because Go's
cache keys cgo packages on the LDFLAGS string, not on the archive's content. On a
fresh CI checkout every C object looks new, so it would wipe any restored Go cache
on every run. Replace the wipe with correct invalidation by content: e.g. export a
hash of the C sources/headers in the cgo flags (`CGO_CFLAGS`/`CGO_LDFLAGS`, which
the Go cache does key on) so a C change relinks exactly the cgo packages and an
unchanged tree keeps its cache. This also speeds local work (a C edit stops
throwing away the whole cache). Guard: a test that a C edit changes the cgo cache
key and an untouched tree doesn't.

## The browser C engine

`src/js/drums.single.js` is committed and today only rebuilt when `emcc` exists
and the timestamp says so. CI rebuilds it with the image's pinned Emscripten and
**fails if the result differs from the committed file** (a staleness guard; tests
run against the committed bytes) *(my call)*. This needs byte-identical emcc
output on the dev machine and in the image; if it is not, the builder reports why
instead of weakening the check.

## Reference audio

`fetch-refs` / `refs verify` / `refs-check` keep today's agreed behaviour: a
failed upstream fetch makes `refs-check` skip loudly rather than fail CI. The
fetched corpus is cached by its catalogue hash so it is rarely downloaded
*(my call: keeps the existing decision, removes most of the network dependency)*.

## Branch protection and admin

The repo owner flips, once: the GHCR package to public and linked to the repo,
and the workflow token's `packages: write` on `main`. A final **verdict** job
depends on every job and is the single required check, so shard and batch counts
can change without touching branch protection *(my call)*.

## CI and local stay one thing

- Every workflow `run:` is a `make` target (extend the existing guard from
  `go build` to any `go`/`node`/`npx` invocation outside make).
- `TestCIWorkflowsOnlyInvokeExistingMakeTargets` + `ci_local_parity_test.go` stay
  and grow with the new rules.
- Per-package and per-job timeouts live in the Makefile, the same locally and in
  CI (today CI's `-timeout 120s` cannot fit `internal/ui`).

## Left to the builder

Exact image split and Dockerfile layout; the cache key scheme; the shard
partition and counts (from measurements); how fork PRs get the image; the
Playwright container flags (sandbox, `--ipc=host`, init); merging shard coverage;
the GHCR tag and retention policy; and the root-cause fixes for each currently
failing job (each reported, never masked).
