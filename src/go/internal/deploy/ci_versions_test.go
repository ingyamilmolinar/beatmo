package deploy

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readVersions(t *testing.T) map[string]string {
	t.Helper()
	f, err := os.Open(filepath.Join(repoRoot(t), "ci", "versions.env"))
	if err != nil {
		t.Fatalf("ci/versions.env: %v", err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("ci/versions.env: malformed line %q", line)
		}
		out[k] = v
	}
	return out
}

func dockerfiles(t *testing.T) map[string]string {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(repoRoot(t), "ci", "images", "*.Dockerfile"))
	if len(paths) == 0 {
		t.Fatal("no ci/images/*.Dockerfile")
	}
	out := map[string]string{}
	for _, p := range paths {
		out[filepath.Base(p)] = mustRead(t, p)
	}
	return out
}

var (
	argRe       = regexp.MustCompile(`(?m)^ARG\s+([A-Z0-9_]+)`)
	versionLit  = regexp.MustCompile(`(?:^|[^0-9.])\d+\.\d+(?:\.\d+)?\b`)
	digestLit   = regexp.MustCompile(`sha256:[0-9a-f]{64}`)
	semverInYML = regexp.MustCompile(`(?i)\b(go|node|playwright|chrome|emsdk|golangci-lint|debian)\b[^\n]*\b\d+\.\d+(\.\d+)?\b`)
)

func TestVersionsFileIsTheOnlyPin(t *testing.T) {
	versions := readVersions(t)
	for name, src := range dockerfiles(t) {
		for _, m := range argRe.FindAllStringSubmatch(src, -1) {
			if _, ok := versions[m[1]]; !ok && m[1] != "BUILD_IMAGE" {
				t.Errorf("%s declares ARG %s, which ci/versions.env does not define", name, m[1])
			}
		}
		code := stripDockerComments(src)
		if v := versionLit.FindString(code); v != "" {
			t.Errorf("%s hard-codes version %q — move it to ci/versions.env", name, v)
		}
		if d := digestLit.FindString(code); d != "" {
			t.Errorf("%s hard-codes digest %s — pin it in ci/versions.env", name, d)
		}
	}
	for _, f := range workflowFiles(t, repoRoot(t)) {
		src := stripYAMLComments(mustRead(t, f))
		if m := semverInYML.FindString(src); m != "" {
			t.Errorf("%s names a tool version (%q) — tool versions live in ci/versions.env", filepath.Base(f), strings.TrimSpace(m))
		}
	}
}

func TestImageGoMatchesGoModToolchain(t *testing.T) {
	gomod := mustRead(t, filepath.Join(repoRoot(t), "src", "go", "go.mod"))
	m := regexp.MustCompile(`(?m)^toolchain go(\S+)$`).FindStringSubmatch(gomod)
	if m == nil {
		t.Fatal("src/go/go.mod has no toolchain line")
	}
	if got := readVersions(t)["GO_VERSION"]; got != m[1] {
		t.Errorf("GO_VERSION=%q, go.mod toolchain go%s", got, m[1])
	}
}

func TestImagePlaywrightMatchesLockfile(t *testing.T) {
	var lock struct {
		Packages map[string]struct{ Version string } `json:"packages"`
	}
	if err := json.Unmarshal([]byte(mustRead(t, filepath.Join(repoRoot(t), "src", "js", "package-lock.json"))), &lock); err != nil {
		t.Fatal(err)
	}
	want := lock.Packages["node_modules/playwright"].Version
	if want == "" {
		t.Fatal("package-lock.json has no node_modules/playwright")
	}
	if got := readVersions(t)["PLAYWRIGHT_VERSION"]; got != want {
		t.Errorf("PLAYWRIGHT_VERSION=%q, package-lock.json playwright %s", got, want)
	}
}

func TestDebianImagePinnedByDigest(t *testing.T) {
	got := readVersions(t)["DEBIAN_IMAGE"]
	if !regexp.MustCompile(`^debian:12-slim@sha256:[0-9a-f]{64}$`).MatchString(got) {
		t.Errorf("DEBIAN_IMAGE=%q, want debian:12-slim@sha256:<64 hex>", got)
	}
}

func stripDockerComments(src string) string {
	var b strings.Builder
	for _, l := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			b.WriteString(l + "\n")
		}
	}
	return b.String()
}

// golangci-lint analysing under an older Go than go.mod's misreads the code:
// a stale `go: "1.23"` turned every t.Fatal-guarded nil check into a SA5011.
func TestLintConfigTakesGoFromGoMod(t *testing.T) {
	cfg := mustRead(t, filepath.Join(repoRoot(t), ".golangci.yml"))
	if regexp.MustCompile(`(?m)^\s+go:\s`).MatchString(cfg) {
		t.Error(".golangci.yml pins `run.go` — leave it out so the linter reads go.mod")
	}
}
