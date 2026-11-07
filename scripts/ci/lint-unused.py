#!/usr/bin/env python3
"""Reports code that is unused in EVERY build configuration that compiles it.

golangci-lint's `unused` sees one configuration, so a helper whose only callers
build under `-tags test` or on js/wasm reads as dead. Run from src/go (or with
--from DIR holding <config>.files.json / <config>.issues.json, for tests).
"""
import json
import os
import shutil
import subprocess
import sys
import tempfile

# js runs without test files: most of them are native-only and do not compile there.
CONFIGS = [
    ("linux", {}, [], True),
    ("linux-test", {"MODFILE": "go.test.mod"}, ["test"], True),
    ("js", {"GOOS": "js", "GOARCH": "wasm", "CGO_ENABLED": "0"}, [], False),
    ("js-release", {"GOOS": "js", "GOARCH": "wasm", "CGO_ENABLED": "0"}, ["release", "noscenes", "parityfatal"], False),
]

SRC_FMT = "{{$d:=.Dir}}{{range .GoFiles}}{{$d}}/{{.}}\n{{end}}{{range .CgoFiles}}{{$d}}/{{.}}\n{{end}}"
TEST_FMT = "{{$d:=.Dir}}{{range .TestGoFiles}}{{$d}}/{{.}}\n{{end}}{{range .XTestGoFiles}}{{$d}}/{{.}}\n{{end}}"


# golangci-lint has no -modfile, and an inherited GOFLAGS=-modfile breaks the
# GO111MODULE=off probe go/packages runs, so a wrapper adds it to our module's
# own build commands only.
WRAPPER = """#!/bin/sh
if [ "$GO111MODULE" != off ]; then
  case "$PWD" in {root}|{root}/*)
    case "$1" in list|build|vet|test) c=$1; shift; exec {go} "$c" -modfile={modfile} "$@";; esac;;
  esac
fi
exec {go} "$@"
"""


def collect(name, env, tags, tests):
    go = shutil.which(os.environ.get("GO", "go"))
    lint = os.environ.get("GOLANGCI_LINT", "golangci-lint")
    e = dict(os.environ, **{k: v for k, v in env.items() if k != "MODFILE"})
    modargs = []
    if "MODFILE" in env:
        modfile = os.path.abspath(env["MODFILE"])
        modargs = ["-modfile=" + modfile]
        bindir = tempfile.mkdtemp()
        with open(os.path.join(bindir, "go"), "w") as f:
            f.write(WRAPPER.format(root=os.getcwd(), go=go, modfile=modfile))
        os.chmod(os.path.join(bindir, "go"), 0o755)
        e["PATH"] = bindir + os.pathsep + e["PATH"]
    tagargs = ["-tags", ",".join(tags)] if tags else []
    fmt = SRC_FMT + TEST_FMT if tests else SRC_FMT
    out = subprocess.run([go, "list", *modargs, *tagargs, "-f", fmt, "./..."], env=e, check=True,
                         capture_output=True, text=True).stdout
    root = os.getcwd() + "/"
    files = sorted({p[len(root):] for p in out.split() if p.startswith(root)})
    lint_tags = ["--build-tags", ",".join(tags)] if tags else []
    res = subprocess.run([lint, "run", "--enable-only", "unused", *lint_tags, "--tests=" + str(tests).lower(),
                          "--max-issues-per-linter=0",
                          "--max-same-issues=0", "--output.json.path", "stdout", "--output.text.path", "stderr",
                          "./..."], env=e, capture_output=True, text=True)
    if not res.stdout.strip():
        sys.exit(f"golangci-lint ({name}) produced no report:\n{res.stderr}")
    report = json.loads(res.stdout.splitlines()[0])
    broken = [i for i in report.get("Issues") or [] if i["FromLinter"] == "typecheck"]
    if broken:
        # A package that does not type-check reports nothing, which would read as "used".
        sys.exit(f"golangci-lint ({name}) could not type-check: " +
                 "; ".join(f'{i["Pos"]["Filename"]}:{i["Pos"]["Line"]}: {i["Text"]}' for i in broken))
    return files, report


def rel(path):
    return path[len("src/go/"):] if path.startswith("src/go/") else path


def main():
    data = {}
    if len(sys.argv) == 3 and sys.argv[1] == "--from":
        for name, _, _, _ in CONFIGS:
            with open(os.path.join(sys.argv[2], name + ".files.json")) as f:
                files = json.load(f)
            with open(os.path.join(sys.argv[2], name + ".issues.json")) as f:
                data[name] = (set(files), json.load(f))
    else:
        for name, env, tags, tests in CONFIGS:
            files, issues = collect(name, env, tags, tests)
            data[name] = (set(files), issues)

    reported = {}
    for name, (_, issues) in data.items():
        for i in issues.get("Issues") or []:
            key = (rel(i["Pos"]["Filename"]), i["Pos"]["Line"], i["Text"])
            reported.setdefault(key, set()).add(name)
    dead = []
    for (path, line, text), seen in sorted(reported.items()):
        building = {name for name, (files, _) in data.items() if path in files}
        if building and building <= seen:
            dead.append(f"{path}:{line}: {text}")
    for d in dead:
        print(d)
    if dead:
        print(f"{len(dead)} symbol(s) unused in every build configuration that compiles them", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
