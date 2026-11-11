package rank

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Verdict is one agent's assessment of a candidate (see the spec's L3).
type Verdict struct {
	ID                    string   `json:"id"`
	Real                  bool     `json:"real"`
	Kind                  string   `json:"kind,omitempty"`
	EssentialVsAccidental float64  `json:"essential_vs_accidental"`
	Clarity               int      `json:"clarity,omitempty"`
	Verbosity             int      `json:"verbosity,omitempty"`
	Proposal              string   `json:"proposal,omitempty"`
	Effort                string   `json:"effort,omitempty"`
	RegressionRisk        string   `json:"regression_risk,omitempty"`
	GuardingTests         []string `json:"guarding_tests,omitempty"`
	Blockers              []string `json:"blockers,omitempty"`
	SuppressReason        string   `json:"suppress_reason,omitempty"`
}

// Suppression dismisses candidates, with a reason, so a known false positive
// never returns. Target matches a candidate's target exactly or as a
// path.Match glob; Kind empty matches every kind.
type Suppression struct {
	Target string `json:"target"`
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason"`
	Date   string `json:"date,omitempty"`
}

// Overlay is what ApplyOverlays found, for the report.
type Overlay struct {
	Warnings          []string      `json:"warnings,omitempty"`
	StaleSuppressions []Suppression `json:"staleSuppressions,omitempty"`
	ProposedSuppress  []Suppression `json:"proposedSuppressions,omitempty"`
}

// LoadVerdicts reads every *.json file in dir. A malformed file is a warning,
// never fatal; a missing dir means no verdicts.
func LoadVerdicts(dir string) (map[string]*Verdict, []string) {
	out := map[string]*Verdict{}
	var warns []string
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	sort.Strings(files)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			warns = append(warns, fmt.Sprintf("verdict %s: %v", filepath.Base(f), err))
			continue
		}
		var vs []*Verdict
		trimmed := strings.TrimSpace(string(b))
		if strings.HasPrefix(trimmed, "[") {
			err = json.Unmarshal(b, &vs)
		} else {
			var v Verdict
			err = json.Unmarshal(b, &v)
			vs = []*Verdict{&v}
		}
		if err != nil {
			warns = append(warns, fmt.Sprintf("verdict %s: malformed: %v", filepath.Base(f), err))
			continue
		}
		for _, v := range vs {
			if v.ID == "" {
				warns = append(warns, fmt.Sprintf("verdict %s: missing id", filepath.Base(f)))
				continue
			}
			out[v.ID] = v
		}
	}
	return out, warns
}

// LoadSuppressions reads the committed suppression list (missing = none).
func LoadSuppressions(file string) ([]Suppression, error) {
	b, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ss []Suppression
	if err := json.Unmarshal(b, &ss); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return ss, nil
}

func (s Suppression) matches(c *Candidate) bool {
	if s.Kind != "" && s.Kind != c.Kind {
		return false
	}
	if s.Target == c.Target {
		return true
	}
	ok, _ := path.Match(s.Target, c.Target)
	return ok
}

// riskFactor scales an agent-confirmed candidate by the regression risk the
// agent judged.
var riskFactor = map[string]float64{"L": 1, "M": 0.85, "H": 0.6}

// ApplyOverlays merges agent verdicts and suppressions into the candidates and
// re-sorts them. A verdict rescales confidence: a dismissed candidate (Real
// false) keeps 0.1 of its score and is marked contested, as is one the agent
// re-classified; a confirmed one scales by how accidental its complexity is
// and by the regression risk.
func ApplyOverlays(cs []*Candidate, verdicts map[string]*Verdict, sups []Suppression) Overlay {
	var ov Overlay
	used := make([]bool, len(sups))
	for _, c := range cs {
		if v := verdicts[c.ID]; v != nil {
			c.Agent = v
			factor := 0.1
			if v.Real {
				rf, ok := riskFactor[v.RegressionRisk]
				if !ok {
					rf = riskFactor["M"]
				}
				factor = (0.5 + clamp01(v.EssentialVsAccidental)) * rf
			}
			c.Contested = !v.Real || (v.Kind != "" && v.Kind != c.Kind)
			c.Confidence = round3(c.Confidence * factor)
			c.Score = round3(c.Payoff * c.Confidence / c.Risk)
			c.Breakdown = append(c.Breakdown, fmt.Sprintf("agent: real=%v accidental=%.2f risk=%s → confidence × %.2f", v.Real, v.EssentialVsAccidental, v.RegressionRisk, factor))
			if v.SuppressReason != "" {
				ov.ProposedSuppress = append(ov.ProposedSuppress, Suppression{Target: c.Target, Kind: c.Kind, Reason: v.SuppressReason})
			}
		}
		for i, s := range sups {
			if s.matches(c) {
				used[i] = true
				if c.Suppressed == "" {
					c.Suppressed = s.Reason
				}
			}
		}
	}
	for i, s := range sups {
		if !used[i] {
			ov.StaleSuppressions = append(ov.StaleSuppressions, s)
		}
	}
	SortCandidates(cs)
	return ov
}

func clamp01(f float64) float64 {
	return max(0, min(1, f))
}
