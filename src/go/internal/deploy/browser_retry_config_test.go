package deploy

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Performance tests are the only class CI retries; a name outside this shape
// is a correctness test, whose flake is a bug to fix, not to re-run.
var perfTestName = regexp.MustCompile(`(bench|stress|perf|budget|latency|timing|landing_load|load_startup|audio_lane_e2e)`)

func bashList(t *testing.T, array string) []string {
	t.Helper()
	cmd := exec.Command("bash", "-c", `source scripts/browser-test-config.sh && printf '%s\n' "${`+array+`[@]}"`)
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("browser-test-config.sh %s: %v\n%s", array, err, out)
	}
	return strings.Fields(string(out))
}

func TestRetryTestsExist(t *testing.T) {
	retry := bashList(t, "RETRY_TESTS")
	if len(retry) == 0 {
		t.Fatal("RETRY_TESTS is empty")
	}
	for _, name := range retry {
		if _, err := os.Stat(filepath.Join(repoRoot(t), "src", "js", name)); err != nil {
			t.Errorf("RETRY_TESTS names %s, which does not exist — it would silently retry nothing", name)
		}
	}
}

func TestOnlyPerformanceTestsRetry(t *testing.T) {
	retry := bashList(t, "RETRY_TESTS")
	for _, name := range retry {
		if !perfTestName.MatchString(name) {
			t.Errorf("RETRY_TESTS includes %s, which is not a performance test", name)
		}
	}
	skip := bashList(t, "SKIP_TESTS")
	all, _ := filepath.Glob(filepath.Join(repoRoot(t), "src", "js", "*.browser.test.js"))
	for _, p := range all {
		name := filepath.Base(p)
		if perfTestName.MatchString(name) && !slices.Contains(retry, name) && !slices.Contains(skip, name) {
			t.Errorf("%s looks like a performance test but is not in RETRY_TESTS", name)
		}
	}
}

func TestMatrixCoversEveryBrowserTest(t *testing.T) {
	cmd := exec.Command("bash", "scripts/browser-test-matrix.sh", "--batch-size", "4")
	cmd.Dir = repoRoot(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("browser-test-matrix.sh: %v", err)
	}
	var m struct {
		Include []struct {
			Tests string `json:"tests"`
		} `json:"include"`
	}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("matrix is not JSON: %v\n%s", err, out)
	}
	retry := bashList(t, "RETRY_TESTS")
	skip := bashList(t, "SKIP_TESTS")
	seen := map[string]int{}
	for _, b := range m.Include {
		tests := strings.Fields(b.Tests)
		for _, name := range tests {
			seen[name]++
			if slices.Contains(retry, name) && len(tests) != 1 {
				t.Errorf("performance test %s shares a batch with %v — it must run alone", name, tests)
			}
		}
	}
	all, _ := filepath.Glob(filepath.Join(repoRoot(t), "src", "js", "*.browser.test.js"))
	for _, p := range all {
		name := filepath.Base(p)
		want := 1
		if slices.Contains(skip, name) {
			want = 0
		}
		if seen[name] != want {
			t.Errorf("%s appears in %d batches, want %d", name, seen[name], want)
		}
	}
}

// Runs the real runner over stand-in tests (bash scripts in place of node) so
// the retry loop itself is exercised: a performance test that fails once is
// retried and passes; a correctness test that fails is never retried.
func TestRunnerRetriesOnlyPerformanceTests(t *testing.T) {
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("webaudio_perf.browser.test.js", `n=$(cat `+counter+` 2>/dev/null || echo 0); echo $((n+1)) > `+counter+`; [ "$n" -ge 1 ]`+"\n")
	write("e2e_transport.browser.test.js", "echo once >> "+filepath.Join(dir, "e2e_runs")+"; echo 'the e2e failure detail'; exit 1\n")

	run := func(tests string) (string, error) {
		cmd := exec.Command("bash", "scripts/run-browser-tests.sh", "--no-fail-fast", "--tests", tests)
		cmd.Dir = repoRoot(t)
		cmd.Env = append(os.Environ(), "BROWSER_TEST_DIR="+dir, "BROWSER_TEST_NODE=bash", "GO=true", "GITHUB_STEP_SUMMARY=")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("webaudio_perf.browser.test.js")
	if err != nil {
		t.Fatalf("a performance test that passes on retry failed the run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "RETRIED: webaudio_perf.browser.test.js passed on attempt 2") {
		t.Errorf("the retry is not reported:\n%s", out)
	}
	out, err = run("e2e_transport.browser.test.js")
	if err == nil {
		t.Fatalf("a failing correctness test passed the run:\n%s", out)
	}
	if !strings.Contains(out, "Failed tests:\n  e2e_transport.browser.test.js") || !strings.Contains(out, "the e2e failure detail") {
		t.Errorf("the failed test is not named, or its log is not printed:\n%s", out)
	}
	if runs := mustRead(t, filepath.Join(dir, "e2e_runs")); strings.Count(runs, "once") != 1 {
		t.Errorf("a correctness test was retried (%d runs)", strings.Count(runs, "once"))
	}
}
