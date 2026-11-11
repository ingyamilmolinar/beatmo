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
	name string // local name (may be empty for default)
	path string // import path without quotes
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
	uiDir := filepath.FromSlash("src/go/internal/ui")
	if err := splitGame(filepath.Join(uiDir, "game.go")); err != nil {
		fatal(err)
	}
	if err := splitDrumView(filepath.Join(uiDir, "drumview.go")); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func splitGame(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	importEnd, err := findImportEnd(src)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
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
			fatal(fmt.Errorf("%s: missing func %q", path, name))
		}
		return ln
	}
	mustType := func(name string) int {
		ln, ok := typeLines[name]
		if !ok {
			fatal(fmt.Errorf("%s: missing type %q", path, name))
		}
		return ln
	}
	mustMarker := func(markerLine string) int {
		off, err := findUniqueLineStart(src, markerLine)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
		return lineForOffset(lineStarts, off)
	}

	specs := []chunkSpec{
		{outFile: filepath.Base(path), startLine: bodyStartLine},
		{outFile: "game_parity_types.go", startLine: mustType("mismatchEntry")},
		{outFile: "game_transport_helpers.go", startLine: mustFunc("Playing")},
		{outFile: "game_row_helpers.go", startLine: mustFunc("rowIndexForNode")},
		{outFile: "game_beat_path.go", startLine: mustFunc("makeBeatKey")},
		{outFile: "game_channel_helpers.go", startLine: mustFunc("sendLatest")},
		{outFile: "game_ui_types.go", startLine: mustMarker("/* ───────────────────────── data types ───────────────────────── */")},
		{outFile: "game_struct.go", startLine: mustType("Game")},
		{outFile: "game_node_screen_rect.go", startLine: mustMarker("/* ───────────────── helper: node’s screen rect ───────────────── */")},
		{outFile: "game_timeline.go", startLine: mustFunc("timelineWindowLen")},
		{outFile: "game_parity_state.go", startLine: mustFunc("ParityMismatchSnapshot")},
		{outFile: "game_components.go", startLine: mustFunc("ensureComponentRegistry")},
		{outFile: "game_node_interaction.go", startLine: mustFunc("nodeAtScreen")},
		{outFile: "game_new.go", startLine: mustMarker("/* ───────────────────── constructor & layout ─────────────────── */")},
		{outFile: "game_trigger_cache.go", startLine: mustFunc("setLastTriggered")},
		{outFile: "game_node_highlight_until.go", startLine: mustFunc("setNodeHighlightUntil")},
		{outFile: "game_predictor_notify.go", startLine: mustFunc("notifyPredictorNode")},
		{outFile: "game_sequencer_loop.go", startLine: mustFunc("sequencerLoop")},
		{outFile: "game_sequencer_schedule.go", startLine: mustFunc("seqScheduleTime")},
		{outFile: "game_audio_loop.go", startLine: mustFunc("audioLoop")},
		{outFile: "game_play_function.go", startLine: mustFunc("SetPlayFunc")},
		{outFile: "game_bpm_loop.go", startLine: mustFunc("bpmLoop")},
		{outFile: "game_layout.go", startLine: mustFunc("Layout")},
		{outFile: "game_graph_nodes.go", startLine: mustMarker("/* ─────────────────────── graph helpers ─────────────────────── */")},
		{outFile: "game_graph_update_beat_infos.go", startLine: mustFunc("updateBeatInfos")},
		{outFile: "game_graph_beat_index.go", startLine: mustFunc("beatInfoAt")},
		{outFile: "game_parity_check.go", startLine: mustFunc("parityExpected")},
		{outFile: "game_parity_diff_scan.go", startLine: mustFunc("diffPredictorTimeline")},
		{outFile: "game_origin_sequences.go", startLine: mustFunc("resetOriginSequences")},
		{outFile: "game_refresh_drum_row.go", startLine: mustFunc("refreshDrumRow")},
		{outFile: "game_row_window_builder.go", startLine: mustFunc("buildRowWindow")},
		{outFile: "game_params_hash.go", startLine: mustFunc("paramsHash")},
		{outFile: "game_graph_edges.go", startLine: mustFunc("addEdgeNoRefresh")},
		{outFile: "game_input_editor.go", startLine: mustMarker("/* ─────────────── input handling ───────────────────────────────────────── */")},
		{outFile: "game_input_node_menu_rects.go", startLine: mustFunc("updateNodeMenuRects")},
		{outFile: "game_input_drag.go", startLine: mustFunc("enqueueUI")},
		{outFile: "game_update.go", startLine: mustMarker("/* ─────────────── Update & tick ────────────────────────────────────────── */")},
		{outFile: "game_draw.go", startLine: mustMarker("/* ─────────────── Draw ─────────────────────────────────────────────────── */")},
		{outFile: "game_draw_grid_pane.go", startLine: mustFunc("drawGridPane")},
		{outFile: "game_draw_helpers.go", startLine: mustFunc("drawDivider")},
		{outFile: "game_beat_display.go", startLine: mustFunc("currentBeat")},
		{outFile: "game_tick_highlight.go", startLine: mustFunc("encodeHighlight")},
		{outFile: "game_audio_schedule.go", startLine: mustFunc("queueSoundParams")},
		{outFile: "game_subdivisions.go", startLine: mustFunc("SetSubdivisions")},
		{outFile: "game_trigger_logic.go", startLine: mustFunc("queueSound")},
		{outFile: "game_highlight_state.go", startLine: mustFunc("highlightSet")},
		{outFile: "game_sequencer_highlight.go", startLine: mustFunc("applySequencerHighlight")},
		{outFile: "game_seek.go", startLine: mustFunc("Seek")},
		{outFile: "game_sync_ui.go", startLine: mustFunc("syncUIToTime")},
		{outFile: "game_draw_worldrect.go", startLine: mustFunc("visibleWorldRect")},
		{outFile: "game_math.go", startLine: mustMarker("/* ─────────────── math helpers ─────────────────────────────────────────── */")},
	}

	chunks, err := finalizeChunks(specs, linesTotal)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := writeChunks(path, src, bodyStart, lineStarts, chunks); err != nil {
		return err
	}
	if err := verifyCoverage(src, bodyStart, lineStarts, chunks); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func splitDrumView(path string) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	importEnd, err := findImportEnd(src)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
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
			fatal(fmt.Errorf("%s: missing func %q", path, name))
		}
		return ln
	}
	mustType := func(name string) int {
		ln, ok := typeLines[name]
		if !ok {
			fatal(fmt.Errorf("%s: missing type %q", path, name))
		}
		return ln
	}
	mustMarker := func(markerLine string) int {
		off, err := findUniqueLineStart(src, markerLine)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", path, err))
		}
		return lineForOffset(lineStarts, off)
	}

	specs := []chunkSpec{
		{outFile: filepath.Base(path), startLine: bodyStartLine},
		{outFile: "drumview_notifications.go", startLine: mustType("notification")},
		{outFile: "drumview_geometry.go", startLine: mustMarker("/* ─── geometry helpers ─────────────────────────────────────── */")},
		{outFile: "drumview_beat_length.go", startLine: mustFunc("SetBeatLength")},
		{outFile: "drumview_ctor.go", startLine: mustMarker("/* ─── ctor ─────────────────────────────────────────────────── */")},
		{outFile: "drumview_components.go", startLine: mustFunc("SetComponentRegistry")},
		{outFile: "drumview_rows.go", startLine: mustFunc("SetBounds")},
		{outFile: "drumview_layout.go", startLine: mustMarker("/* ─── public update ────────────────────────────────────────── */")},
		{outFile: "drumview_instrument_menu.go", startLine: mustFunc("instMenuHasScroll")},
		{outFile: "drumview_color_menu.go", startLine: mustFunc("buildColorMenu")},
		{outFile: "drumview_subdiv_menu.go", startLine: mustFunc("buildSubdivMenu")},
		{outFile: "drumview_instruments.go", startLine: mustFunc("refreshInstruments")},
		{outFile: "drumview_transport.go", startLine: mustFunc("PlayPressed")},
		{outFile: "drumview_toolbar.go", startLine: mustFunc("decayAnims")},
		{outFile: "drumview_update.go", startLine: mustFunc("Update")},
		{outFile: "drumview_draw.go", startLine: mustFunc("Draw")},
		{outFile: "drumview_draw_controls_eq.go", startLine: mustFunc("drawRowControls")},
		{outFile: "drumview_draw_waveform.go", startLine: mustFunc("drawWaveform")},
		{outFile: "drumview_draw_layout_guides.go", startLine: mustFunc("drawLayoutGuides")},
		{outFile: "drumview_audio_eq.go", startLine: mustFunc("analyzerSnapshot")},
		{outFile: "drumview_row_control_overlay.go", startLine: mustFunc("renderRowControlOverlay")},
		{outFile: "drumview_highlight_sprites.go", startLine: mustFunc("ensureHighlightSprites")},
		{outFile: "drumview_timeline_info.go", startLine: mustFunc("timelineInfo")},
		{outFile: "drumview_colors.go", startLine: mustFunc("pickColorFromWheel")},
		{outFile: "drumview_cache_background.go", startLine: mustFunc("bg")},
		{outFile: "drumview_cache_dirty.go", startLine: mustFunc("setTimelineSegments")},
		{outFile: "drumview_cache_row_sprite.go", startLine: mustFunc("buildRowSprite")},
		{outFile: "drumview_cache_rows_layer.go", startLine: mustFunc("rowsLayerMaybeRebuild")},
		{outFile: "drumview_cache_rows_stripes.go", startLine: mustFunc("rowsStripesMaybeRebuild")},
	}

	chunks, err := finalizeChunks(specs, linesTotal)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := writeChunks(path, src, bodyStart, lineStarts, chunks); err != nil {
		return err
	}
	if err := verifyCoverage(src, bodyStart, lineStarts, chunks); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
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

func writeChunks(originalPath string, src []byte, bodyStart int, lineStarts []int, chunks []chunk) error {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)

	// Collect the exact output set (excluding the base file) so reruns are clean.
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
		if c.startLine <= 0 || c.endLine <= 0 || c.startLine > c.endLine {
			return fmt.Errorf("%s: invalid chunk %s lines %d..%d", originalPath, c.outFile, c.startLine, c.endLine)
		}
		startOff := offsetForLine(lineStarts, c.startLine)
		endOff := offsetForLineEnd(src, lineStarts, c.endLine)
		if startOff < bodyStart {
			startOff = bodyStart
		}
		if endOff < startOff {
			return fmt.Errorf("%s: invalid offsets for chunk %s", originalPath, c.outFile)
		}
		chunkBody := src[startOff:endOff]

		usedNames, err := usedImportNames(imports, chunkBody)
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
		buf.WriteString("package ui\n\n")
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

func usedImportNames(allImports []importSpec, chunkBody []byte) (map[string]bool, error) {
	var buf bytes.Buffer
	buf.WriteString("package ui\n")
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

func findImportEnd(src []byte) (int, error) {
	idx := bytes.Index(src, []byte("import ("))
	if idx < 0 {
		return 0, fmt.Errorf("no import block found")
	}
	close := bytes.Index(src[idx:], []byte("\n)\n"))
	if close < 0 {
		return 0, fmt.Errorf("import block missing closing ')'")
	}
	return idx + close + len("\n)\n"), nil
}

func findUniqueLineStart(src []byte, exactLine string) (int, error) {
	needle := []byte(exactLine)
	first := bytes.Index(src, needle)
	if first < 0 {
		return 0, fmt.Errorf("missing marker line %q", exactLine)
	}
	// Ensure uniqueness.
	if next := bytes.Index(src[first+len(needle):], needle); next >= 0 {
		return 0, fmt.Errorf("marker line %q is not unique", exactLine)
	}
	// Ensure it's at the start of a line.
	if first != 0 && src[first-1] != '\n' {
		return 0, fmt.Errorf("marker line %q not found at line start", exactLine)
	}
	return first, nil
}

