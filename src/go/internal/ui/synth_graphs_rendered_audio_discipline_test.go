package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The Synth tab draws ONLY rendered audio (spec 2026-10-24 §4.8, reinstated by
// the user 2026-10-25: "remove the old graphs"). The old graphs modelled the
// voice in pure Go (audio.RenderInstrumentPreview — it ignored every drum
// knob), drew concept schematics per param group, swept a playhead whose
// position was a guess, and thumbnailed every card and pipeline stage. These
// guards keep them from coming back.

// oldGraphIdents are the names of the deleted model and its renderers. Any
// production identifier spelled like one is the old graph returning.
var oldGraphIdents = map[string]string{
	"RenderInstrumentPreview":              "the pure-Go voice model",
	"RenderInstrumentPreviewWithOverrides": "the pure-Go voice model",
	"RenderInstrumentPreviewWave":          "the pure-Go voice model",
	"PreviewOscShape":                      "the pure-Go voice model",
	"PreviewOscShapeModel":                 "the pure-Go voice model",
	"OscModelParams":                       "the pure-Go voice model",
	"OscModelParamsFor":                    "the pure-Go voice model",
	"drawSynthPlayhead":                    "the play-through line",
	"playFrac":                             "the play-through line",
	"synthGraphTimeWindowSec":              "the play-through line",
	"synthCurveHook":                       "the morphing concept curve",
	"viewLegacy":                           "the Legacy view",
	"macroGraphKind":                       "the card signature graphs",
	"macroGraphRenderer":                   "the card signature graphs",
	"synthStageThumbs":                     "the pipeline stage thumbnails",
	"synthMirror":                          "the 'Your sound' mirror",
	"drawSynthFullNote":                    "the 'Your sound' full note",
	"drawSynthFocusGraph":                  "the concept focus graph",
}

// oldGraphIdentPrefixes catch a whole family: the concept renderers
// (conceptOsc, conceptDrawFilledCurve, …). The prefix must be followed by an
// upper-case letter: a bare `concept` is the macro cards' word for a category.
var oldGraphIdentPrefixes = map[string]string{
	"concept": "a concept renderer",
}

func hasIdentPrefix(name, prefix string) bool {
	return len(name) > len(prefix) && strings.HasPrefix(name, prefix) &&
		name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z'
}

func TestOldSynthGraphsAreGone(t *testing.T) {
	root := filepath.Join("..", "..") // src/go
	var bad []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); path != root && (n == "testdata" || n == "vendor" || strings.HasPrefix(n, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			why, hit := oldGraphIdents[id.Name]
			if !hit {
				for p, w := range oldGraphIdentPrefixes {
					if hasIdentPrefix(id.Name, p) {
						why, hit = w, true
					}
				}
			}
			if hit {
				bad = append(bad, fset.Position(id.Pos()).String()+": "+id.Name+" ("+why+")")
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Fatalf("%d references to the deleted Synth-tab graphs (they modelled the voice instead of measuring it):\n%s",
			len(bad), strings.Join(bad, "\n"))
	}
}

// graphAudioAllowed is every audio.* name a Synth graph file may use: the one
// render entry point and the document / param reads that name what to render.
// Anything else from package audio is a second source of sound.
var graphAudioAllowed = map[string]bool{
	"RenderDocForGraph":        true, // the only PCM source
	"GraphRenderKey":           true, // its cache key
	"GraphVoiceSec":            true, // the voice length it renders at
	"SampleEdit":               true, // part of the document rendered
	"SampleEditFor":            true,
	"EnsureInstrumentDoc":      true, // the document to render
	"InstrumentDocWithFields":  true, // a control's end, as a document
	"MergedInstrumentParamsRO": true, // the values a dial reads
	"ParamDef":                 true, // knob metadata (which view)
	"RecipeParams":             true,
	"AnyFamilyGroup":           true,
}

func TestSynthGraphsDrawOnlyRenderedAudio(t *testing.T) {
	files, err := filepath.Glob("synth_*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	var bad []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		if !strings.HasPrefix(path, "synth_graph_") && !strings.HasPrefix(path, "synth_view") {
			continue
		}
		checked++
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if x, ok := sel.X.(*ast.Ident); ok && x.Name == "audio" && !graphAudioAllowed[sel.Sel.Name] {
				bad = append(bad, fset.Position(sel.Pos()).String()+": audio."+sel.Sel.Name)
			}
			return true
		})
	}
	if checked < 5 {
		t.Fatalf("only %d graph files matched synth_graph_*/synth_view*: the guard is looking in the wrong place", checked)
	}
	if len(bad) > 0 {
		t.Fatalf("a Synth graph file reads sound from somewhere other than audio.RenderDocForGraph:\n%s", strings.Join(bad, "\n"))
	}
}
