package rank

import (
	"sync"
	"testing"
)

// fixtureConfig is the rankmod fixture analysed the way the real repo is: a
// linux build of everything, a js/wasm build of the app, and both test builds.
func fixtureConfig() Config {
	return Config{
		Root:         "../testdata/rankmod",
		Module:       "example.com/rankmod",
		ToolPrefixes: []string{"tools/"},
		Prod: []BuildConfig{
			{Name: "linux", GOOS: "linux", GOARCH: "amd64", Patterns: []string{"./..."}, Roots: []string{"./app", "./tools/..."}},
			// brk is loaded under js too, where it does not type-check: that config
			// may prove reachability there, never unreachability.
			{Name: "js", GOOS: "js", GOARCH: "wasm", Patterns: []string{"./app", "./brk"}, Roots: []string{"./app"}},
		},
		TestCfgs: []BuildConfig{
			{Name: "test-untagged", GOOS: "linux", GOARCH: "amd64", Patterns: []string{"./..."}, Tests: true},
			{Name: "test-tagged", GOOS: "linux", GOARCH: "amd64", Tags: "test", Patterns: []string{"./..."}, Tests: true},
		},
	}
}

var (
	fixtureOnce  sync.Once
	fixtureModel *Model
	fixtureErr   error
)

// loadFixture analyses the fixture once per test binary; every test reads it.
func loadFixture(t *testing.T) *Model {
	t.Helper()
	fixtureOnce.Do(func() { fixtureModel, fixtureErr = Analyze(fixtureConfig()) })
	if fixtureErr != nil {
		t.Fatal(fixtureErr)
	}
	return fixtureModel
}

func unitsNamed(m *Model, name string) []*Unit {
	var out []*Unit
	for _, u := range m.Units {
		if u.Name == name {
			out = append(out, u)
		}
	}
	return out
}

func oneUnit(t *testing.T, m *Model, name string) *Unit {
	t.Helper()
	us := unitsNamed(m, name)
	if len(us) != 1 {
		t.Fatalf("unit %q: found %d, want 1", name, len(us))
	}
	return us[0]
}

func TestBuildTagVariantsAreSeparateUnits(t *testing.T) {
	m := loadFixture(t)
	us := unitsNamed(m, "Plat")
	if len(us) != 2 {
		t.Fatalf("Plat: %d units, want 2 (one per build-tag variant)", len(us))
	}
	got := map[string]int{}
	for _, u := range us {
		if len(u.Configs) != 1 {
			t.Fatalf("Plat variant in %s has configs %v, want exactly one", u.File, u.Configs)
		}
		got[u.Configs[0]] = u.Cyclo
	}
	if got["linux"] != 2 || got["js"] != 1 {
		t.Errorf("Plat variants by config = %v, want linux:2 js:1", got)
	}
	for _, u := range us {
		if u.Configs[0] == "js" && !u.JSOnly {
			t.Errorf("js Plat variant not marked JSOnly")
		}
	}
	// A unit compiled by both configs appears once, carrying both.
	if r := oneUnit(t, m, "Run"); len(r.Configs) != 2 {
		t.Errorf("Run configs = %v, want [js linux]", r.Configs)
	}
}

func TestGeneratedAndTaggedTestFiles(t *testing.T) {
	m := loadFixture(t)
	if g := oneUnit(t, m, "Gen"); !g.Generated {
		t.Errorf("Gen not marked generated")
	}
	// tagged_helper.go compiles only under the test tag: it is test code and
	// never a production unit.
	if us := unitsNamed(m, "TaggedHelper"); len(us) != 0 {
		t.Errorf("TaggedHelper (test-tagged file) became a production unit")
	}
	if d := oneUnit(t, m, "Table"); !d.Data {
		t.Errorf("Table not classified as data (DataRatio %.2f)", d.DataRatio)
	}
}

func TestClosuresAreOwnUnits(t *testing.T) {
	m := loadFixture(t)
	reg := oneUnit(t, m, "Reg")
	cl := oneUnit(t, m, "Reg.func1")
	if cl.Kind != "closure" || cl.Parent != reg.ID {
		t.Errorf("closure kind=%q parent=%q, want closure / %q", cl.Kind, cl.Parent, reg.ID)
	}
	if reg.Cognitive != 1 {
		t.Errorf("Reg cognitive = %d, want 1 (closure excluded)", reg.Cognitive)
	}
	if cl.Cognitive < 6 {
		t.Errorf("Reg.func1 cognitive = %d, want >= 6", cl.Cognitive)
	}
}

func TestFanInFanOut(t *testing.T) {
	m := loadFixture(t)
	if h := oneUnit(t, m, "helper"); h.FanIn < 3 {
		// Run, runB, runC, helperForTest is test-only code but a prod file,
		// and both Plat variants call it.
		t.Errorf("helper FanIn = %d, want >= 3", h.FanIn)
	}
	if r := oneUnit(t, m, "Run"); r.FanOut != 3 {
		t.Errorf("Run FanOut = %d, want 3 (helper, runB, runC)", r.FanOut)
	}
}

func TestMethodsCarryReceiver(t *testing.T) {
	m := loadFixture(t)
	fx := oneUnit(t, m, "God.FX")
	if fx.Recv != "God" || fx.Kind != "method" || !fx.Exported {
		t.Errorf("God.FX = recv %q kind %q exported %v", fx.Recv, fx.Kind, fx.Exported)
	}
}
