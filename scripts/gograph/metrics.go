package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// AnalyzeMetrics walks every non-test .go file under root, groups declarations
// by package import path (module + relative dir), and computes LOC, nesting
// depth, and cyclomatic complexity. It does no type checking.
func AnalyzeMetrics(root, module string) (map[string]*Package, error) {
	fset := token.NewFileSet()
	pkgs := map[string]*Package{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil // tolerate unparsable files (e.g. build-constrained syntax)
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkgPath := module
		if rel != "." {
			pkgPath = module + "/" + filepath.ToSlash(rel)
		}
		p := pkgs[pkgPath]
		if p == nil {
			p = &Package{Path: pkgPath}
			pkgs[pkgPath] = p
		}
		p.Files++
		collectFile(fset, file, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, p := range pkgs {
		renestMethods(p)
	}
	// Sort nothing here; model assembly (Task 4) orders for determinism.
	return pkgs, nil
}

// renestMethods moves any package-level func that is really a method (non-empty
// Recv) — stashed in p.Funcs because its receiver type was declared in a
// different file — under its receiver type. Runs after ALL files in the package
// are collected, so p.Types is complete. Go methods always have a package-local
// receiver type, so every such func nests; any that somehow doesn't match a
// declared type is left as a func.
func renestMethods(p *Package) {
	if len(p.Funcs) == 0 {
		return
	}
	typeIdx := map[string]*TypeInfo{}
	for i := range p.Types {
		typeIdx[p.Types[i].Name] = &p.Types[i]
	}
	kept := p.Funcs[:0]
	for _, f := range p.Funcs {
		if f.Recv != "" {
			if ti := typeIdx[f.Recv]; ti != nil {
				ti.Methods = append(ti.Methods, f)
				continue
			}
		}
		kept = append(kept, f)
	}
	p.Funcs = kept
}

// collectFile appends the file's imports, types, funcs, and methods to p and
// adds the file's LOC to the package total.
func collectFile(fset *token.FileSet, file *ast.File, p *Package) {
	// imports
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if !contains(p.Imports, path) {
			p.Imports = append(p.Imports, path)
		}
	}
	// index type specs so methods can attach to their receiver
	typeIdx := map[string]*TypeInfo{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts := spec.(*ast.TypeSpec)
			ti := TypeInfo{Name: ts.Name.Name, Kind: typeKind(ts.Type), LOC: locOf(fset, ts)}
			p.Types = append(p.Types, ti)
			typeIdx[ts.Name.Name] = &p.Types[len(p.Types)-1]
			p.LOC += ti.LOC
		}
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fm := FuncMetric{
			Name:    fn.Name.Name,
			LOC:     locOf(fset, fn),
			Nesting: bodyNest(fn),
			Cyclo:   cyclomatic(fn),
		}
		p.LOC += fm.LOC
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv := recvName(fn.Recv.List[0].Type)
			fm.Recv = recv
			if ti := typeIdx[recv]; ti != nil {
				ti.Methods = append(ti.Methods, fm)
			} else {
				// receiver type declared in another file of the same package:
				// stash as a package-level func carrying its Recv so Task 4 can
				// still resolve its FuncID and CallDepth.
				p.Funcs = append(p.Funcs, fm)
			}
		} else {
			p.Funcs = append(p.Funcs, fm)
		}
	}
}

func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "named"
	}
}

func recvName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr: // generic receiver Foo[T]
		return recvName(x.X)
	case *ast.IndexListExpr:
		return recvName(x.X)
	default:
		return ""
	}
}

func locOf(fset *token.FileSet, n ast.Node) int {
	start := fset.Position(n.Pos()).Line
	end := fset.Position(n.End()).Line
	if end < start {
		return 1
	}
	return end - start + 1
}

// bodyNest returns the deepest control-flow nesting inside a function body.
func bodyNest(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 0
	}
	return maxNest(fn.Body.List)
}

func maxNest(stmts []ast.Stmt) int {
	best := 0
	for _, s := range stmts {
		if d := stmtNest(s); d > best {
			best = d
		}
	}
	return best
}

func stmtNest(s ast.Stmt) int {
	switch n := s.(type) {
	case *ast.BlockStmt:
		return maxNest(n.List)
	case *ast.IfStmt:
		d := 1 + maxNest(n.Body.List)
		if n.Else != nil {
			var ed int
			if b, ok := n.Else.(*ast.BlockStmt); ok {
				ed = 1 + maxNest(b.List)
			} else {
				ed = stmtNest(n.Else) // else-if chain
			}
			if ed > d {
				d = ed
			}
		}
		return d
	case *ast.ForStmt:
		return 1 + maxNest(n.Body.List)
	case *ast.RangeStmt:
		return 1 + maxNest(n.Body.List)
	case *ast.SwitchStmt:
		return 1 + caseNest(n.Body)
	case *ast.TypeSwitchStmt:
		return 1 + caseNest(n.Body)
	case *ast.SelectStmt:
		return 1 + caseNest(n.Body)
	case *ast.LabeledStmt:
		return stmtNest(n.Stmt)
	default:
		return 0
	}
}

func caseNest(body *ast.BlockStmt) int {
	best := 0
	for _, c := range body.List {
		var inner []ast.Stmt
		switch cc := c.(type) {
		case *ast.CaseClause:
			inner = cc.Body
		case *ast.CommClause:
			inner = cc.Body
		}
		if d := maxNest(inner); d > best {
			best = d
		}
	}
	return best
}

// cyclomatic is McCabe complexity: 1 + branch/loop/case/&&/|| count.
func cyclomatic(fn *ast.FuncDecl) int {
	cc := 1
	ast.Inspect(fn, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			cc++
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				cc++
			}
		}
		return true
	})
	return cc
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
