package deploy

// Every make <target> CI invokes must exist: a renamed target behind a non-fatal || in ci.yml failed with "No rule to
// make target" while CI stayed green, so a deliberately non-fatal step is where this hides longest.

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
	// .PHONY names targets too, including those defined by a define'd canned recipe.
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

// Whole-line comments only: a trailing # in a run: block is a shell comment, and guessing would miss invocations or
// mangle commands. Prose about a deleted target lives in whole-line comments and must not read as a live invocation.
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
