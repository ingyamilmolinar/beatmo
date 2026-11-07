// Package rank measures a Go module per build configuration and ranks refactoring candidates.
package rank

import (
	"bytes"
	"go/ast"
	"go/token"
)

// Cyclomatic is McCabe complexity: 1 + if/for/range/case/comm-clause/&&/||.
// Function literals are their own units, so their bodies are skipped.
func Cyclomatic(body ast.Node) int {
	cc := 1
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
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

// Cognitive is Sonar cognitive complexity (structures score 1 + nesting; else, else-if, each &&/|| run, labelled jumps
// and direct recursion score 1; function literals are their own units). nestingBonus is the part flattening removes.
func Cognitive(body *ast.BlockStmt, selfName string) (total, nestingBonus int) {
	if body == nil {
		return 0, 0
	}
	c := &cog{self: selfName}
	c.list(body.List, 0)
	return c.total, c.bonus
}

type cog struct {
	self         string
	total, bonus int
}

func (c *cog) structure(nest int) {
	c.total += 1 + nest
	c.bonus += nest
}

func (c *cog) list(stmts []ast.Stmt, nest int) {
	for _, s := range stmts {
		c.stmt(s, nest)
	}
}

func (c *cog) stmt(s ast.Stmt, nest int) {
	switch x := s.(type) {
	case *ast.IfStmt:
		c.structure(nest)
		c.ifTail(x, nest)
	case *ast.ForStmt:
		c.structure(nest)
		c.opt(x.Init)
		c.expr(x.Cond)
		c.opt(x.Post)
		c.list(x.Body.List, nest+1)
	case *ast.RangeStmt:
		c.structure(nest)
		c.expr(x.X)
		c.list(x.Body.List, nest+1)
	case *ast.SwitchStmt:
		c.structure(nest)
		c.opt(x.Init)
		c.expr(x.Tag)
		c.clauses(x.Body, nest+1)
	case *ast.TypeSwitchStmt:
		c.structure(nest)
		c.opt(x.Init)
		c.clauses(x.Body, nest+1)
	case *ast.SelectStmt:
		c.structure(nest)
		c.clauses(x.Body, nest+1)
	case *ast.BlockStmt:
		c.list(x.List, nest)
	case *ast.LabeledStmt:
		c.stmt(x.Stmt, nest)
	case *ast.BranchStmt:
		if x.Label != nil || x.Tok == token.GOTO {
			c.total++
		}
	default:
		c.node(s)
	}
}

// ifTail scores the condition, the body and the else chain of an if whose own
// structure increment has already been counted.
func (c *cog) ifTail(x *ast.IfStmt, nest int) {
	c.opt(x.Init)
	c.expr(x.Cond)
	c.list(x.Body.List, nest+1)
	switch e := x.Else.(type) {
	case *ast.IfStmt:
		c.total++ // else if: hybrid increment, no nesting
		c.ifTail(e, nest)
	case *ast.BlockStmt:
		c.total++
		c.list(e.List, nest+1)
	}
}

func (c *cog) clauses(body *ast.BlockStmt, nest int) {
	for _, cl := range body.List {
		switch cc := cl.(type) {
		case *ast.CaseClause:
			for _, e := range cc.List {
				c.expr(e)
			}
			c.list(cc.Body, nest)
		case *ast.CommClause:
			c.opt(cc.Comm)
			c.list(cc.Body, nest)
		}
	}
}

func (c *cog) opt(s ast.Stmt) {
	if s != nil {
		c.node(s)
	}
}

func (c *cog) expr(e ast.Expr) {
	if e != nil {
		c.node(e)
	}
}

// node scores boolean-operator runs and recursion inside a simple statement or
// expression, without entering function literals.
func (c *cog) node(n ast.Node) {
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.BinaryExpr:
			if x.Op != token.LAND && x.Op != token.LOR {
				return true
			}
			var ops []token.Token
			var leaves []ast.Expr
			flattenLogical(x, &ops, &leaves)
			runs := 0
			for i, op := range ops {
				if i == 0 || op != ops[i-1] {
					runs++
				}
			}
			c.total += runs
			for _, l := range leaves {
				c.node(l)
			}
			return false
		case *ast.CallExpr:
			if c.self != "" && calleeName(x.Fun) == c.self {
				c.total++
			}
		}
		return true
	})
}

// flattenLogical walks a tree of &&/|| (through parentheses) in source order,
// collecting the operator sequence and the non-logical operands.
func flattenLogical(e ast.Expr, ops *[]token.Token, leaves *[]ast.Expr) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		if b, ok := x.X.(*ast.BinaryExpr); ok && (b.Op == token.LAND || b.Op == token.LOR) {
			flattenLogical(b, ops, leaves)
			return
		}
	case *ast.BinaryExpr:
		if x.Op == token.LAND || x.Op == token.LOR {
			flattenLogical(x.X, ops, leaves)
			*ops = append(*ops, x.Op)
			flattenLogical(x.Y, ops, leaves)
			return
		}
	}
	*leaves = append(*leaves, e)
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// MaxNesting is the deepest control-flow nesting in a body (closures skipped).
func MaxNesting(body *ast.BlockStmt) int {
	if body == nil {
		return 0
	}
	return maxNest(body.List)
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
		switch e := n.Else.(type) {
		case *ast.BlockStmt:
			d = max(d, 1+maxNest(e.List))
		case *ast.IfStmt:
			d = max(d, stmtNest(e))
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
	}
	return 0
}

func caseNest(body *ast.BlockStmt) int {
	best := 0
	for _, c := range body.List {
		switch cc := c.(type) {
		case *ast.CaseClause:
			best = max(best, maxNest(cc.Body))
		case *ast.CommClause:
			best = max(best, maxNest(cc.Body))
		}
	}
	return best
}

// SLOC counts the lines in [startLine, endLine] (1-based, inclusive) of src
// that hold code: neither blank nor wholly comment.
func SLOC(src []byte, startLine, endLine int) int {
	lines := bytes.Split(src, []byte("\n"))
	inBlock := false
	n := 0
	for i := startLine; i <= endLine && i-1 < len(lines); i++ {
		if i < 1 {
			continue
		}
		if hasCode(bytes.TrimSpace(lines[i-1]), &inBlock) {
			n++
		}
	}
	return n
}

// hasCode reports whether a trimmed line holds anything outside comments,
// tracking /* */ state across lines. String literals containing comment
// markers are rare enough at line starts to ignore.
func hasCode(l []byte, inBlock *bool) bool {
	code := false
	for len(l) > 0 {
		if *inBlock {
			end := bytes.Index(l, []byte("*/"))
			if end < 0 {
				return code
			}
			*inBlock = false
			l = bytes.TrimSpace(l[end+2:])
			continue
		}
		if bytes.HasPrefix(l, []byte("//")) {
			return code
		}
		if bytes.HasPrefix(l, []byte("/*")) {
			*inBlock = true
			l = l[2:]
			continue
		}
		code = true
		next := bytes.Index(l, []byte("/*"))
		if next < 0 {
			return true
		}
		l = l[next:]
	}
	return code
}

// DataRatio is the share of a body's lines that lie inside composite literals.
// A high ratio with low branching marks a data table, not logic.
func DataRatio(fset *token.FileSet, body *ast.BlockStmt) float64 {
	if body == nil {
		return 0
	}
	first := fset.Position(body.Lbrace).Line
	last := fset.Position(body.Rbrace).Line
	span := last - first - 1
	if span <= 0 {
		return 0
	}
	covered := map[int]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CompositeLit:
			for l := fset.Position(x.Pos()).Line; l <= fset.Position(x.End()).Line; l++ {
				if l > first && l < last {
					covered[l] = true
				}
			}
			return false
		}
		return true
	})
	return float64(len(covered)) / float64(span)
}
