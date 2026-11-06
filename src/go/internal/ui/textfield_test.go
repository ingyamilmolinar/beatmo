//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TextField contract: a text box built with NewTextField gets its native
// keyboard wiring (desktop soft-keyboard proxy, mobile native <input>) from the
// fact that it is LIVE — updated by its owner this frame or the last, or its
// native input is open. No surface hand-lists its box in native_gesture.go any
// more, so a new text box cannot be forgotten (the Save As dialogs and the FX
// "add effect" search had no mobile keyboard for exactly that reason).

func newTextFieldTestGame(t *testing.T, mobile bool) *Game {
	t.Helper()
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restore)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	if mobile {
		g.SetForceMobileProfile(true)
		g.Layout(390, 844)
	} else {
		g.Layout(1280, 720)
	}
	return g
}

func candidateIDs(cands []NativeRectCandidate, ch NativeChannel) map[string]int {
	out := map[string]int{}
	for _, c := range cands {
		if c.Intent.Channel == ch {
			out[c.Intent.ID]++
		}
	}
	return out
}

func TestTextFieldIsACandidateOnlyWhileLive(t *testing.T) {
	g := newTextFieldTestGame(t, true)
	dv := g.drum
	f := NewTextField(dv.textFields, TextFieldSpec{ID: "probe-field", Owner: "probe"}, 40)
	f.Rect = image.Rect(10, 10, 200, 40)

	dv.textFields.advance()
	if candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-field"] != 0 {
		t.Fatalf("a field never updated must not be a candidate")
	}
	f.Update()
	if n := candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-field"]; n != 1 {
		t.Fatalf("an updated field must be exactly one mobile candidate, got %d", n)
	}
	dv.textFields.advance()
	if candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-field"] != 1 {
		t.Fatalf("a field updated last frame stays a candidate (Update/Draw cadence slack)")
	}
	dv.textFields.advance()
	dv.textFields.advance()
	if candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-field"] != 0 {
		t.Fatalf("a field not updated for several frames must drop out")
	}
	if candidateIDs(dv.softKeyboardCandidates(), NativeSoftKeyboard)["probe-field"] != 0 {
		t.Fatalf("mobile must not arm the desktop soft-keyboard channel")
	}
}

func TestTextFieldStaysACandidateWhileItsNativeInputIsOpen(t *testing.T) {
	g := newTextFieldTestGame(t, true)
	dv := g.drum
	f := NewTextField(dv.textFields, TextFieldSpec{ID: "probe-open", Owner: "probe"}, 40)
	f.Rect = image.Rect(10, 10, 200, 40)
	f.Update()
	prevActive := testMobileInputActive
	testMobileInputActive = map[string]bool{"probe-open": true}
	t.Cleanup(func() { testMobileInputActive = prevActive })
	for i := 0; i < 5; i++ {
		dv.textFields.advance() // owner stops calling Update while the native input is up
	}
	if candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-open"] != 1 {
		t.Fatalf("an open native input must keep its rect armed or the sync tears it down mid-typing")
	}
}

func TestTextFieldDesktopUsesTheSoftKeyboardProxy(t *testing.T) {
	g := newTextFieldTestGame(t, false)
	dv := g.drum
	f := NewTextField(dv.textFields, TextFieldSpec{ID: "probe-desk", Owner: "probe"}, 40)
	f.Rect = image.Rect(10, 10, 200, 40)
	f.Update()
	if candidateIDs(dv.softKeyboardCandidates(), NativeSoftKeyboard)["probe-desk"] != 1 {
		t.Fatalf("desktop browser must arm the soft-keyboard proxy for a live field")
	}
	if candidateIDs(dv.nativeInputCandidates(), NativeTextInput)["probe-desk"] != 0 {
		t.Fatalf("desktop must not arm a mobile native input")
	}
}

func TestTextFieldTriggerAndInputRects(t *testing.T) {
	g := newTextFieldTestGame(t, true)
	dv := g.drum
	trig := image.Rect(300, 300, 340, 340)
	in := image.Rect(5, 100, 200, 130)
	f := NewTextField(dv.textFields, TextFieldSpec{ID: "probe-trig", Owner: "probe",
		TriggerRect: func() image.Rectangle { return trig },
		InputRect:   func() image.Rectangle { return in }}, 32)
	f.Rect = image.Rect(0, 0, 10, 10)
	f.Update()
	for _, c := range dv.nativeInputCandidates() {
		if c.Intent.ID == "probe-trig" {
			if c.Rect != trig || c.Intent.InputRect != in {
				t.Fatalf("trigger/input rects: got %v / %v", c.Rect, c.Intent.InputRect)
			}
			return
		}
	}
	t.Fatal("probe-trig candidate missing")
}

// The two dialogs that had NO keyboard wiring at all before TextField: Save As
// (synth preset) and the FX "add effect" search. Both must now arm a native
// input on mobile and the proxy on the desktop browser, via their real open
// paths.
func TestSaveAsAndFXSearchHaveNativeCandidates(t *testing.T) {
	for _, mobile := range []bool{true, false} {
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name+"/save-as", func(t *testing.T) {
			g := newTextFieldTestGame(t, mobile)
			dv := g.drum
			dv.eqPanelZone.SetActiveTab(TabSynth)
			if mobile {
				dv.setViewMode(viewModeSynth) // the audio panel is only on screen in its view
			}
			dv.openSaveAsDialog()
			if dv.saveAsDialog == nil {
				t.Skip("no Save As dialog for the active instrument")
			}
			for i := 0; i < 3; i++ {
				g.Update()
			}
			assertLiveFieldCandidate(t, dv, mobile, saveAsMobileInputID)
		})
		t.Run(name+"/fx-search", func(t *testing.T) {
			g := newTextFieldTestGame(t, mobile)
			dv := g.drum
			dv.openFXPanel(0)
			dv.fxAddMenuOpen = true
			dv.buildFXPanel()
			for i := 0; i < 3; i++ {
				g.Update()
			}
			if dv.fxAddSearch == nil {
				t.Fatal("fx add-effect search box not built")
			}
			assertLiveFieldCandidate(t, dv, mobile, fxSearchMobileInputID)
		})
	}
}

// assertLiveFieldCandidate checks the ARMED rects (after syncNativeGestures'
// topmost-owner filter), so a wrong Owner tag fails here too.
func assertLiveFieldCandidate(t *testing.T, dv *DrumView, mobile bool, id string) {
	t.Helper()
	want := NativeSoftKeyboard
	if mobile {
		want = NativeTextInput
	}
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == want && r.Intent.ID == id {
			return
		}
	}
	t.Fatalf("mobile=%v: %q not armed (lastNativeRects=%+v)", mobile, id, dv.lastNativeRects)
}

// Every production text box goes through NewTextField: a bare NewTextInput
// would have no native spec and silently no mobile keyboard.
func TestNoNewTextInputOutsideTextField(t *testing.T) {
	allowed := map[string]bool{"textfield.go": true, "textinput.go": true}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || allowed[f] {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			continue // build-tagged-out stubs etc.
		}
		ast.Inspect(file, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "NewTextInput" {
					t.Errorf("%s: NewTextInput outside textfield.go — use NewTextField so the box gets its native keyboard wiring", fset.Position(call.Pos()))
				}
			}
			return true
		})
	}
}

// Native ids must be unique: two live fields sharing an id would fight over one
// native <input>.
func TestTextFieldIDsAreUnique(t *testing.T) {
	ids := map[string]string{}
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, nm := range vs.Names {
				if !strings.HasSuffix(nm.Name, "MobileInputID") || i >= len(vs.Values) {
					continue
				}
				if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
					if prev, dup := ids[lit.Value]; dup {
						t.Errorf("native id %s declared twice: %s and %s", lit.Value, prev, nm.Name)
					}
					ids[lit.Value] = nm.Name
				}
			}
			return true
		})
	}
	if len(ids) < 6 {
		t.Errorf("expected the text-field ids as *MobileInputID constants, found %d: %v", len(ids), ids)
	}
}
