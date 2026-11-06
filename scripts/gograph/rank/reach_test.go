package rank

import "testing"

func TestReachabilitySplitsDeadFromTestOnly(t *testing.T) {
	m := loadFixture(t)
	for _, u := range unitsNamed(m, "init") {
		if u.Reach != "prod" {
			t.Errorf("init at %s:%d reach = %q, want prod", u.File, u.Line, u.Reach)
		}
	}
	for name, want := range map[string]string{
		"deadFunc":      "dead",      // reached by nothing
		"helperForTest": "test-only", // only core_test.go
		"taggedOnly":    "test-only", // only a `test`-tagged non-test file
		"Run":           "prod",
		"Reg.func1":     "prod", // closures inherit their parent
		"JSThing":       "prod", // reached only by the js/wasm app
		"Unused":        "",     // linux says unreachable, but the js build of brk failed: unknown
		"Used":          "prod",
	} {
		if got := oneUnit(t, m, name).Reach; got != want {
			t.Errorf("%s reach = %q, want %q", name, got, want)
		}
	}
}

func TestReachabilityFastIsUnsplit(t *testing.T) {
	cfg := fixtureConfig()
	cfg.Fast = true
	m, err := Analyze(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"deadFunc": "unsplit", "helperForTest": "unsplit", "Run": "prod"} {
		if got := oneUnit(t, m, name).Reach; got != want {
			t.Errorf("fast: %s reach = %q, want %q", name, got, want)
		}
	}
}

func TestBuildTagVariantOfALiveFunctionIsNotDead(t *testing.T) {
	m := loadFixture(t)
	for _, u := range unitsNamed(m, "platInit") {
		want := "prod"
		if u.Configs[0] == "linux" {
			want = "variant"
		}
		if u.Reach != want {
			t.Errorf("platInit in %s reach = %q, want %q", u.File, u.Reach, want)
		}
	}
	for _, c := range Generate(m) {
		if c.Kind == "delete-unreachable" && c.Target == "core/plat_init_stub.go" {
			t.Errorf("a live function's build-tag variant became a deletion candidate")
		}
	}
}

func TestReferencesFromUnanalysedBuildsLowerConfidence(t *testing.T) {
	m := loadFixture(t)
	u := oneUnit(t, m, "legacyHelper")
	if u.Reach != "dead" || len(u.UnanalyzedRefs) != 1 || u.UnanalyzedRefs[0] != "core/legacy_jstest.go" {
		t.Fatalf("legacyHelper reach=%q unanalyzedRefs=%v, want dead + [core/legacy_jstest.go]", u.Reach, u.UnanalyzedRefs)
	}
	for _, c := range Generate(m) {
		if c.Target == "core/legacy.go" {
			if c.Confidence > DefaultWeights.ConfLow {
				t.Errorf("confidence %.2f, want low: a build we do not analyse names it", c.Confidence)
			}
			return
		}
	}
	t.Error("no candidate for core/legacy.go")
}

func TestTestSupportUsedAcrossPackagesCannotMove(t *testing.T) {
	m := loadFixture(t)
	u := oneUnit(t, m, "SupportForTests")
	if u.Reach != "test-only" || len(u.CrossPkgTestRefs) != 1 || u.CrossPkgTestRefs[0] != "web/web_test.go" {
		t.Fatalf("SupportForTests reach=%q crossPkgTestRefs=%v, want test-only + [web/web_test.go]", u.Reach, u.CrossPkgTestRefs)
	}
	for _, c := range Generate(m) {
		if c.Kind == "move-test-only" && c.Target == "core/support.go" {
			if c.Confidence > DefaultWeights.ConfLow {
				t.Errorf("confidence %.2f, want low: other packages' tests use it", c.Confidence)
			}
			return
		}
	}
	t.Error("no move-test-only candidate for core/support.go")
}
