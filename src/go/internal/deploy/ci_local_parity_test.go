package deploy

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A build command written inline in a workflow is one `make test` never runs.
var rawGoBuildRe = regexp.MustCompile(`(?m)^\s*(?:run:\s*)?[^#\n]*\bgo\s+build\b`)

func TestCIBuildsOnlyThroughMakeCIBuild(t *testing.T) {
	root := repoRoot(t)
	for _, f := range workflowFiles(t, root) {
		src := stripYAMLComments(mustRead(t, f))
		if loc := rawGoBuildRe.FindStringIndex(src); loc != nil {
			t.Errorf("%s runs `go build` directly (%q) — put it in `make ci-build` so `make test` runs it too",
				filepath.Base(f), strings.TrimSpace(src[loc[0]:loc[1]]))
		}
	}
	ci := stripYAMLComments(mustRead(t, filepath.Join(root, ".github", "workflows", "ci.yml")))
	if !strings.Contains(ci, "make ci-build") {
		t.Error("ci.yml no longer runs `make ci-build` — the Build job and `make test` have diverged")
	}
}

func TestMakeTestRunsCIBuild(t *testing.T) {
	src := mustRead(t, filepath.Join(repoRoot(t), "Makefile"))
	if !makefileTargets(t, repoRoot(t))["ci-build"] {
		t.Fatal("the Makefile has no `ci-build` target")
	}
	recipe := targetRecipe(src, "test")
	if recipe == "" {
		t.Fatal("found no `test:` target in the Makefile")
	}
	if !strings.Contains(recipe, "$(MAKE) ci-build") {
		t.Errorf("`make test` does not run `$(MAKE) ci-build`, so a local pass no longer implies a CI build pass:\n%s", recipe)
	}
}

// A literal go-version runs CI on a different compiler than the toolchain
// go.mod pins for every developer.
func TestCIGoVersionComesFromGoMod(t *testing.T) {
	root := repoRoot(t)
	setups := 0
	for _, f := range workflowFiles(t, root) {
		src := stripYAMLComments(mustRead(t, f))
		if strings.Contains(src, "go-version:") {
			t.Errorf("%s pins a literal `go-version:` — use `go-version-file: src/go/go.mod`", filepath.Base(f))
		}
		setups += strings.Count(src, "actions/setup-go@")
		if n, m := strings.Count(src, "actions/setup-go@"), strings.Count(src, "go-version-file: src/go/go.mod"); n != m {
			t.Errorf("%s: %d setup-go step(s) but %d read src/go/go.mod", filepath.Base(f), n, m)
		}
	}
	if setups == 0 {
		t.Error("found no setup-go step in any workflow — the scan is reading the wrong files")
	}
	if !strings.Contains(mustRead(t, filepath.Join(root, "src", "go", "go.mod")), "\ntoolchain go") {
		t.Error("src/go/go.mod has no toolchain line, so go-version-file cannot pin an exact compiler")
	}
}

// targetRecipe returns the tab-indented recipe lines (comments included) under `name:`.
func targetRecipe(makefile, name string) string {
	lines := strings.Split(makefile, "\n")
	for i, l := range lines {
		if !strings.HasPrefix(l, name+":") || strings.HasPrefix(l, name+"::") {
			continue
		}
		var b strings.Builder
		for _, r := range lines[i+1:] {
			if strings.HasPrefix(r, "\t") || strings.HasPrefix(r, "#") {
				b.WriteString(r + "\n")
				continue
			}
			break
		}
		return b.String()
	}
	return ""
}

// The bundled toolchain exists only on a developer machine; a recipe that names
// it directly fails with "command not found" in CI and in the image.
func TestMakefileRunsGoOnlyThroughTheGOVariable(t *testing.T) {
	for i, line := range strings.Split(mustRead(t, filepath.Join(repoRoot(t), "Makefile")), "\n") {
		if strings.Contains(line, ".tools/go/bin/go") && !strings.HasPrefix(line, "GO ?=") {
			t.Errorf("Makefile:%d names .tools/go directly — use $(GO): %s", i+1, strings.TrimSpace(line))
		}
	}
}
