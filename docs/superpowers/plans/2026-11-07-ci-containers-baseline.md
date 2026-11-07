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
