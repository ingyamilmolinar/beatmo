package rank

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Report is rank.json: everything a reader, the HTML page, a baseline diff or
// an agent needs.
type Report struct {
	Module     string       `json:"module"`
	Configs    []string     `json:"configs"`
	Totals     Totals       `json:"totals"`
	Candidates []*Candidate `json:"candidates"`
	Areas      []*AreaStat  `json:"areas"`
	Packages   []*AreaStat  `json:"packages"`
	Types      []*TypeStat  `json:"types"`
	TopUnits   []*Unit      `json:"topUnits"`
	Overlay    Overlay      `json:"overlay"`
	Weights    Weights      `json:"weights"`
	Delta      *Delta       `json:"delta,omitempty"`
}

// BuildReport assembles the report; topUnits bounds the unit list (by
// cognitive complexity) carried for the HTML page.
func BuildReport(m *Model, cs []*Candidate, ov Overlay, topUnits int) *Report {
	r := &Report{
		Module: m.Module, Configs: m.Configs, Totals: m.Totals, Candidates: cs,
		Areas: m.Areas, Packages: m.Pkgs, Overlay: ov, Weights: DefaultWeights,
	}
	for _, ts := range m.Types {
		if len(r.Types) == 30 {
			break
		}
		t := *ts
		if len(t.PerFile) > 8 {
			t.PerFile = t.PerFile[:8]
		}
		r.Types = append(r.Types, &t)
	}
	us := make([]*Unit, 0, len(m.Units))
	for _, u := range m.Units {
		if !u.Generated {
			us = append(us, u)
		}
	}
	sort.Slice(us, func(i, j int) bool {
		if us[i].Cognitive != us[j].Cognitive {
			return us[i].Cognitive > us[j].Cognitive
		}
		return us[i].ID < us[j].ID
	})
	if len(us) > topUnits {
		us = us[:topUnits]
	}
	r.TopUnits = us
	return r
}

// Delta compares a run to a baseline report.
type Delta struct {
	Totals   map[string]float64 `json:"totals"`
	Resolved []string           `json:"resolved"` // "kind target" in the baseline, gone now
	New      []string           `json:"new"`      // "kind target" new since the baseline
}

// Diff reports what changed since old. Candidates are matched by kind and
// target, not ID: an ID moves whenever the code it covers changes.
func Diff(old, cur *Report) *Delta {
	d := &Delta{Totals: map[string]float64{}}
	var a, b map[string]float64
	ja, _ := json.Marshal(old.Totals)
	jb, _ := json.Marshal(cur.Totals)
	_ = json.Unmarshal(ja, &a)
	_ = json.Unmarshal(jb, &b)
	for k, v := range b {
		if dv := v - a[k]; dv != 0 {
			d.Totals[k] = dv
		}
	}
	key := func(c *Candidate) string { return c.Kind + " " + c.Target }
	oldSet, curSet := map[string]bool{}, map[string]bool{}
	for _, c := range old.Candidates {
		oldSet[key(c)] = true
	}
	for _, c := range cur.Candidates {
		curSet[key(c)] = true
		if !oldSet[key(c)] {
			d.New = append(d.New, key(c))
		}
	}
	for _, c := range old.Candidates {
		if !curSet[key(c)] {
			d.Resolved = append(d.Resolved, key(c))
		}
	}
	return d
}

// LoadReport reads a previous rank.json.
func LoadReport(file string) (*Report, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &r, nil
}

// RenderMarkdown renders the report for a terminal or an editor; top bounds
// the unified candidate list.
func RenderMarkdown(r *Report, top int) string {
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("# Refactor ranking: %s\n\n", r.Module)
	p("Configs: %s. Score = payoff × confidence / risk; payoff is in complexity points (≈ cognitive complexity removed).\n\n", strings.Join(r.Configs, ", "))

	p("## Totals\n\n| metric | value |")
	if r.Delta != nil {
		p(" Δ baseline |")
	}
	p("\n|---|---:|")
	if r.Delta != nil {
		p("---:|")
	}
	p("\n")
	var tm map[string]any
	jt, _ := json.Marshal(r.Totals)
	_ = json.Unmarshal(jt, &tm)
	keys := make([]string, 0, len(tm))
	for k := range tm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		p("| %s | %v |", k, tm[k])
		if r.Delta != nil {
			if d, ok := r.Delta.Totals[k]; ok {
				p(" %+g |", d)
			} else {
				p(" |")
			}
		}
		p("\n")
	}

	live := make([]*Candidate, 0, len(r.Candidates))
	for _, c := range r.Candidates {
		if c.Suppressed == "" {
			live = append(live, c)
		}
	}
	p("\n## Top candidates\n\n| # | score | kind | target | payoff | conf | risk | where |\n|---:|---:|---|---|---:|---:|---:|---|\n")
	for i, c := range live {
		if i == top {
			break
		}
		mark := ""
		if c.Contested {
			mark = " ⚑"
		}
		p("| %d | %.1f | %s | `%s`%s | %.1f | %.2f | %.2f | %s |\n", i+1, c.Score, c.Kind, c.Target, mark, c.Payoff, c.Confidence, c.Risk, c.Location)
	}
	p("\n### Details\n\n")
	for i, c := range live {
		if i == top {
			break
		}
		p("**%d. %s `%s`** (id %s, %s)\n", i+1, c.Kind, c.Target, c.ID, c.Location)
		for _, l := range c.Breakdown {
			p("- %s\n", l)
		}
		for _, n := range c.Notes {
			p("- note: %s\n", n)
		}
		if c.Agent != nil && c.Agent.Proposal != "" {
			p("- agent: %s (effort %s, risk %s)\n", c.Agent.Proposal, c.Agent.Effort, c.Agent.RegressionRisk)
		}
		p("\n")
	}

	p("## Top by kind\n\n")
	byKind := map[string][]*Candidate{}
	var kinds []string
	for _, c := range live {
		if byKind[c.Kind] == nil {
			kinds = append(kinds, c.Kind)
		}
		byKind[c.Kind] = append(byKind[c.Kind], c)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		p("### %s (%d)\n\n", k, len(byKind[k]))
		for i, c := range byKind[k] {
			if i == 10 {
				break
			}
			p("%d. %.1f `%s` (%s)\n", i+1, c.Score, c.Target, c.Location)
		}
		p("\n")
	}

	areaTable := func(title string, as []*AreaStat, n int) {
		p("## %s\n\n| area | files | lines | units | Σ cognitive | units > 30 | gini | coupling | clone lines | dead lines | compound |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n", title)
		for i, a := range as {
			if i == n {
				break
			}
			p("| `%s` | %d | %d | %d | %d | %d | %.2f | %.2f | %d | %d | %.0f |\n", a.Name, a.Files, a.Lines, a.Units, a.SumCog, a.Over30, a.Gini, a.Coupling, a.CloneLOC, a.DeadLOC, a.Compound)
		}
		p("\n")
	}
	areaTable("Areas", r.Areas, 25)
	areaTable("Packages", r.Packages, 15)

	p("## God types\n\n| type | methods | fields | files | most separable files (exclusive fields / used) |\n|---|---:|---:|---:|---|\n")
	for i, t := range r.Types {
		if i == 12 {
			break
		}
		var fs []string
		for j, f := range t.PerFile {
			if j == 3 {
				break
			}
			fs = append(fs, fmt.Sprintf("%s %d/%d", filepath.Base(f.File), f.Exclusive, f.FieldsUsed))
		}
		p("| `%s.%s` | %d | %d | %d | %s |\n", t.Pkg, t.Name, t.Methods, t.Fields, t.FilesSpanned, strings.Join(fs, ", "))
	}

	section := func(title string, cs []*Candidate, line func(*Candidate) string) {
		if len(cs) == 0 {
			return
		}
		p("\n## %s\n\n", title)
		for _, c := range cs {
			p("- %s\n", line(c))
		}
	}
	var contested, suppressed []*Candidate
	for _, c := range r.Candidates {
		if c.Contested {
			contested = append(contested, c)
		}
		if c.Suppressed != "" {
			suppressed = append(suppressed, c)
		}
	}
	section("Contested (agent disagrees with the tool)", contested, func(c *Candidate) string {
		why := ""
		if c.Agent != nil {
			why = c.Agent.Proposal
		}
		return fmt.Sprintf("%s `%s`: %s", c.Kind, c.Target, why)
	})
	section("Suppressed", suppressed, func(c *Candidate) string {
		return fmt.Sprintf("%s `%s`: %s", c.Kind, c.Target, c.Suppressed)
	})
	if len(r.Overlay.StaleSuppressions) > 0 {
		p("\n## Stale suppressions (match nothing; remove them)\n\n")
		for _, s := range r.Overlay.StaleSuppressions {
			p("- `%s` (%s): %s\n", s.Target, s.Kind, s.Reason)
		}
	}
	if len(r.Overlay.ProposedSuppress) > 0 {
		p("\n## Proposed suppressions (from agents; copy into refactor_suppress.json to accept)\n\n")
		for _, s := range r.Overlay.ProposedSuppress {
			p("- `%s` (%s): %s\n", s.Target, s.Kind, s.Reason)
		}
	}
	if len(r.Overlay.Warnings) > 0 {
		p("\n## Warnings\n\n")
		for _, w := range r.Overlay.Warnings {
			p("- %s\n", w)
		}
	}
	if r.Delta != nil {
		p("\n## Since baseline\n\n%d candidates resolved, %d new.\n\n", len(r.Delta.Resolved), len(r.Delta.New))
		for _, s := range r.Delta.Resolved {
			p("- resolved: %s\n", s)
		}
		for i, s := range r.Delta.New {
			if i == 30 {
				p("- … %d more new\n", len(r.Delta.New)-30)
				break
			}
			p("- new: %s\n", s)
		}
	}
	return b.String()
}

//go:embed rank.html
var rankHTML string

// RenderHTML embeds the report into the standalone rank page.
func RenderHTML(r *Report) ([]byte, error) {
	model, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Replace(rankHTML, "__MODEL__", string(model), 1)), nil
}

// Packet is one candidate prepared for an assessing agent.
type Packet struct {
	Rank      int            `json:"rank"`
	Candidate *Candidate     `json:"candidate"`
	Units     []*Unit        `json:"units,omitempty"`
	Sources   []PacketSource `json:"sources,omitempty"`
	Callers   []string       `json:"callers,omitempty"`
	Callees   []string       `json:"callees,omitempty"`
	Area      *AreaStat      `json:"area,omitempty"`
	AreaFiles []*File        `json:"areaFiles,omitempty"`
	Type      *TypeStat      `json:"type,omitempty"`
	Schema    string         `json:"verdictSchema"`
}

// PacketSource is a source excerpt.
type PacketSource struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	EndLine   int    `json:"endLine"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated,omitempty"`
}

const (
	packetMaxLines   = 400
	packetMaxLinks   = 30
	verdictSchemaDoc = `{"id": "<candidate id>", "real": bool, "kind": "<re-classified kind or empty>", "essential_vs_accidental": 0..1 (1 = all accidental), "clarity": 1..5, "verbosity": 1..5, "proposal": "...", "effort": "S|M|L", "regression_risk": "L|M|H", "guarding_tests": [...], "blockers": [...], "suppress_reason": "<set to propose a suppression>"}`
)

// WritePackets writes up to n packets for the best candidates that are neither
// suppressed nor already assessed. Old packets in dir are removed first.
func WritePackets(m *Model, cs []*Candidate, verdicts map[string]*Verdict, dir string, n int) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, f := range old {
		_ = os.Remove(f)
	}
	g := &gen{m: m}
	written := 0
	for rank, c := range cs {
		if written == n {
			break
		}
		if c.Suppressed != "" || verdicts[c.ID] != nil {
			continue
		}
		p := &Packet{Rank: rank + 1, Candidate: c, Schema: verdictSchemaDoc}
		budget := packetMaxLines
		addSrc := func(file string, from, to int) {
			if budget <= 0 {
				return
			}
			s := PacketSource{File: file, Line: from, EndLine: to}
			if to-from+1 > budget {
				s.EndLine, s.Truncated = from+budget-1, true
			}
			s.Text = g.source(file, s.Line, s.EndLine)
			budget -= s.EndLine - s.Line + 1
			p.Sources = append(p.Sources, s)
		}
		link := func(ids map[string]bool) []string {
			var out []string
			for id := range ids {
				if u := m.byID[id]; u != nil {
					out = append(out, fmt.Sprintf("%s (%s:%d)", qualified(u), u.File, u.Line))
				}
			}
			sort.Strings(out)
			if len(out) > packetMaxLinks {
				out = append(out[:packetMaxLinks], fmt.Sprintf("… %d more", len(out)-packetMaxLinks))
			}
			return out
		}
		switch c.Kind {
		case "dedupe-clone", "consolidate-utility":
			if ms, ok := c.Evidence["members"].([]CloneLoc); ok {
				for _, mem := range ms {
					addSrc(mem.File, mem.Line, mem.EndLine)
				}
			}
		case "restructure-area":
			for _, a := range m.Areas {
				if a.Name == c.Target {
					p.Area = a
				}
			}
			for _, f := range m.Files {
				if f.Area == c.Target {
					p.AreaFiles = append(p.AreaFiles, f)
				}
			}
		case "restructure-type":
			for _, ts := range m.Types {
				if ts.Pkg+"."+ts.Name == c.Target {
					p.Type = ts
				}
			}
		case "extract-struct":
			file := c.Location
			for _, u := range m.Units {
				if u.File == file && u.Kind == "method" {
					p.Units = append(p.Units, u)
					addSrc(u.File, u.Line, u.EndLine)
				}
			}
		}
		for _, id := range c.Units {
			u := m.byID[id]
			if u == nil {
				continue
			}
			p.Units = append(p.Units, u)
			if c.Kind != "dedupe-clone" && c.Kind != "consolidate-utility" {
				addSrc(u.File, u.Line, u.EndLine)
			}
			if len(c.Units) == 1 {
				p.Callers, p.Callees = link(u.callers), link(u.callees)
			}
		}
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(filepath.Join(dir, c.ID+".json"), append(b, '\n'), 0o644); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
}
