// Command gograph emits an interactive HTML view of the module's packages, types and functions (LOC, nesting, cyclomatic
// and call-graph depth, type-resolved call edges). Flags: -root -module -goos (js includes the wasm-only files) -tags
// -out -json. `gograph rank` ranks refactoring candidates instead.
package main

// Config controls the call-resolution build environment.
type Config struct {
	Root   string
	Module string
	GOOS   string
	Tags   string
}

// FuncMetric holds per-function/method metrics.
type FuncMetric struct {
	Name      string `json:"name"`
	Recv      string `json:"recv,omitempty"` // receiver type name (no *), "" for free funcs
	LOC       int    `json:"loc"`
	Nesting   int    `json:"nesting"`
	Cyclo     int    `json:"cyclo"`
	CallDepth int    `json:"callDepth"`
}

// TypeInfo holds a declared type and its methods.
type TypeInfo struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"` // struct | interface | named
	LOC     int          `json:"loc"`
	Methods []FuncMetric `json:"methods,omitempty"`
}

// Package is one in-module package.
type Package struct {
	Path        string       `json:"path"`
	LOC         int          `json:"loc"`
	Files       int          `json:"files"`
	Types       []TypeInfo   `json:"types,omitempty"`
	Funcs       []FuncMetric `json:"funcs,omitempty"`
	Imports     []string     `json:"imports,omitempty"`
	MemberEdges []MemberEdge `json:"memberEdges,omitempty"`
}

// MemberEdge is an intra-package caller→callee edge between drill-down nodes: a top-level function or a type (a method
// call counts for its receiver type). Identifiers are unique across funcs and types, so the name identifies the node.
type MemberEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Calls int    `json:"calls"`
}

// Edge is an aggregated cross-package call relationship.
type Edge struct {
	From    string   `json:"from"`
	To      string   `json:"to"`
	Calls   int      `json:"calls"`
	Methods []string `json:"methods,omitempty"`
}

// Graph is the complete model emitted to JSON and embedded in the HTML.
type Graph struct {
	Module   string            `json:"module"`
	Config   map[string]string `json:"config"`
	Packages []Package         `json:"packages"`
	Edges    []Edge            `json:"edges"`
}

// FuncID is the canonical key joining the metrics pass and the call-depth pass.
// recv must have any leading '*' already stripped; "" for free functions.
func FuncID(pkgPath, recv, name string) string {
	return pkgPath + "\t" + recv + "\t" + name
}
