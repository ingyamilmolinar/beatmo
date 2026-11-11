package deploy

// ci_make_target_drift_test.go — every `make <target>` a CI workflow invokes
// must exist in the Makefile.
//
// The corpus rewrite deleted `fetch-synth-refs` and added `fetch-refs`, but
// .github/workflows/ci.yml kept calling the old name. The call was written as
//
//	run: make fetch-synth-refs || echo "::warning::..."
//
// so the `||` swallowed make's "No rule to make target" and CI stayed green
// while fetching nothing -- every reference-dependent test silently degraded
// from "passing" to "skipped" for as long as nobody read the warnings. A
// deliberately non-fatal step is exactly where a renamed target hides longest,
// which is why this is checked structurally instead of relied on to fail.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// makeTargetRe matches a target definition at the start of a Makefile line:
// `name:` or `name: deps`, excluding `::` (pattern rules) and `=` (variable
// assignments like `NAME := ...`).
var makeTargetRe = regexp.MustCompile(`(?m)^([a-zA-Z0-9][a-zA-Z0-9._-]*)\s*:(?:[^:=]|$)`)

// makeInvocationRe matches a `make` invocation inside a workflow `run:` block,
// capturing the first non-flag, non-assignment word after it.
var makeInvocationRe = regexp.MustCompile(`\bmake\s+((?:-[a-zA-Z]\S*\s+|[A-Z_][A-Z0-9_]*=\S*\s+)*)([a-zA-Z][a-zA-Z0-9._-]*)`)

// makefileTargets returns every target defined in the Makefile, including
// those declared only via .PHONY.
func makefileTargets(t *testing.T, root string) map[string]bool {
	t.Helper()
	src := mustRead(t, filepath.Join(root, "Makefile"))
	out := map[string]bool{}
	for _, m := range makeTargetRe.FindAllStringSubmatch(src, -1) {
		out[m[1]] = true
	}
	// .PHONY lines name targets too; a target defined by a `define`d canned
	// recipe still needs its .PHONY entry, so this also covers those.
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, ".PHONY:") {
			continue
		}
		for _, f := range strings.Fields(strings.TrimPrefix(line, ".PHONY:")) {
			out[f] = true
		}
	}
	return out
}

// workflowFiles lists the CI workflow YAMLs.
func workflowFiles(t *testing.T, root string) []string {
	t.Helper()
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no workflows directory: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if ext := filepath.Ext(e.Name()); ext == ".yml" || ext == ".yaml" {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

// stripYAMLComments drops whole-line `#` comments.
//
// Only whole-line comments, deliberately: a trailing `#` inside a `run:` shell
// command is a shell comment, not a YAML one, and guessing which is which
// would either miss real invocations or mangle commands. Whole-line comments
// are where prose about a target lives -- including the comment in ci.yml
// explaining this very failure mode, which the scanner would otherwise read as
// a live invocation of the deleted target.
func stripYAMLComments(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			out = append(out, "")
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func TestCIWorkflowsOnlyInvokeExistingMakeTargets(t *testing.T) {
	root := repoRoot(t)
	targets := makefileTargets(t, root)
	if len(targets) == 0 {
		t.Fatal("parsed no targets out of the Makefile — the target regexp is wrong")
	}

	files := workflowFiles(t, root)
	if len(files) == 0 {
		t.Skip("no workflow files to check")
	}

	checked := 0
	for _, f := range files {
		src := stripYAMLComments(mustRead(t, f))
		for _, m := range makeInvocationRe.FindAllStringSubmatch(src, -1) {
			target := m[2]
			// `make -C dir` and bare `make` (default goal) have no target here.
			if target == "" {
				continue
			}
			checked++
			if !targets[target] {
				t.Errorf("%s invokes `make %s`, which is not a target in the Makefile — "+
					"a non-fatal `|| echo` step will swallow this and CI will stay green "+
					"while doing nothing", filepath.Base(f), target)
			}
		}
	}
	if checked == 0 {
		t.Error("found no `make <target>` invocations in any workflow — the invocation regexp is wrong, " +
			"so a passing result proves nothing")
	}
	t.Logf("checked %d `make <target>` invocation(s) across %d workflow file(s)", checked, len(files))
}
