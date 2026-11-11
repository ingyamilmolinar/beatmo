package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type importSpec struct {
	name string
	path string
}

type chunkSpec struct {
	outFile   string
	startLine int
}

type chunk struct {
	outFile   string
	startLine int
	endLine   int
}

func main() {
	if err := splitAudioEngine(filepath.FromSlash("src/go/internal/audio/engine.go")); err != nil {
		fatal(err)
	}
	if err := splitTimelineService(filepath.FromSlash("src/go/internal/timeline/service.go")); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func splitAudioEngine(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	headerPrefix, pkgName, pkgPos, err := fileHeaderPrefix(src, path)
	if err != nil {
		return err
	}
	_ = pkgPos
	importEnd, err := findLastImportEnd(src, path)
	if err != nil {
		return err
	}
	bodyStart := importEnd
	lineStarts := computeLineStarts(src)
	linesTotal := len(lineStarts) - 1
	bodyStartLine := lineForOffset(lineStarts, bodyStart)

	funcLines, err := funcStartLineMap(src, path)
	if err != nil {
		return err
	}
	typeLines, err := typeStartLineMap(src, path)
	if err != nil {
		return err
	}
	mustFunc := func(name string) int {
		ln, ok := funcLines[name]
		if !ok {
			return fatalf("%s: missing func %q", path, name)
		}
		return ln
	}
	mustType := func(name string) int {
		ln, ok := typeLines[name]
		if !ok {
			return fatalf("%s: missing type %q", path, name)
		}
		return ln
	}

	specs := []chunkSpec{
		{outFile: filepath.Base(path), startLine: bodyStartLine},
		{outFile: "engine_play.go", startLine: mustFunc("Play")},
		{outFile: "engine_stop.go", startLine: mustFunc("Stop")},
		{outFile: "engine_resample.go", startLine: mustFunc("pow2")},
		{outFile: "engine_instruments.go", startLine: mustFunc("ResetInstruments")},
		{outFile: "engine_state.go", startLine: mustType("scaledVoice")},
		{outFile: "engine_mixer.go", startLine: mustType("mixer")},
	}

	chunks, err := finalizeChunks(specs, linesTotal)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := writeChunks(path, src, headerPrefix, pkgName, bodyStart, lineStarts, chunks); err != nil {
		return err
	}
	if err := verifyCoverage(src, bodyStart, lineStarts, chunks); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func splitTimelineService(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	headerPrefix, pkgName, pkgPos, err := fileHeaderPrefix(src, path)
	if err != nil {
		return err
	}
	_ = pkgPos
	importEnd, err := findLastImportEnd(src, path)
	if err != nil {
		return err
	}
	bodyStart := importEnd
	lineStarts := computeLineStarts(src)
	linesTotal := len(lineStarts) - 1
	bodyStartLine := lineForOffset(lineStarts, bodyStart)

	funcLines, err := funcStartLineMap(src, path)
	if err != nil {
		return err
	}
	mustFunc := func(name string) int {
		ln, ok := funcLines[name]
		if !ok {
			return fatalf("%s: missing func %q", path, name)
		}
		return ln
	}

	specs := []chunkSpec{
		{outFile: filepath.Base(path), startLine: bodyStartLine},
		{outFile: "service_segments.go", startLine: mustFunc("UpdateRowSegments")},
		{outFile: "service_trim.go", startLine: mustFunc("TrimBefore")},
		{outFile: "service_seed.go", startLine: mustFunc("SeedFromWindow")},
		{outFile: "service_internal.go", startLine: mustFunc("valueLocked")},
	}

	chunks, err := finalizeChunks(specs, linesTotal)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := writeChunks(path, src, headerPrefix, pkgName, bodyStart, lineStarts, chunks); err != nil {
		return err
	}
	if err := verifyCoverage(src, bodyStart, lineStarts, chunks); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func fatalf(format string, args ...any) int {
	fatal(fmt.Errorf(format, args...))
	return 0
}

func fileHeaderPrefix(src []byte, filename string) ([]byte, string, token.Pos, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, "", token.NoPos, err
	}
	pkgOff := fset.Position(file.Package).Offset
	if pkgOff < 0 || pkgOff > len(src) {
		return nil, "", token.NoPos, fmt.Errorf("invalid package offset %d", pkgOff)
	}
	return src[:pkgOff], file.Name.Name, file.Package, nil
}

func findLastImportEnd(src []byte, filename string) (int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return 0, err
	}
	end := 0
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		off := fset.Position(gd.End()).Offset
		if off > end {
			end = off
		}
	}
	if end == 0 {
		return 0, fmt.Errorf("no import decl found")
	}
	return end, nil
}

func finalizeChunks(specs []chunkSpec, linesTotal int) ([]chunk, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("no chunks")
	}
	sorted := append([]chunkSpec(nil), specs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].startLine < sorted[j].startLine })
	for i := 0; i < len(sorted); i++ {
		if sorted[i].startLine <= 0 || sorted[i].startLine > linesTotal {
			return nil, fmt.Errorf("invalid start line %d for %s", sorted[i].startLine, sorted[i].outFile)
		}
		if i > 0 && sorted[i].startLine == sorted[i-1].startLine {
			return nil, fmt.Errorf("duplicate start line %d for %s and %s", sorted[i].startLine, sorted[i-1].outFile, sorted[i].outFile)
		}
	}

	chunks := make([]chunk, 0, len(sorted))
	for i := 0; i < len(sorted); i++ {
		start := sorted[i].startLine
		end := linesTotal
		if i+1 < len(sorted) {
			end = sorted[i+1].startLine - 1
		}
		if end < start {
			return nil, fmt.Errorf("invalid chunk range %s: %d..%d", sorted[i].outFile, start, end)
		}
		chunks = append(chunks, chunk{outFile: sorted[i].outFile, startLine: start, endLine: end})
	}
	return chunks, nil
}

func writeChunks(originalPath string, src []byte, headerPrefix []byte, pkgName string, bodyStart int, lineStarts []int, chunks []chunk) error {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)

	// Remove any previously generated files in this split set (excluding base).
	toDelete := map[string]bool{}
	for _, c := range chunks {
		if c.outFile == base {
			continue
		}
		toDelete[c.outFile] = true
	}
	for name := range toDelete {
		_ = os.Remove(filepath.Join(dir, name))
	}

	imports, err := readImports(src, originalPath)
	if err != nil {
		return err
	}

	for _, c := range chunks {
		startOff := offsetForLine(lineStarts, c.startLine)
		endOff := offsetForLineEnd(src, lineStarts, c.endLine)
		if startOff < bodyStart {
			startOff = bodyStart
		}
		if endOff < startOff {
			return fmt.Errorf("%s: invalid offsets for chunk %s", originalPath, c.outFile)
		}
		chunkBody := src[startOff:endOff]

		usedNames, err := usedImportNames(pkgName, imports, chunkBody)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", originalPath, c.outFile, err)
		}

		usedImports := make([]importSpec, 0, len(usedNames))
		for _, imp := range imports {
			local := imp.name
			if local == "" {
				local = defaultImportName(imp.path)
			}
			if usedNames[local] {
				usedImports = append(usedImports, imp)
			}
		}

		var buf bytes.Buffer
		buf.Write(headerPrefix)
		buf.WriteString("package " + pkgName + "\n\n")
		writeImportBlock(&buf, usedImports)
		buf.Write(chunkBody)

		outPath := filepath.Join(dir, c.outFile)
		if err := os.WriteFile(outPath, buf.Bytes(), fs.FileMode(0644)); err != nil {
			return err
		}
	}
	return nil
}

func verifyCoverage(src []byte, bodyStart int, lineStarts []int, chunks []chunk) error {
	want := src[bodyStart:]
	var got bytes.Buffer
	cs := append([]chunk(nil), chunks...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].startLine < cs[j].startLine })
	for _, c := range cs {
		startOff := offsetForLine(lineStarts, c.startLine)
		endOff := offsetForLineEnd(src, lineStarts, c.endLine)
		if startOff < bodyStart {
			startOff = bodyStart
		}
		got.Write(src[startOff:endOff])
	}
	if !bytes.Equal(want, got.Bytes()) {
		return fmt.Errorf("coverage mismatch: concatenated chunks do not match original body")
	}
	return nil
}

func funcStartLineMap(src []byte, filename string) (map[string]int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int)
	for _, d := range file.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		pos := fd.Pos()
		if fd.Doc != nil {
			pos = fd.Doc.Pos()
		}
		out[fd.Name.Name] = fset.Position(pos).Line
	}
	return out, nil
}

func typeStartLineMap(src []byte, filename string) (map[string]int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int)
	for _, d := range file.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			pos := ts.Pos()
			if gd.Doc != nil {
				pos = gd.Doc.Pos()
			}
			out[ts.Name.Name] = fset.Position(pos).Line
		}
	}
	return out, nil
}

func readImports(src []byte, filename string) ([]importSpec, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var out []importSpec
	for _, s := range file.Imports {
		path := strings.Trim(s.Path.Value, "\"")
		name := ""
		if s.Name != nil {
			name = s.Name.Name
		}
		out = append(out, importSpec{name: name, path: path})
	}
	return out, nil
}

func defaultImportName(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	last := parts[len(parts)-1]
	// Handle semantic import version suffixes like /v2, /v3.
	if len(last) >= 2 && last[0] == 'v' {
		isDigits := true
		for i := 1; i < len(last); i++ {
			if last[i] < '0' || last[i] > '9' {
				isDigits = false
				break
			}
		}
		if isDigits && len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}
	return last
}

func usedImportNames(pkgName string, allImports []importSpec, chunkBody []byte) (map[string]bool, error) {
	var buf bytes.Buffer
	buf.WriteString("package " + pkgName + "\n")
	if len(allImports) > 0 {
		buf.WriteString("import (\n")
		for _, imp := range allImports {
			if imp.name != "" {
				buf.WriteString("\t" + imp.name + " \"" + imp.path + "\"\n")
			} else {
				buf.WriteString("\t\"" + imp.path + "\"\n")
			}
		}
		buf.WriteString(")\n")
	}
	buf.Write(chunkBody)

	src := buf.Bytes()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "chunk.go", src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	importNames := make(map[string]bool)
	for _, imp := range allImports {
		local := imp.name
		if local == "" {
			local = defaultImportName(imp.path)
		}
		if local == "_" || local == "." {
			continue
		}
		importNames[local] = true
	}

	used := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if importNames[id.Name] {
			used[id.Name] = true
		}
		return true
	})
	return used, nil
}

func writeImportBlock(buf *bytes.Buffer, imports []importSpec) {
	if len(imports) == 0 {
		return
	}
	buf.WriteString("import (\n")
	prevStd := true
	wroteAny := false
	wroteGap := false
	for _, imp := range imports {
		isStd := isStdImport(imp.path)
		if wroteAny && !wroteGap && prevStd && !isStd {
			buf.WriteString("\n")
			wroteGap = true
		}
		if imp.name != "" {
			buf.WriteString("\t" + imp.name + " \"" + imp.path + "\"\n")
		} else {
			buf.WriteString("\t\"" + imp.path + "\"\n")
		}
		prevStd = isStd
		wroteAny = true
	}
	buf.WriteString(")\n")
}

func isStdImport(path string) bool {
	first := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		first = path[:i]
	}
	return !strings.Contains(first, ".")
}

func computeLineStarts(src []byte) []int {
	// lineStarts is 1-indexed: lineStarts[1] == 0. We add a sentinel at the end.
	starts := []int{0, 0}
	for i := 0; i < len(src); i++ {
		if src[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	if starts[len(starts)-1] != len(src) {
		starts = append(starts, len(src))
	}
	return starts
}

func offsetForLine(lineStarts []int, line int) int {
	if line < 1 {
		return 0
	}
	if line >= len(lineStarts) {
		return lineStarts[len(lineStarts)-1]
	}
	return lineStarts[line]
}

func offsetForLineEnd(src []byte, lineStarts []int, line int) int {
	end := len(src)
	if line+1 < len(lineStarts) {
		next := lineStarts[line+1]
		if next <= len(src) {
			end = next
		}
	}
	return end
}

func lineForOffset(lineStarts []int, off int) int {
	lo, hi := 1, len(lineStarts)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if lineStarts[mid] <= off {
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if hi < 1 {
		return 1
	}
	return hi
}

