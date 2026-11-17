package rank

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Area rule: a package with at least areaMinPkgFiles files is split into
// file-prefix groups; a prefix group above areaSplitFiles files splits again
// by its first two tokens wherever such a subgroup has areaMinSubFiles files.
const (
	areaMinPkgFiles = 15
	areaSplitFiles  = 25
	areaMinSubFiles = 3
	compoundExp     = 1.3
)

// areaOf returns the area of file (a base name) inside pkg, given every base
// name in that package.
func areaOf(pkg, file string, pkgFiles []string) string {
	if len(pkgFiles) < areaMinPkgFiles {
		return pkg
	}
	toks := fileTokens(file)
	one := toks[0]
	n1 := 0
	for _, f := range pkgFiles {
		if fileTokens(f)[0] == one {
			n1++
		}
	}
	if n1 > areaSplitFiles && len(toks) > 1 {
		two := toks[0] + "_" + toks[1]
		n2 := 0
		for _, f := range pkgFiles {
			if t := fileTokens(f); len(t) > 1 && t[0]+"_"+t[1] == two {
				n2++
			}
		}
		if n2 >= areaMinSubFiles {
			return pkg + "/" + two + "_*"
		}
	}
	return pkg + "/" + one + "_*"
}

func fileTokens(base string) []string {
	return strings.Split(strings.TrimSuffix(base, ".go"), "_")
}

// buildStructure fills Files, Types, Areas and Pkgs.
func buildStructure(m *Model) {
	pkgFiles := map[string][]string{}
	for _, rel := range m.src.order {
		sf := m.src.files[rel]
		pkgFiles[sf.Pkg] = append(pkgFiles[sf.Pkg], path.Base(rel))
	}
	fileArea := map[string]string{}
	fileByPath := map[string]*File{}
	for _, rel := range m.src.order {
		sf := m.src.files[rel]
		f := &File{
			Path: rel, Pkg: sf.Pkg, Area: areaOf(sf.Pkg, path.Base(rel), pkgFiles[sf.Pkg]),
			Lines: bytes.Count(sf.Src, []byte("\n")), Generated: sf.Generated, Configs: sf.Configs,
		}
		fileArea[rel] = f.Area
		fileByPath[rel] = f
		m.Files = append(m.Files, f)
	}
	for _, u := range m.Units {
		if u.Kind == "closure" {
			continue
		}
		f := fileByPath[u.File]
		f.Units++
		f.TopUnitLOC = max(f.TopUnitLOC, u.LOC)
	}
	m.Types = typeStats(m)
	m.Areas = aggregate(m, func(u *Unit) string { return fileArea[u.File] }, fileByPath, func(f *File) string { return f.Area })
	m.Pkgs = aggregate(m, func(u *Unit) string { return u.Pkg }, fileByPath, func(f *File) string { return f.Pkg })
}

// typeStats measures every struct type with methods: how many fields, how its
// methods spread over files, and which fields only one file's methods touch.
func typeStats(m *Model) []*TypeStat {
	type decl struct {
		pkg, name string
		fields    []string
	}
	var decls []decl
	for _, rel := range m.src.order {
		sf := m.src.files[rel]
		for _, d := range sf.AST.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, s := range gd.Specs {
				ts := s.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				var fields []string
				for _, fl := range st.Fields.List {
					for _, n := range fl.Names {
						fields = append(fields, n.Name)
					}
					if len(fl.Names) == 0 {
						fields = append(fields, recvTypeName(embeddedType(fl.Type)))
					}
				}
				decls = append(decls, decl{sf.Pkg, ts.Name.Name, fields})
			}
		}
	}
	methods := map[string][]*Unit{}
	for _, u := range m.Units {
		if u.Kind == "method" {
			methods[u.Pkg+"."+u.Recv] = append(methods[u.Pkg+"."+u.Recv], u)
		}
	}
	decls2 := map[string]*ast.FuncDecl{}
	for _, rel := range m.src.order {
		sf := m.src.files[rel]
		for _, d := range sf.AST.Decls {
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv != nil && fd.Body != nil {
				decls2[declUnitID(m.src.fset, rel, fd)] = fd
			}
		}
	}
	var out []*TypeStat
	for _, d := range decls {
		ms := methods[d.pkg+"."+d.name]
		if len(ms) == 0 {
			continue
		}
		fieldSet := map[string]bool{}
		for _, f := range d.fields {
			fieldSet[f] = true
		}
		ts := &TypeStat{Pkg: d.pkg, Name: d.name, Fields: len(d.fields), Methods: len(ms)}
		perFile := map[string]*FileReadiness{}
		fieldFiles := map[string]map[string]bool{}
		fileFields := map[string]map[string]bool{}
		for _, u := range ms {
			ts.MethodLOC += u.LOC
			fr := perFile[u.File]
			if fr == nil {
				fr = &FileReadiness{File: u.File}
				perFile[u.File] = fr
				fileFields[u.File] = map[string]bool{}
			}
			fr.Methods++
			fr.MethodCog += u.Cognitive
			fd := decls2[u.ID]
			if fd == nil || u.recvIdent == "" {
				continue
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				se, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if id, ok := se.X.(*ast.Ident); ok && id.Name == u.recvIdent && fieldSet[se.Sel.Name] {
					fileFields[u.File][se.Sel.Name] = true
					if fieldFiles[se.Sel.Name] == nil {
						fieldFiles[se.Sel.Name] = map[string]bool{}
					}
					fieldFiles[se.Sel.Name][u.File] = true
				}
				return true
			})
		}
		for file, fr := range perFile {
			fr.FieldsUsed = len(fileFields[file])
			for f := range fileFields[file] {
				if len(fieldFiles[f]) == 1 {
					fr.Exclusive++
					fr.Fields = append(fr.Fields, f)
				}
			}
			sort.Strings(fr.Fields)
			ts.PerFile = append(ts.PerFile, *fr)
		}
		ts.FilesSpanned = len(perFile)
		sort.Slice(ts.PerFile, func(i, j int) bool {
			if ts.PerFile[i].Exclusive != ts.PerFile[j].Exclusive {
				return ts.PerFile[i].Exclusive > ts.PerFile[j].Exclusive
			}
			return ts.PerFile[i].File < ts.PerFile[j].File
		})
		out = append(out, ts)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Methods != out[j].Methods {
			return out[i].Methods > out[j].Methods
		}
		return out[i].Pkg+"."+out[i].Name < out[j].Pkg+"."+out[j].Name
	})
	return out
}

func embeddedType(e ast.Expr) ast.Expr {
	if se, ok := e.(*ast.SelectorExpr); ok {
		return se.Sel
	}
	return e
}

// aggregate computes compounded complexity per group (area or package).
func aggregate(m *Model, groupOf func(*Unit) string, files map[string]*File, fileGroup func(*File) string) []*AreaStat {
	stats := map[string]*AreaStat{}
	get := func(name, pkg string) *AreaStat {
		s := stats[name]
		if s == nil {
			s = &AreaStat{Name: name, Pkg: pkg}
			stats[name] = s
		}
		return s
	}
	for _, f := range m.Files {
		s := get(fileGroup(f), f.Pkg)
		s.Files++
		s.Lines += f.Lines
	}
	cogs := map[string][]int{}
	pow := map[string]float64{}
	top := map[string][]*Unit{}
	ext := map[string]map[string]bool{}
	extIn := map[string]map[string]bool{}
	for _, u := range m.Units {
		if u.Generated {
			continue
		}
		g := groupOf(u)
		s := get(g, u.Pkg)
		s.Units++
		s.SumCog += u.Cognitive
		if u.Cognitive > 15 {
			s.Over15++
		}
		if u.Cognitive > 30 {
			s.Over30++
		}
		if u.Cognitive > 60 {
			s.Over60++
		}
		if u.Reach == "dead" && u.Kind != "closure" {
			s.DeadLOC += u.LOC
		}
		if u.Kind != "closure" {
			cogs[g] = append(cogs[g], u.Cognitive) // Gini over real functions only
		}
		pow[g] += math.Pow(float64(u.Cognitive), compoundExp)
		top[g] = append(top[g], u)
		for id := range u.callees {
			if c := m.byID[id]; c != nil && groupOf(c) != g {
				if ext[g] == nil {
					ext[g] = map[string]bool{}
				}
				ext[g][id] = true
			}
		}
		for id := range u.callers {
			if c := m.byID[id]; c != nil && groupOf(c) != g {
				if extIn[g] == nil {
					extIn[g] = map[string]bool{}
				}
				extIn[g][id] = true
			}
		}
	}
	for _, cg := range m.Clones {
		share := float64(cg.Lines) * float64(len(cg.Members)-1) / float64(len(cg.Members))
		for _, mem := range cg.Members {
			if u := m.byID[mem.Unit]; u != nil {
				get(groupOf(u), u.Pkg).CloneLOC += int(math.Round(share))
			}
		}
	}
	var out []*AreaStat
	for name, s := range stats {
		s.ExtCallees, s.ExtCallers = len(ext[name]), len(extIn[name])
		if s.Units > 0 {
			s.Coupling = round3(float64(s.ExtCallees+s.ExtCallers) / float64(s.Units))
		}
		s.Gini = round3(gini(cogs[name]))
		s.Compound = round3(pow[name]*(1+math.Min(1, s.Coupling/4)) + float64(s.CloneLOC+s.DeadLOC))
		us := top[name]
		sort.Slice(us, func(i, j int) bool {
			if us[i].Cognitive != us[j].Cognitive {
				return us[i].Cognitive > us[j].Cognitive
			}
			return us[i].ID < us[j].ID
		})
		for i := 0; i < len(us) && i < 5; i++ {
			s.TopUnits = append(s.TopUnits, us[i].Name)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Compound != out[j].Compound {
			return out[i].Compound > out[j].Compound
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// gini is the Gini coefficient of xs: 0 when complexity is spread evenly, near
// 1 when a few units hold all of it.
func gini(xs []int) float64 {
	n := len(xs)
	if n == 0 {
		return 0
	}
	s := append([]int(nil), xs...)
	sort.Ints(s)
	sum, weighted := 0.0, 0.0
	for i, x := range s {
		sum += float64(x)
		weighted += float64(i+1) * float64(x)
	}
	if sum == 0 {
		return 0
	}
	return 2*weighted/(float64(n)*sum) - float64(n+1)/float64(n)
}

var identRE = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// testFile is one _test.go file: its package directory, imports and names.
type testFile struct {
	rel, dir string
	imports  map[string]bool
	idents   map[string]bool
}

// collectTestRefs records, per unit, the test files of its own package that
// mention its name (evidence of which tests guard a refactor of it), and, for
// exported unreachable units, test files of other packages that import its
// package and name it (test-support API that cannot move into _test.go).
func collectTestRefs(m *Model, root, module string) {
	var tests []*testFile
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if n := d.Name(); n == "testdata" || n == "vendor" || (strings.HasPrefix(n, ".") && p != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := relFile(root, p)
		tf := &testFile{rel: rel, dir: path.Dir(rel), imports: map[string]bool{}, idents: map[string]bool{}}
		if f, err := parser.ParseFile(token.NewFileSet(), p, src, parser.ImportsOnly); err == nil {
			for _, imp := range f.Imports {
				tf.imports[strings.Trim(imp.Path.Value, `"`)] = true
			}
		}
		for _, id := range identRE.FindAll(src, -1) {
			tf.idents[string(id)] = true
		}
		tests = append(tests, tf)
		return nil
	})
	sort.Slice(tests, func(i, j int) bool { return tests[i].rel < tests[j].rel })
	byDir := map[string][]*testFile{}
	for _, tf := range tests {
		byDir[tf.dir] = append(byDir[tf.dir], tf)
	}
	for _, u := range m.Units {
		if u.Kind == "closure" || u.Generated {
			continue
		}
		dir := u.Pkg
		var refs []string
		for _, tf := range byDir[dir] {
			if tf.idents[u.bareName] {
				refs = append(refs, tf.rel)
			}
		}
		if len(refs) > 8 {
			refs = refs[:8]
		}
		u.TestRefs = refs
		if !u.Exported || !unreachable(u.Reach) {
			continue
		}
		imp := module
		if u.Pkg != "." {
			imp = module + "/" + u.Pkg
		}
		for _, tf := range tests {
			if tf.dir != dir && tf.imports[imp] && tf.idents[u.bareName] {
				u.CrossPkgTestRefs = append(u.CrossPkgTestRefs, tf.rel)
			}
		}
	}
}

// computeTotals summarises the model.
func computeTotals(m *Model) {
	t := &m.Totals
	for _, u := range m.Units {
		if u.Generated {
			if u.Kind != "closure" {
				t.GeneratedSLOC += u.SLOC
			}
			continue
		}
		t.Units++
		t.SumCognitive += u.Cognitive
		t.SumCyclo += u.Cyclo
		if u.Kind != "closure" {
			t.SLOC += u.SLOC
		}
		switch {
		case u.Cognitive > 60:
			t.UnitsOverCog60++
			fallthrough
		case u.Cognitive > 30:
			t.UnitsOverCog30++
			fallthrough
		case u.Cognitive > 15:
			t.UnitsOverCog15++
		}
		if u.Kind == "closure" {
			continue
		}
		switch u.Reach {
		case "dead":
			t.DeadUnits++
			t.DeadLOC += u.LOC
		case "test-only":
			t.TestOnlyUnits++
			t.TestOnlyLOC += u.LOC
		case "unsplit":
			t.UnsplitUnits++
		}
	}
	for _, g := range m.Clones {
		t.CloneGroups++
		t.CloneLOC += g.Lines * (len(g.Members) - 1)
	}
	for _, f := range m.Files {
		if f.Lines >= 1500 && !f.Generated {
			t.FilesOver1500++
		}
	}
}
