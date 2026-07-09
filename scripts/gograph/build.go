package main

import "sort"

// BuildGraph runs both analysis passes and joins them into one ordered model.
func BuildGraph(cfg Config) (*Graph, error) {
	pkgMap, err := AnalyzeMetrics(cfg.Root, cfg.Module)
	if err != nil {
		return nil, err
	}
	edges, depth, memberEdges, err := ResolveEdges(cfg)
	if err != nil {
		return nil, err
	}

	g := &Graph{
		Module: cfg.Module,
		Config: map[string]string{"goos": orDefault(cfg.GOOS, "linux"), "tags": cfg.Tags},
		Edges:  edges,
	}

	for _, p := range pkgMap {
		// attach call depth to free funcs and methods by FuncID
		for i := range p.Funcs {
			f := &p.Funcs[i]
			f.CallDepth = depthOr1(depth, FuncID(p.Path, f.Recv, f.Name))
		}
		for ti := range p.Types {
			for mi := range p.Types[ti].Methods {
				m := &p.Types[ti].Methods[mi]
				m.CallDepth = depthOr1(depth, FuncID(p.Path, m.Recv, m.Name))
			}
		}
		p.MemberEdges = memberEdges[p.Path] // already deterministically sorted
		sortPackage(p)
		g.Packages = append(g.Packages, *p)
	}
	sort.Slice(g.Packages, func(i, j int) bool { return g.Packages[i].Path < g.Packages[j].Path })
	return g, nil
}

// depthOr1 looks up a func's call-graph depth, defaulting to 1 when the
// function is absent from the map. ResolveEdges only populates depth entries
// for functions that appear in the in-module call graph; a leaf or fully
// isolated function (calls nothing in-module, called by nothing in-module) is
// absent, and its correct call-graph depth is 1 (itself) — consistent with
// how computeDepths returns 1 for leaves that ARE in the graph.
func depthOr1(m map[string]int, id string) int {
	if d, ok := m[id]; ok {
		return d
	}
	return 1
}

func sortPackage(p *Package) {
	sort.Strings(p.Imports)
	sort.Slice(p.Types, func(i, j int) bool { return p.Types[i].Name < p.Types[j].Name })
	sort.Slice(p.Funcs, func(i, j int) bool { return p.Funcs[i].Name < p.Funcs[j].Name })
	for ti := range p.Types {
		ms := p.Types[ti].Methods
		sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
