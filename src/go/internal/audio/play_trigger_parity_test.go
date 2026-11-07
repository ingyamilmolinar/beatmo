package audio

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The Synth-tab glow and the Sampler playhead read SinceLastTrigger, which only RecordVoiceTrigger advances: a
// platform whose Play forgets it shows neither, as the browser once did.
func TestPlayVariantsRecordTriggerOnEveryPlatform(t *testing.T) {
	// Batch playback (the sequencer's hot path) must stamp triggers too.
	files := []string{
		"engine_play.go", "engine_wasm.go", "stub.go",
		"batch_desktop.go", "batch_wasm.go", "batch_stub.go",
	}
	// The entry points users hear: each must lead to a trigger record.
	entry := map[string]bool{
		"Play": true, "PlayVol": true, "PlayParams": true, "PlayParamsAt": true,
		"PlayBatch": true,
	}
	// A call to RecordVoiceTrigger, to another checked Play* entry point, or to playParamsIdentity, which calls it.
	satisfies := map[string]bool{
		"RecordVoiceTrigger": true,
		"Play":               true, "PlayVol": true, "PlayParams": true, "PlayParamsAt": true,
		"playParamsIdentity": true,
	}

	for _, f := range files {
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, decl := range af.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Body == nil || !entry[fn.Name.Name] {
				continue
			}
			if !bodyCallsAny(fn.Body, satisfies) {
				t.Errorf("%s: %s() never records a voice trigger — call "+
					"RecordVoiceTrigger(id) or delegate to a Play* that does; "+
					"otherwise audio.SinceLastTrigger never advances on this platform "+
					"and trigger-driven UI (synth glow, sampler playhead) stays invisible",
					f, fn.Name.Name)
			}
		}
	}
}

func bodyCallsAny(body *ast.BlockStmt, names map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && names[id.Name] {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
