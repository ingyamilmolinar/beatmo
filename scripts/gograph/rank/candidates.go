package rank

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Candidate is one ranked refactoring opportunity.
type Candidate struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Target     string         `json:"target"`
	Area       string         `json:"area"`
	Location   string         `json:"location"` // file:line of the first unit or the file
	Units      []string       `json:"units,omitempty"`
	Payoff     float64        `json:"payoff"`
	Confidence float64        `json:"confidence"`
	Risk       float64        `json:"risk"`
	Score      float64        `json:"score"`
	Breakdown  []string       `json:"breakdown"`
	Notes      []string       `json:"notes,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	Agent      *Verdict       `json:"agent,omitempty"`
	Contested  bool           `json:"contested,omitempty"`
	Suppressed string         `json:"suppressed,omitempty"`
	content    string
}

// Generate builds every candidate with the shipped weights.
func Generate(m *Model) []*Candidate { return GenerateWith(m, DefaultWeights) }

// GenerateWith builds every candidate, scores it and sorts by score.
func GenerateWith(m *Model, w Weights) []*Candidate {
	g := &gen{m: m, w: w, area: map[string]string{}, jsReg: map[string]bool{}}
	for _, f := range m.Files {
		g.area[f.Path] = f.Area
	}
	for rel, sf := range m.src.files {
		if bytes.Contains(sf.Src, []byte("js.FuncOf(")) || bytes.Contains(sf.Src, []byte("js.Global().Set(")) {
			g.jsReg[rel] = true
		}
	}
	g.units()
	g.unreachable("dead", "delete-unreachable", w.DeadPerLine, true)
	g.unreachable("unsplit", "delete-unreachable", w.DeadPerLine, true)
	g.unreachable("test-only", "move-test-only", w.TestOnlyPerLine, false)
	g.clones()
	g.extract()
	g.godTypes()
	g.files()
	g.areas()
	for _, c := range g.out {
		if g.toolOnly(c) {
			c.Payoff *= w.ToolShare
			c.Breakdown = append(c.Breakdown, fmt.Sprintf("dev tooling: payoff × %.2f", w.ToolShare))
		}
		c.Score = round3(c.Payoff * c.Confidence / c.Risk)
		c.Payoff, c.Risk = round3(c.Payoff), round3(c.Risk)
		sum := sha256.Sum256([]byte(c.Kind + "|" + c.Target + "|" + c.content))
		c.ID = hex.EncodeToString(sum[:6])
	}
	SortCandidates(g.out)
	return g.out
}

// SortCandidates orders by score, then ID, so ties are deterministic.
func SortCandidates(cs []*Candidate) {
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].Score != cs[j].Score {
			return cs[i].Score > cs[j].Score
		}
		return cs[i].ID < cs[j].ID
	})
}

type gen struct {
	m     *Model
	w     Weights
	area  map[string]string
	jsReg map[string]bool
	out   []*Candidate
}

func (g *gen) source(file string, from, to int) string {
	sf := g.m.src.files[file]
	if sf == nil {
		return ""
	}
	lines := strings.Split(string(sf.Src), "\n")
	if from < 1 {
		from = 1
	}
	if to > len(lines) {
		to = len(lines)
	}
	if from > to {
		return ""
	}
	return strings.Join(lines[from-1:to], "\n")
}

func (g *gen) cov(u *Unit) (float64, string) {
	if u.Coverage == nil {
		return g.w.UnknownCoverage, "unknown"
	}
	return *u.Coverage, fmt.Sprintf("%.0f%%", *u.Coverage*100)
}

// unitRisk is how likely changing u breaks something: many callers, little
// coverage, compiled only for the browser (no desktop test sees it), exported.
func (g *gen) unitRisk(u *Unit) (float64, string) {
	cov, covS := g.cov(u)
	fan := g.w.RiskFanIn * math.Log1p(float64(u.FanIn))
	unc := g.w.RiskUncovered * (1 - cov)
	r := 1 + fan + unc
	s := fmt.Sprintf("risk = 1 + fanIn %d → %.2f + coverage %s → %.2f", u.FanIn, fan, covS, unc)
	if u.JSOnly {
		r += g.w.RiskJSOnly
		s += fmt.Sprintf(" + js-only %.2f", g.w.RiskJSOnly)
	}
	if u.Exported {
		r += g.w.RiskExported
		s += fmt.Sprintf(" + exported %.2f", g.w.RiskExported)
	}
	return r, s
}

func qualified(u *Unit) string { return u.Pkg + "." + u.Name }

// signal is one reason a unit is a candidate.
type signal struct {
	kind   string
	payoff float64
	why    string
}

func (g *gen) units() {
	w := g.w
	for _, u := range g.m.Units {
		if u.Generated {
			continue
		}
		if u.Data {
			if u.LOC >= w.DataMinLOC {
				p := float64(u.LOC) * w.DataPer
				g.add(&Candidate{
					Kind: "data-to-asset", Target: qualified(u), Units: []string{u.ID}, Area: g.area[u.File],
					Location: fmt.Sprintf("%s:%d", u.File, u.Line), Payoff: p, Confidence: w.ConfLow, Risk: 1,
					Breakdown: []string{fmt.Sprintf("payoff %.1f = %d lines × %.2f (%.0f%% composite literals, cyclo %d)", p, u.LOC, w.DataPer, u.DataRatio*100, u.Cyclo)},
					content:   g.source(u.File, u.Line, u.EndLine),
				})
			}
			continue
		}
		var sigs []signal
		if u.Cognitive >= w.SplitCog || u.OwnSLOC >= w.SplitSLOC {
			p := float64(max(0, u.Cognitive-w.SplitKeepCog)) + float64(max(0, u.OwnSLOC-w.SplitSLOCFree))*w.SplitSLOCPer
			sigs = append(sigs, signal{"split-function", p, fmt.Sprintf("split-function %.1f = cognitive %d − %d + own sloc %d beyond %d × %.2f", p, u.Cognitive, w.SplitKeepCog, u.OwnSLOC, w.SplitSLOCFree, w.SplitSLOCPer)})
		}
		if u.Nesting >= w.FlattenNesting || (u.NestBonus >= w.FlattenBonusMin && float64(u.NestBonus) >= w.FlattenBonusShare*float64(u.Cognitive)) {
			p := float64(u.NestBonus)
			sigs = append(sigs, signal{"flatten", p, fmt.Sprintf("flatten %.1f = nesting share of cognitive (max depth %d)", p, u.Nesting)})
		}
		v := u.Verb
		if v.RepeatedLines >= w.VerbRepeatedLines || v.SimilarCases >= w.VerbSimilarCases || v.MaxChain >= w.VerbChain || v.MaxCondOps >= w.VerbCondOps {
			p := float64(v.RepeatedLines) + 2*float64(v.SimilarCases) + 3*float64(max(0, v.MaxChain-w.VerbChainFree)) + 2*float64(max(0, v.MaxCondOps-w.VerbCondFree))
			sigs = append(sigs, signal{"simplify-verbose", p, fmt.Sprintf("simplify-verbose %.1f = repeated lines %d + 2×similar cases %d + 3×chain excess (max %d) + 2×condition excess (max %d ops)", p, v.RepeatedLines, v.SimilarCases, v.MaxChain, v.MaxCondOps)})
		}
		if len(sigs) == 0 {
			continue
		}
		sort.SliceStable(sigs, func(i, j int) bool { return sigs[i].payoff > sigs[j].payoff })
		c := &Candidate{
			Kind: sigs[0].kind, Target: qualified(u), Units: []string{u.ID}, Area: g.area[u.File],
			Location: fmt.Sprintf("%s:%d", u.File, u.Line), Payoff: sigs[0].payoff,
			Confidence: map[string]float64{"split-function": w.ConfHigh, "flatten": 0.8, "simplify-verbose": w.ConfMedium}[sigs[0].kind],
			content:    g.source(u.File, u.Line, u.EndLine),
		}
		brk := []string{sigs[0].why}
		for _, s := range sigs[1:] {
			c.Payoff += w.SecondaryShare * s.payoff
			c.Notes = append(c.Notes, s.why)
		}
		if len(sigs) > 1 {
			brk = append(brk, fmt.Sprintf("+ %.1f × secondary signals (see notes)", w.SecondaryShare))
		}
		r, rs := g.unitRisk(u)
		c.Risk = r
		c.Breakdown = append([]string{fmt.Sprintf("payoff %.1f", c.Payoff)}, append(brk, rs)...)
		c.Evidence = unitEvidence(u)
		if u.Reach == "test-only" || u.Reach == "dead" {
			c.Notes = append(c.Notes, "unit is "+u.Reach+": consider the unreachable candidate for its file first")
		}
		g.add(c)
	}
}

func unitEvidence(u *Unit) map[string]any {
	e := map[string]any{
		"loc": u.LOC, "sloc": u.SLOC, "ownSloc": u.OwnSLOC, "cognitive": u.Cognitive, "cyclo": u.Cyclo, "nesting": u.Nesting,
		"params": u.Params, "fanIn": u.FanIn, "fanOut": u.FanOut, "configs": u.Configs, "reach": u.Reach,
	}
	if u.Coverage != nil {
		e["coverage"] = *u.Coverage
	}
	if len(u.TestRefs) > 0 {
		e["testRefs"] = u.TestRefs
	}
	return e
}

// unreachable groups the units of one reach class by file: deleting (or
// moving) a file's worth of dead code is one change, not twenty.
func (g *gen) unreachable(reach, kind string, perLine float64, addCog bool) {
	w := g.w
	byFile := map[string][]*Unit{}
	for _, u := range g.m.Units {
		if u.Reach == reach && u.Kind != "closure" && !u.Generated {
			byFile[u.File] = append(byFile[u.File], u)
		}
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		us := byFile[f]
		loc, cog := 0, 0
		var names []string
		var content strings.Builder
		conf := w.ConfHigh
		var notes []string
		exported, jsOnly := false, false
		for _, u := range us {
			loc += u.LOC
			cog += u.Cognitive
			names = append(names, u.Name)
			content.WriteString(g.source(u.File, u.Line, u.EndLine))
			exported = exported || u.Exported
			jsOnly = jsOnly || u.JSOnly
		}
		p := float64(loc) * perLine
		why := fmt.Sprintf("payoff %.1f = %d lines × %.2f", p, loc, perLine)
		if addCog {
			p += float64(cog)
			why = fmt.Sprintf("payoff %.1f = %d lines × %.2f + cognitive %d", p, loc, perLine, cog)
		}
		if exported {
			conf = math.Min(conf, w.ConfMedium)
			notes = append(notes, "exports a name: reflection or another module could use it")
		}
		if g.jsReg[f] {
			conf = math.Min(conf, w.ConfMedium)
			notes = append(notes, "file registers JS exports by name")
		}
		var unanalyzed, liveRefs []string
		for _, u := range us {
			unanalyzed = append(unanalyzed, u.UnanalyzedRefs...)
			for id := range u.callers {
				if caller := g.m.byID[id]; caller != nil && caller.Reach == "prod" {
					liveRefs = append(liveRefs, qualified(caller))
				}
			}
		}
		var crossTests []string
		for _, u := range us {
			crossTests = append(crossTests, u.CrossPkgTestRefs...)
		}
		if len(crossTests) > 0 && reach == "test-only" {
			conf = w.ConfLow
			sort.Strings(crossTests)
			crossTests = slices.Compact(crossTests)
			if len(crossTests) > 5 {
				crossTests = append(crossTests[:5], fmt.Sprintf("… %d more", len(crossTests)-5))
			}
			notes = append(notes, "tests of other packages use it ("+strings.Join(crossTests, ", ")+"): it cannot move into _test.go; keep it as documented test-support API or move it to a test-support package")
		}
		if len(unanalyzed) > 0 {
			conf = w.ConfLow
			sort.Strings(unanalyzed)
			notes = append(notes, "named by files no analysed build compiles: "+strings.Join(slices.Compact(unanalyzed), ", "))
		}
		if len(liveRefs) > 0 {
			conf = w.ConfLow
			sort.Strings(liveRefs)
			notes = append(notes, "referenced (as a value?) by live code: "+strings.Join(slices.Compact(liveRefs), ", "))
		}
		if reach == "unsplit" {
			conf = math.Min(conf, w.ConfMedium)
			notes = append(notes, "test reachability was skipped (-fast): some of these may be test-only")
		}
		risk := 1.0
		rs := "risk = 1"
		if jsOnly {
			risk += w.RiskJSOnly
			rs += fmt.Sprintf(" + js-only %.2f", w.RiskJSOnly)
		}
		if exported {
			risk += w.RiskExported
			rs += fmt.Sprintf(" + exported %.2f", w.RiskExported)
		}
		ids := make([]string, len(us))
		for i, u := range us {
			ids[i] = u.ID
		}
		g.add(&Candidate{
			Kind: kind, Target: f, Units: ids, Area: g.area[f], Location: fmt.Sprintf("%s:%d", f, us[0].Line),
			Payoff: p, Confidence: conf, Risk: risk, Notes: notes,
			Breakdown: []string{why, rs, fmt.Sprintf("%d %s funcs: %s", len(us), reach, strings.Join(names, ", "))},
			Evidence:  map[string]any{"funcs": len(us), "loc": loc, "reach": reach},
			content:   content.String(),
		})
	}
}

func (g *gen) clones() {
	w := g.w
	for _, cg := range g.m.Clones {
		redundant := cg.Lines * (len(cg.Members) - 1)
		p := float64(redundant) * w.ClonePerLine
		conf := map[string]float64{tierExact: w.ConfHigh, tierRenamed: w.ConfMedium, tierStructural: w.ConfLow}[cg.Tier]
		kind := "dedupe-clone"
		if cg.CrossPkg {
			kind = "consolidate-utility"
		}
		var ids, names []string
		var content strings.Builder
		maxFan, covSum := 0, 0.0
		for _, mem := range cg.Members {
			u := g.m.byID[mem.Unit]
			ids = append(ids, u.ID)
			names = append(names, qualified(u))
			content.WriteString(g.source(mem.File, mem.Line, mem.EndLine))
			maxFan = max(maxFan, u.FanIn)
			c, _ := g.cov(u)
			covSum += c
		}
		cov := covSum / float64(len(cg.Members))
		var notes []string
		if cg.Tier != tierExact && cgoBound(content.String()) {
			conf = w.ConfLow
			notes = append(notes, "the copies call C functions through cgo: per-binding boilerplate that generics cannot merge")
		}
		risk := 1 + w.RiskFanIn*math.Log1p(float64(maxFan)) + w.RiskUncovered*(1-cov)
		scope := "regions"
		if cg.Whole {
			scope = "whole functions"
		}
		target := names[0]
		if len(names) > 1 {
			target = fmt.Sprintf("%s (+%d)", names[0], len(names)-1)
		}
		g.add(&Candidate{
			Kind: kind, Target: target, Units: ids, Area: g.area[cg.Members[0].File],
			Location: fmt.Sprintf("%s:%d", cg.Members[0].File, cg.Members[0].Line),
			Payoff:   p, Confidence: conf, Risk: risk, Notes: notes,
			Breakdown: []string{
				fmt.Sprintf("payoff %.1f = %d redundant lines (%d × %d copies − 1) × %.2f", p, redundant, cg.Lines, len(cg.Members), w.ClonePerLine),
				fmt.Sprintf("risk = 1 + max fanIn %d + mean coverage %.0f%%", maxFan, cov*100),
				fmt.Sprintf("%s clone of %s: %s", cg.Tier, scope, strings.Join(names, ", ")),
			},
			Evidence: map[string]any{"tier": cg.Tier, "lines": cg.Lines, "copies": len(cg.Members), "members": cg.Members, "crossPkg": cg.CrossPkg},
			content:  content.String(),
		})
	}
}

func (g *gen) extract() {
	w := g.w
	for _, ts := range g.m.Types {
		if ts.Methods < w.GodMinMethods {
			continue
		}
		for _, fr := range ts.PerFile {
			if fr.Exclusive < w.ExtractMinExclusive || float64(fr.Methods) > w.ExtractMaxShare*float64(ts.Methods) {
				continue
			}
			p := float64(fr.Exclusive)*w.ExtractPerField + float64(fr.MethodCog)*w.ExtractCogShare
			shared := fr.FieldsUsed - fr.Exclusive
			risk := 1 + float64(shared)*w.ExtractSharedRisk
			g.add(&Candidate{
				Kind: "extract-struct", Target: fmt.Sprintf("%s.%s ← %s", ts.Pkg, ts.Name, fr.File),
				Area: g.area[fr.File], Location: fr.File, Payoff: p, Confidence: 0.7, Risk: risk,
				Breakdown: []string{
					fmt.Sprintf("payoff %.1f = %d exclusive fields × %.1f + method cognitive %d × %.2f", p, fr.Exclusive, w.ExtractPerField, fr.MethodCog, w.ExtractCogShare),
					fmt.Sprintf("risk = 1 + %d shared fields × %.2f", shared, w.ExtractSharedRisk),
					fmt.Sprintf("%s has %d methods over %d files and %d fields; %s holds %d of its methods", ts.Name, ts.Methods, ts.FilesSpanned, ts.Fields, fr.File, fr.Methods),
				},
				Evidence: map[string]any{"exclusiveFields": fr.Fields, "fieldsUsed": fr.FieldsUsed, "methods": fr.Methods, "typeMethods": ts.Methods, "typeFields": ts.Fields},
				content:  fmt.Sprint(fr.Fields, fr.Methods, fr.MethodCog),
			})
		}
	}
}

// godTypes ranks whole types: method complexity compounded like an area, plus
// the state the type carries.
func (g *gen) godTypes() {
	w := g.w
	type scored struct {
		ts      *TypeStat
		pow     float64
		cog     int
		content string
	}
	var all []scored
	for _, ts := range g.m.Types {
		if ts.Methods < w.GodMinMethods {
			continue
		}
		s := scored{ts: ts}
		for _, u := range g.m.Units {
			if u.Pkg == ts.Pkg && u.Recv == ts.Name && !u.Generated {
				s.pow += math.Pow(float64(u.Cognitive), compoundExp)
				s.cog += u.Cognitive
			}
		}
		s.content = fmt.Sprint(ts.Fields, ts.Methods, s.cog)
		all = append(all, s)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].pow > all[j].pow })
	for i, s := range all {
		if i == w.TypeTop {
			break
		}
		ts := s.ts
		p := w.AreaPayoffScale*math.Sqrt(s.pow) + w.TypeFieldWeight*float64(ts.Fields)
		var files []string
		for k, f := range ts.PerFile {
			if k == 5 {
				break
			}
			files = append(files, fmt.Sprintf("%s (%d exclusive / %d used)", f.File, f.Exclusive, f.FieldsUsed))
		}
		g.add(&Candidate{
			Kind: "restructure-type", Target: ts.Pkg + "." + ts.Name, Area: ts.Pkg, Location: ts.Pkg + "." + ts.Name,
			Payoff: p, Confidence: 0.5, Risk: 1 + math.Min(1, float64(ts.FilesSpanned)/40),
			Breakdown: []string{
				fmt.Sprintf("payoff %.1f = %.1f × √(Σ method cognitive^%.1f = %.0f) + %d fields × %.2f", p, w.AreaPayoffScale, compoundExp, s.pow, ts.Fields, w.TypeFieldWeight),
				fmt.Sprintf("risk = 1 + %d files spanned / 40 (capped at 1)", ts.FilesSpanned),
				fmt.Sprintf("%d methods (%d lines, Σ cognitive %d) over %d files, %d fields; most separable: %s", ts.Methods, ts.MethodLOC, s.cog, ts.FilesSpanned, ts.Fields, strings.Join(files, ", ")),
			},
			Evidence: map[string]any{"methods": ts.Methods, "fields": ts.Fields, "files": ts.FilesSpanned, "sumCog": s.cog},
			content:  s.content,
		})
	}
}

func (g *gen) files() {
	w := g.w
	for _, f := range g.m.Files {
		if f.Generated || f.Lines < w.SplitFileLines || float64(f.TopUnitLOC) >= w.SplitFileOneUnit*float64(f.Lines) {
			continue
		}
		p := float64(f.Lines-w.SplitFileKeep) * w.SplitFilePer
		g.add(&Candidate{
			Kind: "split-file", Target: f.Path, Area: f.Area, Location: f.Path, Payoff: p, Confidence: 0.8, Risk: 1,
			Breakdown: []string{fmt.Sprintf("payoff %.1f = (%d lines − %d) × %.2f; %d units, largest %d lines", p, f.Lines, w.SplitFileKeep, w.SplitFilePer, f.Units, f.TopUnitLOC)},
			Evidence:  map[string]any{"lines": f.Lines, "units": f.Units, "topUnitLoc": f.TopUnitLOC},
			content:   fmt.Sprint(f.Lines, f.Units),
		})
	}
}

func (g *gen) areas() {
	w := g.w
	n := 0
	for _, a := range g.m.Areas {
		if n >= w.AreaTop {
			break
		}
		if a.Units < w.AreaMinUnits {
			continue
		}
		n++
		p := w.AreaPayoffScale * math.Sqrt(a.Compound)
		risk := 1 + math.Min(1, a.Coupling/4)
		g.add(&Candidate{
			Kind: "restructure-area", Target: a.Name, Area: a.Name, Location: a.Name, Payoff: p, Confidence: 0.5, Risk: risk,
			Breakdown: []string{
				fmt.Sprintf("payoff %.1f = %.1f × √compound %.0f", p, w.AreaPayoffScale, a.Compound),
				fmt.Sprintf("risk = 1 + coupling %.2f / 4 (capped at 1)", a.Coupling),
				fmt.Sprintf("%d files, %d units, Σ cognitive %d, %d units > 30, gini %.2f, clone lines %d, dead lines %d; top: %s",
					a.Files, a.Units, a.SumCog, a.Over30, a.Gini, a.CloneLOC, a.DeadLOC, strings.Join(a.TopUnits, ", ")),
			},
			Evidence: map[string]any{"files": a.Files, "units": a.Units, "sumCog": a.SumCog, "over30": a.Over30, "gini": a.Gini, "coupling": a.Coupling, "compound": a.Compound},
			content:  fmt.Sprint(a.Files, a.Units, a.SumCog),
		})
	}
}

func (g *gen) add(c *Candidate) { g.out = append(g.out, c) }

// toolOnly reports whether every unit (or, for file/area candidates, the
// package) of c is dev tooling.
func (g *gen) toolOnly(c *Candidate) bool {
	if len(c.Units) > 0 {
		for _, id := range c.Units {
			if u := g.m.byID[id]; u == nil || !u.Tool {
				return false
			}
		}
		return true
	}
	return isTool(c.Area, g.m.toolPrefixes)
}

var cgoCall = regexp.MustCompile(`\bC\.[A-Za-z_]\w*\(`)

// cgoBound reports whether code calls C through cgo: clones of such bindings
// differ in which C function they call, which Go cannot abstract over.
func cgoBound(src string) bool { return cgoCall.MatchString(src) }
