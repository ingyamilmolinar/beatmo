package rank

import (
	"go/ast"
	"go/token"
)

// measureVerbosity computes the readability proxies for one body (closures
// excluded: they are measured as their own units).
func measureVerbosity(fset *token.FileSet, body *ast.BlockStmt, locals map[string]bool) Verbosity {
	var v Verbosity
	if body == nil {
		return v
	}
	v.RepeatedLines = repeatedLines(fset, body, locals)
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.SwitchStmt:
			v.SimilarCases += similarCases(x.Body)
		case *ast.TypeSwitchStmt:
			v.SimilarCases += similarCases(x.Body)
		case *ast.SelectorExpr:
			v.MaxChain = max(v.MaxChain, chainLen(x))
		case *ast.BinaryExpr:
			if x.Op == token.LAND || x.Op == token.LOR {
				var ops []token.Token
				var leaves []ast.Expr
				flattenLogical(x, &ops, &leaves)
				v.MaxCondOps = max(v.MaxCondOps, len(ops))
				for _, l := range leaves {
					ast.Inspect(l, func(n ast.Node) bool {
						if s, ok := n.(*ast.SelectorExpr); ok {
							v.MaxChain = max(v.MaxChain, chainLen(s))
						}
						_, lit := n.(*ast.FuncLit)
						return !lit
					})
				}
				return false
			}
		}
		return true
	})
	return v
}

// similarCases counts switch arms whose (non-trivial) body has the same
// structural shape as another arm of the same switch: a table or a helper
// would usually say it once.
func similarCases(body *ast.BlockStmt) int {
	count := map[string]int{}
	for _, c := range body.List {
		cc, ok := c.(*ast.CaseClause)
		if !ok || len(cc.Body) < 2 {
			continue
		}
		nodes := make([]ast.Node, len(cc.Body))
		for i, s := range cc.Body {
			nodes[i] = s
		}
		count[shapeHash(tierStructural, nil, nodes...)]++
	}
	n := 0
	for _, k := range count {
		if k > 1 {
			n += k
		}
	}
	return n
}

// chainLen is the number of names in a selector chain, looking through
// index, star and paren expressions but stopping at calls: a.b[i].c.d is 4.
func chainLen(e ast.Expr) int {
	n := 0
	for {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			n++
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		case *ast.Ident:
			return n + 1
		default:
			return n
		}
	}
}
