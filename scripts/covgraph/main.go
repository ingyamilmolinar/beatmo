// Command covgraph generates a DOT graph visualizing per-function test coverage.
// It parses a Go coverage profile and matches coverage blocks to AST function
// bodies, then produces a Graphviz DOT file with HTML-label tables where each
// function cell is color-coded by its coverage percentage.
//
// Two modes:
//   - Summary (default): One compact node per package with an aggregate coverage
//     bar, suitable for a full-codebase overview PNG.
//   - Detail (-detail or -pkg): Per-function tables with individual coverage
//     colors, for drilling into specific packages.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

type funcInfo struct {
	name       string
	sourceFile string // base filename without .go
	receiver   string // "" for standalone functions
	line       int    // start line
	endLine    int    // end line (closing brace)
	coverage   float64
	profiled   bool // whether any coverage block matched
	isStub     bool // platform stub file
	exported   bool
}

type typeInfo struct {
	name       string
	kind       string // "struct", "interface", "type"
	sourceFile string
}

type pkgInfo struct {
	name       string
	importPath string
	shortPath  string
	types      []typeInfo
	funcs      []funcInfo // standalone + methods unified
}

type covBlock struct {
	startLine int
	endLine   int
	stmts     int
	count     int
}

// fileGroup clusters declarations from files sharing a common prefix.
type fileGroup struct {
	prefix string
	label  string
	types  []typeInfo
	funcs  []funcInfo
}

// ---------------------------------------------------------------------------
// Coverage profile parsing
// ---------------------------------------------------------------------------

func parseCoverProfile(path string) (map[string][]covBlock, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	blocks := map[string][]covBlock{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		// Format: file:startLine.startCol,endLine.endCol stmts count
		colonIdx := strings.LastIndex(line, ":")
		if colonIdx < 0 {
			continue
		}
		file := line[:colonIdx]
		rest := line[colonIdx+1:]

		parts := strings.Fields(rest)
		if len(parts) != 3 {
			continue
		}
		rangePart := parts[0]
		stmts, _ := strconv.Atoi(parts[1])
		count, _ := strconv.Atoi(parts[2])

		comma := strings.Index(rangePart, ",")
		if comma < 0 {
			continue
		}
		startPart := rangePart[:comma]
		endPart := rangePart[comma+1:]

		blocks[file] = append(blocks[file], covBlock{
			startLine: lineFromPos(startPart),
			endLine:   lineFromPos(endPart),
			stmts:     stmts,
			count:     count,
		})
	}
	return blocks, scanner.Err()
}

func lineFromPos(s string) int {
	dot := strings.Index(s, ".")
	if dot < 0 {
		n, _ := strconv.Atoi(s)
		return n
	}
	n, _ := strconv.Atoi(s[:dot])
	return n
}

// ---------------------------------------------------------------------------
// AST discovery
// ---------------------------------------------------------------------------

func discover(root, mod string, exportedOnly bool) map[string]*pkgInfo {
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

		base := filepath.Base(path)
		isStub := strings.HasSuffix(base, "_stub.go") ||
			strings.HasSuffix(base, "_wasm.go") ||
			strings.HasSuffix(base, "_desktop.go")

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
					if !ok {
						continue
					}
					if exportedOnly && !ts.Name.IsExported() {
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
				if exportedOnly && !d.Name.IsExported() {
					continue
				}
				if d.Body == nil {
					continue
				}
				fi := funcInfo{
					name:       d.Name.Name,
					sourceFile: srcFile,
					line:       fset.Position(d.Pos()).Line,
					endLine:    fset.Position(d.Body.End()).Line,
					coverage:   -1,
					isStub:     isStub,
					exported:   d.Name.IsExported(),
				}
				if d.Recv != nil {
					fi.receiver = receiverTypeName(d.Recv)
				}
				pkg.funcs = append(pkg.funcs, fi)
			}
		}

		return nil
	})

	return pkgs
}

// ---------------------------------------------------------------------------
// Merge coverage data with AST functions
// ---------------------------------------------------------------------------

func mergeCoverage(pkgs map[string]*pkgInfo, covBlocks map[string][]covBlock, mod string) {
	for filePath, blocks := range covBlocks {
		if !strings.HasPrefix(filePath, mod+"/") {
			continue
		}
		relFile := filePath[len(mod)+1:]
		dir := filepath.Dir(relFile)
		importPath := mod + "/" + filepath.ToSlash(dir)

		pkg, ok := pkgs[importPath]
		if !ok {
			continue
		}

		base := filepath.Base(relFile)
		srcFile := strings.TrimSuffix(base, ".go")

		for i := range pkg.funcs {
			fn := &pkg.funcs[i]
			if fn.sourceFile != srcFile {
				continue
			}

			totalStmts := 0
			coveredStmts := 0

			for _, b := range blocks {
				if b.startLine >= fn.line && b.endLine <= fn.endLine {
					totalStmts += b.stmts
					if b.count > 0 {
						coveredStmts += b.stmts
					}
				}
			}

			if totalStmts > 0 {
				fn.profiled = true
				fn.coverage = float64(coveredStmts) / float64(totalStmts) * 100.0
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Grouping (adapted from depgraph)
// ---------------------------------------------------------------------------

func filePrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return ""
	}
	return name[:idx]
}

func buildGroups(pkg *pkgInfo, threshold int) []fileGroup {
	totalDecls := len(pkg.types) + len(pkg.funcs)
	if totalDecls < threshold {
		return []fileGroup{{
			prefix: "",
			label:  pkg.name,
			types:  pkg.types,
			funcs:  pkg.funcs,
		}}
	}

	prefixFiles := map[string]map[string]bool{}
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

// ---------------------------------------------------------------------------
// Coverage color scheme
// ---------------------------------------------------------------------------

type covStyle struct {
	fill   string
	border string
	label  string
}

func coverageStyle(fn funcInfo) covStyle {
	if fn.isStub && !fn.profiled {
		return covStyle{"#F5F5F5", "#9E9E9E", "stub"}
	}
	if !fn.profiled {
		return covStyle{"#FFFFFF", "#BDBDBD", "\u2014"}
	}
	switch {
	case fn.coverage >= 80:
		return covStyle{"#C8E6C9", "#2E7D32", fmt.Sprintf("%.0f%%", fn.coverage)}
	case fn.coverage >= 50:
		return covStyle{"#FFF9C4", "#F9A825", fmt.Sprintf("%.0f%%", fn.coverage)}
	case fn.coverage > 0:
		return covStyle{"#FFCDD2", "#C62828", fmt.Sprintf("%.0f%%", fn.coverage)}
	default:
		return covStyle{"#E0E0E0", "#757575", "0%"}
	}
}

func coverageFill(pct float64) string {
	switch {
	case pct >= 80:
		return "#C8E6C9"
	case pct >= 50:
		return "#FFF9C4"
	case pct > 0:
		return "#FFCDD2"
	default:
		return "#E0E0E0"
	}
}

// ---------------------------------------------------------------------------
// Summary stats
// ---------------------------------------------------------------------------

type coverageStats struct {
	total    int
	covered  int
	pctSum   float64
	pctCount int
}

func groupStats(funcs []funcInfo, includeStubs bool) coverageStats {
	var s coverageStats
	for _, fn := range funcs {
		if fn.isStub && !includeStubs {
			continue
		}
		s.total++
		if fn.profiled && fn.coverage > 0 {
			s.covered++
		}
		if fn.profiled {
			s.pctSum += fn.coverage
			s.pctCount++
		}
	}
	return s
}

func (s coverageStats) avgPct() float64 {
	if s.pctCount == 0 {
		return 0
	}
	return s.pctSum / float64(s.pctCount)
}

// ---------------------------------------------------------------------------
// Sorting
// ---------------------------------------------------------------------------

func sortFuncs(funcs []funcInfo, order string) {
	switch order {
	case "coverage":
		sort.Slice(funcs, func(i, j int) bool {
			ci, cj := funcs[i].coverage, funcs[j].coverage
			if !funcs[i].profiled {
				ci = -1
			}
			if !funcs[j].profiled {
				cj = -1
			}
			if ci != cj {
				return ci < cj
			}
			return funcs[i].name < funcs[j].name
		})
	case "coverage-desc":
		sort.Slice(funcs, func(i, j int) bool {
			ci, cj := funcs[i].coverage, funcs[j].coverage
			if !funcs[i].profiled {
				ci = -1
			}
			if !funcs[j].profiled {
				cj = -1
			}
			if ci != cj {
				return ci > cj
			}
			return funcs[i].name < funcs[j].name
		})
	default: // "name"
		sort.Slice(funcs, func(i, j int) bool {
			return funcs[i].name < funcs[j].name
		})
	}
}

// ---------------------------------------------------------------------------
// Package filtering
// ---------------------------------------------------------------------------

func filterPkgs(pkgs map[string]*pkgInfo, filter string, _ string) map[string]*pkgInfo {
	if filter == "" {
		return pkgs
	}
	patterns := strings.Split(filter, ",")
	result := map[string]*pkgInfo{}
	for ip, pkg := range pkgs {
		for _, pat := range patterns {
			pat = strings.TrimSpace(pat)
			if pat == "" {
				continue
			}
			// Match against shortPath (e.g., "core/engine", "internal/ui")
			if pkg.shortPath == pat || strings.HasPrefix(pkg.shortPath, pat+"/") {
				result[ip] = pkg
				break
			}
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// DOT generation — Package-level color palette (same as depgraph)
// ---------------------------------------------------------------------------

var groupColor = map[string]string{
	"cmd":                     "#E8F5E9",
	"core/model":              "#E3F2FD",
	"core/beat":               "#E3F2FD",
	"core/engine":             "#E3F2FD",
	"internal/ui":             "#FFF3E0",
	"internal/ui/preview":     "#FFF3E0",
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

func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", ".", "_", "-", "_")
	return r.Replace(s)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}

type dotConfig struct {
	maxFuncs       int
	maxMethods     int
	maxTypes       int
	groupThreshold int
	includeStubs   bool
	sortOrder      string
	summary        bool
}

// ---------------------------------------------------------------------------
// Summary mode — one compact node per package
// ---------------------------------------------------------------------------

func generateSummaryDOT(w io.Writer, pkgs map[string]*pkgInfo, cfg dotConfig) {
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fmt.Fprintln(w, "digraph beatmo_coverage {")
	fmt.Fprintln(w, "  rankdir=TB;")
	fmt.Fprintln(w, `  fontname="Helvetica";`)
	fmt.Fprintln(w, `  node [fontname="Helvetica" fontsize=11];`)
	fmt.Fprintln(w, `  edge [style=invis];`) // invisible edges just for ordering
	fmt.Fprintln(w, "  nodesep=0.4;")
	fmt.Fprintln(w, "  ranksep=0.6;")
	fmt.Fprintln(w, `  label=<<FONT POINT-SIZE="18"><B>Beatmo Test Coverage Map</B></FONT>>;`)
	fmt.Fprintln(w, "  labelloc=t;")
	fmt.Fprintln(w)

	emitLegend(w)

	// Group packages by area for clustering.
	areas := []struct {
		label string
		color string
		match func(string) bool
	}{
		{"Core", "#E3F2FD", func(s string) bool { return strings.HasPrefix(s, "core/") }},
		{"Internal", "#FFF3E0", func(s string) bool { return strings.HasPrefix(s, "internal/") }},
		{"Cmd", "#E8F5E9", func(s string) bool { return s == "cmd" || s == "cmd/export_audio" }},
	}

	for ai, area := range areas {
		var areaPkgs []string
		for _, ip := range paths {
			pkg := pkgs[ip]
			if len(pkg.funcs) == 0 && len(pkg.types) == 0 {
				continue
			}
			if area.match(pkg.shortPath) {
				areaPkgs = append(areaPkgs, ip)
			}
		}
		if len(areaPkgs) == 0 {
			continue
		}

		clusterID := fmt.Sprintf("area_%d", ai)
		fmt.Fprintf(w, "  subgraph cluster_%s {\n", clusterID)
		fmt.Fprintf(w, "    label=<%s>;\n", escapeHTML(area.label))
		fmt.Fprintf(w, "    style=filled; fillcolor=\"%s\"; color=\"#BDBDBD\";\n", area.color)
		fmt.Fprintln(w, "    fontsize=14; labeljust=l;")

		for _, ip := range areaPkgs {
			pkg := pkgs[ip]
			emitSummaryNode(w, pkg, cfg, "    ")
		}

		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "}")
}

func emitSummaryNode(w io.Writer, pkg *pkgInfo, cfg dotConfig, indent string) {
	sid := sanitize(pkg.shortPath)
	stats := groupStats(pkg.funcs, cfg.includeStubs)
	avgPct := stats.avgPct()
	fill := coverageFill(avgPct)

	// For large packages, show sub-group breakdown rows.
	groups := buildGroups(pkg, cfg.groupThreshold)
	hasSubgroups := len(groups) > 1

	fmt.Fprintf(w, "%s%s [shape=plaintext label=<\n", indent, sid)
	fmt.Fprintf(w, "%s  <TABLE BORDER=\"1\" CELLBORDER=\"0\" CELLSPACING=\"0\" CELLPADDING=\"4\" COLOR=\"#BDBDBD\">\n", indent)

	// Header row with package name + overall coverage
	fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" COLSPAN=\"3\"><B>%s</B></TD></TR>\n",
		indent, fill, escapeHTML(pkg.shortPath))
	fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" COLSPAN=\"3\">%d/%d funcs covered \u2014 avg %.0f%%</TD></TR>\n",
		indent, fill, stats.covered, stats.total, avgPct)

	if hasSubgroups {
		// Show top sub-groups as rows
		for _, grp := range groups {
			grpStats := groupStats(grp.funcs, cfg.includeStubs)
			grpPct := grpStats.avgPct()
			grpFill := coverageFill(grpPct)
			label := grp.label + "_*"
			if grp.prefix == "_other" {
				label = "other"
			}
			fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" ALIGN=\"LEFT\"><FONT POINT-SIZE=\"9\">%s</FONT></TD>"+
				"<TD BGCOLOR=\"%s\" ALIGN=\"RIGHT\"><FONT POINT-SIZE=\"9\">%d/%d</FONT></TD>"+
				"<TD BGCOLOR=\"%s\" ALIGN=\"RIGHT\"><FONT POINT-SIZE=\"9\">%.0f%%</FONT></TD></TR>\n",
				indent, grpFill, escapeHTML(label), grpFill, grpStats.covered, grpStats.total, grpFill, grpPct)
		}
	}

	fmt.Fprintf(w, "%s  </TABLE>\n", indent)
	fmt.Fprintf(w, "%s>];\n", indent)
}

// ---------------------------------------------------------------------------
// Detail mode — per-function tables (original behavior)
// ---------------------------------------------------------------------------

func generateDetailDOT(w io.Writer, pkgs map[string]*pkgInfo, cfg dotConfig) {
	paths := make([]string, 0, len(pkgs))
	for p := range pkgs {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fmt.Fprintln(w, "digraph beatmo_coverage {")
	fmt.Fprintln(w, "  rankdir=LR;")
	fmt.Fprintln(w, `  fontname="Helvetica";`)
	fmt.Fprintln(w, `  node [fontname="Helvetica" fontsize=10];`)
	fmt.Fprintln(w, `  edge [fontname="Helvetica" fontsize=8 color="#666666"];`)
	fmt.Fprintln(w, "  compound=true;")
	fmt.Fprintln(w, "  nodesep=0.3;")
	fmt.Fprintln(w, "  ranksep=1.0;")
	fmt.Fprintln(w, `  label=<<FONT POINT-SIZE="16"><B>Beatmo Test Coverage Map</B></FONT>>;`)
	fmt.Fprintln(w, "  labelloc=t;")
	fmt.Fprintln(w)

	emitLegend(w)

	for _, ip := range paths {
		pkg := pkgs[ip]
		if len(pkg.funcs) == 0 && len(pkg.types) == 0 {
			continue
		}

		sid := sanitize(pkg.shortPath)
		bgColor := pkgColor(pkg.shortPath)
		stats := groupStats(pkg.funcs, cfg.includeStubs)

		groups := buildGroups(pkg, cfg.groupThreshold)
		hasSubgroups := len(groups) > 1

		fmt.Fprintf(w, "  subgraph cluster_%s {\n", sid)
		fmt.Fprintf(w, "    label=<%s<BR/><FONT POINT-SIZE=\"9\">%s \u2014 %d/%d covered (%.0f%%)</FONT>>;\n",
			escapeHTML(pkg.name), escapeHTML(pkg.shortPath), stats.covered, stats.total, stats.avgPct())
		fmt.Fprintf(w, "    style=filled; fillcolor=\"%s\"; color=\"#BDBDBD\";\n", bgColor)
		fmt.Fprintln(w, "    fontsize=12; labeljust=l;")

		if hasSubgroups {
			for gi, grp := range groups {
				gsid := fmt.Sprintf("%s_g%d", sid, gi)
				subColor := "#F5F5F5"
				if c, ok := uiSubgroupColor[grp.prefix]; ok {
					subColor = c
				}
				grpStats := groupStats(grp.funcs, cfg.includeStubs)
				fmt.Fprintf(w, "    subgraph cluster_%s {\n", gsid)
				fmt.Fprintf(w, "      label=<%s_*<BR/><FONT POINT-SIZE=\"8\">%d/%d covered (%.0f%%)</FONT>>;\n",
					escapeHTML(grp.label), grpStats.covered, grpStats.total, grpStats.avgPct())
				fmt.Fprintf(w, "      style=\"filled,rounded\"; fillcolor=\"%s\"; color=\"#E0E0E0\";\n", subColor)
				fmt.Fprintln(w, "      fontsize=11; labeljust=l;")
				emitGroupContents(w, gsid, grp, cfg, "      ")
				fmt.Fprintln(w, "    }")
			}
		} else if len(groups) == 1 {
			emitGroupContents(w, sid, groups[0], cfg, "    ")
		}

		fmt.Fprintln(w, "  }")
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "}")
}

func emitLegend(w io.Writer) {
	fmt.Fprintln(w, `  legend [shape=plaintext label=<`)
	fmt.Fprintln(w, `    <TABLE BORDER="1" CELLBORDER="0" CELLSPACING="0" CELLPADDING="4" COLOR="#BDBDBD">`)
	fmt.Fprintln(w, `      <TR><TD COLSPAN="2"><B>Coverage Legend</B></TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#C8E6C9" WIDTH="20"> </TD><TD ALIGN="LEFT">&gt;= 80% (well covered)</TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#FFF9C4" WIDTH="20"> </TD><TD ALIGN="LEFT">50-79% (partial)</TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#FFCDD2" WIDTH="20"> </TD><TD ALIGN="LEFT">1-49% (weak)</TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#E0E0E0" WIDTH="20"> </TD><TD ALIGN="LEFT">0% (untested)</TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#FFFFFF" WIDTH="20"> </TD><TD ALIGN="LEFT">Not profiled</TD></TR>`)
	fmt.Fprintln(w, `      <TR><TD BGCOLOR="#F5F5F5" WIDTH="20"> </TD><TD ALIGN="LEFT">Platform stub</TD></TR>`)
	fmt.Fprintln(w, `    </TABLE>`)
	fmt.Fprintln(w, `  >];`)
	fmt.Fprintln(w)
}

func emitGroupContents(w io.Writer, id string, grp fileGroup, cfg dotConfig, indent string) {
	methodsByRecv := map[string][]funcInfo{}
	var standaloneFuncs []funcInfo

	for _, fn := range grp.funcs {
		if fn.receiver != "" {
			methodsByRecv[fn.receiver] = append(methodsByRecv[fn.receiver], fn)
		} else {
			standaloneFuncs = append(standaloneFuncs, fn)
		}
	}

	sort.Slice(grp.types, func(i, j int) bool {
		return grp.types[i].name < grp.types[j].name
	})

	typesToShow := grp.types
	extraTypes := 0
	if cfg.maxTypes > 0 && len(typesToShow) > cfg.maxTypes {
		extraTypes = len(typesToShow) - cfg.maxTypes
		typesToShow = typesToShow[:cfg.maxTypes]
	}

	for _, ti := range typesToShow {
		methods := methodsByRecv[ti.name]
		sortFuncs(methods, cfg.sortOrder)

		shown := methods
		extraMethods := 0
		if cfg.maxMethods > 0 && len(shown) > cfg.maxMethods {
			extraMethods = len(shown) - cfg.maxMethods
			shown = shown[:cfg.maxMethods]
		}

		nodeID := fmt.Sprintf("%s_%s", id, sanitize(ti.name))
		emitTypeNode(w, nodeID, ti, shown, extraMethods, indent)
	}

	if extraTypes > 0 {
		fmt.Fprintf(w, "%s%s_extra_types [label=\"... +%d more types\" shape=plaintext fontsize=9 fontcolor=\"#757575\"];\n",
			indent, id, extraTypes)
	}

	sortFuncs(standaloneFuncs, cfg.sortOrder)

	shown := standaloneFuncs
	extraFuncs := 0
	if cfg.maxFuncs > 0 && len(shown) > cfg.maxFuncs {
		extraFuncs = len(shown) - cfg.maxFuncs
		shown = shown[:cfg.maxFuncs]
	}

	if len(shown) > 0 {
		nodeID := fmt.Sprintf("%s_funcs", id)
		emitFuncsNode(w, nodeID, shown, extraFuncs, indent)
	}

	// Methods for types not in the types list
	var orphanReceivers []string
	typeSet := map[string]bool{}
	for _, t := range grp.types {
		typeSet[t.name] = true
	}
	for recv := range methodsByRecv {
		if !typeSet[recv] {
			orphanReceivers = append(orphanReceivers, recv)
		}
	}
	sort.Strings(orphanReceivers)

	for _, recv := range orphanReceivers {
		methods := methodsByRecv[recv]
		sortFuncs(methods, cfg.sortOrder)

		shown := methods
		extra := 0
		if cfg.maxMethods > 0 && len(shown) > cfg.maxMethods {
			extra = len(shown) - cfg.maxMethods
			shown = shown[:cfg.maxMethods]
		}

		ti := typeInfo{name: recv, kind: "struct", sourceFile: ""}
		nodeID := fmt.Sprintf("%s_%s", id, sanitize(recv))
		emitTypeNode(w, nodeID, ti, shown, extra, indent)
	}
}

func emitTypeNode(w io.Writer, nodeID string, ti typeInfo, methods []funcInfo, extraMethods int, indent string) {
	kindLabel := "\u00AB" + ti.kind + "\u00BB"

	headerBG := "#E3F2FD"
	switch ti.kind {
	case "interface":
		headerBG = "#E8F5E9"
	case "type":
		headerBG = "#FFF3E0"
	}

	fmt.Fprintf(w, "%s%s [shape=plaintext label=<\n", indent, nodeID)
	fmt.Fprintf(w, "%s  <TABLE BORDER=\"1\" CELLBORDER=\"0\" CELLSPACING=\"0\" CELLPADDING=\"3\" COLOR=\"#BDBDBD\">\n", indent)
	fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" COLSPAN=\"2\"><B>%s %s</B></TD></TR>\n",
		indent, headerBG, escapeHTML(kindLabel), escapeHTML(ti.name))

	for _, m := range methods {
		style := coverageStyle(m)
		fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" ALIGN=\"LEFT\">%s()</TD><TD BGCOLOR=\"%s\" ALIGN=\"RIGHT\"><FONT POINT-SIZE=\"8\">%s</FONT></TD></TR>\n",
			indent, style.fill, escapeHTML(m.name), style.fill, style.label)
	}

	if extraMethods > 0 {
		fmt.Fprintf(w, "%s    <TR><TD COLSPAN=\"2\" ALIGN=\"LEFT\"><FONT POINT-SIZE=\"8\" COLOR=\"#757575\">... +%d more</FONT></TD></TR>\n",
			indent, extraMethods)
	}

	fmt.Fprintf(w, "%s  </TABLE>\n", indent)
	fmt.Fprintf(w, "%s>];\n", indent)
}

func emitFuncsNode(w io.Writer, nodeID string, funcs []funcInfo, extraFuncs int, indent string) {
	fmt.Fprintf(w, "%s%s [shape=plaintext label=<\n", indent, nodeID)
	fmt.Fprintf(w, "%s  <TABLE BORDER=\"1\" CELLBORDER=\"0\" CELLSPACING=\"0\" CELLPADDING=\"3\" COLOR=\"#757575\" STYLE=\"dashed\">\n", indent)
	fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"#FAFAFA\" COLSPAN=\"2\"><B>Functions</B></TD></TR>\n", indent)

	for _, fn := range funcs {
		style := coverageStyle(fn)
		fmt.Fprintf(w, "%s    <TR><TD BGCOLOR=\"%s\" ALIGN=\"LEFT\">%s()</TD><TD BGCOLOR=\"%s\" ALIGN=\"RIGHT\"><FONT POINT-SIZE=\"8\">%s</FONT></TD></TR>\n",
			indent, style.fill, escapeHTML(fn.name), style.fill, style.label)
	}

	if extraFuncs > 0 {
		fmt.Fprintf(w, "%s    <TR><TD COLSPAN=\"2\" ALIGN=\"LEFT\"><FONT POINT-SIZE=\"8\" COLOR=\"#757575\">... +%d more</FONT></TD></TR>\n",
			indent, extraFuncs)
	}

	fmt.Fprintf(w, "%s  </TABLE>\n", indent)
	fmt.Fprintf(w, "%s>];\n", indent)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	root := flag.String("root", "src/go", "Go source root")
	mod := flag.String("module", "github.com/ingyamilmolinar/beatmo", "Module path")
	cover := flag.String("cover", "coverage/go.out", "Coverage profile path")
	maxFuncs := flag.Int("max-funcs", 20, "Max standalone functions per group (0=all)")
	maxMethods := flag.Int("max-methods", 10, "Max methods per type (0=all)")
	maxTypes := flag.Int("max-types", 15, "Max types per group (0=all)")
	groupThreshold := flag.Int("group-threshold", 20, "Min declarations to trigger file-prefix grouping")
	exportedOnly := flag.Bool("exported-only", false, "Only show exported functions")
	includeStubs := flag.Bool("include-stubs", false, "Include stubs in coverage stats")
	sortOrder := flag.String("sort", "coverage", "Sort order: coverage (asc), name, coverage-desc")
	detail := flag.Bool("detail", false, "Show per-function detail (default is summary overview)")
	pkgFilter := flag.String("pkg", "", "Comma-separated package paths to include (e.g., core/engine,internal/ui)")
	flag.Parse()

	pkgs := discover(*root, *mod, *exportedOnly)

	covBlocks, err := parseCoverProfile(*cover)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading coverage profile: %v\n", err)
		os.Exit(1)
	}

	mergeCoverage(pkgs, covBlocks, *mod)

	// If -pkg is set, filter and auto-enable detail mode.
	if *pkgFilter != "" {
		pkgs = filterPkgs(pkgs, *pkgFilter, *mod)
		if len(pkgs) == 0 {
			fmt.Fprintf(os.Stderr, "no packages matched filter %q\n", *pkgFilter)
			os.Exit(1)
		}
		*detail = true
	}

	cfg := dotConfig{
		maxFuncs:       *maxFuncs,
		maxMethods:     *maxMethods,
		maxTypes:       *maxTypes,
		groupThreshold: *groupThreshold,
		includeStubs:   *includeStubs,
		sortOrder:      *sortOrder,
		summary:        !*detail,
	}

	if cfg.summary {
		generateSummaryDOT(os.Stdout, pkgs, cfg)
	} else {
		generateDetailDOT(os.Stdout, pkgs, cfg)
	}
}
