package rank

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractionReadinessPerFile(t *testing.T) {
	m := loadFixture(t)
	var god *TypeStat
	for _, ts := range m.Types {
		if ts.Name == "God" {
			god = ts
		}
	}
	if god == nil {
		t.Fatal("God type missing")
	}
	if god.Methods != 4 || god.FilesSpanned != 3 || god.Fields != 12 {
		t.Errorf("God methods=%d files=%d fields=%d, want 4/3/12", god.Methods, god.FilesSpanned, god.Fields)
	}
	got := map[string]int{}
	for _, f := range god.PerFile {
		got[filepath.Base(f.File)] = f.Exclusive
	}
	if got["god_fx.go"] != 9 || got["god_menu.go"] != 2 || got["god.go"] != 0 {
		t.Errorf("exclusive fields per file = %v, want god_fx.go:9 god_menu.go:2 god.go:0", got)
	}
}

func TestAreaSplitRule(t *testing.T) {
	var files []string
	for i := 0; i < 20; i++ {
		files = append(files, "x.go")
	}
	files = files[:0]
	add := func(prefix string, n int) {
		for i := 0; i < n; i++ {
			files = append(files, prefix+string(rune('a'+i))+".go")
		}
	}
	add("synth_panel_", 4)
	add("synth_dial_", 3)
	add("synth_", 20) // synth_* totals 27 files > 25: split by two tokens
	add("game_", 5)
	got := map[string]int{}
	for _, f := range files {
		got[areaOf("internal/ui", f, files)]++
	}
	want := map[string]int{
		"internal/ui/synth_panel_*": 4,
		"internal/ui/synth_dial_*":  3,
		"internal/ui/synth_*":       20,
		"internal/ui/game_*":        5,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("area %s = %d files, want %d (all: %v)", k, got[k], v, got)
		}
	}
	small := []string{"a.go", "b_x.go"}
	if a := areaOf("core", "b_x.go", small); a != "core" {
		t.Errorf("small package area = %q, want the package itself", a)
	}
}

func TestGini(t *testing.T) {
	if g := gini([]int{0, 0, 10}); math.Abs(g-2.0/3) > 1e-9 {
		t.Errorf("gini = %v, want 2/3", g)
	}
	if g := gini([]int{5, 5, 5}); g != 0 {
		t.Errorf("gini of equal = %v, want 0", g)
	}
}

func TestCoverageMergesProfilesAndKeepsUnknown(t *testing.T) {
	dir := t.TempDir()
	// Run (core/core.go) is covered only in profile B; deadFunc is in neither.
	a := "mode: set\nexample.com/rankmod/core/core.go:4.12,8.2 3 0\n"
	b := "mode: set\nexample.com/rankmod/core/core.go:4.12,8.2 3 1\n"
	pa, pb := filepath.Join(dir, "a.out"), filepath.Join(dir, "b.out")
	if err := os.WriteFile(pa, []byte(a), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pb, []byte(b), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := fixtureConfig()
	cfg.Fast = true
	cfg.Covers = []string{pa, pb}
	m, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c := oneUnit(t, m, "Run").Coverage; c == nil || *c != 1 {
		t.Errorf("Run coverage = %v, want 1 (covered in profile B)", c)
	}
	if c := oneUnit(t, m, "deadFunc").Coverage; c != nil {
		t.Errorf("deadFunc coverage = %v, want unknown (nil)", *c)
	}
}

// A profile written before a source file changed says nothing about it.
func TestCoverageIgnoresProfilesOlderThanTheSource(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "old.out")
	if err := os.WriteFile(p, []byte("mode: set\nexample.com/rankmod/core/core.go:4.12,8.2 3 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	cfg := fixtureConfig()
	cfg.Fast = true
	cfg.Covers = []string{p}
	m, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c := oneUnit(t, m, "Run").Coverage; c != nil {
		t.Errorf("Run coverage = %v from a stale profile, want unknown", *c)
	}
	if len(m.Warnings) != 1 || !strings.Contains(m.Warnings[0], "old.out") {
		t.Errorf("warnings = %v, want one naming old.out", m.Warnings)
	}
}

func TestTestRefsAndTotals(t *testing.T) {
	m := loadFixture(t)
	if refs := oneUnit(t, m, "helperForTest").TestRefs; len(refs) != 1 || filepath.Base(refs[0]) != "core_test.go" {
		t.Errorf("helperForTest test refs = %v, want [core_test.go]", refs)
	}
	tot := m.Totals
	if tot.DeadUnits != 2 || tot.TestOnlyUnits != 3 || tot.MergedIDs != 0 { // deadFunc, legacyHelper; helperForTest, taggedOnly, SupportForTests
		t.Errorf("totals dead=%d testOnly=%d merged=%d, want 2/3/0", tot.DeadUnits, tot.TestOnlyUnits, tot.MergedIDs)
	}
	if tot.GeneratedSLOC == 0 || tot.CloneGroups == 0 {
		t.Errorf("totals generated=%d cloneGroups=%d, want both > 0", tot.GeneratedSLOC, tot.CloneGroups)
	}
}

// Gini measures concentration among real functions: closures (mostly callback
// wiring) and trivial shims would otherwise make every area look concentrated.
func TestAreaGiniIgnoresClosures(t *testing.T) {
	m := loadFixture(t)
	for _, a := range m.Pkgs {
		if a.Name != "core" {
			continue
		}
		var cogs []int
		for _, u := range m.Units {
			if u.Pkg == "core" && u.Kind != "closure" && !u.Generated {
				cogs = append(cogs, u.Cognitive)
			}
		}
		if want := round3(gini(cogs)); a.Gini != want {
			t.Errorf("core gini = %v, want %v (closures excluded)", a.Gini, want)
		}
		return
	}
	t.Fatal("core package missing")
}
