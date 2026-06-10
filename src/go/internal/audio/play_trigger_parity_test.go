package audio

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestPlayVariantsRecordTriggerOnEveryPlatform asserts that every platform's
// Play* entry point feeds the per-instrument trigger clock — RecordVoiceTrigger
// — either directly or by delegating to a sibling Play* that does.
//
// The UI's time-since-trigger reads (the Synth-tab pulse glow and the Sampler
// preview playhead line) all derive from audio.SinceLastTrigger, which is only
// advanced by RecordVoiceTrigger. If a single platform's Play forgets the call,
// those signals silently never fire on that platform.
//
// This guards the exact regression that made the Sampler playhead invisible in
// the browser: engine_wasm.go's Play/PlayVol/PlayParams forwarded straight to
// the JS WebAudio bridge without stamping the trigger, so SinceLastTrigger stayed
// at its "never triggered" sentinel forever on WASM while desktop/stub worked.
func TestPlayVariantsRecordTriggerOnEveryPlatform(t *testing.T) {
	// Every file that defines an audio entry point, per platform. Batch
	// playback (the sequencer's hot path) lives in batch_*.go and must stamp
	// triggers too, else sequencer-driven hits never advance SinceLastTrigger.
	files := []string{
		"engine_play.go", "engine_wasm.go", "stub.go",
		"batch_desktop.go", "batch_wasm.go", "batch_stub.go",
	}
	// The entry points users hear: each must lead to a trigger record.
	entry := map[string]bool{
		"Play": true, "PlayVol": true, "PlayParams": true, "PlayParamsAt": true,
		"PlayBatch": true,
	}
	// A body satisfies the invariant if it calls RecordVoiceTrigger directly, or
	// delegates to another Play* entry point (which is itself checked).
	satisfies := map[string]bool{
		"RecordVoiceTrigger": true,
		"Play":               true, "PlayVol": true, "PlayParams": true, "PlayParamsAt": true,
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

// bodyCallsAny reports whether body contains a call to any plain-identifier
// function whose name is in names.
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
