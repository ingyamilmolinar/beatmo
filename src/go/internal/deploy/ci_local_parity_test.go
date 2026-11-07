package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	Jobs map[string]struct {
		Needs     any `yaml:"needs"`
		Container any `yaml:"container"`
		Steps     []struct {
			Uses string `yaml:"uses"`
			Run  string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readCI(t *testing.T) workflow {
	t.Helper()
	var w workflow
	if err := yaml.Unmarshal([]byte(mustRead(t, filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))), &w); err != nil {
		t.Fatal(err)
	}
	if len(w.Jobs) == 0 {
		t.Fatal("ci.yml has no jobs")
	}
	return w
}

func needsList(v any) []string {
	switch n := v.(type) {
	case string:
		return []string{n}
	case []any:
		var out []string
		for _, x := range n {
			out = append(out, x.(string))
		}
		return out
	}
	return nil
}

// A command written inline in a workflow is one `make test` never runs.
func TestCIRunsOnlyMakeTargets(t *testing.T) {
	sawCIBuild := false
	for id, job := range readCI(t).Jobs {
		for _, st := range job.Steps {
			run := strings.TrimSpace(st.Run)
			if run == "" {
				continue
			}
			if strings.Contains(run, "\n") || (!strings.HasPrefix(run, "make ")) {
				t.Errorf("job %s runs %q — CI steps must be one `make <target>` so they run the same locally", id, run)
			}
			sawCIBuild = sawCIBuild || run == "make ci-build"
		}
	}
	if !sawCIBuild {
		t.Error("ci.yml no longer runs `make ci-build` — the Build job and `make test` have diverged")
	}
}

func TestEveryCIJobRunsInTheImage(t *testing.T) {
	src := stripYAMLComments(mustRead(t, filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml")))
	for _, banned := range []string{"actions/setup-go", "actions/setup-node", "apt-get"} {
		if strings.Contains(src, banned) {
			t.Errorf("ci.yml uses %s — every tool comes from the pinned image", banned)
		}
	}
	for id, job := range readCI(t).Jobs {
		if hostJobs[id] {
			continue
		}
		c, _ := yaml.Marshal(job.Container)
		if !strings.Contains(string(c), "needs.images.outputs.") {
			t.Errorf("job %s does not run in the CI image (container: %s)", id, strings.TrimSpace(string(c)))
		}
		if !slices.Contains(needsList(job.Needs), "images") {
			t.Errorf("job %s does not need `images`, so its container ref is empty", id)
		}
	}
}

// Jobs that legitimately run on the runner host: one builds the images, one
// only prints the browser matrix, one only reads the other jobs' results.
var hostJobs = map[string]bool{"images": true, "browser-matrix": true, "verdict": true}

func TestVerdictNeedsEveryJob(t *testing.T) {
	w := readCI(t)
	v, ok := w.Jobs["verdict"]
	if !ok {
		t.Fatal("ci.yml has no `verdict` job — the one required check")
	}
	got := needsList(v.Needs)
	for id := range w.Jobs {
		if id != "verdict" && !slices.Contains(got, id) {
			t.Errorf("verdict does not need %s, so a failure there would not block a merge", id)
		}
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

// A target that `go run`s a program linking libdrums.a works on any machine that
// built the library before, and fails to link on a fresh clone or CI runner.
func TestTargetsThatLinkCgoDependOnTheCLib(t *testing.T) {
	root := repoRoot(t)
	src := mustRead(t, filepath.Join(root, "Makefile"))
	runRe := regexp.MustCompile(`\$\(GO\) run (\./cmd/[A-Za-z0-9_./-]+)`)
	headerRe := regexp.MustCompile(`(?m)^([a-zA-Z0-9][a-zA-Z0-9._-]*):([^:=\n][^\n]*|)$`)
	cgo := map[string]bool{}
	linksCgo := func(pkg string) bool {
		if v, ok := cgo[pkg]; ok {
			return v
		}
		cmd := exec.Command(goBin(), "list", "-deps", "-f", "{{if .CgoFiles}}{{.ImportPath}}{{end}}", pkg)
		cmd.Dir = filepath.Join(root, "src", "go")
		out, err := cmd.Output()
		cgo[pkg] = err == nil && strings.Contains(string(out), "github.com/ingyamilmolinar/beatmo/")
		return cgo[pkg]
	}
	checked := 0
	for _, h := range headerRe.FindAllStringSubmatchIndex(src, -1) {
		name, deps := src[h[2]:h[3]], src[h[4]:h[5]]
		deps, _, _ = strings.Cut(deps, "##")
		recipe := targetRecipe(src, name)
		for _, line := range strings.Split(recipe, "\n") {
			m := runRe.FindStringSubmatch(line)
			if m == nil || strings.Contains(line, "-tags test") {
				continue
			}
			checked++
			if linksCgo(m[1]) && !strings.Contains(deps, "$(C_LIB)") {
				t.Errorf("target %s runs %s, which links libdrums.a, but does not depend on $(C_LIB)", name, m[1])
				break
			}
		}
	}
	if checked == 0 {
		t.Fatal("found no `$(GO) run ./cmd/...` recipes — the scan is broken")
	}
}

func goBin() string {
	if g := os.Getenv("GO"); g != "" {
		return g
	}
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}
