package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// TestAuditFindsWhatValueOfWouldPanicOn runs the audit on a fixture whose
// lines that must be reported carry "// finding:" — no more, no fewer.
func TestAuditFindsWhatValueOfWouldPanicOn(t *testing.T) {
	src, err := os.ReadFile("testdata/ex/ex.go")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for i, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "// finding:") {
			want[fmt.Sprintf("ex.go:%d:", i+1)] = true
		}
	}
	got, err := audit("testdata/ex", "./...")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, f := range got {
		key := ""
		if i := strings.Index(f, "ex.go:"); i >= 0 {
			rest := f[i+len("ex.go:"):]
			key = "ex.go:" + rest[:strings.Index(rest, ":")+1]
		}
		if !want[key] {
			t.Errorf("false positive: %s", f)
		}
		seen[key] = true
	}
	for l := range want {
		if !seen[l] {
			t.Errorf("missed the value at %s", l)
		}
	}
}
