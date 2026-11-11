package rank

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"sort"
	"strings"
)

// Clone thresholds. Whole functions need cloneMinUnitLines; statement-window
// clones need cloneWindow statements spanning cloneMinWindowLines.
const (
	cloneMinUnitLines   = 8
	cloneWindow         = 6
	cloneMinWindowLines = 10
	repeatWindow        = 3
)

// Clone tiers, from most to least certain. exact renames only the unit's own
// locals (parameters, := and var declarations): a copy with renamed locals
// matches, but code touching different fields, functions or package variables
// does not. renamed renames every identifier consistently, so it also catches
// copies that differ only in which names they touch (three toggles over three
// different overlays). structural abstracts every identifier and literal: the
// same shape holding different data.
const (
	tierExact      = "exact"
	tierRenamed    = "renamed"
	tierStructural = "structural"
)

// shapeHash serialises nodes in the tier's normal form and hashes it.
// locals is the enclosing unit's local names (used by tierExact only).
func shapeHash(tier string, locals map[string]bool, nodes ...ast.Node) string {
	var b strings.Builder
	names := map[string]int{}
	sel := map[*ast.Ident]bool{}
	if tier == tierExact {
		for _, n := range nodes {
			ast.Inspect(n, func(x ast.Node) bool {
				if s, ok := x.(*ast.SelectorExpr); ok {
					sel[s.Sel] = true
				}
				return true
			})
		}
	}
	for _, n := range nodes {
		serialize(&b, n, tier, locals, sel, names)
		b.WriteByte(';')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:12])
}

func serialize(b *strings.Builder, root ast.Node, tier string, locals map[string]bool, sel map[*ast.Ident]bool, names map[string]int) {
	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			b.WriteByte(')')
			return false
		}
		switch n.(type) {
		case *ast.CommentGroup, *ast.Comment:
			return false
		}
		b.WriteString(reflect.TypeOf(n).Elem().Name())
		switch x := n.(type) {
		case *ast.Ident:
			switch {
			case tier == tierStructural:
			case tier == tierExact && (sel[x] || !locals[x.Name]):
				b.WriteString("=" + x.Name)
			default:
				k, ok := names[x.Name]
				if !ok {
					k = len(names)
					names[x.Name] = k
				}
				fmt.Fprintf(b, "#%d", k)
			}
		case *ast.BasicLit:
			b.WriteString(x.Kind.String())
			if tier != tierStructural {
				b.WriteString(x.Value)
			}
		case *ast.BinaryExpr:
			b.WriteString(x.Op.String())
		case *ast.UnaryExpr:
			b.WriteString(x.Op.String())
		case *ast.AssignStmt:
			b.WriteString(x.Tok.String())
		case *ast.IncDecStmt:
			b.WriteString(x.Tok.String())
		case *ast.BranchStmt:
			b.WriteString(x.Tok.String())
		case *ast.RangeStmt:
			b.WriteString(x.Tok.String())
		case *ast.ChanType:
			fmt.Fprintf(b, "%d", x.Dir)
		}
		b.WriteByte('(')
		return true
	})
}

// localsOf collects the names a function declares for itself: receiver,
// parameters, results, and every := / var / range definition in its body
// (closures included: their locals are locals of the region too).
func localsOf(recv *ast.FieldList, ft *ast.FuncType, body *ast.BlockStmt) map[string]bool {
	ls := map[string]bool{}
	fields := func(fl *ast.FieldList) {
		if fl == nil {
			return
		}
		for _, f := range fl.List {
			for _, n := range f.Names {
				ls[n.Name] = true
			}
		}
	}
	fields(recv)
	if ft != nil {
		fields(ft.Params)
		fields(ft.Results)
	}
	if body == nil {
		return ls
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				for _, e := range x.Lhs {
					if id, ok := e.(*ast.Ident); ok {
						ls[id.Name] = true
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				for _, e := range []ast.Expr{x.Key, x.Value} {
					if id, ok := e.(*ast.Ident); ok {
						ls[id.Name] = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				ls[id.Name] = true
			}
		case *ast.FuncLit:
			fields(x.Type.Params)
			fields(x.Type.Results)
		case *ast.TypeSwitchStmt:
			if a, ok := x.Assign.(*ast.AssignStmt); ok {
				for _, e := range a.Lhs {
					if id, ok := e.(*ast.Ident); ok {
						ls[id.Name] = true
					}
				}
			}
		}
		return true
	})
	delete(ls, "_")
	return ls
}

// stmtLists returns every statement list in body (blocks and case/comm clause
// bodies), not entering function literals: a closure is its own unit.
func stmtLists(body ast.Node) [][]ast.Stmt {
	var out [][]ast.Stmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit:
			return n == body
		case *ast.BlockStmt:
			out = append(out, x.List)
		case *ast.CaseClause:
			out = append(out, x.Body)
		case *ast.CommClause:
			out = append(out, x.Body)
		}
		return true
	})
	return out
}

// window is one run of statements inside one statement list.
type window struct {
	list, idx     int // statement list id, start index
	unit          *Unit
	line, endLine int
	hash          map[string]string // tier -> hash
}

// findClones reports whole-function clones and maximal statement-window clone
// regions. Generated code is excluded.
func findClones(m *Model) []*CloneGroup {
	fset := m.src.fset
	bodies := unitBodies(m)
	var groups []*CloneGroup

	// Whole functions: each tier reports one representative per class of the
	// tier above, so a group is reported once, at its most certain tier.
	tiers := []string{tierExact, tierRenamed, tierStructural}
	var whole []*Unit
	hashes := map[string]map[string]string{} // tier -> unit ID -> hash
	for _, t := range tiers {
		hashes[t] = map[string]string{}
	}
	for _, u := range m.Units {
		f := bodies[u.ID]
		if f == nil || u.Generated || u.LOC < cloneMinUnitLines {
			continue
		}
		whole = append(whole, u)
		for _, t := range tiers {
			hashes[t][u.ID] = shapeHash(t, f.locals, f.body)
		}
	}
	inWhole := map[string]bool{}
	for ti, t := range tiers {
		byHash := map[string][]*Unit{}
		for _, u := range whole {
			byHash[hashes[t][u.ID]] = append(byHash[hashes[t][u.ID]], u)
		}
		for h, us := range byHash {
			reps := us
			if ti > 0 {
				seen := map[string]bool{}
				reps = nil
				for _, u := range us {
					if k := hashes[tiers[ti-1]][u.ID]; !seen[k] {
						seen[k] = true
						reps = append(reps, u)
					}
				}
			}
			if len(reps) > 1 {
				groups = append(groups, wholeGroup(t, h, reps))
				for _, u := range us {
					inWhole[u.ID] = true
				}
			}
		}
	}

	// Statement windows.
	var wins []*window
	listID := 0
	for _, u := range m.Units {
		f := bodies[u.ID]
		if f == nil || u.Generated {
			continue
		}
		for _, list := range stmtLists(f.body) {
			listID++
			for i := 0; i+cloneWindow <= len(list); i++ {
				run := list[i : i+cloneWindow]
				w := &window{
					list: listID, idx: i, unit: u,
					line:    fset.Position(run[0].Pos()).Line,
					endLine: fset.Position(run[len(run)-1].End()).Line,
				}
				nodes := make([]ast.Node, len(run))
				for k, s := range run {
					nodes[k] = s
				}
				w.hash = map[string]string{}
				for _, t := range tiers {
					w.hash[t] = shapeHash(t, f.locals, nodes...)
				}
				wins = append(wins, w)
			}
		}
	}
	var above []*region
	for _, t := range tiers {
		rs := regions(wins, func(w *window) string { return w.hash[t] })
		for _, r := range rs {
			if coveredBy(r, above) {
				continue
			}
			if g := windowGroup(t, r, inWhole); g != nil {
				groups = append(groups, g)
			}
		}
		above = append(above, rs...)
	}

	for _, g := range groups {
		pk := map[string]bool{}
		for _, mem := range g.Members {
			pk[m.byID[mem.Unit].Pkg] = true
		}
		g.CrossPkg = len(pk) > 1
		sort.Slice(g.Members, func(i, j int) bool {
			if g.Members[i].File != g.Members[j].File {
				return g.Members[i].File < g.Members[j].File
			}
			return g.Members[i].Line < g.Members[j].Line
		})
	}
	sort.Slice(groups, func(i, j int) bool {
		if a, b := groups[i].Lines*(len(groups[i].Members)-1), groups[j].Lines*(len(groups[j].Members)-1); a != b {
			return a > b
		}
		return groups[i].ID < groups[j].ID
	})
	return groups
}

// funcNode is a unit's body with the locals its exact-tier hash renames.
type funcNode struct {
	body   *ast.BlockStmt
	locals map[string]bool
}

func unitBodies(m *Model) map[string]*funcNode {
	out := map[string]*funcNode{}
	for _, rel := range m.src.order {
		sf := m.src.files[rel]
		ast.Inspect(sf.AST, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Body != nil {
					out[declUnitID(m.src.fset, sf.Rel, x)] = &funcNode{x.Body, localsOf(x.Recv, x.Type, x.Body)}
				}
			case *ast.FuncLit:
				p := m.src.fset.Position(x.Pos())
				out[fmt.Sprintf("%s:%d:%d", sf.Rel, p.Line, p.Column)] = &funcNode{x.Body, localsOf(nil, x.Type, x.Body)}
			}
			return true
		})
	}
	return out
}

// declUnitID is a declaration's unit ID, built the way collectUnits builds it.
func declUnitID(fset *token.FileSet, rel string, fd *ast.FuncDecl) string {
	name := fd.Name.Name
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		name = recvTypeName(fd.Recv.List[0].Type) + "." + name
	}
	return fmt.Sprintf("%s:%d:%s", rel, fset.Position(fd.Name.Pos()).Line, name)
}

func wholeGroup(tier, hash string, us []*Unit) *CloneGroup {
	g := &CloneGroup{ID: "clone-" + hash[:10], Tier: tier, Whole: true}
	for _, u := range us {
		g.Members = append(g.Members, CloneLoc{File: u.File, Line: u.Line, EndLine: u.EndLine, Unit: u.ID})
		if g.Lines == 0 || u.LOC < g.Lines {
			g.Lines = u.LOC
		}
	}
	return g
}

// region is a maximal run of consecutive windows that match across a fixed
// set of member locations.
type region struct {
	hash    string
	members []*window // first window of each member
	ends    []*window // last window of each member
}

// regions groups windows by hash, drops members overlapping an earlier member
// of the same list, and extends each starting group while every member's next
// window still forms the same group. A group whose members all continue the
// same previous group is part of that region, not a start.
func regions(wins []*window, key func(*window) string) []*region {
	at := map[[2]int]*window{}
	for _, w := range wins {
		at[[2]int{w.list, w.idx}] = w
	}
	byHash := map[string][]*window{}
	for _, w := range wins {
		byHash[key(w)] = append(byHash[key(w)], w)
	}
	members := func(h string) []*window {
		ws := byHash[h]
		var out []*window
		last := map[int]int{}
		for _, w := range ws { // ws is in source order
			if prev, ok := last[w.list]; ok && w.idx-prev < cloneWindow {
				continue
			}
			last[w.list] = w.idx
			out = append(out, w)
		}
		return out
	}
	sameSet := func(a, b []*window, shift int) bool {
		if len(a) != len(b) {
			return false
		}
		set := map[[2]int]bool{}
		for _, w := range a {
			set[[2]int{w.list, w.idx + shift}] = true
		}
		for _, w := range b {
			if !set[[2]int{w.list, w.idx}] {
				return false
			}
		}
		return true
	}
	// step returns the members' neighbouring windows (dir = ±1) if they all
	// exist and form exactly one group equal to the shifted member set.
	step := func(cur []*window, dir int) []*window {
		var next []*window
		h := ""
		for _, w := range cur {
			n := at[[2]int{w.list, w.idx + dir}]
			if n == nil || (h != "" && key(n) != h) {
				return nil
			}
			h = key(n)
			next = append(next, n)
		}
		if !sameSet(cur, members(h), dir) {
			return nil
		}
		return next
	}
	var hashes []string
	for h := range byHash {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	var out []*region
	for _, h := range hashes {
		ms := members(h)
		if len(ms) < 2 || step(ms, -1) != nil {
			continue
		}
		r := &region{hash: h, members: ms, ends: ms}
		for next := step(ms, +1); next != nil; next = step(next, +1) {
			r.ends = next
		}
		out = append(out, r)
	}
	return out
}

func windowGroup(tier string, r *region, inWhole map[string]bool) *CloneGroup {
	allWhole := true
	g := &CloneGroup{ID: "clone-" + r.hash[:10], Tier: tier}
	for i, w := range r.members {
		end := r.ends[i]
		g.Members = append(g.Members, CloneLoc{File: w.unit.File, Line: w.line, EndLine: end.endLine, Unit: w.unit.ID})
		if lines := end.endLine - w.line + 1; g.Lines == 0 || lines < g.Lines {
			g.Lines = lines
		}
		if !inWhole[w.unit.ID] {
			allWhole = false
		}
	}
	if allWhole || g.Lines < cloneMinWindowLines {
		return nil
	}
	return g
}

// coveredBy reports whether every member of r lies inside a member region of
// one of the exact regions (a structural region adds nothing then).
func coveredBy(r *region, exact []*region) bool {
	for i, w := range r.members {
		end := r.ends[i]
		ok := false
		for _, e := range exact {
			for k, ew := range e.members {
				if ew.list == w.list && ew.idx <= w.idx && e.ends[k].idx >= end.idx {
					ok = true
				}
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// repeatedLines counts the lines of a body that repeat (exactly, up to renamed
// locals) an earlier repeatWindow-statement run of the same body.
func repeatedLines(fset *token.FileSet, body *ast.BlockStmt, locals map[string]bool) int {
	if fset == nil {
		return 0
	}
	type pos struct{ list, idx int }
	firstAt := map[string]pos{}
	lines := map[int]bool{}
	for li, list := range stmtLists(body) {
		for i := 0; i+repeatWindow <= len(list); i++ {
			run := list[i : i+repeatWindow]
			nodes := make([]ast.Node, len(run))
			for k, s := range run {
				nodes[k] = s
			}
			h := shapeHash(tierExact, locals, nodes...)
			if p, ok := firstAt[h]; ok {
				if p.list == li && i-p.idx < repeatWindow {
					continue
				}
				for l := fset.Position(run[0].Pos()).Line; l <= fset.Position(run[len(run)-1].End()).Line; l++ {
					lines[l] = true
				}
				continue
			}
			firstAt[h] = pos{li, i}
		}
	}
	return len(lines)
}
