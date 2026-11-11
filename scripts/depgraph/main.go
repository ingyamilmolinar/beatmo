// Command depgraph generates a DOT graph of the internal package structure
// of the beatmo Go codebase, including packages, types, and functions.
// Large packages (like ui) are split into logical sub-groups by file prefix.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type typeInfo struct {
	name       string
	kind       string // "struct", "interface", "type"
	sourceFile string // base filename (no .go)
}

type methodInfo struct {
	receiver   string
	name       string
	sourceFile string
}

type funcInfo struct {
	name       string
	sourceFile string
}

type pkgInfo struct {
	name       string
	importPath string
	shortPath  string
	types      []typeInfo
	funcs      []funcInfo
	methods    []methodInfo
	imports    []string
}

// fileGroup clusters declarations from files sharing a common prefix.
type fileGroup struct {
	prefix  string
	label   string
	types   []typeInfo
	funcs   []funcInfo
	methods []methodInfo
}

func main() {
	root := flag.String("root", "src/go", "Go source root")
	mod := flag.String("module", "github.com/ingyamilmolinar/beatmo", "Module path")
	maxFuncs := flag.Int("max-funcs", 12, "Max standalone functions per group (0=all)")
	maxMethods := flag.Int("max-methods", 6, "Max methods per type (0=all)")
	maxTypes := flag.Int("max-types", 12, "Max types per group (0=all)")
	groupThreshold := flag.Int("group-threshold", 20, "Min declarations to trigger file-prefix grouping")
	flag.Parse()

	pkgs := discover(*root, *mod)
	generateDOT(os.Stdout, pkgs, *maxFuncs, *maxMethods, *maxTypes, *groupThreshold)
}

func discover(root, mod string) map[string]*pkgInfo {
	pkgs := map[string]*pkgInfo{}
	fset := token.NewFileSet()

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "/vendor/") || strings.Contains(path, "/testdata/") {
			return nil
		}
		// Skip platform-specific stubs that won't contribute useful structure.
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_stub.go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		dir := filepath.Dir(path)
		rel, _ := filepath.Rel(root, dir)
		importPath := mod + "/" + filepath.ToSlash(rel)
		srcFile := strings.TrimSuffix(base, ".go")

		pkg, ok := pkgs[importPath]
		if !ok {
			pkg = &pkgInfo{
				name:       f.Name.Name,
				importPath: importPath,
				shortPath:  filepath.ToSlash(rel),
			}
			pkgs[importPath] = pkg
		}

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					ti := typeInfo{name: ts.Name.Name, sourceFile: srcFile}
					switch ts.Type.(type) {
					case *ast.StructType:
						ti.kind = "struct"
					case *ast.InterfaceType:
						ti.kind = "interface"
					default:
						ti.kind = "type"
					}
					if !hasType(pkg.types, ti.name) {
						pkg.types = append(pkg.types, ti)
					}
				}
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv != nil {
					pkg.methods = append(pkg.methods, methodInfo{
						receiver:   receiverTypeName(d.Recv),
						name:       d.Name.Name,
						sourceFile: srcFile,
					})
				} else {
					fn := funcInfo{name: d.Name.Name, sourceFile: srcFile}
					if !hasFuncName(pkg.funcs, fn.name) {
						pkg.funcs = append(pkg.funcs, fn)
					}
				}
			}
		}

		for _, imp := range f.Imports {
			impPath := strings.Trim(imp.Path.Value, "\"")
			if strings.HasPrefix(impPath, mod) {
				if !containsStr(pkg.imports, impPath) {
					pkg.imports = append(pkg.imports, impPath)
				}
			}
		}

		return nil
	})

	return pkgs
}

// filePrefix extracts the grouping prefix from a source file name.
// "game_draw_helpers" → "game", "drumview_ctor" → "drumview", "slider" → "".
func filePrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return ""
	}
	return name[:idx]
}

// buildGroups splits a large package into sub-groups by file prefix.
func buildGroups(pkg *pkgInfo, threshold int) []fileGroup {
	totalDecls := len(pkg.types) + len(pkg.funcs)
	if totalDecls < threshold {
		// Single group — the whole package.
		return []fileGroup{{
			prefix:  "",
			label:   pkg.name,
			types:   pkg.types,
			funcs:   pkg.funcs,
			methods: pkg.methods,
		}}
	}

	// Count files per prefix to find significant groups.
	prefixFiles := map[string]map[string]bool{} // prefix → set of filenames
	for _, t := range pkg.types {
		p := filePrefix(t.sourceFile)
		if prefixFiles[p] == nil {
			prefixFiles[p] = map[string]bool{}
		}
		prefixFiles[p][t.sourceFile] = true
	}
	for _, f := range pkg.funcs {
		p := filePrefix(f.sourceFile)
		if prefixFiles[p] == nil {
			prefixFiles[p] = map[string]bool{}
		}
		prefixFiles[p][f.sourceFile] = true
	}
	for _, m := range pkg.methods {
		p := filePrefix(m.sourceFile)
		if prefixFiles[p] == nil {
			prefixFiles[p] = map[string]bool{}
		}
		prefixFiles[p][m.sourceFile] = true
	}

	// Prefixes with >=2 files become named groups; rest goes to "other".
	significantPrefixes := map[string]bool{}
	for p, files := range prefixFiles {
		if p != "" && len(files) >= 2 {
			significantPrefixes[p] = true
		}
	}

	groupMap := map[string]*fileGroup{}
	getGroup := func(srcFile string) *fileGroup {
		p := filePrefix(srcFile)
		if !significantPrefixes[p] {
			p = "_other"
		}
		g, ok := groupMap[p]
		if !ok {
			label := p
			if p == "_other" {
				label = "other"
			}
			g = &fileGroup{prefix: p, label: label}
			groupMap[p] = g
		}
		return g
	}

	for _, t := range pkg.types {
		g := getGroup(t.sourceFile)
		g.types = append(g.types, t)
	}
	for _, f := range pkg.funcs {
		g := getGroup(f.sourceFile)
		g.funcs = append(g.funcs, f)
	}
	for _, m := range pkg.methods {
		g := getGroup(m.sourceFile)
		g.methods = append(g.methods, m)
	}

	// Sort groups: named prefixes alphabetically, _other last.
	groups := make([]fileGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].prefix == "_other" {
			return false
		}
		if groups[j].prefix == "_other" {
			return true
		}
		return groups[i].prefix < groups[j].prefix
	})

	return groups
}

func receiverTypeName(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	t := fl.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func hasType(types []typeInfo, name string) bool {
	for _, t := range types {
		if t.name == name {
			return true
		}
	}
	return false
}

func hasFuncName(funcs []funcInfo, name string) bool {
	for _, f := range funcs {
		if f.name == name {
			return true
		}
	}
	return false
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	return r.Replace(s)
}

func truncateStr(items []string, max int) ([]string, int) {
	if max <= 0 || len(items) <= max {
		return items, 0
	}
	return items[:max], len(items) - max
}

// Package-level color palette for groups.
var groupColor = map[string]string{
	"cmd":                     "#E8F5E9",
	"core/model":              "#E3F2FD",
	"core/beat":               "#E3F2FD",
	"core/engine":             "#E3F2FD",
	"internal/ui":             "#FFF3E0",
	"internal/audio":          "#FCE4EC",
	"internal/audio/playtest": "#FCE4EC",
	"internal/timeline":       "#F3E5F5",
	"internal/assets":         "#E0F2F1",
	"internal/gamestate":      "#F3E5F5",
	"internal/graphruntime":   "#E3F2FD",
	"internal/ebitestub":      "#ECEFF1",
	"internal/log":            "#ECEFF1",
	"internal/utils":          "#ECEFF1",
}

// Sub-group colors within the ui package.
var uiSubgroupColor = map[string]string{
	"game":     "#FFE0B2",
	"drumview": "#FFCCBC",
	"grid":     "#FFF9C4",
	"widget":   "#C8E6C9",
	"overlay":  "#D1C4E9",
	"js":       "#B3E5FC",
	"_other":   "#F5F5F5",
}

func pkgColor(shortPath string) string {
	if c, ok := groupColor[shortPath]; ok {
		return c
	}
	for prefix, c := range groupColor {
		if strings.HasPrefix(shortPath, prefix+"/") {
			return c
		}
	}
	return "#F5F5F5"
}

func generateDOT(w io.Writer, pkgs map[string]*pkgInfo, maxFuncs, maxMethods, maxTypes, groupThreshold int) {
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fmt.Fprintln(w, "digraph beatmo {")
	fmt.Fprintln(w, "  rankdir=LR;")
	fmt.Fprintln(w, "  fontname=\"Helvetica\";")
	fmt.Fprintln(w, "  node [fontname=\"Helvetica\" fontsize=10];")
	fmt.Fprintln(w, "  edge [fontname=\"Helvetica\" fontsize=8 color=\"#666666\"];")
	fmt.Fprintln(w, "  compound=true;")
	fmt.Fprintln(w, "  nodesep=0.3;")
	fmt.Fprintln(w, "  ranksep=1.0;")
	fmt.Fprintln(w)

	for _, ip := range paths {
		pkg := pkgs[ip]
		sid := sanitize(pkg.shortPath)
		bgColor := pkgColor(pkg.shortPath)

		groups := buildGroups(pkg, groupThreshold)
		hasSubgroups := len(groups) > 1

		fmt.Fprintf(w, "  subgraph cluster_%s {\n", sid)
		fmt.Fprintf(w, "    label=\"%s\\n(%s)\";\n", pkg.name, pkg.shortPath)
		fmt.Fprintf(w, "    style=filled; fillcolor=\"%s\"; color=\"#BDBDBD\";\n", bgColor)
		fmt.Fprintf(w, "    fontsize=12; labeljust=l;\n")
		fmt.Fprintf(w, "    %s_anchor [label=\"\" shape=point width=0 height=0];\n", sid)

		if hasSubgroups {
			for gi, grp := range groups {
				gsid := fmt.Sprintf("%s_g%d", sid, gi)
				subColor := "#F5F5F5"
				if c, ok := uiSubgroupColor[grp.prefix]; ok {
					subColor = c
				}
				fmt.Fprintf(w, "    subgraph cluster_%s {\n", gsid)
				fmt.Fprintf(w, "      label=\"%s_*\";\n", grp.label)
				fmt.Fprintf(w, "      style=\"filled,rounded\"; fillcolor=\"%s\"; color=\"#E0E0E0\";\n", subColor)
				fmt.Fprintf(w, "      fontsize=11; labeljust=l;\n")
				emitGroupContents(w, gsid, grp, maxFuncs, maxMethods, maxTypes, "      ")
				fmt.Fprintln(w, "    }")
			}
		} else if len(groups) == 1 {
			emitGroupContents(w, sid, groups[0], maxFuncs, maxMethods, maxTypes, "    ")
		}

		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}

	// Emit edges.
	for _, ip := range paths {
		pkg := pkgs[ip]
		srcID := sanitize(pkg.shortPath)
		sort.Strings(pkg.imports)
		for _, imp := range pkg.imports {
			target, ok := pkgs[imp]
			if !ok {
				continue
			}
			dstID := sanitize(target.shortPath)
			fmt.Fprintf(w, "  %s_anchor -> %s_anchor [ltail=cluster_%s lhead=cluster_%s];\n",
				srcID, dstID, srcID, dstID)
		}
	}

	fmt.Fprintln(w, "}")
}

func emitGroupContents(w io.Writer, id string, grp fileGroup, maxFuncs, maxMethods, maxTypes int, indent string) {
	// Group methods by receiver.
	methodsByRecv := map[string][]string{}
	for _, m := range grp.methods {
		methodsByRecv[m.receiver] = append(methodsByRecv[m.receiver], m.name)
	}

	// Types.
	sort.Slice(grp.types, func(i, j int) bool {
		return grp.types[i].name < grp.types[j].name
	})

	typesToShow := grp.types
	extraTypes := 0
	if maxTypes > 0 && len(typesToShow) > maxTypes {
		extraTypes = len(typesToShow) - maxTypes
		typesToShow = typesToShow[:maxTypes]
	}

	for _, ti := range typesToShow {
		kindLabel := "\u00AB" + ti.kind + "\u00BB" // «kind»
		methods := methodsByRecv[ti.name]
		sort.Strings(methods)
		shown, extra := truncateStr(methods, maxMethods)

		label := fmt.Sprintf("{%s %s", kindLabel, ti.name)
		if len(shown) > 0 {
			label += "|"
			for _, m := range shown {
				label += m + "()\\l"
			}
			if extra > 0 {
				label += fmt.Sprintf("... +%d more\\l", extra)
			}
		}
		label += "}"

		borderColor := "#1976D2"
		switch ti.kind {
		case "interface":
			borderColor = "#388E3C"
		case "type":
			borderColor = "#F57C00"
		}

		fmt.Fprintf(w, "%s%s_%s [label=\"%s\" shape=record style=filled fillcolor=white color=\"%s\"];\n",
			indent, id, sanitize(ti.name), label, borderColor)
	}

	if extraTypes > 0 {
		fmt.Fprintf(w, "%s%s_extra_types [label=\"... +%d more types\" shape=plaintext fontsize=9 fontcolor=\"#757575\"];\n",
			indent, id, extraTypes)
	}

	// Standalone functions.
	funcNames := make([]string, len(grp.funcs))
	for i, f := range grp.funcs {
		funcNames[i] = f.name
	}
	sort.Strings(funcNames)
	shown, extra := truncateStr(funcNames, maxFuncs)
	if len(shown) > 0 {
		label := "{Functions|"
		for _, fn := range shown {
			label += fn + "()\\l"
		}
		if extra > 0 {
			label += fmt.Sprintf("... +%d more\\l", extra)
		}
		label += "}"
		fmt.Fprintf(w, "%s%s_funcs [label=\"%s\" shape=record style=\"filled,dashed\" fillcolor=\"#FAFAFA\" color=\"#757575\"];\n",
			indent, id, label)
	}
}
