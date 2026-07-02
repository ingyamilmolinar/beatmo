package ui

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// parseTypographySizes extracts the `fontSize: Npx` value for each role under
// the `typography:` block of DESIGN.md's YAML front matter.
func parseTypographySizes(md string) map[string]float64 {
	out := map[string]float64{}
	roleRe := regexp.MustCompile(`^  ([a-z-]+):\s*$`)
	sizeRe := regexp.MustCompile(`^    fontSize:\s*([0-9.]+)px`)
	inTypo := false
	cur := ""
	for _, line := range regexp.MustCompile(`\r?\n`).Split(md, -1) {
		if line == "typography:" {
			inTypo = true
			continue
		}
		if !inTypo {
			continue
		}
		// A new top-level key (no indent, not a comment) ends the block.
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' {
			break
		}
		if m := roleRe.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if m := sizeRe.FindStringSubmatch(line); m != nil && cur != "" {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				out[cur] = v
			}
		}
	}
	return out
}

// TestTextRoleSizesMatchDesignMD pins every TextRole's render px to the
// typography spec in DESIGN.md — the typography parallel to TestDesignMDDrift
// (colors). DESIGN.md is the source of truth; if a role's size diverges, or a
// title site hardcodes a different px (the bug this pass fixed), update both
// DESIGN.md and TextRole.size() together. Guards against typography re-drift.
func TestTextRoleSizesMatchDesignMD(t *testing.T) {
	body, err := os.ReadFile(findDesignMD(t))
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	sizes := parseTypographySizes(string(body))
	want := []struct {
		key  string
		role TextRole
	}{
		{"panel-title", RolePanelTitle},
		{"section-header", RoleSectionHeader},
		{"body", RoleBody},
		{"caption", RoleCaption},
	}
	for _, w := range want {
		md, ok := sizes[w.key]
		if !ok {
			t.Errorf("DESIGN.md typography.%s has no fontSize", w.key)
			continue
		}
		if got := w.role.size(); got != md {
			t.Errorf("role %v size = %vpx, but DESIGN.md typography.%s = %vpx — keep them in sync", w.role, got, w.key, md)
		}
	}
}

// TestTextRoleHierarchy proves the role API exposes a real, distinct type
// scale (panel-title > section-header > body > caption) in every build mode —
// this is what kills the "everything is one chunky size" look.
func TestTextRoleHierarchy(t *testing.T) {
	pt := StyledTextHeight(RolePanelTitle)
	sh := StyledTextHeight(RoleSectionHeader)
	bd := StyledTextHeight(RoleBody)
	cap := StyledTextHeight(RoleCaption)
	if !(pt > sh && sh > bd && bd > cap) {
		t.Fatalf("role heights must strictly decrease: panelTitle=%d sectionHeader=%d body=%d caption=%d", pt, sh, bd, cap)
	}
	if cap < 10 {
		t.Fatalf("caption height %d too small to read", cap)
	}
}

// TestStyledTextWidthScalesWithLength is a sanity check that width grows with
// string length (guards against a constant/0 fallback regression).
func TestStyledTextWidthScalesWithLength(t *testing.T) {
	short := StyledTextWidth("Hi", RoleBody)
	long := StyledTextWidth("Hello world", RoleBody)
	if long <= short {
		t.Fatalf("expected longer string wider: short=%d long=%d", short, long)
	}
}
