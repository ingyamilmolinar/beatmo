//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every native rect must flow through the tree-owned, z-gated syncNativeGestures, so the file-picker leak
// cannot return.
func TestNativeGesture_NoDirectRegistrarCallsOutsideSeam(t *testing.T) {
	banned := regexp.MustCompile(`\b(filePickerRegisterRect|softKeyboardRegisterRect|mobileInputRegister|mobileInputRegisterTrigger|filePickerClearRects|softKeyboardClearRects|mobileInputClear)\s*\(`)
	seam := map[string]bool{
		"native_gesture.go": true, "native_gesture_wasm.go": true, "native_gesture_stub.go": true,
		// the low-level bindings themselves DEFINE these funcs:
		"filepicker_wasm.go": true, "filepicker_stub.go": true,
		"softkeyboard_wasm.go": true, "softkeyboard_stub.go": true,
		"mobile_input_wasm.go": true, "mobile_input_stub.go": true,
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

// A second caller could sync a stale owner set taken before Layout/dispatch finished. native_gesture.go
// defines the method and is skipped.
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
