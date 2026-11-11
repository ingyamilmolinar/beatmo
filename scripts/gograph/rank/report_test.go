package rank

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlaysVerdictsSuppressionsAndWarnings(t *testing.T) {
	m := loadFixture(t)
	cs := Generate(m)
	dir := t.TempDir()
	var dead, clone *Candidate
	for _, c := range cs {
		if c.Kind == "delete-unreachable" && c.Target == "core/dead.go" {
			dead = c
		}
		if c.Kind == "dedupe-clone" && strings.HasPrefix(c.Target, "core.w1") {
			clone = c
		}
	}
	if dead == nil || clone == nil {
		t.Fatal("fixture candidates missing")
	}
	before := dead.Score
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.json", `{"id":"`+dead.ID+`","real":false,"essential_vs_accidental":0,"suppress_reason":"called via reflection"}`)
	write("b.json", `{not json`)
	write("c.json", `[{"id":"`+clone.ID+`","real":true,"essential_vs_accidental":1,"regression_risk":"L"}]`)
	vs, warns := LoadVerdicts(dir)
	if len(warns) != 1 || !strings.Contains(warns[0], "b.json") {
		t.Errorf("warnings = %v, want one for b.json", warns)
	}
	sups := []Suppression{{Target: "core/never*.go", Reason: "stale"}, {Target: "core.w1*", Kind: "dedupe-clone", Reason: "kept on purpose"}}
	ov := ApplyOverlays(cs, vs, sups)
	if !dead.Contested || dead.Score > before*0.1+1e-9 {
		t.Errorf("dismissed candidate: contested=%v score %v (before %v), want contested and ≤ 10%%", dead.Contested, dead.Score, before)
	}
	if clone.Suppressed != "kept on purpose" {
		t.Errorf("suppression not applied: %q", clone.Suppressed)
	}
	if len(ov.StaleSuppressions) != 1 || ov.StaleSuppressions[0].Target != "core/never*.go" {
		t.Errorf("stale suppressions = %v", ov.StaleSuppressions)
	}
	if len(ov.ProposedSuppress) != 1 || ov.ProposedSuppress[0].Target != "core/dead.go" {
		t.Errorf("proposed suppressions = %v", ov.ProposedSuppress)
	}
}

func TestBaselineDeltaReportsResolvedAndNew(t *testing.T) {
	m := loadFixture(t)
	r := BuildReport(m, Generate(m), Overlay{}, 50)
	old := *r
	old.Totals.SumCognitive += 40
	old.Candidates = append([]*Candidate{{ID: "gone", Kind: "split-function", Target: "core.Removed", Score: 9}}, r.Candidates[1:]...)
	d := Diff(&old, r)
	if d.Totals["sumCognitive"] != -40 {
		t.Errorf("sumCognitive delta = %v, want -40", d.Totals["sumCognitive"])
	}
	if len(d.Resolved) != 1 || d.Resolved[0] != "split-function core.Removed" {
		t.Errorf("resolved = %v", d.Resolved)
	}
	if len(d.New) != 1 {
		t.Errorf("new = %v, want the one candidate the baseline lacked", d.New)
	}
}

func TestReportsAreDeterministicAndComplete(t *testing.T) {
	m := loadFixture(t)
	render := func() (string, string) {
		r := BuildReport(m, Generate(m), Overlay{}, 50)
		j, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		return string(j), RenderMarkdown(r, 20)
	}
	j1, md1 := render()
	j2, md2 := render()
	if j1 != j2 || md1 != md2 {
		t.Fatal("reports differ between two runs")
	}
	for _, want := range []string{"## Totals", "## Top candidates", "## Areas", "## God types", "delete-unreachable", "core/dead.go"} {
		if !strings.Contains(md1, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	html, err := RenderHTML(&Report{Module: "x"})
	if err != nil || !strings.Contains(string(html), `"module":"x"`) {
		t.Errorf("html model not embedded (err %v)", err)
	}
}

func TestPacketsCarrySourceAndSkipCachedVerdicts(t *testing.T) {
	m := loadFixture(t)
	cs := Generate(m)
	dir := t.TempDir()
	cached := map[string]*Verdict{cs[0].ID: {ID: cs[0].ID, Real: true}}
	n, err := WritePackets(m, cs, cached, dir, 5)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("wrote %d packets, want 5", n)
	}
	if _, err := os.Stat(filepath.Join(dir, cs[0].ID+".json")); err == nil {
		t.Errorf("packet written for a candidate with a cached verdict")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	b, _ := os.ReadFile(files[0])
	var p Packet
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.Candidate == nil || (len(p.Sources) == 0 && p.Candidate.Kind != "restructure-area" && p.Candidate.Kind != "split-file") {
		t.Errorf("packet lacks candidate or sources: %+v", p)
	}
}
