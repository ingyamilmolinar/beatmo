package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestDesignMDContrast computes WCAG AA contrast ratios for every
// (textColor, backgroundColor) pair in DESIGN.md's components block and
// asserts each is ≥ 4.5:1 unless explicitly exempted.
//
// This duplicates the @google/design.md Stitch lint's contrast check on
// the Go side so the rule applies hermetically — no network, no npx.
// design_md_lint_test.go pins the npx output; this test pins the same
// invariant via a local computation. The exception list is shared with
// the lint snapshot at testdata/design_md_lint.expected.json so a
// component cannot drift between the two guards.
//
// WCAG 2.1 luminance:
//
//	c_lin = c/12.92                 if c ≤ 0.03928
//	c_lin = ((c + 0.055)/1.055)^2.4 otherwise
//	L     = 0.2126·R_lin + 0.7152·G_lin + 0.0722·B_lin
//	ratio = (L_max + 0.05) / (L_min + 0.05)
const wcagAAThreshold = 4.5

func TestDesignMDContrast(t *testing.T) {
	path := findDesignMD(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	front := frontMatter(string(body))
	if front == "" {
		t.Fatalf("DESIGN.md has no YAML front matter")
	}

	colors := extractScalarBlock(front, "colors")
	components := parseComponentsBlock(front)
	exceptions := loadContrastExceptions(t)

	type pair struct {
		component  string
		bg, text   string
		bgHex      string
		textHex    string
		ratio      float64
	}
	var checked []pair
	var below []pair
	var unexpectedExempt []string

	componentNames := make([]string, 0, len(components))
	for name := range components {
		componentNames = append(componentNames, name)
	}
	sort.Strings(componentNames)

	for _, name := range componentNames {
		comp := components[name]
		bgRef, hasBG := comp["backgroundColor"]
		textRef, hasText := comp["textColor"]
		if !hasBG || !hasText {
			continue
		}
		bgHex, ok := resolveColorRef(bgRef, colors)
		if !ok {
			t.Errorf("components.%s.backgroundColor = %q: cannot resolve color reference", name, bgRef)
			continue
		}
		textHex, ok := resolveColorRef(textRef, colors)
		if !ok {
			t.Errorf("components.%s.textColor = %q: cannot resolve color reference", name, textRef)
			continue
		}

		bgRGB, err := parseHex(bgHex)
		if err != nil {
			t.Errorf("components.%s background %s: %v", name, bgHex, err)
			continue
		}
		textRGB, err := parseHex(textHex)
		if err != nil {
			t.Errorf("components.%s text %s: %v", name, textHex, err)
			continue
		}

		ratio := contrastRatio(textRGB.R, textRGB.G, textRGB.B, bgRGB.R, bgRGB.G, bgRGB.B)
		p := pair{component: name, bg: bgRef, text: textRef, bgHex: bgHex, textHex: textHex, ratio: ratio}
		checked = append(checked, p)

		exemptKey := "components." + name
		if ratio < wcagAAThreshold {
			if !exceptions[exemptKey] {
				below = append(below, p)
			}
		} else {
			if exceptions[exemptKey] {
				unexpectedExempt = append(unexpectedExempt, name)
			}
		}
	}

	if len(below) > 0 {
		var msgs []string
		for _, p := range below {
			msgs = append(msgs, fmt.Sprintf("  components.%s: text=%s on bg=%s = %.2f:1 (need ≥%.1f:1)",
				p.component, p.textHex, p.bgHex, p.ratio, wcagAAThreshold))
		}
		t.Errorf("WCAG AA contrast violations not on the exception list:\n%s\n"+
			"If intentional, add the entry to testdata/design_md_lint.expected.json with a justification.",
			strings.Join(msgs, "\n"))
	}
	if len(unexpectedExempt) > 0 {
		sort.Strings(unexpectedExempt)
		t.Errorf("component(s) listed as contrast exceptions but currently meet AA — remove from testdata/design_md_lint.expected.json:\n  %s",
			strings.Join(unexpectedExempt, "\n  "))
	}
	if len(checked) == 0 {
		t.Fatalf("no (background, text) component pairs found — parser regression?")
	}
}

// loadContrastExceptions reads the lint snapshot and returns the set of
// component paths exempted from the AA threshold. The snapshot is shared
// with design_md_lint_test.go; the warnings list there is the exception
// list here.
func loadContrastExceptions(t *testing.T) map[string]bool {
	t.Helper()
	uiDir := findUIDir(t)
	expectedPath := filepath.Join(uiDir, "testdata", "design_md_lint.expected.json")
	body, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read snapshot %s: %v", expectedPath, err)
	}
	var doc struct {
		Warnings []struct {
			Path     string `json:"path"`
			Severity string `json:"severity"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse snapshot: %v", err)
	}
	out := map[string]bool{}
	for _, w := range doc.Warnings {
		out[w.Path] = true
	}
	return out
}

// parseComponentsBlock walks the YAML front matter and returns
// `components.<name> -> { backgroundColor: <ref>, textColor: <ref>, ... }`.
// The grammar is restricted to two-space indent for component names and
// four-space indent for key:value scalars, matching DESIGN.md's actual
// shape. Any deviation is silently ignored — `parseComponentsBlock` is
// not a general YAML parser.
func parseComponentsBlock(front string) map[string]map[string]string {
	out := map[string]map[string]string{}
	lines := strings.Split(front, "\n")
	in := false
	currentName := ""
	componentNameRE := regexp.MustCompile(`^  ([a-z][a-zA-Z0-9-]*):\s*$`)
	scalarRE := regexp.MustCompile(`^    ([a-zA-Z][a-zA-Z0-9-]*):\s*"?([^"\n]*?)"?\s*$`)
	for _, ln := range lines {
		if ln == "components:" {
			in = true
			continue
		}
		if !in {
			continue
		}
		if ln == "" {
			continue
		}
		// End of components: another top-level key.
		if len(ln) > 0 && ln[0] != ' ' && ln[0] != '#' {
			break
		}
		// Skip comments inside the block.
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := componentNameRE.FindStringSubmatch(ln); m != nil {
			currentName = m[1]
			out[currentName] = map[string]string{}
			continue
		}
		if currentName == "" {
			continue
		}
		if m := scalarRE.FindStringSubmatch(ln); m != nil {
			out[currentName][m[1]] = m[2]
		}
	}
	return out
}

// resolveColorRef resolves a `{colors.NAME}` reference to a #RRGGBB hex
// string by looking up `colors[NAME]`. A literal `#RRGGBB` value is
// passed through.
func resolveColorRef(ref string, colors map[string]string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, "#") {
		return ref, true
	}
	if strings.HasPrefix(ref, "{colors.") && strings.HasSuffix(ref, "}") {
		key := strings.TrimSuffix(strings.TrimPrefix(ref, "{colors."), "}")
		hex, ok := colors[key]
		return hex, ok
	}
	return "", false
}

func contrastRatio(r1, g1, b1, r2, g2, b2 uint8) float64 {
	L1 := relativeLuminance(r1, g1, b1)
	L2 := relativeLuminance(r2, g2, b2)
	if L1 < L2 {
		L1, L2 = L2, L1
	}
	return (L1 + 0.05) / (L2 + 0.05)
}

func relativeLuminance(r, g, b uint8) float64 {
	return 0.2126*srgbLin(r) + 0.7152*srgbLin(g) + 0.0722*srgbLin(b)
}

func srgbLin(c uint8) float64 {
	v := float64(c) / 255.0
	if v <= 0.03928 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
