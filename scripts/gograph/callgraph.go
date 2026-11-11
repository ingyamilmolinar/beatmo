package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ResolveEdges type-checks the module under cfg and returns aggregated exported
// cross-package call edges, a per-function call-graph depth map, and per-package
// intra-package member edges (type/function caller→callee relationships).
func ResolveEdges(cfg Config) ([]Edge, map[string]int, map[string][]MemberEdge, error) {
	mode := packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps
	env := os.Environ()
	if cfg.GOOS != "" {
		env = append(env, "GOOS="+cfg.GOOS)
	}
	var buildFlags []string
	if cfg.Tags != "" {
		buildFlags = []string{"-tags=" + cfg.Tags}
	}
	pc := &packages.Config{
		Mode:       mode,
		Dir:        cfg.Root,
		Env:        env,
		BuildFlags: buildFlags,
		Tests:      false,
	}
	pkgs, err := packages.Load(pc, "./...")
	if err != nil {
		return nil, nil, nil, err
	}

	// module prefix to keep only in-module callees
	prefix := cfg.Module

	// caller-func-id -> set of in-module callee func-ids (for depth)
	callees := map[string]map[string]bool{}
	// aggregated package edges
	edgeAgg := map[string]*Edge{}
	// pkgPath -> "from\x00to" -> call count (intra-package member edges)
	memberAgg := map[string]map[string]int{}

	for _, p := range pkgs {
		if p.TypesInfo == nil || !inModule(p.PkgPath, prefix) {
			continue
		}
		if len(p.Errors) > 0 {
			fmt.Fprintf(os.Stderr, "gograph: skipping call edges for %s (%d type errors)\n", p.PkgPath, len(p.Errors))
			// still allow depth for whatever resolved; continue walking
		}
		for _, f := range p.Syntax {
			// (1) edges: every call in the file, including package-level initializers
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				obj := calleeObject(p.TypesInfo, call.Fun)
				if obj == nil || obj.Pkg() == nil {
					return true
				}
				toPath := obj.Pkg().Path()
				if !inModule(toPath, prefix) {
					return true
				}
				if toPath == p.PkgPath || !obj.Exported() {
					return true // same-package or unexported: no cross edge
				}
				key := p.PkgPath + "\x00" + toPath
				e := edgeAgg[key]
				if e == nil {
					e = &Edge{From: p.PkgPath, To: toPath}
					edgeAgg[key] = e
				}
				e.Calls++
				m := qualifiedName(obj)
				if !contains(e.Methods, m) {
					e.Methods = append(e.Methods, m)
				}
				return true
			})
			// (2) depth + intra-package member edges: attribute in-module
			// resolvable calls to their enclosing func.
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				curFn := funcIDFromDecl(p.PkgPath, fd)
				callerMember := memberName(recvOfDecl(fd), fd.Name.Name)
				ast.Inspect(fd, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					obj := calleeObject(p.TypesInfo, call.Fun)
					if obj == nil || obj.Pkg() == nil || !inModule(obj.Pkg().Path(), prefix) {
						return true
					}
					id, idOK := funcIDFromObj(obj)
					if !idOK {
						return true
					}
					if callees[curFn] == nil {
						callees[curFn] = map[string]bool{}
					}
					callees[curFn][id] = true
					// intra-package member edge (caller node → callee node)
					calleePkg, calleeRecv, calleeName := splitFuncID(id)
					if calleePkg == p.PkgPath {
						to := memberName(calleeRecv, calleeName)
						if callerMember != to && callerMember != "" && to != "" {
							if memberAgg[p.PkgPath] == nil {
								memberAgg[p.PkgPath] = map[string]int{}
							}
							memberAgg[p.PkgPath][callerMember+"\x00"+to]++
						}
					}
					return true
				})
			}
		}
	}

	depth := computeDepths(callees)

	edges := make([]Edge, 0, len(edgeAgg))
	for _, e := range edgeAgg {
		sort.Strings(e.Methods)
		edges = append(edges, *e)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})

	// per-package member edges, deterministically ordered
	memberEdges := map[string][]MemberEdge{}
	for pkgPath, m := range memberAgg {
		list := make([]MemberEdge, 0, len(m))
		for k, calls := range m {
			ft := strings.SplitN(k, "\x00", 2)
			list = append(list, MemberEdge{From: ft[0], To: ft[1], Calls: calls})
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].From != list[j].From {
				return list[i].From < list[j].From
			}
			return list[i].To < list[j].To
		})
		memberEdges[pkgPath] = list
	}

	return edges, depth, memberEdges, nil
}

// splitFuncID splits a canonical FuncID ("pkg\trecv\tname") into its parts.
func splitFuncID(id string) (pkg, recv, name string) {
	parts := strings.SplitN(id, "\t", 3)
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}

// memberName is the package-level node name for a func/method: the receiver type
// name for a method, or the function name for a top-level func.
func memberName(recv, name string) string {
	if recv != "" {
		return recv
	}
	return name
}

// recvOfDecl returns the receiver type name (pointer/generics stripped) of a
// declaration, or "" for a free function.
func recvOfDecl(fd *ast.FuncDecl) string {
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		return recvName(fd.Recv.List[0].Type)
	}
	return ""
}

// inModule reports whether pkgPath is the module root or a package under it,
// without matching a sibling like "github.com/x/barbaz" against "github.com/x/bar".
func inModule(pkgPath, module string) bool {
	return pkgPath == module || strings.HasPrefix(pkgPath, module+"/")
}

// computeDepths returns, for each function in the call graph, the number of
// distinct strongly-connected components on the longest downstream path starting
// at that function (a cycle collapses to a single component). It is deterministic
// — independent of Go map iteration order — via Tarjan SCC condensation followed
// by longest-path over the resulting DAG. A leaf (or a lone self-recursive
// function) has depth 1.
func computeDepths(callees map[string]map[string]bool) map[string]int {
	// Collect all nodes (callers and callees), deterministically ordered.
	nodeSet := map[string]bool{}
	for u, outs := range callees {
		nodeSet[u] = true
		for v := range outs {
			nodeSet[v] = true
		}
	}
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	// Tarjan SCC.
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	comp := map[string]int{} // node -> scc id
	var stack []string
	next := 0
	sccCount := 0

	var strongconnect func(v string)
	strongconnect = func(v string) {
		index[v] = next
		low[v] = next
		next++
		stack = append(stack, v)
		onStack[v] = true
		outs := make([]string, 0, len(callees[v]))
		for w := range callees[v] {
			outs = append(outs, w)
		}
		sort.Strings(outs)
		for _, w := range outs {
			if _, seen := index[w]; !seen {
				strongconnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] {
				if index[w] < low[v] {
					low[v] = index[w]
				}
			}
		}
		if low[v] == index[v] {
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp[w] = sccCount
				if w == v {
					break
				}
			}
			sccCount++
		}
	}
	for _, n := range nodes {
		if _, seen := index[n]; !seen {
			strongconnect(n)
		}
	}

	// Condensation DAG edges.
	sccAdj := make([]map[int]bool, sccCount)
	for i := range sccAdj {
		sccAdj[i] = map[int]bool{}
	}
	for u, outs := range callees {
		for v := range outs {
			if cu, cv := comp[u], comp[v]; cu != cv {
				sccAdj[cu][cv] = true
			}
		}
	}

	// Longest path (count of SCCs) via memoized DFS — safe: condensation is acyclic.
	sccDepth := make([]int, sccCount)
	var dfs func(c int) int
	dfs = func(c int) int {
		if sccDepth[c] != 0 {
			return sccDepth[c]
		}
		best := 0
		outs := make([]int, 0, len(sccAdj[c]))
		for d := range sccAdj[c] {
			outs = append(outs, d)
		}
		sort.Ints(outs)
		for _, d := range outs {
			if x := dfs(d); x > best {
				best = x
			}
		}
		sccDepth[c] = 1 + best
		return sccDepth[c]
	}

	depth := map[string]int{}
	for _, n := range nodes {
		depth[n] = dfs(comp[n])
	}
	return depth
}

// calleeObject resolves the object being called (function or method).
func calleeObject(info *types.Info, fun ast.Expr) types.Object {
	switch e := fun.(type) {
	case *ast.Ident:
		return info.Uses[e]
	case *ast.SelectorExpr:
		if sel, ok := info.Selections[e]; ok {
			return sel.Obj() // method / field call on a value: resolves recv pkg
		}
		return info.Uses[e.Sel] // qualified pkg.Func()
	}
	return nil
}

// funcIDFromObj derives the canonical FuncID for a resolved callee.
func funcIDFromObj(obj types.Object) (string, bool) {
	fn, ok := obj.(*types.Func)
	if !ok || fn.Pkg() == nil {
		return "", false
	}
	recv := ""
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		recv = recvTypeName(sig.Recv().Type())
	}
	return FuncID(fn.Pkg().Path(), recv, fn.Name()), true
}

func recvTypeName(t types.Type) string {
	switch x := t.(type) {
	case *types.Pointer:
		return recvTypeName(x.Elem())
	case *types.Named:
		return x.Obj().Name()
	default:
		return ""
	}
}

// funcIDFromDecl builds the FuncID for a function declaration in pkgPath.
func funcIDFromDecl(pkgPath string, fd *ast.FuncDecl) string {
	recv := ""
	if fd.Recv != nil && len(fd.Recv.List) > 0 {
		recv = recvName(fd.Recv.List[0].Type)
	}
	return FuncID(pkgPath, recv, fd.Name.Name)
}

// qualifiedName renders a readable exported callee name for edge labels.
func qualifiedName(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	if !ok {
		return obj.Pkg().Name() + "." + obj.Name()
	}
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
		return "(" + fn.Pkg().Name() + "." + recvTypeName(sig.Recv().Type()) + ")." + fn.Name()
	}
	return fn.Pkg().Name() + "." + fn.Name()
}
