package rank

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// srcFile is one production file parsed from source (not the cgo-compiled
// form), with the configurations that compile it.
type srcFile struct {
	Rel, Pkg  string
	AST       *ast.File
	Src       []byte
	Generated bool
	Configs   []string
}

type sourceSet struct {
	root  string
	fset  *token.FileSet
	files map[string]*srcFile // by root-relative path
	order []string
}

// collectSources parses every file a production config compiles. Metrics come
// from this parse, so a file is measured once however many configs build it,
// and a build-tag variant only counts for the configs that build it.
func collectSources(l *Loaded) (*sourceSet, error) {
	ss := &sourceSet{root: l.Root, fset: token.NewFileSet(), files: map[string]*srcFile{}}
	for _, c := range l.Prod {
		for _, p := range c.module {
			for _, abs := range p.GoFiles {
				rel, err := filepath.Rel(l.Root, abs)
				if err != nil || strings.HasPrefix(rel, "..") {
					continue
				}
				rel = filepath.ToSlash(rel)
				sf := ss.files[rel]
				if sf == nil {
					src, err := os.ReadFile(abs)
					if err != nil {
						return nil, err
					}
					f, err := parser.ParseFile(ss.fset, rel, src, parser.ParseComments|parser.SkipObjectResolution)
					if err != nil {
						continue // unparsable: the type-checker reported it already
					}
					sf = &srcFile{Rel: rel, Pkg: relPkg(p.PkgPath, l.Module), AST: f, Src: src, Generated: ast.IsGenerated(f)}
					ss.files[rel] = sf
					ss.order = append(ss.order, rel)
				}
				if !slices.Contains(sf.Configs, c.cfg.Name) {
					sf.Configs = append(sf.Configs, c.cfg.Name)
				}
			}
		}
	}
	sort.Strings(ss.order)
	for _, sf := range ss.files {
		sort.Strings(sf.Configs)
	}
	return ss, nil
}

// collectUnits turns every function declaration and function literal into a
// Unit with its own metrics (a closure's body never counts for its parent).
func collectUnits(ss *sourceSet) []*Unit {
	var units []*Unit
	for _, rel := range ss.order {
		sf := ss.files[rel]
		for _, d := range sf.AST.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			u := newUnit(ss, sf, fd.Pos(), fd.End(), fd.Recv, fd.Type, fd.Body)
			u.bareName = fd.Name.Name
			u.Name = fd.Name.Name
			u.Kind = "func"
			u.Exported = fd.Name.IsExported()
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				u.Kind = "method"
				u.Recv = recvTypeName(fd.Recv.List[0].Type)
				u.Name = u.Recv + "." + fd.Name.Name
				if len(fd.Recv.List[0].Names) > 0 {
					u.recvIdent = fd.Recv.List[0].Names[0].Name
				}
			}
			nameLine := ss.fset.Position(fd.Name.Pos()).Line
			u.ID = declUnitID(ss.fset, rel, fd)
			u.declKey = fmt.Sprintf("%s:%d:%s", rel, nameLine, fd.Name.Name)
			u.Cognitive, u.NestBonus = Cognitive(fd.Body, fd.Name.Name)
			units = append(units, u)
			units = append(units, closures(ss, sf, u, fd.Body)...)
		}
	}
	// A unit's own lines exclude its direct closures (they are units too).
	byID := map[string]*Unit{}
	for _, u := range units {
		u.OwnSLOC = u.SLOC
		byID[u.ID] = u
	}
	for _, u := range units {
		if p := byID[u.Parent]; p != nil {
			p.OwnSLOC -= u.SLOC
		}
	}
	return units
}

// closures returns the function literals directly or transitively inside
// body, each as a unit named Parent.funcN in source order.
func closures(ss *sourceSet, sf *srcFile, top *Unit, body ast.Node) []*Unit {
	var out []*Unit
	n := 0
	var walk func(node ast.Node, parent *Unit)
	walk = func(node ast.Node, parent *Unit) {
		ast.Inspect(node, func(x ast.Node) bool {
			fl, ok := x.(*ast.FuncLit)
			if !ok || x == node {
				return true
			}
			n++
			u := newUnit(ss, sf, fl.Pos(), fl.End(), nil, fl.Type, fl.Body)
			u.Kind = "closure"
			u.Parent = parent.ID
			u.Name = fmt.Sprintf("%s.func%d", top.Name, n)
			u.Recv = top.Recv
			u.recvIdent = top.recvIdent
			pos := ss.fset.Position(fl.Pos())
			u.ID = fmt.Sprintf("%s:%d:%d", sf.Rel, pos.Line, pos.Column)
			u.Cognitive, u.NestBonus = Cognitive(fl.Body, "")
			out = append(out, u)
			walk(fl, u)
			return false
		})
	}
	walk(body, top)
	return out
}

func newUnit(ss *sourceSet, sf *srcFile, pos, end token.Pos, recv *ast.FieldList, ft *ast.FuncType, body *ast.BlockStmt) *Unit {
	start, stop := ss.fset.Position(pos).Line, ss.fset.Position(end).Line
	u := &Unit{
		Pkg:       sf.Pkg,
		File:      sf.Rel,
		Line:      start,
		EndLine:   stop,
		Generated: sf.Generated,
		Configs:   sf.Configs,
		LOC:       stop - start + 1,
		SLOC:      SLOC(sf.Src, start, stop),
		Cyclo:     Cyclomatic(body),
		Nesting:   MaxNesting(body),
		DataRatio: round3(DataRatio(ss.fset, body)),
		callers:   map[string]bool{},
		callees:   map[string]bool{},
	}
	u.JSOnly = len(u.Configs) == 1 && u.Configs[0] == "js"
	if ft.Params != nil {
		u.Params = ft.Params.NumFields()
	}
	if ft.Results != nil {
		u.Results = ft.Results.NumFields()
	}
	u.Data = u.Cyclo <= 3 && u.DataRatio >= 0.7
	u.Verb = measureVerbosity(ss.fset, body, localsOf(recv, ft, body))
	return u
}

func recvTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return recvTypeName(x.X)
	case *ast.IndexListExpr:
		return recvTypeName(x.X)
	case *ast.ParenExpr:
		return recvTypeName(x.X)
	}
	return ""
}

// unitIndex finds units by declaration key and by source line.
type unitIndex struct {
	byDecl map[string]*Unit
	byFile map[string][]*Unit
}

func newUnitIndex(units []*Unit) *unitIndex {
	ix := &unitIndex{byDecl: map[string]*Unit{}, byFile: map[string][]*Unit{}}
	for _, u := range units {
		if u.declKey != "" {
			ix.byDecl[u.declKey] = u
		}
		ix.byFile[u.File] = append(ix.byFile[u.File], u)
	}
	return ix
}

// at returns the innermost unit whose span contains line in file.
func (ix *unitIndex) at(file string, line int) *Unit {
	var best *Unit
	for _, u := range ix.byFile[file] {
		if line >= u.Line && line <= u.EndLine && (best == nil || u.EndLine-u.Line < best.EndLine-best.Line) {
			best = u
		}
	}
	return best
}

// computeFans counts calls and function values alike (a callback registration is a dependency); interface method
// calls resolve to the interface and are not counted.
func computeFans(l *Loaded, m *Model) {
	ix := newUnitIndex(m.Units)
	for _, c := range l.Prod {
		for _, p := range c.module {
			if p.TypesInfo == nil {
				continue
			}
			for id, obj := range p.TypesInfo.Uses {
				fn, ok := obj.(*types.Func)
				if !ok || fn.Pkg() == nil || !inModule(fn.Pkg().Path(), l.Module) {
					continue
				}
				fn = fn.Origin()
				callee := ix.byDecl[declKeyOf(l.Root, p.Fset, fn.Pos(), fn.Name())]
				pos := p.Fset.Position(id.Pos())
				rel, ok := relFile(l.Root, pos.Filename)
				if !ok || callee == nil {
					continue
				}
				caller := ix.at(rel, pos.Line)
				if caller == nil || caller == callee {
					continue
				}
				caller.callees[callee.ID] = true
				callee.callers[caller.ID] = true
			}
		}
	}
	for _, u := range m.Units {
		u.FanIn, u.FanOut = len(u.callers), len(u.callees)
	}
}

func declKeyOf(root string, fset *token.FileSet, pos token.Pos, name string) string {
	p := fset.Position(pos)
	rel, ok := relFile(root, p.Filename)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%s:%d:%s", rel, p.Line, name)
}

func relFile(root, abs string) (string, bool) {
	if abs == "" {
		return "", false
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func round3(f float64) float64 {
	return float64(int64(f*1000+0.5)) / 1000
}
