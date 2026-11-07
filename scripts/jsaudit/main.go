// jsaudit reports every value handed to syscall/js that js.ValueOf cannot convert (it panics on them, in the browser only):
// typed ids and enums are named types ValueOf does not switch on. Checks ValueOf / Set / Call / Invoke / New arguments,
// []any and map[string]any elements, and what jsSafe cannot convert in jsFn results. Usage: jsaudit <module dir> <pkgs...>
package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: jsaudit <module dir> <packages...>")
		os.Exit(2)
	}
	findings, err := audit(os.Args[1], os.Args[2:]...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "jsaudit:", err)
		os.Exit(2)
	}
	for _, f := range findings {
		fmt.Println(f)
	}
	if len(findings) > 0 {
		fmt.Fprintf(os.Stderr, "jsaudit: %d value(s) js.ValueOf would panic on; pass string(id) / id.String()\n", len(findings))
		os.Exit(1)
	}
}

func audit(dir string, patterns ...string) ([]string, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedImports,
		Dir: dir,
		Env: append(os.Environ(), "GOOS=js", "GOARCH=wasm"),
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range pkgs {
		if p.TypesInfo == nil {
			continue
		}
		for _, f := range p.Syntax {
			if importsSyscallJS(f) {
				out = append(out, auditFile(p.Fset, p.TypesInfo, f)...)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func importsSyscallJS(f *ast.File) bool {
	for _, im := range f.Imports {
		if im.Path.Value == `"syscall/js"` {
			return true
		}
	}
	return false
}

func auditFile(fset *token.FileSet, info *types.Info, f *ast.File) []string {
	var out []string
	report := func(e ast.Expr, ok func(types.Type) bool, why string) {
		tv, found := info.Types[e]
		if !found || tv.Type == nil || ok(tv.Type) {
			return
		}
		out = append(out, fmt.Sprintf("%s: %s %s", fset.Position(e.Pos()), tv.Type, why))
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if fn := calledFunc(info, x); fn != nil {
				if fn.Pkg() != nil && fn.Pkg().Path() == "syscall/js" {
					switch fn.Name() {
					case "ValueOf", "Set", "SetIndex", "Call", "Invoke", "New":
						for _, a := range x.Args {
							report(a, valueOfAccepts, "-> js."+fn.Name())
						}
					}
				}
				if fn.Name() == "jsFn" && len(x.Args) == 1 {
					if lit, ok := x.Args[0].(*ast.FuncLit); ok {
						for _, r := range returnsOf(lit) {
							report(r, jsSafeAccepts, "returned through jsFn")
						}
					}
				}
			}
			if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "append" && len(x.Args) > 1 && isAnySlice(info.TypeOf(x.Args[0])) {
				for _, a := range x.Args[1:] {
					report(a, valueOfAccepts, "appended to []any")
				}
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				if ix, ok := lhs.(*ast.IndexExpr); ok && i < len(x.Rhs) && isAnyMap(info.TypeOf(ix.X)) {
					report(x.Rhs[i], valueOfAccepts, "stored in map[string]any")
				}
			}
		case *ast.CompositeLit:
			t := info.TypeOf(x)
			switch {
			case isAnySlice(t):
				for _, el := range x.Elts {
					report(el, valueOfAccepts, "in []any literal")
				}
			case isAnyMap(t):
				for _, el := range x.Elts {
					if kv, ok := el.(*ast.KeyValueExpr); ok {
						report(kv.Value, valueOfAccepts, "in map[string]any literal")
					}
				}
			}
		}
		return true
	})
	return out
}

func calledFunc(info *types.Info, c *ast.CallExpr) *types.Func {
	switch f := c.Fun.(type) {
	case *ast.SelectorExpr:
		fn, _ := info.Uses[f.Sel].(*types.Func)
		return fn
	case *ast.Ident:
		fn, _ := info.Uses[f].(*types.Func)
		return fn
	}
	return nil
}

// returnsOf lists the results the function literal itself returns (not those
// of literals nested in it).
func returnsOf(lit *ast.FuncLit) []ast.Expr {
	var out []ast.Expr
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			out = append(out, x.Results...)
		}
		return true
	})
	return out
}

func isAnySlice(t types.Type) bool {
	if t == nil {
		return false
	}
	s, ok := t.Underlying().(*types.Slice)
	return ok && isEmptyInterface(s.Elem())
}

func isAnyMap(t types.Type) bool {
	if t == nil {
		return false
	}
	m, ok := t.Underlying().(*types.Map)
	return ok && isEmptyInterface(m.Elem())
}

func isEmptyInterface(t types.Type) bool {
	i, ok := types.Unalias(t).Underlying().(*types.Interface)
	return ok && i.Empty()
}

// valueOfAccepts mirrors the cases js.ValueOf switches on. An interface is
// accepted: its dynamic value cannot be known statically.
func valueOfAccepts(t types.Type) bool {
	t = types.Unalias(t)
	if _, ok := t.Underlying().(*types.Interface); ok {
		return true
	}
	if n, ok := t.(*types.Named); ok {
		obj := n.Obj()
		if obj.Pkg() != nil && obj.Pkg().Path() == "syscall/js" {
			return true // js.Value, js.Func
		}
		return false // any other named type: ValueOf's type switch never matches it
	}
	switch u := t.(type) {
	case *types.Basic:
		return u.Info()&types.IsComplex == 0
	case *types.Slice:
		return isEmptyInterface(u.Elem())
	case *types.Map:
		k, ok := u.Key().(*types.Basic)
		return ok && k.Kind() == types.String && isEmptyInterface(u.Elem())
	}
	return false
}

// jsSafeAccepts is what ui.jsSafe can turn into a ValueOf-able value: what
// ValueOf takes, plus named basic types and structs with a MarshalText.
func jsSafeAccepts(t types.Type) bool {
	if valueOfAccepts(t) {
		return true
	}
	t = types.Unalias(t)
	if _, ok := t.Underlying().(*types.Basic); ok {
		return true
	}
	if _, ok := t.Underlying().(*types.Struct); ok {
		return hasMarshalText(t)
	}
	return false
}

func hasMarshalText(t types.Type) bool {
	ms := types.NewMethodSet(t)
	for i := 0; i < ms.Len(); i++ {
		if ms.At(i).Obj().Name() == "MarshalText" {
			return true
		}
	}
	return false
}
