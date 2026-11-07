# CI in Pinned Containers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every CI job runs a `make` target inside a pinned Debian 12 image, cached and sharded, green, with push-to-verdict ≤ 15 min.

**Architecture:** One versions file feeds two Dockerfiles (build, browser) published to GHCR under a content-hash tag; `ci.yml` builds the image only when the tag is missing, then fans every job out in parallel inside it with actions/cache for Go/npm/C/refs. Sharding and retries live in Makefile targets and the browser runner, so every CI step reproduces natively and via `make dev`.

**Tech Stack:** GitHub Actions, Docker (buildx) + GHCR, Debian 12 slim, Go 1.26.5, emsdk 4.0.12, Node 22, Playwright 1.54.2, golangci-lint 2.10.1, bash, Go tests in `internal/deploy`.

**Spec:** `docs/superpowers/specs/2026-11-07-ci-containers-design.md`

## Global Constraints

- Base image `debian:12-slim`, pinned by digest; apt from bookworm + bookworm-security only.
- Every tool version appears exactly once, in `ci/versions.env`.
- No workflow `run:` invokes `go`, `node`, `npx`, `apt-get` directly — only `make <target>` (and `./scripts/browser-test-matrix.sh` for the matrix output).
- No test is skipped, weakened, allowlisted or deleted to make CI green; every failure is root-caused.
- Only performance tests are retried: at most 2 retries, each retry named in the run summary.
- Local default stays native; `make test` keeps running `ci-build` first.
- Comments follow CLAUDE.md (why only, ≤ 3 lines, no history/cross-refs); `make comment-census` findings must not grow in touched files.
- No container runtime on the dev host yet: images are built and verified in CI; `make dev` is verified once the user installs Podman.
- Commit only the files a task touches (the tree carries unrelated untracked files and other sessions' work); push to `add-core-node-types` after each task and read the CI result before the next.

## Review Focus

- A fork PR (no `packages: write`) must still get an image: when the tag is missing it builds locally with the GHA layer cache instead of failing.
- A C header edit (not just a `.c`) must change the cgo cache key — Task 4's test edits a header.
- A restored GOCACHE from a different image tag must not be reused: cache keys include the image tag (Task 5).
- A test file added to `src/js` after the matrix is computed must still run somewhere: the shard/batch partition is computed from the directory at run time, and a guard asserts the union of shards == the full list (Tasks 6, 7).
- `RETRY_TESTS` naming a deleted test must fail, not silently retry nothing (Task 7 guard).

---

### Task 1: Land the build fix and take a baseline

**Files:**
- Already modified (this session): `Makefile` (`ci-build`), `.github/workflows/ci.yml`, `src/go/internal/deploy/ci_local_parity_test.go`, synthmatch/songrender/refs/gen-showcase build tags.
- Add: this plan + the spec.
- Create: `docs/superpowers/plans/2026-11-07-ci-containers-baseline.md`

- [ ] **Step 1:** `make ci-build` and `go test -tags test -modfile=go.test.mod ./internal/deploy/` pass locally.
- [ ] **Step 2:** Commit exactly those paths; push; wait for the run.
- [ ] **Step 3:** Record in the baseline file, per job: duration, pass/fail, the first real error line. This is the failure list Task 9 works through and the timing baseline Task 10 compares against.

### Task 2: Versions file, images, publish job

**Files:**
- Create: `ci/versions.env`, `ci/images/build.Dockerfile`, `ci/images/browser.Dockerfile`, `scripts/ci/image-tag.sh`, `scripts/ci/doctor.sh`
- Modify: `Makefile` (targets `image-tag`, `image`, `dev`, `doctor`; `$(GOLANGCI_LINT)` installs `GOLANGCI_LINT_VERSION`), `.github/workflows/ci.yml` (job `images`)
- Test: `src/go/internal/deploy/ci_versions_test.go`

**Interfaces:**
- Produces: `ci/versions.env` keys `DEBIAN_IMAGE` (`debian:12-slim@sha256:…`), `GO_VERSION` (`1.26.5`), `EMSDK_VERSION` (`4.0.12`), `NODE_MAJOR` (`22`), `PLAYWRIGHT_VERSION` (`1.54.2`), `CHROME_VERSION` (exact .deb version), `GOLANGCI_LINT_VERSION` (`2.10.1`).
- Produces: `scripts/ci/image-tag.sh <build|browser>` → prints `ghcr.io/ingyamilmolinar/beatmo-ci-<kind>:<first 16 hex of sha256 over versions.env + both Dockerfiles>`.
- Produces: `make dev TARGET=<t> [IMAGE=build|browser]` — runs `make <t>` in the image via `podman` or `docker` (first found), repo at its own path, `--user $(id -u):$(id -g)`, named volumes `beatmo-gocache`, `beatmo-gomod`, `beatmo-npm`.
- Produces: `make doctor` — prints each pinned tool's wanted vs host version, exit 1 on mismatch.
- Produces: ci.yml job `images` with outputs `build` and `browser` (full image refs).

- [ ] **Step 1: Write failing tests** in `ci_versions_test.go`:
  - `TestVersionsFileIsTheOnlyPin`: the Dockerfiles use only `ARG`s named in `versions.env` (no literal version after `FROM`, `@sha256` only via `DEBIAN_IMAGE`); no workflow contains a semver literal next to a tool name.
  - `TestImageGoMatchesGoModToolchain`: `GO_VERSION` == `go.mod`'s `toolchain go<ver>`.
  - `TestImagePlaywrightMatchesLockfile`: `PLAYWRIGHT_VERSION` == `package-lock.json`'s `node_modules/playwright` version.
  - `TestDebianImagePinnedByDigest`: `DEBIAN_IMAGE` matches `^debian:12-slim@sha256:[0-9a-f]{64}$`.
- [ ] **Step 2:** Run `go test -tags test -modfile=go.test.mod -run 'TestVersionsFile|TestImage|TestDebianImage' ./internal/deploy/` → FAIL (files missing).
- [ ] **Step 3:** Write the files. Build image: apt `build-essential pkg-config cppcheck xvfb xauth python3 git ca-certificates curl xz-utils libasound2-dev libgl1-mesa-dev xorg-dev libxcursor-dev libxrandr-dev libxinerama-dev libxi-dev libxxf86vm-dev libgtk-3-dev`, Go tarball (sha256-verified), emsdk at `EMSDK_VERSION` with its cache pre-warmed by one `emcc` compile, golangci-lint at its version; `GOTOOLCHAIN=local`; world-writable `HOME`-independent caches so `--user` works. Browser image `FROM` the build image ref (passed as ARG): Node from NodeSource for `NODE_MAJOR`, `npx playwright@$PLAYWRIGHT_VERSION install --with-deps chromium firefox webkit`, Chrome stable `.deb` at `CHROME_VERSION`. Get the digest with `curl` against the Docker Hub registry API (no local docker).
- [ ] **Step 4:** `images` job: for each kind, compute the tag; `docker manifest inspect` → exists: output it; missing and `packages: write` available (push to the base repo): `docker buildx build --push` with `cache-from/to type=gha`; missing on a fork PR: build with the GHA cache and `--load`, export as an artifact the jobs `docker load`. Permissions `packages: write` on this job only.
- [ ] **Step 5:** Tests pass locally; push; the `images` job builds and pushes both images. Ask the user to make the two GHCR packages public (one-time). Re-run: the job finds both tags and finishes in seconds.

### Task 3: Every job in the container, no serial gate, verdict job

**Files:**
- Modify: `.github/workflows/ci.yml`, `Makefile` (targets each job calls), `src/go/internal/deploy/ci_local_parity_test.go`
- Modify: `scripts/run-browser-tests.sh` (GO fallback to `go` on PATH — the `.tools/go` path does not exist in CI)

**Interfaces:**
- Consumes: `needs.images.outputs.build|browser`.
- Produces: ci.yml jobs all `needs: [images]` with `container: image: ${{ needs.images.outputs.<kind> }}`; `concurrency: {group: ci-${{ github.ref }}, cancel-in-progress: true}`; job `verdict` (`needs:` every job, `if: always()`, fails if any needed job's result is not `success`).

- [ ] **Step 1: Extend guards** in `ci_local_parity_test.go`:
  - `TestCIRunsOnlyMakeTargets`: every `run:` line (comments stripped) starts with `make ` or is `./scripts/browser-test-matrix.sh …` (the matrix output step); replaces `TestCIBuildsOnlyThroughMakeCIBuild`.
  - `TestEveryCIJobRunsInTheImage`: every job except `images` and `verdict` has `container:` from `needs.images.outputs`; no `actions/setup-go`, `setup-node`, `apt-get` anywhere. (Supersedes `TestCIGoVersionComesFromGoMod`, which becomes vacuous — delete it.)
  - `TestVerdictNeedsEveryJob`: the `verdict` job's `needs` == every other job id.
- [ ] **Step 2:** Run → FAIL on the current ci.yml.
- [ ] **Step 3:** Rewrite ci.yml jobs: build (`make ci-build`), lint (`make lint`), test-go (Task 6 replaces with shards; for now `make test-go-tagged`), test-real-go (`make test-real-go`), race (`make test-race`), coverage-c (`make coverage-c-check`), refs (`make refs-ci`), browser matrix, verdict. Add the Makefile wrappers `test-go-tagged` (the tagged `./...` line of `test`), `test-real-go` (the Go line of `test-real`), `refs-ci` (fetch soft-fail + verify + check, the current three steps) and make `test`/`test-real` call them so they stay one definition. Add `--ipc=host --init` container options for browser jobs.
- [ ] **Step 4:** Guards pass; push; every job starts within ~1 min of `images`. Failures inside jobs are Task 9's list — record new ones in the baseline file.

### Task 4: C library invalidates the Go cache by content, not by wiping it

**Files:**
- Modify: `Makefile` (`$(C_LIB)` rule, a `CLIB_HASH` variable, `export CGO_CFLAGS`)
- Test: `src/go/internal/deploy/clib_cache_key_test.go`

**Interfaces:**
- Produces: `CLIB_HASH` = first 16 hex of sha256 over the sorted contents of `src/c/*.c src/c/*.h`; exported as `CGO_CFLAGS += -DBEATMO_CLIB_HASH=$(CLIB_HASH)` for every Go command the Makefile runs. The `go clean -cache` line is deleted.

- [ ] **Step 1: Failing test** `TestCLibHashKeysTheGoCache`: copies `Makefile` + `src/c` into `t.TempDir()`, runs `make -s print-clib-hash` twice (equal), touches a `.c` (mtime only → equal), edits one byte of a `.h` (→ different). Plus `TestCLibRuleNeverCleansTheGoCache`: the Makefile contains no `go clean -cache` / `$(GO) clean -cache`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement; add `print-clib-hash` target.
- [ ] **Step 4:** Real check: `go build ./cmd` twice (second is a no-op), edit a C comment, rebuild → cgo packages relink (`go build -x` shows the link), revert. Tests pass; push.

### Task 5: Caches

**Files:**
- Modify: `.github/workflows/ci.yml`, `Makefile` (`build/` objects reusable)

- [ ] **Step 1:** Each job restores/saves with `actions/cache@v4`: `GOCACHE` (key `go-<job>-<image tag>-<hash of go.sum + **/*.go>`, restore-keys `go-<job>-<image tag>-`), `GOMODCACHE` (`gomod-<go.sum hash>`), npm (`~/.npm`, lockfile hash), `build/*.o` + `libdrums.a` (`clib-<CLIB_HASH>`; restoring it must not trigger a rebuild — restore then `touch` objects newer than sources, or key `make` on the stamp), the refs corpus (`refs-<catalogue.json hash>`). Paths set via env `GOCACHE`/`GOMODCACHE` inside the container.
- [ ] **Step 2:** Push twice with no Go change: the second run's Go test steps report `(cached)` for unchanged packages and the build job finishes in under 2 min. Record both runs in the baseline file.

### Task 6: Shard the Go suites

**Files:**
- Create: `scripts/ci/go-shard.sh`
- Modify: `Makefile` (`test-go-shard`, `test-real-go-shard`, `coverage-merge`; `test`/`test-real` unchanged in behaviour), `.github/workflows/ci.yml`
- Test: `src/go/internal/deploy/go_shard_test.go`

**Interfaces:**
- Produces: `scripts/ci/go-shard.sh <i> <n> <tags-args…>` → prints lines `<pkg> <run-regex>`: packages split whole except `internal/ui` (and any package whose test list > `SPLIT_MIN` tests), whose tests are split by `go test -list .` into `n` contiguous-by-hash groups; regexes are `^(TestA|TestB…)$`.
- Produces: `make test-go-shard SHARD=i/N` and `make test-real-go-shard SHARD=i/N`, each writing `coverage/go-<i>.out`; `make coverage-merge` merges `coverage/go-*.out` into `coverage/go.out`.
- Timeouts: the same `-timeout` values as `make test` / `make test-real` (no separate CI number).

- [ ] **Step 1: Failing test** `TestShardsCoverEveryTestExactlyOnce`: for N ∈ {1, 3, 6} over a fixture module in `testdata/`, the union of shards' (pkg, test) pairs == the full `go test -list` set and no pair repeats. `TestShardPartitionIsStable`: same input → same output.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement the script + targets; ci.yml matrices `shard: [1..N]` with N from measured package times (start: tagged 6, real 4).
- [ ] **Step 4:** Push; compare the summed shard time against Task 1's serial time; tune N so no shard exceeds ~8 min. Coverage summary still published.

### Task 7: Browser jobs — all engines, perf batches, retries

**Files:**
- Modify: `scripts/browser-test-config.sh` (`RETRY_TESTS`), `scripts/run-browser-tests.sh` (retry loop + summary), `scripts/browser-test-matrix.sh` (perf tests one per batch, others by `--batch-size`), `.github/workflows/ci.yml`
- Test: `src/go/internal/deploy/browser_retry_config_test.go`

**Interfaces:**
- Produces: `RETRY_TESTS` array (performance class: every `*_bench`, `*_stress`, `webaudio_perf*`, `*_budget`, `landing_load`, `audio_lane_e2e`, `webaudio_timing_stress`, `webaudio_three_stage_latency`); runner env `BROWSER_TEST_RETRIES` (default 2) applies only to them; the end-of-run summary prints `RETRIED: <test> passed on attempt <k>`; in GitHub Actions the same line is appended to `$GITHUB_STEP_SUMMARY`.
- Produces: `make test-browser-batch TESTS="<files>"` (wasm-all once, then the runner with `--no-fail-fast --tests`), used by each matrix entry.

- [ ] **Step 1: Failing tests:** `TestRetryTestsExist` (every `RETRY_TESTS` entry is a file in `src/js`), `TestOnlyPerformanceTestsRetry` (no entry outside the perf naming set without an inline reason), `TestMatrixCoversEveryBrowserTest` (runs `browser-test-matrix.sh`, union of batches == all non-SKIP tests, perf tests alone in their batch).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. A retry re-runs only the failed test, with the same env and a fresh port base.
- [ ] **Step 4:** Push; every batch runs in the browser image with Chromium, Firefox, WebKit and Chrome available.

### Task 8: The browser C engine is checked against its source

**Files:**
- Modify: `Makefile` (`check-ma-js`: rebuild `src/js/drums.single.js` into `build/drums.single.js` with the image's emcc, `cmp` with the committed file; add to `ci-build`)

- [ ] **Step 1:** `make check-ma-js` in CI (build job, build image). If it differs: decide by evidence — rebuild locally with emcc 4.0.12 and compare; if the committed file is stale, commit the rebuilt one; if outputs differ between hosts with the same emcc, find the non-determinism (paths embedded? `-ffile-prefix-map`) and fix it, reporting to the user. Never weaken to a skip.
- [ ] **Step 2:** Locally without emcc, `ci-build` prints that the check needs emcc and fails (so `make test` makes the dependency visible); with emcc it passes.

### Task 9: Every job green

**Files:** whatever each root cause needs.

- [ ] **Step 1:** For each failure in the baseline file, in order Lint → test-go → test-real-go → race → coverage-c → refs → browser: reproduce locally (natively, or the same command the job runs), find the root cause (systematic-debugging), fix, add/keep the guard that would have caught it, push, confirm the job turns green. One root cause per commit.
- [ ] **Step 2:** A performance test that fails all retries because of runner hardware (proved by its numbers vs a local run): stop and report to the user with the numbers — that is the paid-runner trigger, not a gate change.

### Task 10: Measure, tune, report

**Files:**
- Modify: `docs/superpowers/plans/2026-11-07-ci-containers-baseline.md` (final table), `CLAUDE.md` (Quick Start rows for `make ci-build`, `make dev`, `make doctor`, `make test-go-shard`; the CI section)

- [ ] **Step 1:** Three consecutive pushes: cold-ish (cache miss), warm (no Go change), a UI-only change. Table: per job time, the critical path, total wall clock.
- [ ] **Step 2:** If a normal push exceeds 15 min, rebalance shards/batches from the measured times; if the critical path is a single unsplittable test or job on hardware limits, report it with the paid-runner option and its cost.
- [ ] **Step 3:** Update CLAUDE.md; push; final run green.
