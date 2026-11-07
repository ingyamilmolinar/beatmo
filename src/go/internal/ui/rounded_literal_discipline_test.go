//go:build test

package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Tokens and computed expressions (RadiusMD/2, hr) do not match.
var bareRadiusRE = regexp.MustCompile(`drawRoundedRect\(.*, [0-9]+, (?:true|false)\)`)

// Every radius comes from the DESIGN.md `rounded:` scale, so rounding stays retunable from one place.
func TestNoBareRadiusLiterals(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, ".gen.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			if bareRadiusRE.MatchString(line) {
				offenders = append(offenders, fmt.Sprintf("%s:%d: %s", name, i+1, strings.TrimSpace(line)))
			}
		}
	}
	if len(offenders) > 0 {
		t.Errorf("%d bare integer radius literal(s) in drawRoundedRect — replace with a Radius* token:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}
