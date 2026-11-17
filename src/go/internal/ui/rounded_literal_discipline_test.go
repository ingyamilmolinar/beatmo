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

// bareRadiusRE matches a drawRoundedRect(...) call whose radius argument (the
// int just before the trailing fill/stroke bool) is a BARE INTEGER LITERAL.
// Token reads (RadiusXXS/XS/SM/MD/...) and computed expressions (RadiusMD/2,
// hr, pillRadius) do not match.
var bareRadiusRE = regexp.MustCompile(`drawRoundedRect\(.*, [0-9]+, (?:true|false)\)`)

// TestNoBareRadiusLiterals enforces that every corner radius drawn in
// internal/ui comes from the DESIGN.md `rounded:` scale (a Radius* token),
// never a hand-typed pixel literal. This is what keeps corner rounding
// re-styleable from one place — a new literal here drifts off the scale and
// can't be retuned centrally. Use RadiusXXS(4)/XS(6)/SM(8)/MD(12)/LG/XL.
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
