package deploy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// shardFixture writes a module whose package b is big enough (SPLIT_MIN=4 in
// the tests) to be split by test name, while a and c are assigned whole.
func shardFixture(t *testing.T) (dir string, want map[string]bool) {
	t.Helper()
	dir = t.TempDir()
	want = map[string]bool{}
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.24\n")
	for pkg, tests := range map[string][]string{"a": {"TestA1", "TestA2", "TestShared"}, "b": {"TestB1", "TestB2", "TestB3", "TestB4", "TestB5", "TestShared"}, "c": {"TestC1"}} {
		var b strings.Builder
		fmt.Fprintf(&b, "package %s\n\nimport \"testing\"\n\n", pkg)
		for _, name := range tests {
			fmt.Fprintf(&b, "func %s(t *testing.T) {}\n", name)
			want["example.com/m/"+pkg+" "+name] = true
		}
		write(pkg+"/x_test.go", b.String())
	}
	return dir, want
}

func shardPlan(t *testing.T, dir, shard string) string {
	t.Helper()
	cmd := exec.Command("bash", filepath.Join(repoRoot(t), "scripts", "ci", "go-shard.sh"), "plan", shard)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "SPLIT_MIN=4", "GOFLAGS=-mod=mod", "GO="+goBin())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go-shard.sh plan %s: %v\n%s", shard, err, out)
	}
	return string(out)
}

// pairs expands a plan into the (package, test) pairs it runs.
func pairs(t *testing.T, plan string, all map[string]bool) []string {
	t.Helper()
	var out []string
	runRe := regexp.MustCompile(`^\^\((.*)\)\$$`)
	for _, line := range strings.Split(strings.TrimSpace(plan), "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) == 0:
		case f[0] == "WHOLE":
			for _, pkg := range f[1:] {
				for k := range all {
					if strings.HasPrefix(k, pkg+" ") {
						out = append(out, k)
					}
				}
			}
		case f[0] == "RUN" && len(f) == 3:
			m := runRe.FindStringSubmatch(f[2])
			if m == nil {
				t.Fatalf("RUN regex %q is not ^(a|b)$", f[2])
			}
			for _, name := range strings.Split(m[1], "|") {
				out = append(out, f[1]+" "+name)
			}
		default:
			t.Fatalf("unexpected plan line %q", line)
		}
	}
	return out
}

func TestShardsCoverEveryTestExactlyOnce(t *testing.T) {
	dir, want := shardFixture(t)
	for _, n := range []int{1, 3, 6} {
		seen := map[string]int{}
		for i := 1; i <= n; i++ {
			for _, p := range pairs(t, shardPlan(t, dir, fmt.Sprintf("%d/%d", i, n)), want) {
				seen[p]++
			}
		}
		for k := range want {
			if seen[k] != 1 {
				t.Errorf("N=%d: %s runs in %d shards, want 1", n, k, seen[k])
			}
		}
		for k := range seen {
			if !want[k] {
				t.Errorf("N=%d: shards run %s, which does not exist", n, k)
			}
		}
	}
}

func TestShardPartitionIsStable(t *testing.T) {
	dir, _ := shardFixture(t)
	if a, b := shardPlan(t, dir, "2/3"), shardPlan(t, dir, "2/3"); a != b {
		t.Errorf("the same shard planned twice differs:\n%s\n---\n%s", a, b)
	}
}

func TestBigPackagesAreSplitByTestName(t *testing.T) {
	dir, _ := shardFixture(t)
	var plans []string
	for i := 1; i <= 3; i++ {
		plans = append(plans, shardPlan(t, dir, fmt.Sprintf("%d/3", i)))
	}
	all := strings.Join(plans, "\n")
	if strings.Contains(all, "WHOLE") && slices.ContainsFunc(strings.Split(all, "\n"), func(l string) bool {
		return strings.HasPrefix(l, "WHOLE") && strings.Contains(l, "example.com/m/b")
	}) {
		t.Error("package b (over SPLIT_MIN) was assigned whole instead of split by test name")
	}
	if !strings.Contains(all, "RUN example.com/m/b ") {
		t.Errorf("package b never appears as a RUN slice:\n%s", all)
	}
}
