//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// No production file outside the platform seam (native_gesture*.go) may call the
// low-level JS gesture registrars directly. This keeps every native rect flowing
// through the tree-owned, z-gated syncNativeGestures — so the file-picker leak
// class cannot be reintroduced.
func TestNativeGesture_NoDirectRegistrarCallsOutsideSeam(t *testing.T) {
	banned := regexp.MustCompile(`\b(filePickerRegisterRect|softKeyboardRegisterRect|mobileInputRegister|mobileInputRegisterTrigger|filePickerClearRects|softKeyboardClearRects|mobileInputClear)\s*\(`)
	seam := map[string]bool{
		"native_gesture.go": true, "native_gesture_wasm.go": true, "native_gesture_stub.go": true,
		// the low-level bindings themselves DEFINE these funcs:
		"filepicker_wasm.go": true, "filepicker_stub.go": true,
		"softkeyboard_wasm.go": true, "softkeyboard_stub.go": true,
		"mobile_input_wasm.go": true, "mobile_input_stub.go": true,
		// User-approved pragmatic exception (see native_gesture.go's
		// activeParamEditorNativeRects doc comment): the shared numeric
		// ParamValueEditor (synth-param / sampler-param / eq-db-N) is not a
		// portal, so it has no ownerID for the tree's TopmostOwnerAt gate to
		// key on — there is no "owner" to route through the seam's producer
		// pattern. It keeps its one-shot open-time mobileInputRegister call
		// (OpenValue, ~line 148) to arm the native <input> the instant the
		// editor opens. Safety net: activeParamEditorNativeRects() in
		// native_gesture.go re-adds the active editor's rect every frame so
		// the seam's clear-and-re-arm cycle in syncNativeGestures does not
		// wipe it. See TestParamEditorNativeRectArmedBySync (native_gesture_test.go)
		// and TestParamEditorOpenValueArmsMobileInput (this task) for the two
		// halves of that contract.
		"param_value_editor.go": true,
	}
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		base := filepath.Base(f)
		if seam[base] || strings.HasSuffix(base, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if loc := banned.FindIndex(b); loc != nil {
			t.Errorf("%s calls a low-level native-gesture registrar directly (offset %d) — "+
				"route it through a producer in native_gesture.go instead", base, loc[0])
		}
	}
}

// syncNativeGestures must have exactly ONE production call site
// (drumview_update.go). A second caller could sync a stale owner set (e.g. a
// snapshot taken before the tree's Layout/dispatch phase finished) and
// reintroduce the file-picker-leak class this feature closed. native_gesture.go
// is skipped entirely — it DEFINES the method, so it can't legitimately be
// counted as a "caller" of itself.
func TestSyncNativeGestures_SingleCallSite(t *testing.T) {
	callRe := regexp.MustCompile(`\.syncNativeGestures\s*\(`)
	files, _ := filepath.Glob("*.go")
	var callers []string
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasSuffix(base, "_test.go") || base == "native_gesture.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if callRe.Match(b) {
			callers = append(callers, base)
		}
	}
	if len(callers) != 1 || callers[0] != "drumview_update.go" {
		t.Fatalf("syncNativeGestures must have exactly one call site (drumview_update.go), got %v — "+
			"a second caller can sync a stale owner set and reintroduce the file-picker leak class",
			callers)
	}
}
