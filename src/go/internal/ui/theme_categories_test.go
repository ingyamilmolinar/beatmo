//go:build test

package ui

import (
	"os"
	"regexp"
	"testing"
)

func designMDColorTokenNames(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile("../../../../DESIGN.md")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	start := regexp.MustCompile(`(?m)^colors:\s*$`).FindStringIndex(src)
	if start == nil {
		t.Fatal("colors: block not found")
	}
	rest := src[start[1]:]
	end := regexp.MustCompile(`(?m)^[a-zA-Z]`).FindStringIndex(rest)
	if end != nil {
		rest = rest[:end[0]]
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):`).FindAllStringSubmatch(rest, -1) {
		names = append(names, m[1])
	}
	if len(names) < 100 {
		t.Fatalf("parsed only %d color tokens; parser broken?", len(names))
	}
	return names
}

func TestThemeCategoryTableComplete(t *testing.T) {
	seen := map[string]string{}
	for _, c := range themeCategories {
		if !isSwatchName(c.Anchor) {
			t.Errorf("category %q anchor %q is not a swatch name", c.ID, c.Anchor)
		}
		for _, tok := range c.Tokens {
			if _, ok := genColorByName[tok]; !ok {
				t.Errorf("category %q lists unknown token %q", c.ID, tok)
			}
			if prev, dup := seen[tok]; dup {
				t.Errorf("token %q in both %q and %q", tok, prev, c.ID)
			}
			seen[tok] = c.ID
			if themeFixedTokens[tok] {
				t.Errorf("token %q is both fixed and in category %q", tok, c.ID)
			}
		}
	}
	for _, name := range designMDColorTokenNames(t) {
		if _, ok := seen[name]; ok {
			continue
		}
		if themeFixedTokens[name] || isRawPaletteToken(name) {
			continue
		}
		t.Errorf("DESIGN.md color token %q is neither in a theme category nor in themeFixedTokens — classify it in theme_categories.go", name)
	}
	for tok := range themeFixedTokens {
		if _, ok := genColorByName[tok]; !ok {
			t.Errorf("themeFixedTokens names unknown token %q", tok)
		}
	}
	if id, ok := themeCategoryOfToken("primary"); !ok || id != "accent" {
		t.Fatalf("themeCategoryOfToken(primary) = %q,%v", id, ok)
	}
	if _, ok := themeCategoryOfToken("error"); ok {
		t.Fatal("fixed token must not resolve to a category")
	}
}
