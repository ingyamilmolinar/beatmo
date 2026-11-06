//go:build test

// Structural guard for the platform input-dynamics boundary.
//
// The runaway-browser-wheel bug was not a typo — it was an ARCHITECTURAL gap.
// The normalization helpers existed in wheel_js.go / wheel_nonjs.go, but
// nothing stopped a call site from reading the raw wheel instead, and three of
// them did: DrumViewTree's dispatch (menus scrolled ~100 rows per browser
// notch), Camera zoom (~50x too fast in a browser), and TimelineZone's vertical
// axis. Each was individually reasonable and collectively wrong.
//
// This test makes the boundary structural rather than a code-review habit:
// raw platform wheel units may be read ONLY inside the two build-tagged shims,
// which immediately convert them to detents. Everything else consumes detents.
//
// Mirrors the shape of zone_input_isolation_discipline_test.go.

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// wheelRawReaders are the only files permitted to call the raw wheel().
// Both immediately convert to detents via wheelNotchesFromRaw.
var wheelRawReaders = map[string]string{
	"input.go":       "declares the wheel func var itself",
	"wheel_js.go":    "browser half of the platform boundary",
	"wheel_nonjs.go": "native half of the platform boundary",
}

// rawWheelCall matches a call to wheel() that is not part of a longer
// identifier (wheelNotches(), wheelScrollSteps(), ...).
var rawWheelCall = regexp.MustCompile(`(^|[^\w.])wheel\(\)`)

func TestNoRawWheelReadsOutsidePlatformShims(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if _, ok := wheelRawReaders[name]; ok {
			continue
		}
		src, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			code := line
			// Ignore comments — several files legitimately DISCUSS wheel().
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}
			if rawWheelCall.MatchString(code) {
				t.Errorf("%s:%d reads the raw platform wheel:\n\t%s\n\n"+
					"Raw wheel units are NOT comparable across runtimes: native GLFW "+
					"reports +/-1 per detent, a browser reports the DOM's raw deltaY "+
					"(~100/notch in Chrome, ~3 in Firefox). Call wheelNotches() for "+
					"detents, wheelScrollSteps()/wheelScrollStepsX() for row steps, or "+
					"wheelZoomDelta() for a continuous magnitude. Only %v may read raw.",
					name, i+1, strings.TrimSpace(line), wheelRawReaderNames())
			}
		}
	}
}

// TestPlatformShimsAgreeOnTheDetentContract pins that both halves of the
// boundary expose the same function, so neither can drift out of the contract
// while the other build keeps compiling.
func TestPlatformShimsAgreeOnTheDetentContract(t *testing.T) {
	for _, f := range []string{"wheel_js.go", "wheel_nonjs.go"} {
		src, err := os.ReadFile(filepath.Clean(f))
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if !strings.Contains(string(src), "func wheelNotches() (x, y float64)") {
			t.Errorf("%s must declare `func wheelNotches() (x, y float64)` — it is the "+
				"single platform boundary every runtime has to implement", f)
		}
		if !strings.Contains(string(src), "wheelNotchesFromRaw") {
			t.Errorf("%s must convert through wheelNotchesFromRaw so the unit "+
				"conversion stays in one unit-tested place", f)
		}
	}
}

func wheelRawReaderNames() []string {
	out := make([]string, 0, len(wheelRawReaders))
	for k := range wheelRawReaders {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
