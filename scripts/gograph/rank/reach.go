package rank

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// reachability classifies every function (closures inherit their parent) by RTA over SSA, as x/tools' deadcode does:
// prod, test-only, or dead ("unsplit" with the test pass skipped). A config gives evidence only for packages it
// type-checked cleanly; a unit with none stays unclassified.
func reachability(l *Loaded, m *Model, fast bool) error {
	type result struct {
		name    string
		reached map[string]bool
		// Packages whose callers this config cannot fully see (they or an importer failed to type-check; nil + allBlind when a
		// root failed): it may prove their units reachable, never unreachable.
		blind    map[string]bool
		allBlind bool
	}
	run := func(c *cfgLoad) result {
		r, blind, all := rtaReach(c, l.Root, l.Module)
		return result{c.cfg.Name, r, blind, all}
	}
	var prod, test []result
	for _, c := range l.Prod {
		prod = append(prod, run(c))
	}
	for _, bc := range l.TestCfgs {
		c, err := loadOne(l.Root, l.Module, bc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gograph rank: %s: %v (test reachability skipped)\n", bc.Name, err)
			continue
		}
		l.noteCompiled(c)
		test = append(test, run(c))
	}
	for _, u := range m.Units {
		if u.Kind == "closure" {
			continue
		}
		evidence, reached, incomplete := false, false, false
		for _, r := range prod {
			if !slices.Contains(u.Configs, r.name) {
				continue
			}
			reached = reached || r.reached[u.declKey]
			if r.allBlind || r.blind[u.Pkg] {
				incomplete = true
			} else {
				evidence = true
			}
		}
		switch {
		case reached:
			u.Reach = "prod"
		case !evidence || incomplete:
		case fast:
			u.Reach = "unsplit"
		default:
			u.Reach = "dead"
			for _, r := range test {
				if r.reached[u.declKey] {
					u.Reach = "test-only"
				}
			}
		}
	}
	markVariants(m)
	for _, u := range m.Units {
		if u.Kind == "closure" {
			u.Reach = topParent(m, u).Reach
		}
	}
	findUnanalyzedRefs(m, l)
	return nil
}

// unreachable reports whether a reach class means no production caller.
func unreachable(reach string) bool {
	return reach == "dead" || reach == "test-only" || reach == "unsplit"
}

// markVariants: an unreachable function with a reachable build-tag sibling is the other build's implementation, needed
// by callers this run does not compile, never dead code.
func markVariants(m *Model) {
	live := map[string]bool{}
	for _, u := range m.Units {
		if u.Kind != "closure" && (u.Reach == "prod" || u.Reach == "test-only") {
			live[u.Pkg+"."+u.Name] = true
		}
	}
	for _, u := range m.Units {
		if u.Kind == "closure" || !unreachable(u.Reach) || !live[u.Pkg+"."+u.Name] {
			continue
		}
		for _, o := range m.Units {
			if o != u && o.Pkg == u.Pkg && o.Name == u.Name && o.File != u.File && (o.Reach == "prod" || o.Reach == "test-only") {
				u.Reach = "variant"
				break
			}
		}
	}
}

var lineComment = regexp.MustCompile(`//.*`)

// findUnanalyzedRefs records, for unreachable units, the files of their
// package directory that no analysed config compiles (a js && test build, a
// `ignore` tool) yet name them: deleting the unit could break that build.
func findUnanalyzedRefs(m *Model, l *Loaded) {
	byDir := map[string][]string{}
	srcs := map[string]string{}
	for _, u := range m.Units {
		if u.Kind == "closure" || !unreachable(u.Reach) {
			continue
		}
		dir := u.Pkg
		if _, ok := byDir[dir]; !ok {
			byDir[dir] = nil
			files, _ := filepath.Glob(filepath.Join(l.Root, filepath.FromSlash(dir), "*.go"))
			sort.Strings(files)
			for _, f := range files {
				rel, ok := relFile(l.Root, f)
				if !ok || l.compiled[rel] {
					continue
				}
				b, err := os.ReadFile(f)
				if err != nil {
					continue
				}
				byDir[dir] = append(byDir[dir], rel)
				srcs[rel] = lineComment.ReplaceAllString(string(b), "")
			}
		}
		pat := `(^|[^.\w])` + regexp.QuoteMeta(u.bareName) + `\b`
		if u.Kind == "method" {
			pat = `\.` + regexp.QuoteMeta(u.bareName) + `\b`
		}
		re := regexp.MustCompile(pat)
		for _, rel := range byDir[dir] {
			if rel != u.File && re.MatchString(srcs[rel]) {
				u.UnanalyzedRefs = append(u.UnanalyzedRefs, rel)
			}
		}
	}
}

func topParent(m *Model, u *Unit) *Unit {
	for u.Parent != "" && m.byID[u.Parent] != nil {
		u = m.byID[u.Parent]
	}
	return u
}

// rtaReach builds SSA for every cleanly type-checked package of the config and returns the reachable in-module
// declarations, the packages whose callers it cannot fully see, and whether a root failed.
func rtaReach(c *cfgLoad, root, module string) (reached, blind map[string]bool, allBlind bool) {
	tainted := map[*packages.Package]bool{}
	var order []*packages.Package
	packages.Visit(c.initial, nil, func(p *packages.Package) {
		bad := p.IllTyped || len(p.Errors) > 0 || p.Types == nil
		for _, imp := range p.Imports {
			bad = bad || tainted[imp]
		}
		tainted[p] = bad
		order = append(order, p)
	})
	blind = map[string]bool{}
	for _, p := range order {
		if !tainted[p] || !inModule(p.PkgPath, module) {
			continue
		}
		blind[relPkg(p.PkgPath, module)] = true
		for _, imp := range p.Imports {
			if inModule(imp.PkgPath, module) {
				blind[relPkg(imp.PkgPath, module)] = true
			}
		}
	}
	var fset = c.initial[0].Fset
	prog := ssa.NewProgram(fset, ssa.InstantiateGenerics)
	ssaOf := map[*packages.Package]*ssa.Package{}
	for _, p := range order {
		if !tainted[p] {
			ssaOf[p] = prog.CreatePackage(p.Types, p.Syntax, p.TypesInfo, true)
		}
	}
	prog.Build()
	var roots []*ssa.Function
	for _, p := range c.roots {
		sp := ssaOf[p]
		if sp == nil {
			fmt.Fprintf(os.Stderr, "gograph rank: %s: root %s has errors; its reachability is unknown\n", c.cfg.Name, p.ID)
			allBlind = true
			continue
		}
		for _, name := range []string{"init", "main"} {
			if f := sp.Func(name); f != nil {
				roots = append(roots, f)
			}
		}
	}
	reached = map[string]bool{}
	if len(roots) == 0 {
		return reached, blind, true
	}
	res := rta.Analyze(roots, false)
	for fn := range res.Reachable {
		if o := fn.Origin(); o != nil {
			fn = o
		}
		if fn.Parent() != nil || fn.Pkg == nil || !inModule(fn.Pkg.Pkg.Path(), module) || !fn.Pos().IsValid() {
			continue
		}
		name := fn.Name()
		if strings.HasPrefix(name, "init#") { // explicit init funcs are init#1, init#2, … in SSA
			name = "init"
		}
		if k := declKeyOf(root, fset, fn.Pos(), name); k != "" {
			reached[k] = true
		}
	}
	return reached, blind, allBlind
}
